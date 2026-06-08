// Package config provides configuration management for MarkGo blog engine.
// It handles environment variable parsing, validation, and structured configuration
// for all application components including server, cache, email, and preview services.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

// Environment constants
const (
	DevelopmentEnvironment = "development"
	ProductionEnvironment  = "production"
	TestEnvironment        = "test"
)

// Config represents the main application configuration.
type Config struct {
	Environment   string `json:"environment"`
	Port          int    `json:"port"`
	ArticlesPath  string `json:"articles_path"`
	StaticPath    string `json:"static_path"`
	TemplatesPath string `json:"templates_path"`
	BaseURL       string `json:"base_url"`

	// TrustedProxies is the set of normalized CIDRs (bare IPs widened to /32 or
	// /128) from which X-Forwarded-For / X-Real-IP may be trusted. Parsed and
	// validated from TRUSTED_PROXIES at load (fail-loud on malformed). nil when
	// unset: the server then trusts the loopback defaults (see
	// middleware.EffectiveTrustedProxies) so a same-host reverse proxy keys the
	// rate limiter on the real client, while a forged header from a public peer
	// is still ignored. Operator-set CIDRs are used verbatim (this field stays
	// nil when unset — the loopback default is applied at wiring, not here).
	TrustedProxies []string `json:"trusted_proxies"`

	Server    ServerConfig    `json:"server"`
	Cache     CacheConfig     `json:"cache"`
	Email     EmailConfig     `json:"email"`
	RateLimit RateLimitConfig `json:"rate_limit"`
	CORS      CORSConfig      `json:"cors"`
	Admin     AdminConfig     `json:"admin"`
	Blog      BlogConfig      `json:"blog"`
	About     AboutConfig     `json:"about"`
	Logging   LoggingConfig   `json:"logging"`
	SEO       SEOConfig       `json:"seo"`
	Upload    UploadConfig    `json:"upload"`
	AMA       AMAConfig       `json:"ama"`
	Security  SecurityConfig  `json:"security"`

	// OrphanSweepDisabled skips the post-boot orphan-upload sweep when true.
	// Default false: the sweep runs once per startup, removing files in
	// <UPLOAD_PATH>/<slug>/ that no article (banner field or body content)
	// references. Operators who manually drop assets into the uploads tree
	// without referencing them from an article should set this to true.
	OrphanSweepDisabled bool `json:"orphan_sweep_disabled"`
}

// SecurityConfig holds security-header configuration. Most security headers
// always ship (X-Content-Type-Options, X-Frame-Options, Referrer-Policy,
// HSTS, Permissions-Policy); only Content-Security-Policy is operator-toggleable
// because edge proxies (Caddy, nginx) often emit their own policy.
type SecurityConfig struct {
	CSPDisable bool `json:"csp_disable"` // CSP_DISABLE=true skips the Content-Security-Policy header (use when edge proxy emits its own)
}

// AMAConfig holds operator-controlled copy for the AMA (Ask Me Anything) submission
// surface. Plaintext only — values are HTML-attribute-escaped on render; multi-line
// values render as a single block per HTML whitespace rules. Operators wanting
// paragraph breaks override the template via TEMPLATES_PATH.
type AMAConfig struct {
	PageHeading     string `json:"page_heading"`
	PageIntro       string `json:"page_intro"`
	FormPlaceholder string `json:"form_placeholder"`
	SubmitLabel     string `json:"submit_label"`
	ThankyouCopy    string `json:"thankyou_copy"`
}

// ServerConfig holds server-related configuration options.
type ServerConfig struct {
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

// CacheConfig holds cache-related configuration options.
type CacheConfig struct {
	TTL             time.Duration `json:"ttl"`
	MaxSize         int           `json:"max_size"`
	CleanupInterval time.Duration `json:"cleanup_interval"`
}

// EmailConfig holds email-related configuration options.
type EmailConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	UseSSL   bool   `json:"use_ssl"`
}

// RateLimitConfig holds rate limiting configuration options.
type RateLimitConfig struct {
	General RateLimit `json:"general"`
	Contact RateLimit `json:"contact"`
	Upload  RateLimit `json:"upload"`
	Login   RateLimit `json:"login"`
}

