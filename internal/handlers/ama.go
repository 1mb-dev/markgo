package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
	articlepkg "github.com/1mb-dev/markgo/internal/services/article"
	"github.com/1mb-dev/markgo/internal/services/compose"
	slugutil "github.com/1mb-dev/markgo/internal/slug"
)

// maxAMABodySize caps the AMA submission body before binding. The largest valid
// field (question) is 500 chars, so 64KB is generous headroom while bounding the
// JSON an unauthenticated caller can force us to parse.
const maxAMABodySize = 64 << 10 // 64KB

// AMAHandler handles AMA question submission and moderation.
type AMAHandler struct {
	*BaseHandler
	composeService *compose.Service
	articleService services.ArticleServiceInterface
}

// NewAMAHandler creates a new AMA handler.
func NewAMAHandler(base *BaseHandler, composeService *compose.Service, articleService services.ArticleServiceInterface) *AMAHandler {
	return &AMAHandler{
		BaseHandler:    base,
		composeService: composeService,
		articleService: articleService,
	}
}

// Submit handles AMA question submissions from unauthenticated readers.
func (h *AMAHandler) Submit(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAMABodySize)

	var form models.AMASubmission
	if err := c.ShouldBindJSON(&form); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Submission too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid submission",
			"details": err.Error(),
		})
		return
	}

	// Honeypot check — bots fill hidden fields; silently accept to avoid detection
	if form.Website != "" {
		c.JSON(http.StatusOK, gin.H{
			"message": "Question submitted successfully",
			"status":  "success",
		})
		return
	}

	// Create post via compose service with type=ama, draft=true. The question
	// is stored in frontmatter (not the body) — the body holds the answer once
	// published, so the question never shares the answer's render path.
	slug, err := h.composeService.CreatePost(&compose.Input{
		Question:   form.Question,
		Title:      "",
		Draft:      true,
		Asker:      form.Name,
		AskerEmail: form.Email,
		Type:       templateAMA,
	})
	if err != nil {
		h.logger.Error("Failed to create AMA submission", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to submit question",
		})
		return
	}

	// Reload articles so new submission is visible in admin
	if reloadErr := h.articleService.ReloadArticles(); reloadErr != nil {
		h.logger.Warn("Failed to reload articles after AMA submission", "slug", slug, "error", reloadErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Question submitted successfully",
		"status":  "success",
	})
}

// ListPending renders the admin AMA moderation page with pending (draft) AMAs.
func (h *AMAHandler) ListPending(c *gin.Context) {
	drafts := h.articleService.GetDraftArticles()

	var pending []*models.Article
	for _, a := range drafts {
		if a.Type == templateAMA {
			pending = append(pending, a)
		}
	}

	if h.shouldReturnJSON(c) {
		c.JSON(http.StatusOK, gin.H{
			"pending":       pending,
			"pending_count": len(pending),
		})
		return
	}

	data := h.buildBaseTemplateData("AMA — " + h.config.Blog.Title)
	data["description"] = "Moderate AMA questions"
	data["template"] = "admin_ama"
	data["pending"] = pending
	data["pending_count"] = len(pending)
	data["csrf_token"] = csrfToken(c)

	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// Answer publishes an AMA by writing the author's answer and removing draft status.
func (h *AMAHandler) Answer(c *gin.Context) {
	slug := c.Param("slug")
	if !slugutil.WellFormed(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug"})
		return
	}

	var form struct {
		Answer string `json:"answer" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Answer is required",
			"details": err.Error(),
		})
		return
	}

	// Load existing article to get current metadata
	input, err := h.composeService.LoadArticle(slug)
	if err != nil {
		h.handleError(c, err, "Failed to load AMA question")
		return
	}

	// Legacy promotion: a pre-v3.20.0 pending draft kept the question in the
	// body with no frontmatter question. Recover it via the shared split (which
	// also handles the rare body that already contains a separator) so
	// overwriting the body with the answer never drops or corrupts the question.
	// New drafts carry the question in frontmatter, so this is a no-op for them.
	if input.Question == "" {
		input.Question, _ = articlepkg.SplitLegacyAMA(input.Content)
	}

	// The answer becomes the body; the question rides in frontmatter (preserved
	// by UpdateArticle's set-if-provided round-trip).
	input.Content = form.Answer
	input.Draft = false

	if _, err := h.composeService.UpdateArticle(slug, input); err != nil {
		h.handleError(c, err, "Failed to publish answer")
		return
	}

	if reloadErr := h.articleService.ReloadArticles(); reloadErr != nil {
		h.logger.Warn("Failed to reload articles after AMA answer", "slug", slug, "error", reloadErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Answer published",
		"status":  "success",
		"slug":    slug,
	})
}

// Delete removes an AMA submission file.
func (h *AMAHandler) Delete(c *gin.Context) {
	slug := c.Param("slug")
	if !slugutil.WellFormed(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug"})
		return
	}

	if err := h.composeService.DeletePost(slug); err != nil {
		h.handleError(c, err, "Failed to delete AMA question")
		return
	}

	if reloadErr := h.articleService.ReloadArticles(); reloadErr != nil {
		h.logger.Warn("Failed to reload articles after AMA delete", "slug", slug, "error", reloadErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Question deleted",
		"status":  "success",
	})
}
