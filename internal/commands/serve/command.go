// Package serve provides the HTTP server command for the MarkGo blog platform.
package serve

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/constants"
	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/handlers"
	"github.com/1mb-dev/markgo/internal/middleware"
	"github.com/1mb-dev/markgo/internal/services"
	"github.com/1mb-dev/markgo/internal/services/article"
	"github.com/1mb-dev/markgo/internal/services/compose"
	"github.com/1mb-dev/markgo/internal/services/feed"
	"github.com/1mb-dev/markgo/internal/services/seo"
	"github.com/1mb-dev/markgo/web"
)

const (
	envDevelopment = "development"
)

// Run starts the MarkGo HTTP server.
func Run(args []string) {
	// Parse command-line flags
	flagSet := flag.NewFlagSet("serve", flag.ContinueOnError)
	flagSet.SetOutput(os.Stdout)
	port := flagSet.Int("port", 0, "Override server port (default: from .env or 3000)")
	flagSet.Usage = printUsage

	if err := flagSet.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}

	var logger *slog.Logger
	var server *http.Server
	var templateService *services.TemplateService

	// Cleanup function for graceful shutdown
	cleanup := func() {
		if server != nil && logger != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logger.Info("Performing graceful shutdown...")

			if err := server.Shutdown(ctx); err != nil {
				logger.Error("Error during shutdown", "error", err)
			}
		}
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("configuration loading", "Failed to load configuration", err, 1),
			cleanup,
		)
	}

	// Apply CLI overrides
	if *port != 0 {
		if *port < 1 || *port > 65535 {
			fmt.Fprintf(os.Stderr, "Error: port must be between 1 and 65535, got %d\n", *port)
			os.Exit(1)
		}
		cfg.Port = *port
	}

	// Setup enhanced logging with configuration
	loggingService, err := services.NewLoggingService(&cfg.Logging)
	if err != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("logging initialization", "Failed to initialize logging service", err, 1),
			cleanup,
		)
	}

	logger = loggingService.GetLogger()
	slog.SetDefault(logger)

	// Initialize services and configure router
	var router *gin.Engine
	var sessionStore *middleware.SessionStore
	router, templateService, sessionStore, err = setupServer(cfg, logger)
	if err != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("server setup", "Failed to set up server", err, 1),
			cleanup,
		)
	}

	// Create HTTP server
	server = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting MarkGo server",
			"port", cfg.Port,
			"environment", cfg.Environment,
			"version", constants.AppVersion)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", "error", err)
			apperrors.HandleCLIError(
				apperrors.NewCLIError("server startup", "Server failed to start", err, 1),
				cleanup,
			)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	cleanups := []func(){
		func() {
			if templateService != nil {
				templateService.Shutdown()
			}
		},
		func() {
			if sessionStore != nil {
				sessionStore.Shutdown()
			}
		},
		middleware.ShutdownRateLimiters,
	}

	shutdownErr := gracefulShutdown(ctx, server.Shutdown, cleanups, logger)
	if shutdownErr != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("server shutdown", "Server forced to shutdown", shutdownErr, 1),
			nil,
		)
	}
	logger.Info("Server exited gracefully")
}

// gracefulShutdown runs the HTTP-server shutdown, then runs each cleanup
// regardless of the shutdown outcome. Returns the shutdown error (or nil)
// so the caller decides how to surface a failure. The unconditional cleanup
// loop is the load-bearing invariant: a context-deadline-exceeded on
// server.Shutdown must not prevent session-store / rate-limiter goroutines
// from being stopped.
func gracefulShutdown(
	ctx context.Context,
	serverShutdown func(context.Context) error,
	cleanups []func(),
	logger *slog.Logger,
) error {
	err := serverShutdown(ctx)
	if err != nil {
		logger.Error("HTTP server forced to shutdown", "error", err)
	}
	for _, cleanup := range cleanups {
		cleanup()
	}
	return err
}