// RateLimit defines rate limiting parameters.
type RateLimit struct {
	Requests int           `json:"requests"`
	Window   time.Duration `json:"window"`
}

// CORSConfig holds CORS-related configuration options.
type CORSConfig struct {
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
}

// AdminConfig holds admin authentication configuration.
type AdminConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BlogConfig holds blog-specific configuration options.
type BlogConfig struct {
	Title        string `json:"title"`
	Tagline      string `json:"tagline"`
	Description  string `json:"description"`
	Author       string `json:"author"`
	AuthorEmail  string `json:"author_email"`
	Language     string `json:"language"`
	Theme        string `json:"theme"`
	Style        string `json:"style"`
	PostsPerPage int    `json:"posts_per_page"`
}

// LoggingConfig holds logging-related configuration options.
type LoggingConfig struct {
	Level      string `json:"level"`          // debug, info, warn, error
	Format     string `json:"format"`         // json, text
	Output     string `json:"output"`         // stdout, stderr, file
	File       string `json:"file,omitempty"` // log file path when output=file
	MaxSize    int    `json:"max_size"`       // max size in MB before rotation
	MaxBackups int    `json:"max_backups"`    // max number of backup files to keep
	MaxAge     int    `json:"max_age"`        // max age in days to keep backups
	Compress   bool   `json:"compress"`       // compress rotated files
	AddSource  bool   `json:"add_source"`     // add source file and line number
	TimeFormat string `json:"time_format"`    // custom time format for text logs
}

// AboutConfig holds about page configuration options.
type AboutConfig struct {
	Avatar       string `json:"avatar"`        // path relative to static dir
	Tagline      string `json:"tagline"`       // one-liner under name
	Bio          string `json:"bio"`           // markdown text (alt to about.md)
	Location     string `json:"location"`      // e.g. "San Francisco, CA"
	GitHub       string `json:"github"`        // username or full URL
	Twitter      string `json:"twitter"`       // handle or full URL
	LinkedIn     string `json:"linkedin"`      // full URL
	Mastodon     string `json:"mastodon"`      // full URL
	Website      string `json:"website"`       // full URL
	ReachHeading string `json:"reach_heading"` // h2 above the reach cards
	EmailHeading string `json:"email_heading"` // email-card h3
	EmailIntro   string `json:"email_intro"`   // email-card descriptive text
}

// UploadConfig holds file upload configuration options.
type UploadConfig struct {
	Path    string `json:"path"`
	MaxSize int64  `json:"max_size"`
}

// SEOConfig holds SEO-related configuration options.
type SEOConfig struct {
	Enabled            bool     `json:"enabled"`
	SitemapEnabled     bool     `json:"sitemap_enabled"`
	SchemaEnabled      bool     `json:"schema_enabled"`
	OpenGraphEnabled   bool     `json:"open_graph_enabled"`
	TwitterCardEnabled bool     `json:"twitter_card_enabled"`
	RobotsAllowed      []string `json:"robots_allowed"`
	RobotsDisallowed   []string `json:"robots_disallowed"`
	RobotsCrawlDelay   int      `json:"robots_crawl_delay"`
	DefaultImage       string   `json:"default_image"`
	TwitterSite        string   `json:"twitter_site"`
	TwitterCreator     string   `json:"twitter_creator"`
	FacebookAppID      string   `json:"facebook_app_id"`
	GoogleSiteVerify   string   `json:"google_site_verify"`
	BingSiteVerify     string   `json:"bing_site_verify"`
}

