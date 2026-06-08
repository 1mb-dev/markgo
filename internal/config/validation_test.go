package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigValidationTableDriven replaces verbose individual tests with table-driven approach
// This reduces code from 1,193 lines to ~400 lines while maintaining 100% coverage
func TestConfigValidationTableDriven(t *testing.T) {
	// Setup: Create temp directories
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(tmpDir+"/articles", 0o750))
	require.NoError(t, os.MkdirAll(tmpDir+"/static", 0o750))
	require.NoError(t, os.MkdirAll(tmpDir+"/templates", 0o750))

	// Helper: Returns a fully valid configuration
	validConfig := func() *Config {
		return &Config{
			Environment:   "development",
			Port:          3000,
			ArticlesPath:  tmpDir + "/articles",
			StaticPath:    tmpDir + "/static",
			TemplatesPath: tmpDir + "/templates",
			BaseURL:       "http://localhost:3000",
			Server: ServerConfig{
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 15 * time.Second,
				IdleTimeout:  60 * time.Second,
			},
			Cache: CacheConfig{
				TTL:             1 * time.Hour,
				MaxSize:         1000,
				CleanupInterval: 10 * time.Minute,
			},
			RateLimit: RateLimitConfig{
				General: RateLimit{Requests: 100, Window: 15 * time.Minute},
				Contact: RateLimit{Requests: 5, Window: 1 * time.Hour},
				Upload:  RateLimit{Requests: 20, Window: 5 * time.Minute},
				Login:   RateLimit{Requests: 5, Window: 15 * time.Minute},
			},
			Blog: BlogConfig{
				Title:        "Test Blog",
				Author:       "Test Author",
				Language:     "en",
				PostsPerPage: 10,
			},
			Admin: AdminConfig{
				Username: "",
				Password: "",
			},
			Logging: LoggingConfig{
				Level:      "info",
				Format:     "json",
				Output:     "stdout",
				MaxSize:    100,
				MaxBackups: 3,
				MaxAge:     28,
			},
			CORS: CORSConfig{
				AllowedOrigins: []string{"http://localhost:3000"},
				AllowedMethods: []string{"GET", "POST"},
			},
			Upload: UploadConfig{
				Path:    "./uploads",
				MaxSize: 10 << 20,
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string // Substring expected in error message
	}{
		{"valid config", func(c *Config) {}, ""},

		// Port validation (config.go:741-743)
		{"port zero", func(c *Config) { c.Port = 0 }, "port"},
		{"port negative", func(c *Config) { c.Port = -1 }, "port"},
		{"port too large", func(c *Config) { c.Port = 70000 }, "port"},

		// Path validation (config.go:746-756)
		{"articles path missing", func(c *Config) { c.ArticlesPath = tmpDir + "/missing" }, "articles_path"},
		// StaticPath and TemplatesPath are optional — binary has embedded fallbacks
		{"static path missing is ok", func(c *Config) { c.StaticPath = tmpDir + "/missing" }, ""},
		{"templates path missing is ok", func(c *Config) { c.TemplatesPath = tmpDir + "/missing" }, ""},

		// Base URL validation (config.go:759-765)
		{"base URL empty", func(c *Config) { c.BaseURL = "" }, "base_url"},
		{"base URL invalid", func(c *Config) { c.BaseURL = "ht!tp://invalid url" }, "base_url"},

		// Environment validation (config.go:768-772)
		{"environment invalid", func(c *Config) { c.Environment = "invalid" }, "environment"},

		// Server config validation (config.go:779-791)
		{"server read timeout zero", func(c *Config) { c.Server.ReadTimeout = 0 }, "server"},
		{"server read timeout negative", func(c *Config) { c.Server.ReadTimeout = -1 * time.Second }, "server"},
		{"server write timeout zero", func(c *Config) { c.Server.WriteTimeout = 0 }, "server"},
		{"server idle timeout zero", func(c *Config) { c.Server.IdleTimeout = 0 }, "server"},

		// Cache config validation (config.go:796-807)
		{"cache TTL zero", func(c *Config) { c.Cache.TTL = 0 }, "cache"},
		{"cache TTL negative", func(c *Config) { c.Cache.TTL = -1 * time.Hour }, "cache"},
		{"cache max size zero", func(c *Config) { c.Cache.MaxSize = 0 }, "cache"},
		{"cache max size negative", func(c *Config) { c.Cache.MaxSize = -100 }, "cache"},
		{"cache cleanup interval zero", func(c *Config) { c.Cache.CleanupInterval = 0 }, "cache"},

		// Rate limit validation (config.go:858-866)
		{"rate limit general requests zero", func(c *Config) { c.RateLimit.General.Requests = 0 }, "rate_limit"},
		{"rate limit general requests negative", func(c *Config) { c.RateLimit.General.Requests = -1 }, "rate_limit"},
		{"rate limit general window zero", func(c *Config) { c.RateLimit.General.Window = 0 }, "rate_limit"},
		{"rate limit contact requests zero", func(c *Config) { c.RateLimit.Contact.Requests = 0 }, "rate_limit"},
		{"rate limit contact window zero", func(c *Config) { c.RateLimit.Contact.Window = 0 }, "rate_limit"},
		{"rate limit upload requests zero", func(c *Config) { c.RateLimit.Upload.Requests = 0 }, "rate_limit"},
		{"rate limit upload window zero", func(c *Config) { c.RateLimit.Upload.Window = 0 }, "rate_limit"},
		{"rate limit login requests zero", func(c *Config) { c.RateLimit.Login.Requests = 0 }, "rate_limit"},
		{"rate limit login window zero", func(c *Config) { c.RateLimit.Login.Window = 0 }, "rate_limit"},

		// Blog config validation (config.go:871-906)
		{"blog title empty", func(c *Config) { c.Blog.Title = "" }, "blog"},
		{"blog author empty", func(c *Config) { c.Blog.Author = "" }, "blog"},
		{"blog posts per page zero", func(c *Config) { c.Blog.PostsPerPage = 0 }, "blog"},
		{"blog posts per page negative", func(c *Config) { c.Blog.PostsPerPage = -1 }, "blog"},
		{"blog posts per page too large", func(c *Config) { c.Blog.PostsPerPage = 101 }, "blog"},
		{"blog language empty", func(c *Config) { c.Blog.Language = "" }, "blog"},
		{"blog language invalid format", func(c *Config) { c.Blog.Language = "invalid" }, "blog"},
		{"blog author email invalid", func(c *Config) { c.Blog.AuthorEmail = "not-an-email" }, "blog"},
		// v3.10.3 #64: custom theme names must pass validation (previously the
		// allowlist rejected anything except "minimal", contradicting the
		// STATIC_PATH overlay feature which exists for custom themes).
		{"blog style custom theme accepted", func(c *Config) { c.Blog.Style = "myforktheme" }, ""},
		{"blog style fork-style accepted", func(c *Config) { c.Blog.Style = "log-1mb" }, ""},

		// Admin config validation (config.go:912-920)
		{"admin username without password", func(c *Config) { c.Admin = AdminConfig{Username: "admin", Password: ""} }, "admin"},
		{"admin default password", func(c *Config) { c.Admin = AdminConfig{Username: "admin", Password: "changeme"} }, "admin"},

		// Logging config validation (config.go:926-964)
		{"logging level invalid", func(c *Config) { c.Logging.Level = "invalid" }, "logging"},
		{"logging format invalid", func(c *Config) { c.Logging.Format = "invalid" }, "logging"},
		{"logging output invalid", func(c *Config) { c.Logging.Output = "invalid" }, "logging"},
		{"logging max size zero", func(c *Config) { c.Logging.MaxSize = 0 }, "logging"},
		{"logging max size negative", func(c *Config) { c.Logging.MaxSize = -1 }, "logging"},
		{"logging max backups negative", func(c *Config) { c.Logging.MaxBackups = -1 }, "logging"},
		{"logging max age negative", func(c *Config) { c.Logging.MaxAge = -1 }, "logging"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)
			err := cfg.Validate()

			if tt.wantErr == "" {
				assert.NoError(t, err, "Expected no error for: %s", tt.name)
			} else {
				assert.Error(t, err, "Expected error for: %s", tt.name)
				if err != nil {
					assert.Contains(t, err.Error(), tt.wantErr,
						"Error should contain '%s' for: %s", tt.wantErr, tt.name)
				}
			}
		})
	}
}