func setupServer(cfg *config.Config, logger *slog.Logger) (*gin.Engine, *services.TemplateService, *middleware.SessionStore, error) {
	// Initialize services
	articleService, err := services.NewArticleService(cfg.ArticlesPath, cfg.Upload.Path, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("article service: %w", err)
	}

	go runOrphanSweep(cfg, articleService, logger)

	emailService := services.NewEmailService(&cfg.Email, cfg.Blog.Title, logger)
	composeService := compose.NewService(cfg.ArticlesPath, cfg.Blog.Author)

	// Initialize SEO helper (stateless utility)
	siteConfig := services.SiteConfig{
		Name:        cfg.Blog.Title,
		Description: cfg.Blog.Description,
		BaseURL:     cfg.BaseURL,
		Language:    cfg.Blog.Language,
		Author:      cfg.Blog.Author,
		Image:       cfg.SEO.DefaultImage,
	}
	robotsConfig := services.RobotsConfig{
		UserAgent:  "*",
		Allow:      cfg.SEO.RobotsAllowed,
		Disallow:   cfg.SEO.RobotsDisallowed,
		CrawlDelay: cfg.SEO.RobotsCrawlDelay,
		SitemapURL: cfg.BaseURL + "/sitemap.xml",
	}
	seoService := seo.NewHelper(articleService, &siteConfig, &robotsConfig, logger, cfg.SEO.Enabled)
	if cfg.SEO.Enabled {
		logger.Info("SEO features enabled")
	}

	// Setup Gin router
	configureGinMode(cfg, logger)
	router := gin.New()

	// Set trusted proxies before any request is served. gin trusts ALL proxies
	// by default (spoofable X-Forwarded-For); passing the operator's parsed CIDRs
	// — or the loopback defaults when TRUSTED_PROXIES is unset — makes ClientIP()
	// honor forwarded headers only from trusted peers, else return the direct
	// peer. The rate limiter keys on ClientIP(), so this is the difference
	// between throttling real clients and throttling everyone as the proxy's
	// single IP. Loopback is auto-trusted when unset because a loopback peer is
	// unspoofable from the network, fixing the common same-host reverse-proxy
	// topology without operator action (#119); off-host proxies still require
	// TRUSTED_PROXIES, and the ProxyTrustWarning detector below catches them.
	trustedProxies := middleware.EffectiveTrustedProxies(cfg.TrustedProxies)
	if err = router.SetTrustedProxies(trustedProxies); err != nil {
		return nil, nil, nil, fmt.Errorf("set trusted proxies: %w", err)
	}

	// Initialize template service
	templateService, err := services.NewTemplateService(cfg.TemplatesPath, cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("template service: %w", err)
	}

	if err := setupTemplates(router, templateService); err != nil {
		return nil, nil, nil, fmt.Errorf("template setup: %w", err)
	}

	// Log rate limiting configuration, including the client-IP keying posture so
	// the proxy-trust mode is visible at boot (t=0) rather than only after the
	// runtime ProxyTrustWarning trips. "loopback-default" means TRUSTED_PROXIES
	// is unset: same-host proxies key correctly, but an off-host proxy operator
	// must set TRUSTED_PROXIES to their proxy CIDR(s).
	trustSource := "explicit"
	if len(cfg.TrustedProxies) == 0 {
		trustSource = "loopback-default"
	}
	logger.Info("Rate limiting configuration",
		"environment", cfg.Environment,
		"general_requests", cfg.RateLimit.General.Requests,
		"general_window", cfg.RateLimit.General.Window,
		"general_rate_per_sec", float64(cfg.RateLimit.General.Requests)/(cfg.RateLimit.General.Window.Minutes()*60),
		"contact_requests", cfg.RateLimit.Contact.Requests,
		"contact_window", cfg.RateLimit.Contact.Window,
		"trusted_proxies", strings.Join(trustedProxies, ","),
		"trusted_proxies_source", trustSource)

	// Global middleware
	router.Use(
		middleware.RecoveryWithErrorHandler(logger),
		middleware.Logger(logger),
		middleware.Performance(logger),
		middleware.SmartCacheHeaders(),
		middleware.CORS(cfg.CORS.AllowedOrigins, cfg.Environment == envDevelopment),
		middleware.Security(cfg),
		middleware.RateLimit(cfg.RateLimit.General.Requests, cfg.RateLimit.General.Window),
		middleware.ErrorHandler(logger),
		middleware.DiscardBodyOnHEAD(),
	)

	// Advise (once) if we appear to be behind an OFF-HOST proxy but
	// TRUSTED_PROXIES is unset. Loopback is auto-trusted (so a same-host proxy
	// keys correctly and never trips this), but an off-host proxy stays
	// untrusted and collapses every client onto its IP under rate limiting.
	if len(cfg.TrustedProxies) == 0 {
		router.Use(middleware.ProxyTrustWarning(logger))
	}

	if cfg.Environment == envDevelopment {
		router.Use(middleware.RequestTracker())
		logger.Info("Development logging enhancements enabled")
	}

	// Initialize feed service
	feedService := feed.NewService(articleService, cfg)

	// Initialize markdown renderer for compose preview
	markdownRenderer := article.NewMarkdownContentProcessor(logger)

	// Initialize session store for admin authentication
	sessionStore := middleware.NewSessionStore()
	secureCookie := cfg.Environment != envDevelopment

	// Initialize handlers
	h := handlers.New(&handlers.Config{
		ArticleService:   articleService,
		EmailService:     emailService,
		FeedService:      feedService,
		TemplateService:  templateService,
		SEOService:       seoService,
		ComposeService:   composeService,
		MarkdownRenderer: markdownRenderer,
		SessionStore:     sessionStore,
		SecureCookie:     secureCookie,
		Config:           cfg,
		Logger:           logger,
		BuildInfo: &handlers.BuildInfo{
			Version:   constants.AppVersion,
			GitCommit: constants.GitCommit,
			BuildTime: constants.BuildTime,
		},
	})

	// Session awareness on all routes (sets authenticated=true when valid session exists)
	// Must come after session store init, before route setup
	router.Use(middleware.SessionAware(sessionStore, secureCookie))

	setupRoutes(router, h, sessionStore, secureCookie, cfg, logger)
	return router, templateService, sessionStore, nil
}