// Load reads and validates the application configuration from environment variables.
func Load() (*Config, error) {
	// Load .env file if it exists (ignore errors as it's optional)
	if err := godotenv.Load(); err != nil {
		// Silently ignore .env file loading errors as it's optional
		// This is intentional behavior - .env files are not required
		_ = err // Explicitly acknowledge the ignored error
	}

	// Get environment first to determine appropriate defaults
	environment := getEnv("ENVIRONMENT", "development")

	// Set environment-aware rate limit defaults
	var generalRequestsDefault int
	var contactRequestsDefault int
	var uploadRequestsDefault int
	var loginRequestsDefault int

	switch environment {
	case ProductionEnvironment:
		// Conservative limits for production
		generalRequestsDefault = 100 // 100 requests per 15 min = ~0.11 req/sec
		contactRequestsDefault = 5   // 5 contact submissions per hour
		uploadRequestsDefault = 20   // 20 uploads per 5 min
		loginRequestsDefault = 5     // 5 login attempts per 15 min — fat-finger tolerant, brute-force hostile
	case TestEnvironment:
		// Higher limits for automated testing
		generalRequestsDefault = 5000 // 5000 requests per 15 min = ~5.5 req/sec
		contactRequestsDefault = 50   // 50 contact submissions per hour for test suites
		uploadRequestsDefault = 500   // 500 uploads per 5 min for test suites
		loginRequestsDefault = 500    // 500 login attempts per 15 min so suites driving /login don't self-throttle
	default: // development
		// Permissive limits for development and manual testing
		generalRequestsDefault = 3000 // 3000 requests per 15 min = ~3.3 req/sec
		contactRequestsDefault = 20   // 20 contact submissions per hour
		uploadRequestsDefault = 100   // 100 uploads per 5 min
		loginRequestsDefault = 50     // 50 login attempts per 15 min — generous for local iteration
	}

	// Parse + validate trusted proxies up front so a malformed CIDR fails the
	// whole load (a silently-dropped entry would re-open XFF spoofing).
	trustedProxiesRaw := getEnv("TRUSTED_PROXIES", "")
	trustedProxies, err := parseTrustedProxies(trustedProxiesRaw)
	if err != nil {
		return nil, apperrors.NewConfigError("trusted_proxies", trustedProxiesRaw,
			"Invalid TRUSTED_PROXIES (expect comma-separated CIDRs and/or bare IPs)", err)
	}

	cfg := &Config{
		Environment:    environment,
		Port:           getEnvInt("PORT", 3000),
		ArticlesPath:   getEnv("ARTICLES_PATH", "./articles"),
		TrustedProxies: trustedProxies,
		StaticPath:     getEnv("STATIC_PATH", ""),
		TemplatesPath:  getEnv("TEMPLATES_PATH", ""),
		BaseURL:        getEnv("BASE_URL", "http://localhost:3000"),

		Server: ServerConfig{
			ReadTimeout:     getEnvDuration("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getEnvDuration("SERVER_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getEnvDuration("SERVER_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		},

		Cache: CacheConfig{
			TTL:             getEnvDuration("CACHE_TTL", 1*time.Hour),
			MaxSize:         getEnvInt("CACHE_MAX_SIZE", 1000),
			CleanupInterval: getEnvDuration("CACHE_CLEANUP_INTERVAL", 10*time.Minute),
		},

		Email: EmailConfig{
			Host:     getEnv("EMAIL_HOST", ""),
			Port:     getEnvInt("EMAIL_PORT", 587),
			Username: getEnv("EMAIL_USERNAME", ""),
			Password: getEnv("EMAIL_PASSWORD", ""),
			From:     getEnv("EMAIL_FROM", "noreply@yourdomain.com"),
			To:       getEnv("EMAIL_TO", "your.email@example.com"),
			UseSSL:   getEnvBool("EMAIL_USE_SSL", true),
		},

		RateLimit: RateLimitConfig{
			General: RateLimit{
				Requests: getEnvInt("RATE_LIMIT_GENERAL_REQUESTS", generalRequestsDefault),
				Window:   getEnvDuration("RATE_LIMIT_GENERAL_WINDOW", 15*time.Minute),
			},
			Contact: RateLimit{
				Requests: getEnvInt("RATE_LIMIT_CONTACT_REQUESTS", contactRequestsDefault),
				Window:   getEnvDuration("RATE_LIMIT_CONTACT_WINDOW", 1*time.Hour),
			},
			Upload: RateLimit{
				Requests: getEnvInt("RATE_LIMIT_UPLOAD_REQUESTS", uploadRequestsDefault),
				Window:   getEnvDuration("RATE_LIMIT_UPLOAD_WINDOW", 5*time.Minute),
			},
			Login: RateLimit{
				Requests: getEnvInt("RATE_LIMIT_LOGIN_REQUESTS", loginRequestsDefault),
				Window:   getEnvDuration("RATE_LIMIT_LOGIN_WINDOW", 15*time.Minute),
			},
		},

		CORS: CORSConfig{
			AllowedOrigins: getEnvSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "https://yourdomain.com"}),
			AllowedMethods: getEnvSlice("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
			AllowedHeaders: getEnvSlice("CORS_ALLOWED_HEADERS", []string{"Origin", "Content-Type", "Accept", "Authorization"}),
		},

		Admin: AdminConfig{
			Username: getEnv("ADMIN_USERNAME", ""),
			Password: getEnv("ADMIN_PASSWORD", ""),
		},

		Blog: BlogConfig{
			Title:        getEnv("BLOG_TITLE", "Your Blog Title"),
			Tagline:      getEnv("BLOG_TAGLINE", ""),
			Description:  getEnv("BLOG_DESCRIPTION", "Your blog description goes here"),
			Author:       getEnv("BLOG_AUTHOR", "Your Name"),
			AuthorEmail:  getEnv("BLOG_AUTHOR_EMAIL", ""),
			Language:     getEnv("BLOG_LANGUAGE", "en"),
			Theme:        getEnv("BLOG_THEME", "default"),
			Style:        getEnv("BLOG_STYLE", "minimal"),
			PostsPerPage: getEnvInt("BLOG_POSTS_PER_PAGE", 10),
		},

		About: AboutConfig{
			Avatar:       getEnv("ABOUT_AVATAR", ""),
			Tagline:      getEnv("ABOUT_TAGLINE", ""),
			Bio:          getEnv("ABOUT_BIO", ""),
			Location:     getEnv("ABOUT_LOCATION", ""),
			GitHub:       getEnv("ABOUT_GITHUB", ""),
			Twitter:      getEnv("ABOUT_TWITTER", ""),
			LinkedIn:     getEnv("ABOUT_LINKEDIN", ""),
			Mastodon:     getEnv("ABOUT_MASTODON", ""),
			Website:      getEnv("ABOUT_WEBSITE", ""),
			ReachHeading: getEnv("ABOUT_REACH_HEADING", "Reach out"),
			EmailHeading: getEnv("ABOUT_EMAIL_HEADING", "Email"),
			EmailIntro:   getEnv("ABOUT_EMAIL_INTRO", "Or drop a line directly."),
		},

		Logging: LoggingConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "json"),
			Output:     getEnv("LOG_OUTPUT", "stdout"),
			File:       getEnv("LOG_FILE", ""),
			MaxSize:    getEnvInt("LOG_MAX_SIZE", 100),
			MaxBackups: getEnvInt("LOG_MAX_BACKUPS", 3),
			MaxAge:     getEnvInt("LOG_MAX_AGE", 28),
			Compress:   getEnvBool("LOG_COMPRESS", true),
			AddSource:  getEnvBool("LOG_ADD_SOURCE", false),
			TimeFormat: getEnv("LOG_TIME_FORMAT", "2006-01-02T15:04:05Z07:00"),
		},

		SEO: SEOConfig{
			Enabled:            getEnvBool("SEO_ENABLED", true),
			SitemapEnabled:     getEnvBool("SEO_SITEMAP_ENABLED", true),
			SchemaEnabled:      getEnvBool("SEO_SCHEMA_ENABLED", true),
			OpenGraphEnabled:   getEnvBool("SEO_OPEN_GRAPH_ENABLED", true),
			TwitterCardEnabled: getEnvBool("SEO_TWITTER_CARD_ENABLED", true),
			RobotsAllowed:      getEnvSlice("SEO_ROBOTS_ALLOWED", []string{"/"}),
			RobotsDisallowed:   getEnvSlice("SEO_ROBOTS_DISALLOWED", []string{"/admin", "/api"}),
			RobotsCrawlDelay:   getEnvInt("SEO_ROBOTS_CRAWL_DELAY", 1),
			DefaultImage:       getEnv("SEO_DEFAULT_IMAGE", ""),
			TwitterSite:        getEnv("SEO_TWITTER_SITE", ""),
			TwitterCreator:     getEnv("SEO_TWITTER_CREATOR", ""),
			FacebookAppID:      getEnv("SEO_FACEBOOK_APP_ID", ""),
			GoogleSiteVerify:   getEnv("SEO_GOOGLE_SITE_VERIFY", ""),
			BingSiteVerify:     getEnv("SEO_BING_SITE_VERIFY", ""),
		},

		Upload: UploadConfig{
			Path:    getEnv("UPLOAD_PATH", "./uploads"),
			MaxSize: getEnvInt64("UPLOAD_MAX_SIZE", 10<<20),
		},

		AMA: AMAConfig{
			PageHeading:     getEnv("AMA_PAGE_HEADING", "Ask me anything"),
			PageIntro:       getEnv("AMA_PAGE_INTRO", "Curious about something? Submit a question and I'll answer it publicly."),
			FormPlaceholder: getEnv("AMA_FORM_PLACEHOLDER", "What would you like to know?"),
			SubmitLabel:     getEnv("AMA_SUBMIT_LABEL", "Submit Question"),
			ThankyouCopy:    getEnv("AMA_THANKYOU_COPY", "Question submitted! It will appear once answered."),
		},

		Security: SecurityConfig{
			CSPDisable: getEnvBool("CSP_DISABLE", false),
		},

		OrphanSweepDisabled: getEnvBool("ORPHAN_SWEEP_DISABLED", false),
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
		// Structured logging (slog key/value) is not vulnerable to log injection —
		// values are encoded as JSON or key=value pairs, not interpolated.
		slog.Warn("Invalid int64 env var, using default", //nolint:gosec // G706: slog encodes values safely
			"key", key, "value", value, "default", defaultValue)
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Simple split by comma - in production you might want a more robust parser
		return splitString(value, ",")
	}
	return defaultValue
}

