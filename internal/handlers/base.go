// Package handlers provides HTTP request handlers for the MarkGo blog engine.
// It includes handlers for admin operations, article management, API endpoints, and more.
package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/config"
	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/middleware"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

const templateArticle = "article"
const templateAMA = "ama"
const unknownValue = "unknown"

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
	config          *config.Config
	logger          *slog.Logger
	templateService services.TemplateServiceInterface
	buildInfo       *BuildInfo
	seoService      services.SEOServiceInterface
	seoHelper       *SEODataHelper
}

// NewBaseHandler creates a new base handler
func NewBaseHandler(
	cfg *config.Config,
	logger *slog.Logger,
	templateService services.TemplateServiceInterface,
	buildInfo *BuildInfo,
	seoService services.SEOServiceInterface,
) *BaseHandler {
	var seoHelper *SEODataHelper
	if seoService != nil {
		seoHelper = NewSEODataHelper(seoService, cfg)
	}

	return &BaseHandler{
		config:          cfg,
		logger:          logger,
		templateService: templateService,
		buildInfo:       buildInfo,
		seoService:      seoService,
		seoHelper:       seoHelper,
	}
}

// enhanceWithSEO adds SEO data to template data
func (h *BaseHandler) enhanceWithSEO(data map[string]any, seoType string) {
	if h.seoHelper == nil || h.seoService == nil || !h.seoService.IsEnabled() {
		return
	}

	var seoData map[string]interface{}

	switch seoType {
	case templateArticle:
		if article, ok := data["article"].(*models.Article); ok && article != nil {
			seoData = h.seoHelper.GenerateArticleSEOData(article)
		}
	case "home":
		seoData = h.seoHelper.GenerateHomeSEOData()
	case "page":
		title, _ := data["title"].(string)             //nolint:errcheck // safe type assertion with zero-value fallback
		description, _ := data["description"].(string) //nolint:errcheck // safe type assertion with zero-value fallback
		path := ""
		if pageData, ok := data["path"].(string); ok {
			path = pageData
		}
		seoData = h.seoHelper.GeneratePageSEOData(title, description, path)
	}

	// Merge SEO data into template data
	if seoData != nil {
		h.seoHelper.AddSEODataToTemplateData(data, seoData)
	}
}

// enhanceTemplateDataWithSEO intelligently adds SEO data based on page context
func (h *BaseHandler) enhanceTemplateDataWithSEO(data map[string]any, path string) {
	if h.seoHelper == nil || h.seoService == nil || !h.seoService.IsEnabled() {
		return
	}

	// Determine SEO type based on data content and path
	seoType := "page" // default

	// Check if this is an article page
	if article, ok := data["article"].(*models.Article); ok && article != nil {
		seoType = templateArticle
	} else if path == "/" || path == "" {
		seoType = "home"
	}

	h.enhanceWithSEO(data, seoType)
}

// injectAuthState copies authentication context values from gin context into template data.
// Called from renderHTML so the nav login popover and auth gates render correctly on every page.
func (h *BaseHandler) injectAuthState(c *gin.Context, data map[string]any) {
	if authenticated, exists := c.Get("authenticated"); exists {
		data["authenticated"] = authenticated
	}
	if authRequired, exists := c.Get("auth_required"); exists {
		data["auth_required"] = authRequired
	}

	// Ensure a CSRF token exists for the login popover on every page where admin is configured.
	// Protected pages get one from SoftSessionAuth/CSRF middleware, but public pages need one too.
	if _, exists := c.Get("csrf_token"); !exists {
		if h.config.Admin.Username != "" && h.config.Admin.Password != "" {
			secureCookie := h.config.Environment != "development"
			middleware.GenerateCSRFToken(c, secureCookie)
			if c.IsAborted() {
				return
			}
		}
	}
	if csrfToken, exists := c.Get("csrf_token"); exists {
		data["csrf_token"] = csrfToken
	}

	// Always return to the current page after login.
	data["login_next"] = c.Request.URL.RequestURI()
}

