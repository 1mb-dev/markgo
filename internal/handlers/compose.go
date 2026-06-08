package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
	articlepkg "github.com/1mb-dev/markgo/internal/services/article"
	"github.com/1mb-dev/markgo/internal/services/compose"
	slugutil "github.com/1mb-dev/markgo/internal/slug"
)

const (
	templateCompose    = "compose"
	maxPreviewBodySize = 1 << 20 // 1MB
)

// MarkdownRenderer renders markdown to HTML.
// Narrow interface — only the method needed for preview.
type MarkdownRenderer interface {
	ProcessMarkdown(content string) (string, error)
}

// ComposeHandler handles the compose page for creating new posts.
type ComposeHandler struct {
	*BaseHandler
	composeService   *compose.Service
	articleService   services.ArticleServiceInterface
	markdownRenderer MarkdownRenderer
}

// NewComposeHandler creates a new compose handler.
func NewComposeHandler(
	base *BaseHandler,
	composeService *compose.Service,
	articleService services.ArticleServiceInterface,
	markdownRenderer MarkdownRenderer,
) *ComposeHandler {
	return &ComposeHandler{
		BaseHandler:      base,
		composeService:   composeService,
		articleService:   articleService,
		markdownRenderer: markdownRenderer,
	}
}

// validatePageInput returns a human-readable error message if the input
// is not acceptable for a new type:page article, or empty string on
// success. Pages need (a) a non-empty title because the canonical URL
// surface (the /p index, browser tab, social previews) is title-driven,
// and (b) a slug that satisfies the strict slug.Validate contract
// (charset, length, reserved set) and is unique within the article
// store. The handler renders the message in the error banner.
func (h *ComposeHandler) validatePageInput(input *compose.Input) string {
	if strings.TrimSpace(input.Title) == "" {
		return "Title is required for pages"
	}
	if err := slugutil.Validate(input.Slug); err != nil {
		return "Slug invalid: " + err.Error()
	}
	if existing, err := h.articleService.GetArticleBySlug(input.Slug); err == nil && existing != nil {
		return "Slug already in use: " + input.Slug
	}
	return ""
}

// canonicalPathForSlug resolves the canonical URL for a composed or
// edited post by looking up the article in the in-memory store. Existing
// type:page articles edited via /compose/edit/:slug must redirect to
// /p/<slug>, not /writing/<slug> (which would hit the v3.13.0 301 and
// add a needless round-trip). For the rare case where lookup fails
// (article not yet reloaded into the repo after a failed reload), falls
// back to a synthetic empty-Type article — /writing/<slug>. The 301
// redirect catches that fallback if it ever surfaces a type:page slug.
func (h *ComposeHandler) canonicalPathForSlug(slug string) string {
	if art, err := h.articleService.GetArticleBySlug(slug); err == nil {
		return articlepkg.CanonicalURLFor(art)
	}
	return articlepkg.CanonicalURLFor(&models.Article{Slug: slug})
}