func splitString(s, delimiter string) []string {
	if s == "" {
		return []string{}
	}

	var result []string
	parts := strings.SplitSeq(s, delimiter)
	for part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parseTrustedProxies parses a comma-separated TRUSTED_PROXIES value into a
// normalized list of CIDR strings suitable for gin's SetTrustedProxies. Bare
// IPs are widened to a single-host CIDR (/32 for IPv4, /128 for IPv6); entries
// already in CIDR form are validated with net.ParseCIDR. Any malformed entry is
// a hard error — silently dropping one would re-open X-Forwarded-For spoofing.
// Empty input returns (nil, nil); the server then applies the loopback defaults
// at wiring time (see middleware.EffectiveTrustedProxies).
func parseTrustedProxies(raw string) ([]string, error) {
	entries := splitString(raw, ",")
	if len(entries) == 0 {
		return nil, nil
	}

	cidrs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", entry, err)
			}
			cidrs = append(cidrs, entry)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP %q", entry)
		}
		if ip.To4() != nil {
			cidrs = append(cidrs, entry+"/32")
		} else {
			cidrs = append(cidrs, entry+"/128")
		}
	}
	return cidrs, nil
}

// Validate validates all configuration settings
func (c *Config) Validate() error {
	// Validate basic fields
	if err := c.validateBasic(); err != nil {
		return err
	}

	// Validate server configuration
	if err := c.Server.Validate(); err != nil {
		return apperrors.NewConfigError("server", c.Server, "Invalid server configuration", err)
	}

	// Validate cache configuration
	if err := c.Cache.Validate(); err != nil {
		return apperrors.NewConfigError("cache", c.Cache, "Invalid cache configuration", err)
	}

	// Validate email configuration (only if configured)
	if c.Email.Username != "" || c.Email.Password != "" {
		if err := c.Email.Validate(); err != nil {
			return apperrors.NewConfigError("email", c.Email, "Invalid email configuration", err)
		}
	}

	// Validate rate limit configuration
	if err := c.RateLimit.Validate(); err != nil {
		return apperrors.NewConfigError("rate_limit", c.RateLimit, "Invalid rate limit configuration", err)
	}

	// Validate blog configuration
	if err := c.Blog.Validate(); err != nil {
		return apperrors.NewConfigError("blog", c.Blog, "Invalid blog configuration", err)
	}

	// Validate admin configuration
	if err := c.Admin.Validate(); err != nil {
		return apperrors.NewConfigError("admin", c.Admin, "Invalid admin configuration", err)
	}

	// Validate logging configuration
	if err := c.Logging.Validate(); err != nil {
		return apperrors.NewConfigError("logging", c.Logging, "Invalid logging configuration", err)
	}

	// Validate CORS configuration
	if err := c.CORS.Validate(); err != nil {
		return apperrors.NewConfigError("cors", c.CORS, "Invalid CORS configuration", err)
	}

	// Validate upload configuration
	if err := c.Upload.Validate(); err != nil {
		return apperrors.NewConfigError("upload", c.Upload, "Invalid upload configuration", err)
	}

	return nil
}