func configureGinMode(cfg *config.Config, logger *slog.Logger) {
	switch cfg.Environment {
	case "production":
		gin.SetMode(gin.ReleaseMode)
		_ = os.Setenv("GIN_MODE", "release")
		logger.Info("Gin router configured for production", "gin_mode", "release")
	case "test":
		gin.SetMode(gin.TestMode)
		_ = os.Setenv("GIN_MODE", "test")
		logger.Info("Gin router configured for testing", "gin_mode", "test")
	default:
		gin.SetMode(gin.DebugMode)
		_ = os.Setenv("GIN_MODE", "debug")
		logger.Info("Gin router configured for development", "gin_mode", "debug")
	}
}

// registerGET registers the handler on both GET and HEAD. The global
// DiscardBodyOnHEAD middleware suppresses the response body on HEAD so
// uptime probes and conditional requests work without per-route changes.
// Use for any route that should be HEAD-probeable; skip for admin debug
// and pprof endpoints (not monitor targets).
func registerGET(r gin.IRoutes, path string, handlerFuncs ...gin.HandlerFunc) {
	r.GET(path, handlerFuncs...)
	r.HEAD(path, handlerFuncs...)
}

func setupRoutes(router *gin.Engine, h *handlers.Router, sessionStore *middleware.SessionStore, secureCookie bool, cfg *config.Config, logger *slog.Logger) { //nolint:funlen // route wiring is inherently long
	// Static files — overlay STATIC_PATH onto embedded assets per file. When
	// STATIC_PATH is set and exists, local files take precedence and missing
	// paths fall through to embedded. When unset/missing, everything serves
	// from embedded. Both branches wrap in gin.OnlyFilesFS to suppress
	// directory listings (gin.Static auto-wraps for local mode but raw
	// StaticFS(http.FS(...)) does not — see gin@v1.12.0/routergroup.go:221).
	staticSub, subErr := fs.Sub(web.Assets, "static")
	if subErr != nil {
		logger.Error("Failed to load embedded static assets — cannot start server", "error", subErr)
		os.Exit(1)
	}
	embeddedFS := http.FS(staticSub)

	staticFS := embeddedFS
	if dirExists(cfg.StaticPath) {
		staticFS = newOverlayFS(http.Dir(cfg.StaticPath), embeddedFS, logger)
		logger.Info("Static overlay enabled", "local_path", cfg.StaticPath)
	} else {
		logger.Info("Using embedded static assets", "checked_path", cfg.StaticPath)
	}

	router.StaticFS("/static", &gin.OnlyFilesFS{FileSystem: staticFS})

	// sw.js: substituted-version embedded body cached at startup; operator
	// overlay at <STATIC_PATH>/sw.js serves raw bytes (operator owns their
	// cache version, bypassing auto-bump). Startup fail-loud if the embedded
	// placeholder is missing — build invariant.
	swBody, swModTime, swErr := loadServiceWorker(staticSub, swCacheVersion(constants.AppVersion))
	if swErr != nil {
		logger.Error("Failed to load embedded sw.js — cannot start server", "error", swErr)
		os.Exit(1)
	}
	var swLocalFS http.FileSystem
	if dirExists(cfg.StaticPath) {
		swLocalFS = http.Dir(cfg.StaticPath)
	}
	registerGET(router, "/sw.js", serveSwJs(swLocalFS, swBody, swModTime, logger))
	// Uploaded assets — filesystem only, never embedded
	if cfg.Upload.Path != "" {
		if err := os.MkdirAll(cfg.Upload.Path, 0o755); err != nil { //nolint:gosec // upload dir needs to be accessible
			logger.Error("Could not create upload directory — uploads may not work",
				"path", cfg.Upload.Path, "error", err)
		}
		// Verify the upload directory is writable
		if tmpFile, err := os.CreateTemp(cfg.Upload.Path, ".write-check-*"); err != nil {
			logger.Warn("Upload directory is not writable", "path", cfg.Upload.Path, "error", err)
		} else {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}
		// Always register the static route. The upload handler creates
		// slug subdirectories per-request; Gin's Static serves existing files.
		// Security headers: nosniff prevents browsers from MIME-sniffing HTML
		// inside benign extensions; attachment forces download instead of inline render.
		uploadsGroup := router.Group("/uploads")
		uploadsGroup.Use(func(c *gin.Context) {
			c.Header("X-Content-Type-Options", "nosniff")
			ext := strings.ToLower(filepath.Ext(c.Request.URL.Path))
			switch ext {
			case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".ico":
				c.Header("Content-Disposition", "inline")
			default:
				c.Header("Content-Disposition", "attachment")
			}
			c.Next()
		})
		uploadsGroup.Static("/", cfg.Upload.Path)
	}

	// Redirect legacy /favicon.ico to SVG favicon
	registerGET(router, "/favicon.ico", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/static/img/favicon.svg")
	})
	registerGET(router, "/robots.txt", h.Syndication.RobotsTxt)
	registerGET(router, "/humans.txt", h.Syndication.HumansTxt)

	// Health check, manifest, offline (public — used by uptime probes + PWA)
	// /metrics moved into the admin group (line ~419) — it returns admin-tier
	// data (memory, goroutines, environment) and must not be public.
	registerGET(router, "/health", h.Health.Health)
	registerGET(router, "/manifest.json", h.Health.Manifest)
	registerGET(router, "/offline", h.Health.Offline)

	// Public routes
	registerGET(router, "/", h.Feed.Home)
	registerGET(router, "/writing", h.Post.Articles)
	registerGET(router, "/writing/:slug", h.Post.Article)
	registerGET(router, "/p", h.Post.Pages)
	registerGET(router, "/p/:slug", h.Post.Page)
	registerGET(router, "/tags", h.Taxonomy.Tags)
	registerGET(router, "/tags/:tag", h.Taxonomy.ArticlesByTag)
	registerGET(router, "/categories", h.Taxonomy.Categories)
	registerGET(router, "/categories/:category", h.Taxonomy.ArticlesByCategory)
	registerGET(router, "/search", h.Search.Search)
	registerGET(router, "/about", h.About.ShowAbout)

	// /contact redirects to about page; POST stays for form submission
	registerGET(router, "/contact", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/about#contact")
	})
	// Contact: no CSRF (public form, no session side effects, JSON-only, rate-limited)
	contactGroup := router.Group("/contact")
	contactGroup.Use(middleware.RateLimit(cfg.RateLimit.Contact.Requests, cfg.RateLimit.Contact.Window))
	contactGroup.POST("", h.Contact.Submit)

	// AMA: public submission, rate-limited (reuses contact config), no CSRF (public, JSON-only)
	if h.AMA != nil {
		amaGroup := router.Group("/ama")
		amaGroup.Use(middleware.RateLimit(cfg.RateLimit.Contact.Requests, cfg.RateLimit.Contact.Window))
		amaGroup.POST("/submit", h.AMA.Submit)
	}

	// Feeds and SEO
	registerGET(router, "/feed.xml", h.Syndication.RSS)
	registerGET(router, "/feed.json", h.Syndication.JSONFeed)
	registerGET(router, "/sitemap.xml", h.Syndication.Sitemap)

	// Login/logout routes (public, CSRF on login POST). The login-specific rate
	// limit runs before CSRF so credential-stuffing floods are throttled early,
	// before the stateful token check — a stricter bucket than the global limiter.
	if h.Auth != nil {
		loginGroup := router.Group("/login")
		loginGroup.Use(middleware.RateLimit(cfg.RateLimit.Login.Requests, cfg.RateLimit.Login.Window))
		loginGroup.Use(middleware.CSRF(secureCookie))
		loginGroup.POST("", h.Auth.HandleLogin)
		registerGET(router, "/logout", h.Auth.HandleLogout)
	}

	// Compose routes — all gated behind auth (owner-only feature)
	if cfg.Admin.Username != "" && cfg.Admin.Password != "" && h.Compose != nil {
		composeGroup := router.Group("/compose")
		composeGroup.Use(
			middleware.RecoveryWithErrorHandler(logger),
			middleware.SoftSessionAuth(sessionStore, secureCookie),
			middleware.NoCache(),
			middleware.CSRF(secureCookie),
		)
		registerGET(composeGroup, "", h.Compose.ShowCompose)
		registerGET(composeGroup, "/new-page", h.Compose.ShowComposeNewPage)
		composeGroup.POST("", h.Compose.HandleSubmit)
		registerGET(composeGroup, "/edit/:slug", h.Compose.ShowEdit)
		composeGroup.POST("/edit/:slug", h.Compose.HandleEdit)
		composeGroup.POST("/preview", h.Compose.Preview)
		composeGroup.POST("/upload/:slug",
			middleware.RateLimit(cfg.RateLimit.Upload.Requests, cfg.RateLimit.Upload.Window),
			h.Compose.Upload)
		composeGroup.POST("/quick", h.Compose.HandleQuickPublish)
		composeGroup.POST("/publish/:slug", h.Compose.PublishDraft)
	}

	// Admin routes (soft auth — renders login popover when unauthenticated)
	if cfg.Admin.Username != "" && cfg.Admin.Password != "" {
		adminGroup := router.Group("/admin")
		adminGroup.Use(
			middleware.RecoveryWithErrorHandler(logger),
			middleware.SoftSessionAuth(sessionStore, secureCookie),
			middleware.NoCache(),
			middleware.CSRF(secureCookie),
		)
		registerGET(adminGroup, "", h.Admin.AdminHome)
		registerGET(adminGroup, "/writing", h.Admin.Writing)
		registerGET(adminGroup, "/drafts", h.Admin.Drafts)
		adminGroup.POST("/cache/clear", h.ClearCache)
		registerGET(adminGroup, "/stats", h.Admin.Stats)
		registerGET(adminGroup, "/metrics", h.Admin.Metrics)
		adminGroup.POST("/articles/reload", h.Admin.ReloadArticles)

		// AMA moderation routes
		if h.AMA != nil {
			registerGET(adminGroup, "/ama", h.AMA.ListPending)
			adminGroup.POST("/ama/:slug/answer", h.AMA.Answer)
			adminGroup.POST("/ama/:slug/delete", h.AMA.Delete)
		}
	}

	// Debug endpoints (development only)
	if cfg.Environment == envDevelopment {
		debugGroup := router.Group("/debug")

		if cfg.Admin.Username != "" && cfg.Admin.Password != "" {
			debugGroup.Use(middleware.SessionAuth(sessionStore))
			logger.Info("Debug endpoints enabled with authentication", "environment", cfg.Environment)
		} else {
			logger.Warn("Debug endpoints enabled WITHOUT authentication - configure ADMIN_USERNAME/PASSWORD for security")
		}

		debugGroup.GET("/memory", h.Admin.Debug)
		debugGroup.GET("/runtime", h.Admin.Debug)
		debugGroup.GET("/config", h.Admin.Debug)
		debugGroup.GET("/requests", h.Admin.Debug)

		// Go pprof profiling endpoints — registered directly via stdlib
		pprofGroup := debugGroup.Group("/pprof")
		pprofGroup.GET("/", gin.WrapF(pprof.Index))
		pprofGroup.GET("/cmdline", gin.WrapF(pprof.Cmdline))
		pprofGroup.GET("/profile", gin.WrapF(pprof.Profile))
		pprofGroup.GET("/symbol", gin.WrapF(pprof.Symbol))
		pprofGroup.GET("/trace", gin.WrapF(pprof.Trace))
		pprofGroup.GET("/heap", gin.WrapH(pprof.Handler("heap")))
		pprofGroup.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		pprofGroup.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
		pprofGroup.GET("/block", gin.WrapH(pprof.Handler("block")))
		pprofGroup.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
	}

	// 404 handler
	router.NoRoute(h.NotFound)
}