// renderHTML renders HTML template with common error handling
func (h *BaseHandler) renderHTML(c *gin.Context, status int, template string, data any) {
	if h.shouldReturnJSON(c) {
		// Extract only safe fields for JSON response — never serialize template
		// data directly, as it contains the full config (including credentials).
		safeResp := gin.H{"status": status}
		if mapData, ok := data.(map[string]any); ok {
			if title, exists := mapData["title"]; exists {
				safeResp["title"] = title
			}
			if desc, exists := mapData["description"]; exists {
				safeResp["description"] = desc
			}
		}
		if status >= 400 {
			safeResp["error"] = http.StatusText(status)
		}
		c.JSON(status, safeResp)
		return
	}

	// Inject auth state into template data for nav popover rendering
	if mapData, ok := data.(map[string]any); ok {
		h.injectAuthState(c, mapData)
	}

	c.Status(status)
	if err := h.templateService.Render(c.Writer, template, data); err != nil {
		h.logger.Error("Template rendering failed", "template", template, "error", err)
		// Don't call handleError here — it would call renderHTML again, creating
		// an infinite loop. Render a styled fallback if headers haven't been flushed.
		if !c.Writer.Written() {
			c.Data(http.StatusInternalServerError, "text/html; charset=utf-8",
				[]byte(apperrors.FallbackErrorHTML))
		}
		c.Abort()
	}
}

// handleError provides standardized error handling across handlers
func (h *BaseHandler) handleError(c *gin.Context, err error, defaultMsg string) {
	var httpStatus int
	var message string

	switch {
	case apperrors.IsArticleNotFound(err) || apperrors.IsNotFound(err):
		httpStatus = http.StatusNotFound
		message = "Resource not found"
	case apperrors.IsValidationError(err):
		httpStatus = http.StatusBadRequest
		message = "Invalid request"
	case apperrors.IsConfigurationError(err):
		httpStatus = http.StatusServiceUnavailable
		message = "Service temporarily unavailable"
	default:
		httpStatus = http.StatusInternalServerError
		message = defaultMsg
	}

	h.logger.Error("Handler error", "error", err, "status", httpStatus)

	// If the response is already committed (headers flushed), we can't change
	// the status code or render a new page. Just log and abort.
	if c.Writer.Written() {
		c.Abort()
		return
	}

	if h.shouldReturnJSON(c) {
		c.JSON(httpStatus, gin.H{
			"error":   message,
			"status":  httpStatus,
			"details": err.Error(),
		})
		return
	}

	// Render error page — use 500 template for server errors, 404 for everything else
	data := h.buildBaseTemplateData(message)
	if httpStatus >= http.StatusInternalServerError {
		data["template"] = "500"
	} else {
		data["template"] = "404"
	}
	data["description"] = message
	h.renderHTML(c, httpStatus, "base.html", data)
}

// shouldReturnJSON determines if response should be JSON based on request.
// Only returns true for explicit application/json requests or API paths.
// Accept: */* returns HTML (correct for curl, browsers, and most clients).
func (h *BaseHandler) shouldReturnJSON(c *gin.Context) bool {
	// API paths always return JSON regardless of Accept header
	path := c.Request.URL.Path
	apiPaths := []string{"/health", "/api/", "/admin/stats"}
	for _, apiPath := range apiPaths {
		if path == apiPath || strings.HasPrefix(path, apiPath) {
			return true
		}
	}

	// Only return JSON for explicit application/json requests
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "application/json")
}

// requireDevelopmentEnv checks if we're in development environment
func (h *BaseHandler) requireDevelopmentEnv(c *gin.Context) bool {
	if h.config.Environment != "development" {
		h.handleError(c, apperrors.NewHTTPError(
			http.StatusForbidden,
			"Endpoint only available in development",
			nil,
		), "Development endpoint access denied")
		return false
	}
	return true
}

// buildBaseTemplateData creates common template data that most pages need
func (h *BaseHandler) buildBaseTemplateData(title string) map[string]any {
	appVersion := unknownValue
	if h.buildInfo != nil && h.buildInfo.Version != "" {
		appVersion = h.buildInfo.Version
	}
	return map[string]any{
		"title":       title,
		"config":      h.config,
		"app_version": appVersion,
	}
}

// buildArticlePageData creates template data for article pages
func (h *BaseHandler) buildArticlePageData(title string, recentArticles []*models.Article) map[string]any {
	appVersion := unknownValue
	if h.buildInfo != nil && h.buildInfo.Version != "" {
		appVersion = h.buildInfo.Version
	}
	return map[string]any{
		"title":           title,
		"config":          h.config,
		"recent_articles": recentArticles,
		"app_version":     appVersion,
	}
}

// respondWithJSON provides pooled JSON response handling
func (h *BaseHandler) respondWithJSON(c *gin.Context, status int, dataBuilder func() map[string]any) {
	data := dataBuilder()

	c.JSON(status, data)
}