// ShowCompose renders the compose form.
// Reads optional query params (title, text, url) to support PWA share_target.
func (h *ComposeHandler) ShowCompose(c *gin.Context) {
	data := h.buildBaseTemplateData("Compose - " + h.config.Blog.Title)
	data["template"] = templateCompose
	data["csrf_token"] = csrfToken(c)

	// Pre-fill from share_target query params (or any deep link)
	title := c.Query("title")
	text := c.Query("text")
	sharedURL := c.Query("url")
	if title != "" || text != "" || sharedURL != "" {
		input := compose.Input{
			Title:   title,
			Content: text,
			LinkURL: sharedURL,
		}
		data["input"] = input
	}

	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// ShowComposeNewPage renders the compose form in page-authoring mode.
// Pages need an explicit slug and skip date/tags/categories — the template
// branches on data["mode"] == "page" to surface a slug input and hide
// the article-shaped fields.
func (h *ComposeHandler) ShowComposeNewPage(c *gin.Context) {
	data := h.buildBaseTemplateData("New page - " + h.config.Blog.Title)
	data["template"] = templateCompose
	data["mode"] = articlepkg.TypePage
	data["csrf_token"] = csrfToken(c)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// ShowEdit renders the compose form pre-filled with an existing article.
func (h *ComposeHandler) ShowEdit(c *gin.Context) {
	slug := c.Param("slug")
	if !slugutil.WellFormed(slug) {
		h.handleError(c, fmt.Errorf("invalid slug %q: %w", slug, apperrors.ErrArticleNotFound), "Article not found")
		return
	}

	input, err := h.composeService.LoadArticle(slug)
	if err != nil {
		h.handleError(c, err, "Article not found")
		return
	}

	// Legacy AMA recovery: pre-v3.20.0 files keep the question in the body (no
	// frontmatter question). Split it out so the edit form shows the question
	// read-only and the textarea holds only the answer — same single split site
	// the load-time normalizer uses.
	if input.Type == articlepkg.TypeAMA && input.Question == "" {
		input.Question, input.Content = articlepkg.SplitLegacyAMA(input.Content)
	}

	data := h.buildBaseTemplateData("Edit - " + h.config.Blog.Title)
	data["template"] = templateCompose
	data["input"] = input
	data["editing"] = true
	data["slug"] = slug
	data["canonicalPath"] = h.canonicalPathForSlug(slug)
	if input.Type == articlepkg.TypePage {
		// Surface page-mode so the template can hide article-only fields
		// (link_url, tags, categories, banner) when editing a page.
		data["mode"] = articlepkg.TypePage
	}
	data["csrf_token"] = csrfToken(c)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// csrfToken pulls the CSRF token from the gin context (set by CSRF middleware).
func csrfToken(c *gin.Context) string {
	if token, exists := c.Get("csrf_token"); exists {
		if s, ok := token.(string); ok {
			return s
		}
	}
	return ""
}

// refreshCSRFToken generates a new CSRF token and sets the cookie.
// Used when re-rendering a form after a POST validation error.
// Aborts with 500 if token generation fails (crypto/rand failure is a system emergency).
func refreshCSRFToken(c *gin.Context) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return ""
	}
	token := hex.EncodeToString(b)
	var isSecure bool
	if secureCookie, exists := c.Get("csrf_secure"); exists {
		if v, ok := secureCookie.(bool); ok {
			isSecure = v
		}
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("_csrf", token, 3600, "", "", isSecure, true)
	return token
}

// HandleEdit processes the edit form submission.
func (h *ComposeHandler) HandleEdit(c *gin.Context) {
	slug := c.Param("slug")
	if !slugutil.WellFormed(slug) {
		h.handleError(c, fmt.Errorf("invalid slug %q: %w", slug, apperrors.ErrArticleNotFound), "Article not found")
		return
	}

	input := compose.Input{
		Content:     c.PostForm("content"),
		Title:       c.PostForm("title"),
		Description: c.PostForm("description"),
		LinkURL:     c.PostForm("link_url"),
		Tags:        c.PostForm("tags"),
		Categories:  c.PostForm("categories"),
		Banner:      c.PostForm("banner"),
		BannerAlt:   c.PostForm("banner_alt"),
		Draft:       c.PostForm("draft") == "on",
		Type:        c.PostForm("type"),
	}

	if input.Content == "" {
		data := h.buildBaseTemplateData("Edit - " + h.config.Blog.Title)
		data["template"] = templateCompose
		data["error"] = "Content is required"
		data["input"] = input
		data["editing"] = true
		data["slug"] = slug
		data["canonicalPath"] = h.canonicalPathForSlug(slug)
		if input.Type == articlepkg.TypePage {
			data["mode"] = articlepkg.TypePage
		}
		data["csrf_token"] = refreshCSRFToken(c)
		if c.IsAborted() {
			return
		}
		h.renderHTML(c, http.StatusBadRequest, "base.html", data)
		return
	}

	prevBanner, err := h.composeService.UpdateArticle(slug, &input)
	if err != nil {
		h.logger.Error("Failed to update post", "error", err, "slug", slug)
		data := h.buildBaseTemplateData("Edit - " + h.config.Blog.Title)
		data["template"] = templateCompose
		data["error"] = "Failed to update post. Please try again."
		data["input"] = input
		data["editing"] = true
		data["slug"] = slug
		data["canonicalPath"] = h.canonicalPathForSlug(slug)
		if input.Type == articlepkg.TypePage {
			data["mode"] = articlepkg.TypePage
		}
		data["csrf_token"] = refreshCSRFToken(c)
		if c.IsAborted() {
			return
		}
		h.renderHTML(c, http.StatusInternalServerError, "base.html", data)
		return
	}
	compose.UnlinkOwnedUpload(prevBanner, input.Banner, slug, h.config.Upload.Path, h.logger)

	reloadOK := true
	if err := h.articleService.ReloadArticles(); err != nil {
		h.logger.Error("Failed to reload articles after edit", "error", err)
		reloadOK = false
	}

	// Drafts have no public URL — canonicalPathForSlug would 404. Send to the
	// drafts list where the operator can find and publish.
	if input.Draft {
		c.Redirect(http.StatusSeeOther, "/admin/drafts")
		return
	}
	// Redirect to the edited article, or feed if reload failed (stale cache would show old version)
	if reloadOK {
		c.Redirect(http.StatusSeeOther, h.canonicalPathForSlug(slug))
	} else {
		c.Redirect(http.StatusSeeOther, "/")
	}
}

// HandleSubmit processes the compose form submission.
func (h *ComposeHandler) HandleSubmit(c *gin.Context) {
	input := compose.Input{
		Content:     c.PostForm("content"),
		Title:       c.PostForm("title"),
		Description: c.PostForm("description"),
		LinkURL:     c.PostForm("link_url"),
		Tags:        c.PostForm("tags"),
		Categories:  c.PostForm("categories"),
		Banner:      c.PostForm("banner"),
		BannerAlt:   c.PostForm("banner_alt"),
		Draft:       c.PostForm("draft") == "on",
		Type:        c.PostForm("type"),
		Slug:        c.PostForm("slug"),
	}

	if input.Content == "" {
		data := h.buildBaseTemplateData("Compose - " + h.config.Blog.Title)
		data["template"] = templateCompose
		data["error"] = "Content is required"
		data["input"] = input
		if input.Type == articlepkg.TypePage {
			data["mode"] = articlepkg.TypePage
		}
		data["csrf_token"] = refreshCSRFToken(c)
		if c.IsAborted() {
			return
		}
		h.renderHTML(c, http.StatusBadRequest, "base.html", data)
		return
	}

	// Page-mode validation: pages need a valid, unique slug. Run before
	// CreatePost so the operator sees a render with their input intact
	// rather than a generic 500.
	if input.Type == articlepkg.TypePage {
		if errMsg := h.validatePageInput(&input); errMsg != "" {
			data := h.buildBaseTemplateData("New page - " + h.config.Blog.Title)
			data["template"] = templateCompose
			data["mode"] = articlepkg.TypePage
			data["error"] = errMsg
			data["input"] = input
			data["csrf_token"] = refreshCSRFToken(c)
			if c.IsAborted() {
				return
			}
			h.renderHTML(c, http.StatusBadRequest, "base.html", data)
			return
		}
	}

	slug, err := h.composeService.CreatePost(&input)
	if err != nil {
		h.logger.Error("Failed to create post", "error", err)
		data := h.buildBaseTemplateData("Compose - " + h.config.Blog.Title)
		data["template"] = templateCompose
		data["error"] = "Failed to create post. Please try again."
		data["input"] = input
		if input.Type == articlepkg.TypePage {
			data["mode"] = articlepkg.TypePage
		}
		data["csrf_token"] = refreshCSRFToken(c)
		if c.IsAborted() {
			return
		}
		h.renderHTML(c, http.StatusInternalServerError, "base.html", data)
		return
	}

	// Reload articles so the new post appears in the feed
	reloadOK := true
	if err := h.articleService.ReloadArticles(); err != nil {
		h.logger.Error("Failed to reload articles after compose", "error", err)
		reloadOK = false
	}

	// Drafts have no public URL — canonicalPathForSlug would 404. Send to the
	// drafts list where the operator can find and publish.
	if input.Draft {
		c.Redirect(http.StatusSeeOther, "/admin/drafts")
		return
	}
	// Redirect to the new post, or feed if reload failed (article won't be in memory)
	if !reloadOK || input.Title == "" {
		c.Redirect(http.StatusSeeOther, "/")
	} else {
		c.Redirect(http.StatusSeeOther, h.canonicalPathForSlug(slug))
	}
}

// Preview renders markdown content as HTML for the compose preview panel.
// Returns an HTML fragment (not a full page). Self-XSS via html.WithUnsafe()
// is acceptable — compose is behind session auth (admin-only).
func (h *ComposeHandler) Preview(c *gin.Context) {
	// Limit request body before Gin parses the form (defense in depth)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxPreviewBodySize))

	content := c.PostForm("content")
	if content == "" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", nil)
		return
	}

	// Explicit length check catches oversized content that MaxBytesReader truncated
	// (Gin silently returns empty PostForm on MaxBytesReader overflow, but if any
	// content came through, check it hasn't been silently truncated)
	if len(content) > maxPreviewBodySize {
		c.String(http.StatusRequestEntityTooLarge, "Content too large for preview")
		return
	}

	html, err := h.markdownRenderer.ProcessMarkdown(content)
	if err != nil {
		h.logger.Error("Preview render failed", "error", err)
		c.String(http.StatusInternalServerError, "Preview unavailable")
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// HandleQuickPublish creates a post from a JSON request body.
// Used by the SPA compose sheet for fast content capture.
func (h *ComposeHandler) HandleQuickPublish(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxPreviewBodySize))

	var input compose.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Content too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if input.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content is required"})
		return
	}

	slug, err := h.composeService.CreatePost(&input)
	if err != nil {
		h.logger.Error("Quick publish failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create post"})
		return
	}

	// Infer type for the response (mirrors inferPostType in article service)
	postType := "article"
	if input.LinkURL != "" {
		postType = "link"
	} else if input.Title == "" && wordCount(input.Content) < 100 {
		postType = "thought"
	}

	message := "Published"
	if input.Draft {
		message = "Saved as draft"
	}

	if err := h.articleService.ReloadArticles(); err != nil {
		h.logger.Error("Failed to reload articles after quick publish", "error", err)
		c.JSON(http.StatusCreated, gin.H{
			"slug":    slug,
			"url":     "/",
			"type":    postType,
			"draft":   input.Draft,
			"message": message + " (will appear after next reload)",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"slug":    slug,
		"url":     h.canonicalPathForSlug(slug),
		"type":    postType,
		"draft":   input.Draft,
		"message": message,
	})
}