func printUsage() {
	fmt.Printf(`markgo serve - Start the blog server

USAGE:
    markgo serve [options]

OPTIONS:
    --port PORT    Override server port (default: from .env or 3000)
    --help         Show this help message

CONFIGURATION:
    Most server settings are configured via .env file.
    Run 'markgo init' to generate a default configuration.
    See docs/configuration.md for all options.

EXAMPLES:
    markgo serve              # Start with .env configuration
    markgo serve --port 8080  # Start on port 8080

`)
}

// setupTemplates configures Gin's HTML template renderer using TemplateService
func setupTemplates(router *gin.Engine, templateService *services.TemplateService) error {
	// Validate that required templates exist
	requiredTemplates := []string{
		"base.html", "feed.html", "compose.html", "article.html", "articles.html",
		"404.html", "500.html", "offline.html", "about.html", "search.html", "tags.html", "categories.html",
		"drafts.html",
		"admin_home.html",
		"admin_writing.html",
		"admin_ama.html",
		"category.html",
		"tag.html",
		"pages.html",
	}

	for _, tmplName := range requiredTemplates {
		if !templateService.HasTemplate(tmplName) {
			return fmt.Errorf("required template %s not found", tmplName)
		}
	}

	// Get the internal template from TemplateService
	tmpl := templateService.GetTemplate()
	if tmpl == nil {
		return fmt.Errorf("template service has no loaded templates")
	}

	// Set the HTML template renderer
	router.SetHTMLTemplate(tmpl)

	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