func (c *Config) validateBasic() error {
	// Validate port
	if c.Port <= 0 || c.Port > 65535 {
		return apperrors.NewConfigError("port", c.Port, "Port must be between 1 and 65535", apperrors.ErrConfigValidation)
	}

	// Validate paths exist and are accessible
	// ArticlesPath must exist — it's user content
	if err := validatePath(c.ArticlesPath, "articles_path"); err != nil {
		return err
	}

	// StaticPath and TemplatesPath are optional — binary has embedded fallbacks
	if c.StaticPath != "" {
		warnMissingPath(c.StaticPath, "static_path")
	}
	if c.TemplatesPath != "" {
		warnMissingPath(c.TemplatesPath, "templates_path")
	}

	// Validate base URL
	if c.BaseURL == "" {
		return apperrors.NewConfigError("base_url", c.BaseURL, "Base URL cannot be empty", apperrors.ErrMissingConfig)
	}

	if _, err := url.Parse(c.BaseURL); err != nil {
		return apperrors.NewConfigError("base_url", c.BaseURL, "Invalid base URL format", apperrors.ErrConfigValidation)
	}

	// Validate environment
	validEnvs := []string{DevelopmentEnvironment, ProductionEnvironment, TestEnvironment}
	if !contains(validEnvs, c.Environment) {
		return apperrors.NewConfigError("environment", c.Environment,
			"Environment must be one of: development, production, test", apperrors.ErrConfigValidation)
	}

	return nil
}

