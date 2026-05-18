package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/services"
)

// AboutHandler handles the config-driven about page.
type AboutHandler struct {
	*BaseHandler
	articleService   services.ArticleServiceInterface
	markdownRenderer MarkdownRenderer
}

// NewAboutHandler creates a new about handler.
func NewAboutHandler(
	base *BaseHandler,
	articleService services.ArticleServiceInterface,
	markdownRenderer MarkdownRenderer,
) *AboutHandler {
	return &AboutHandler{
		BaseHandler:      base,
		articleService:   articleService,
		markdownRenderer: markdownRenderer,
	}
}

// ShowAbout handles the GET /about route. Template data keys consumed by
// about.html:
//
//	Identity:  about_avatar, about_tagline, about_location
//	Bio:       bio_html (rendered HTML; absent if neither about.md nor
//	           ABOUT_BIO set)
//	Social:    social_links []socialLink, has_social
//	Contact:   has_contact (BLOG_AUTHOR_EMAIL set),
//	           has_contact_form (full SMTP-backed form available)
//	Reach:     about_ama_heading, about_ama_intro, about_ama_label
//	           (v3.14.0+, closes #75 — AMA copy from the existing v3.11.0
//	           AMA_PAGE_* env vars so /ama and /about speak in the same
//	           operator voice; AMA half always renders, see below)
//
// Plus standard base-template keys via buildBaseTemplateData.
func (h *AboutHandler) ShowAbout(c *gin.Context) {
	cfg := h.config
	data := h.buildBaseTemplateData("About - " + cfg.Blog.Title)
	data["template"] = "about"
	data["path"] = "/about"
	data["canonicalPath"] = "/about"
	if cfg.About.Tagline != "" {
		data["description"] = cfg.About.Tagline
	} else {
		data["description"] = "About " + cfg.Blog.Author
	}

	// Identity (always present — BLOG_AUTHOR is required)
	data["about_avatar"] = cfg.About.Avatar
	data["about_tagline"] = cfg.About.Tagline
	data["about_location"] = cfg.About.Location

	// Social links — normalize short handles to full URLs
	socialLinks := h.buildSocialLinks()
	data["social_links"] = socialLinks
	data["has_social"] = len(socialLinks) > 0

	// Bio: prefer about.md article, fall back to ABOUT_BIO config
	article, articleErr := h.articleService.GetArticleBySlug("about")
	if articleErr != nil && !apperrors.IsArticleNotFound(articleErr) {
		h.logger.Warn("Failed to load about article", "error", articleErr)
	}
	if articleErr == nil && article != nil {
		bioHTML := article.Content
		if article.ProcessedContent != "" {
			bioHTML = article.ProcessedContent
		}
		data["bio_html"] = bioHTML
	} else if cfg.About.Bio != "" && h.markdownRenderer != nil {
		if rendered, err := h.markdownRenderer.ProcessMarkdown(cfg.About.Bio); err != nil {
			h.logger.Warn("Failed to render ABOUT_BIO markdown", "error", err)
		} else {
			data["bio_html"] = rendered
		}
	}

	// Contact section
	hasEmail := cfg.Blog.AuthorEmail != ""
	hasContactForm := cfg.Email.Username != "" && cfg.Email.Host != ""
	data["has_contact"] = hasEmail
	data["has_contact_form"] = hasContactForm

	// Reach section (v3.14.0+): operator-voiced AMA promo reusing the
	// v3.11.0 AMA_PAGE_* env vars. AMA half always renders here, matching
	// pre-v3.14.0 behavior — getEnv treats empty as unset and falls back to
	// the non-empty default, so there's no operator path to hide it via
	// .env. Hiding the AMA half requires overriding about.html via
	// TEMPLATES_PATH.
	data["about_ama_heading"] = cfg.AMA.PageHeading
	data["about_ama_intro"] = cfg.AMA.PageIntro
	data["about_ama_label"] = cfg.AMA.SubmitLabel

	h.enhanceTemplateDataWithSEO(data, "/about")
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// socialLink represents a single social link for the template.
type socialLink struct {
	Platform string
	URL      string
	Label    string
}

// buildSocialLinks normalizes configured social links into full URLs.
func (h *AboutHandler) buildSocialLinks() []socialLink {
	cfg := h.config.About
	var links []socialLink

	if cfg.GitHub != "" {
		links = append(links, socialLink{
			Platform: "github",
			URL:      normalizeURL(cfg.GitHub, "https://github.com/"),
			Label:    "GitHub",
		})
	}
	if cfg.Twitter != "" {
		url := cfg.Twitter
		if !strings.HasPrefix(url, "http") {
			url = strings.TrimPrefix(url, "@")
			url = "https://x.com/" + url
		}
		links = append(links, socialLink{
			Platform: "twitter",
			URL:      url,
			Label:    "Twitter",
		})
	}
	if cfg.LinkedIn != "" {
		links = append(links, socialLink{
			Platform: "linkedin",
			URL:      normalizeURL(cfg.LinkedIn, "https://linkedin.com/in/"),
			Label:    "LinkedIn",
		})
	}
	if cfg.Mastodon != "" {
		links = append(links, socialLink{
			Platform: "mastodon",
			URL:      cfg.Mastodon, // Mastodon URLs vary by instance, require full URL
			Label:    "Mastodon",
		})
	}
	if cfg.Website != "" {
		links = append(links, socialLink{
			Platform: "website",
			URL:      normalizeURL(cfg.Website, "https://"),
			Label:    "Website",
		})
	}

	return links
}

// normalizeURL prepends a prefix if the value doesn't already start with "http".
func normalizeURL(value, prefix string) string {
	if strings.HasPrefix(value, "http") {
		return value
	}
	return prefix + value
}
