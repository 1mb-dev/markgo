package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/middleware"
	"github.com/1mb-dev/markgo/internal/services"
	"github.com/1mb-dev/markgo/internal/services/compose"
)

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	GitCommit string
	BuildTime string
}

// Config for handler initialization
type Config struct {
	ArticleService   services.ArticleServiceInterface
	EmailService     services.EmailServiceInterface
	FeedService      services.FeedServiceInterface
	TemplateService  services.TemplateServiceInterface
	SEOService       services.SEOServiceInterface
	ComposeService   *compose.Service
	MarkdownRenderer MarkdownRenderer
	SessionStore     *middleware.SessionStore
	SecureCookie     bool
	Config           *config.Config
	Logger           *slog.Logger
	BuildInfo        *BuildInfo
}

// Router holds all handler types for direct route registration.
// Routes are registered directly against handler methods — no delegation.
type Router struct {
	Feed        *FeedHandler
	Post        *PostHandler
	Taxonomy    *TaxonomyHandler
	Search      *SearchHandler
	Health      *HealthHandler
	Contact     *ContactHandler
	Syndication *SyndicationHandler
	Admin       *AdminHandler
	About       *AboutHandler
	Auth        *AuthHandler    // nil when admin credentials not configured
	Compose     *ComposeHandler // nil when compose not configured
	AMA         *AMAHandler     // nil when compose not configured

	base           *BaseHandler
	articleService services.ArticleServiceInterface
	logger         *slog.Logger
}

// New creates a new router with all handlers initialized.
func New(cfg *Config) *Router {
	base := NewBaseHandler(cfg.Config, cfg.Logger, cfg.TemplateService, cfg.BuildInfo, cfg.SEOService)

	var composeHandler *ComposeHandler
	var amaHandler *AMAHandler
	if cfg.ComposeService != nil {
		composeHandler = NewComposeHandler(base, cfg.ComposeService, cfg.ArticleService, cfg.MarkdownRenderer)
		amaHandler = NewAMAHandler(base, cfg.ComposeService, cfg.ArticleService)
	}

	var authHandler *AuthHandler
	if cfg.SessionStore != nil && cfg.Config.Admin.Username != "" && cfg.Config.Admin.Password != "" {
		authHandler = NewAuthHandler(base, cfg.Config.Admin.Username, cfg.Config.Admin.Password, cfg.SessionStore, cfg.SecureCookie)
	}

	aboutHandler := NewAboutHandler(base, cfg.ArticleService, cfg.MarkdownRenderer)

	return &Router{
		Feed:        NewFeedHandler(base, cfg.ArticleService),
		Post:        NewPostHandler(base, cfg.ArticleService),
		Taxonomy:    NewTaxonomyHandler(base, cfg.ArticleService),
		Search:      NewSearchHandler(base, cfg.ArticleService),
		Health:      NewHealthHandler(base, cfg.ArticleService, time.Now()),
		Contact:     NewContactHandler(base, cfg.EmailService),
		Syndication: NewSyndicationHandler(base, cfg.FeedService),
		Admin:       NewAdminHandler(base, cfg.ArticleService, time.Now()),
		About:       aboutHandler,
		Auth:        authHandler,
		Compose:     composeHandler,
		AMA:         amaHandler,

		base:           base,
		articleService: cfg.ArticleService,
		logger:         cfg.Logger,
	}
}

// ClearCache triggers a full article reload from disk.
func (r *Router) ClearCache(c *gin.Context) {
	if err := r.articleService.ReloadArticles(); err != nil {
		r.logger.Error("Failed to reload articles", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload articles"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Cache cleared"})
}

// NotFound handles 404 errors.
func (r *Router) NotFound(c *gin.Context) {
	data := r.base.buildBaseTemplateData("Page Not Found")
	data["template"] = "404"
	data["description"] = "The page you're looking for was not found"
	r.base.renderHTML(c, http.StatusNotFound, "base.html", data)
}