// Validate server configuration
func (s *ServerConfig) Validate() error {
	if s.ReadTimeout <= 0 {
		return apperrors.NewConfigError("read_timeout", s.ReadTimeout,
			"Read timeout must be positive", apperrors.ErrConfigValidation)
	}
	if s.WriteTimeout <= 0 {
		return apperrors.NewConfigError("write_timeout", s.WriteTimeout,
			"Write timeout must be positive", apperrors.ErrConfigValidation)
	}
	if s.IdleTimeout <= 0 {
		return apperrors.NewConfigError("idle_timeout", s.IdleTimeout,
			"Idle timeout must be positive", apperrors.ErrConfigValidation)
	}
	return nil
}

// Validate cache configuration
func (c *CacheConfig) Validate() error {
	if c.TTL <= 0 {
		return apperrors.NewConfigError("ttl", c.TTL, "Cache TTL must be positive", apperrors.ErrConfigValidation)
	}
	if c.MaxSize <= 0 {
		return apperrors.NewConfigError("max_size", c.MaxSize,
			"Cache max size must be positive", apperrors.ErrConfigValidation)
	}
	if c.CleanupInterval <= 0 {
		return apperrors.NewConfigError("cleanup_interval", c.CleanupInterval,
			"Cleanup interval must be positive", apperrors.ErrConfigValidation)
	}
	return nil
}

// Validate email configuration
func (e *EmailConfig) Validate() error {
	if e.Host == "" {
		return apperrors.NewConfigError("host", e.Host, "Email host cannot be empty", apperrors.ErrMissingConfig)
	}
	if e.Port <= 0 || e.Port > 65535 {
		return apperrors.NewConfigError("port", e.Port,
			"Email port must be between 1 and 65535", apperrors.ErrConfigValidation)
	}
	if e.Username == "" {
		return apperrors.NewConfigError("username", e.Username,
			"Email username cannot be empty when email is configured", apperrors.ErrMissingConfig)
	}
	if e.Password == "" {
		return apperrors.NewConfigError("password", "***",
			"Email password cannot be empty when email is configured", apperrors.ErrMissingConfig)
	}
	if e.From == "" {
		return apperrors.NewConfigError("from", e.From, "Email 'from' address cannot be empty", apperrors.ErrMissingConfig)
	}
	if e.To == "" {
		return apperrors.NewConfigError("to", e.To, "Email 'to' address cannot be empty", apperrors.ErrMissingConfig)
	}

	// Validate email addresses format
	if _, err := mail.ParseAddress(e.From); err != nil {
		return apperrors.NewConfigError("from", e.From, "Invalid 'from' email address format", apperrors.ErrConfigValidation)
	}
	if _, err := mail.ParseAddress(e.To); err != nil {
		return apperrors.NewConfigError("to", e.To, "Invalid 'to' email address format", apperrors.ErrConfigValidation)
	}

	return nil
}