// PublishDraft publishes a draft article by setting draft=false.
// JSON endpoint for fetch-based publish from the drafts list.
func (h *ComposeHandler) PublishDraft(c *gin.Context) {
	slug := c.Param("slug")
	if !slugutil.WellFormed(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug"})
		return
	}

	input, err := h.composeService.LoadArticle(slug)
	if err != nil {
		if errors.Is(err, apperrors.ErrArticleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		} else {
			h.logger.Error("Failed to load article for publish", "error", err, "slug", slug)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load article"})
		}
		return
	}

	if !input.Draft {
		c.JSON(http.StatusOK, gin.H{
			"slug":    slug,
			"url":     h.canonicalPathForSlug(slug),
			"message": "Already published",
		})
		return
	}

	input.Draft = false
	if _, err := h.composeService.UpdateArticle(slug, input); err != nil {
		h.logger.Error("Failed to publish draft", "error", err, "slug", slug)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish draft"})
		return
	}

	if err := h.articleService.ReloadArticles(); err != nil {
		h.logger.Error("Failed to reload articles after publish", "error", err)
		c.JSON(http.StatusOK, gin.H{
			"slug":    slug,
			"url":     "/",
			"message": "Published (article will appear after next reload)",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"slug":    slug,
		"url":     h.canonicalPathForSlug(slug),
		"message": "Published",
	})
}

// wordCount returns an approximate word count by splitting on whitespace.
func wordCount(s string) int {
	return len(strings.Fields(s))
}