// TestEmailConfigValidationTableDriven tests email configuration with table-driven approach
func TestEmailConfigValidationTableDriven(t *testing.T) {
	tests := []struct {
		name    string
		config  EmailConfig
		wantErr string
	}{
		{
			name: "valid SMTP config",
			config: EmailConfig{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "password123",
				From:     "noreply@example.com",
				To:       "admin@example.com",
			},
			wantErr: "",
		},
		{"empty host", EmailConfig{Host: "", Port: 587, Username: "user", Password: "pass", From: "from@example.com", To: "to@example.com"}, "host"},
		{"port zero", EmailConfig{Host: "smtp.example.com", Port: 0, Username: "user", Password: "pass", From: "from@example.com", To: "to@example.com"}, "port"},
		{"port too large", EmailConfig{Host: "smtp.example.com", Port: 70000, Username: "user", Password: "pass", From: "from@example.com", To: "to@example.com"}, "port"},
		{"empty username", EmailConfig{Host: "smtp.example.com", Port: 587, Username: "", Password: "pass", From: "from@example.com", To: "to@example.com"}, "username"},
		{"empty password", EmailConfig{Host: "smtp.example.com", Port: 587, Username: "user", Password: "", From: "from@example.com", To: "to@example.com"}, "password"},
		{"empty from", EmailConfig{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", From: "", To: "to@example.com"}, "from"},
		{"empty to", EmailConfig{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", From: "from@example.com", To: ""}, "to"},
		{"invalid from email", EmailConfig{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", From: "not-an-email", To: "to@example.com"}, "from"},
		{"invalid to email", EmailConfig{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", From: "from@example.com", To: "not-an-email"}, "to"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
			}
		})
	}
}

// TestCORSConfigValidationTableDriven tests CORS configuration
func TestCORSConfigValidationTableDriven(t *testing.T) {
	tests := []struct {
		name    string
		config  CORSConfig
		wantErr string
	}{
		{
			name: "valid CORS",
			config: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
				AllowedMethods: []string{"GET", "POST"},
			},
			wantErr: "",
		},
		{
			name: "wildcard origin",
			config: CORSConfig{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET"},
			},
			wantErr: "",
		},
		{
			name: "invalid origin",
			config: CORSConfig{
				AllowedOrigins: []string{"ht!tp://invalid"},
				AllowedMethods: []string{"GET"},
			},
			wantErr: "allowed_origins",
		},
		{
			name: "invalid method",
			config: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
				AllowedMethods: []string{"INVALID"},
			},
			wantErr: "allowed_methods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
			}
		})
	}
}