// Validate rate limit configuration
func (r *RateLimitConfig) Validate() error {
	if err := r.General.Validate("general"); err != nil {
		return err
	}
	if err := r.Contact.Validate("contact"); err != nil {
		return err
	}
	if err := r.Upload.Validate("upload"); err != nil {
		return err
	}
	if err := r.Login.Validate("login"); err != nil {
		return err
	}
	return nil
}

// Validate individual rate limit
func (r *RateLimit) Validate(name string) error {
	if r.Requests <= 0 {
		return apperrors.NewConfigError("requests", r.Requests,
			name+" rate limit requests must be positive", apperrors.ErrConfigValidation)
	}
	if r.Window <= 0 {
		return apperrors.NewConfigError("window", r.Window,
			name+" rate limit window must be positive", apperrors.ErrConfigValidation)
	}
	return nil
}

// Validate blog configuration
func (b *BlogConfig) Validate() error {
	if b.Title == "" {
		return apperrors.NewConfigError("title", b.Title, "Blog title cannot be empty", apperrors.ErrMissingConfig)
	}
	if b.Author == "" {
		return apperrors.NewConfigError("author", b.Author, "Blog author cannot be empty", apperrors.ErrMissingConfig)
	}
	if b.PostsPerPage <= 0 {
		return apperrors.NewConfigError("posts_per_page", b.PostsPerPage,
			"Posts per page must be positive", apperrors.ErrConfigValidation)
	}
	if b.PostsPerPage > 100 {
		return apperrors.NewConfigError("posts_per_page", b.PostsPerPage,
			"Posts per page should not exceed 100 for performance reasons",
			apperrors.ErrConfigValidation)
	}
	if b.Language == "" {
		return apperrors.NewConfigError("language", b.Language, "Blog language cannot be empty", apperrors.ErrMissingConfig)
	}

	// Validate language code format (basic check)
	langRegex := regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)
	if !langRegex.MatchString(b.Language) {
		return apperrors.NewConfigError("language", b.Language,
			"Blog language must be a valid language code (e.g., 'en', 'en-US')",
			apperrors.ErrConfigValidation)
	}

	// Style accepts any non-empty string. The template renders a stylesheet
	// link to /static/css/themes/<style>.css; the file is resolved from
	// STATIC_PATH (overlay, v3.10.2+) or the embedded FS. A missing theme
	// produces a harmless browser 404 on the link, not a server-side failure.
	// Closes #64.

	// Validate author email if provided
	if b.AuthorEmail != "" {
		if _, err := mail.ParseAddress(b.AuthorEmail); err != nil {
			return apperrors.NewConfigError("author_email", b.AuthorEmail,
				"Invalid author email address format", apperrors.ErrConfigValidation)
		}
	}

	return nil
}

// Validate admin configuration
func (a *AdminConfig) Validate() error {
	// Admin config is optional, but if username is provided, password must also be provided
	if a.Username != "" && a.Password == "" {
		return apperrors.NewConfigError("password", "***",
			"Admin password is required when username is provided", apperrors.ErrMissingConfig)
	}
	if a.Username != "" && a.Password == "changeme" {
		return apperrors.NewConfigError("password", "***",
			"Admin password must be changed from default value", apperrors.ErrConfigValidation)
	}
	return nil
}

