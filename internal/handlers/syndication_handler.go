package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/services"
)

// SyndicationHandler handles RSS, JSON Feed, and Sitemap generation.
type SyndicationHandler struct {
	*BaseHandler
	feedService services.FeedServiceInterface
}

// NewSyndicationHandler creates a new syndication handler.
func NewSyndicationHandler(base *BaseHandler, feedService services.FeedServiceInterface) *SyndicationHandler {
	return &SyndicationHandler{
		BaseHandler: base,
		feedService: feedService,
	}
}

// feedCacheControl caps regeneration cost: feeds/sitemap change at most when
// content does, so a 1-hour shared cache is safe and spares the server from
// re-rendering the whole corpus on every crawler hit.
const feedCacheControl = "public, max-age=3600"

// RSS handles RSS feed generation.
func (h *SyndicationHandler) RSS(c *gin.Context) {
	data, err := h.feedService.GenerateRSS()
	if err != nil {
		h.handleError(c, err, "Failed to generate RSS feed")
		return
	}

	c.Header("Content-Type", "application/rss+xml; charset=utf-8")
	c.Header("Cache-Control", feedCacheControl)
	c.String(http.StatusOK, data)
}

// JSONFeed handles JSON feed generation.
func (h *SyndicationHandler) JSONFeed(c *gin.Context) {
	data, err := h.feedService.GenerateJSONFeed()
	if err != nil {
		h.handleError(c, err, "Failed to generate JSON feed")
		return
	}

	c.Header("Content-Type", "application/feed+json; charset=utf-8")
	c.Header("Cache-Control", feedCacheControl)
	c.String(http.StatusOK, data)
}

// Sitemap handles sitemap.xml generation.
func (h *SyndicationHandler) Sitemap(c *gin.Context) {
	data, err := h.feedService.GenerateSitemap()
	if err != nil {
		h.handleError(c, err, "Failed to generate sitemap")
		return
	}

	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Header("Cache-Control", feedCacheControl)
	c.String(http.StatusOK, data)
}

// HumansTxt serves a humans.txt with team and site information.
func (h *SyndicationHandler) HumansTxt(c *gin.Context) {
	version := unknownValue
	if h.buildInfo != nil && h.buildInfo.Version != "" {
		version = h.buildInfo.Version
	}

	content := fmt.Sprintf(`/* TEAM */
Author: %s
Site: %s

/* SITE */
Software: MarkGo %s
Language: Go
Framework: Gin
Standards: HTML5, CSS3, ES Modules
`, h.config.Blog.Author, h.config.BaseURL, version)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, content)
}

// RobotsTxt serves a dynamically generated robots.txt using the configured BASE_URL.
func (h *SyndicationHandler) RobotsTxt(c *gin.Context) {
	if h.seoService == nil || !h.seoService.IsEnabled() {
		c.String(http.StatusOK, "User-agent: *\nAllow: /\n")
		return
	}

	data, err := h.seoService.GenerateRobotsTxt()
	if err != nil {
		h.logger.Error("Failed to generate robots.txt, serving permissive fallback", "error", err)
		c.String(http.StatusOK, "User-agent: *\nAllow: /\n")
		return
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, string(data))
}