// Validate logging configuration
func (l *LoggingConfig) Validate() error {
	// Validate log level
	validLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLevels, l.Level) {
		return apperrors.NewConfigError("level", l.Level,
			"Log level must be one of: debug, info, warn, error", apperrors.ErrConfigValidation)
	}

	// Validate log format
	validFormats := []string{"json", "text"}
	if !contains(validFormats, l.Format) {
		return apperrors.NewConfigError("format", l.Format,
			"Log format must be one of: json, text", apperrors.ErrConfigValidation)
	}

	// Validate log output
	validOutputs := []string{"stdout", "stderr", "file"}
	if !contains(validOutputs, l.Output) {
		return apperrors.NewConfigError("output", l.Output,
			"Log output must be one of: stdout, stderr, file", apperrors.ErrConfigValidation)
	}

	// If output is file, file path is required
	if l.Output == "file" && l.File == "" {
		return apperrors.NewConfigError("file", l.File,
			"Log file path is required when output is 'file'", apperrors.ErrMissingConfig)
	}

	// Validate rotation settings
	if l.MaxSize <= 0 {
		return apperrors.NewConfigError("max_size", l.MaxSize, "Log max size must be positive", apperrors.ErrConfigValidation)
	}
	if l.MaxBackups < 0 {
		return apperrors.NewConfigError("max_backups", l.MaxBackups,
			"Log max backups cannot be negative", apperrors.ErrConfigValidation)
	}
	if l.MaxAge < 0 {
		return apperrors.NewConfigError("max_age", l.MaxAge, "Log max age cannot be negative", apperrors.ErrConfigValidation)
	}

	return nil
}

// Validate CORS configuration
func (c *CORSConfig) Validate() error {
	// Validate allowed origins
	for _, origin := range c.AllowedOrigins {
		if origin != "*" {
			if _, err := url.Parse(origin); err != nil {
				return apperrors.NewConfigError("allowed_origins", origin,
					"Invalid CORS allowed origin URL", apperrors.ErrConfigValidation)
			}
		}
	}

	// Validate allowed methods
	validMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	for _, method := range c.AllowedMethods {
		if !contains(validMethods, method) {
			return apperrors.NewConfigError("allowed_methods", method,
				"Invalid HTTP method in CORS allowed methods", apperrors.ErrConfigValidation)
		}
	}

	return nil
}

// Validate upload configuration
func (u *UploadConfig) Validate() error {
	if u.Path == "" {
		return apperrors.NewConfigError("path", u.Path,
			"Upload path cannot be empty", apperrors.ErrMissingConfig)
	}
	if u.MaxSize <= 0 {
		return apperrors.NewConfigError("max_size", u.MaxSize,
			"Upload max size must be positive", apperrors.ErrConfigValidation)
	}
	if u.MaxSize > 100<<20 { // 100MB sanity cap
		return apperrors.NewConfigError("max_size", u.MaxSize,
			"Upload max size should not exceed 100MB", apperrors.ErrConfigValidation)
	}
	return nil
}

// warnMissingPath logs a warning if a configured path doesn't exist or isn't a directory
func warnMissingPath(path, fieldName string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		slog.Warn("Invalid path format, will use embedded assets", "field", fieldName, "path", path, "error", err)
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		slog.Warn("Path not accessible, will use embedded assets", "field", fieldName, "path", path, "error", err)
		return
	}
	if !info.IsDir() {
		slog.Warn("Path is not a directory, will use embedded assets", "field", fieldName, "path", path)
	}
}

// validatePath validates that a path exists and is accessible
func validatePath(path, fieldName string) error {
	if path == "" {
		return apperrors.NewConfigError(fieldName, path, fieldName+" cannot be empty", apperrors.ErrMissingConfig)
	}

	// Convert to absolute path for validation
	absPath, err := filepath.Abs(path)
	if err != nil {
		return apperrors.NewConfigError(fieldName, path, "Invalid path format", apperrors.ErrConfigValidation)
	}

	// Check if directory exists and is actually a directory
	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return apperrors.NewConfigError(fieldName, path, fieldName+" directory does not exist", apperrors.ErrConfigValidation)
	}
	if err != nil {
		return apperrors.NewConfigError(fieldName, path, "Cannot access "+fieldName, apperrors.ErrConfigValidation)
	}
	if !info.IsDir() {
		return apperrors.NewConfigError(fieldName, path, fieldName+" must be a directory", apperrors.ErrConfigValidation)
	}

	return nil
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
