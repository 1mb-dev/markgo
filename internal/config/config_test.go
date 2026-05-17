package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// Clear environment variables
	clearEnvVars()

	// Create required directories for validation
	require.NoError(t, os.MkdirAll("./articles", 0o750))
	require.NoError(t, os.MkdirAll("./web/static", 0o750))
	require.NoError(t, os.MkdirAll("./web/templates", 0o750))

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Test default values
	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, 3000, cfg.Port)
	assert.Equal(t, "./articles", cfg.ArticlesPath)
	assert.Equal(t, "", cfg.StaticPath)
	assert.Equal(t, "", cfg.TemplatesPath)
	assert.Equal(t, "http://localhost:3000", cfg.BaseURL)

	// Test server config defaults
	assert.Equal(t, 15*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 15*time.Second, cfg.Server.WriteTimeout)
	assert.Equal(t, 60*time.Second, cfg.Server.IdleTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.ShutdownTimeout)

	// Test cache config defaults
	assert.Equal(t, 1*time.Hour, cfg.Cache.TTL)
	assert.Equal(t, 1000, cfg.Cache.MaxSize)
	assert.Equal(t, 10*time.Minute, cfg.Cache.CleanupInterval)

	// Test email config defaults
	assert.Equal(t, "", cfg.Email.Host)
	assert.Equal(t, 587, cfg.Email.Port)
	assert.Equal(t, "noreply@yourdomain.com", cfg.Email.From)
	assert.Equal(t, "your.email@example.com", cfg.Email.To)
	assert.True(t, cfg.Email.UseSSL)

	// Test rate limit defaults (development environment)
	assert.Equal(t, 3000, cfg.RateLimit.General.Requests) // Development default
	assert.Equal(t, 15*time.Minute, cfg.RateLimit.General.Window)
	assert.Equal(t, 20, cfg.RateLimit.Contact.Requests) // Development default
	assert.Equal(t, 1*time.Hour, cfg.RateLimit.Contact.Window)
	assert.Equal(t, 100, cfg.RateLimit.Upload.Requests) // Development default
	assert.Equal(t, 5*time.Minute, cfg.RateLimit.Upload.Window)

	// Test CORS defaults
	assert.Contains(t, cfg.CORS.AllowedOrigins, "http://localhost:3000")
	assert.Contains(t, cfg.CORS.AllowedOrigins, "https://yourdomain.com")
	assert.Contains(t, cfg.CORS.AllowedMethods, "GET")
	assert.Contains(t, cfg.CORS.AllowedMethods, "POST")

	// Test admin defaults (disabled by default)
	assert.Equal(t, "", cfg.Admin.Username)
	assert.Equal(t, "", cfg.Admin.Password)

	// Test blog defaults
	// Test blog config defaults
	assert.Equal(t, "Your Blog Title", cfg.Blog.Title)
	assert.Equal(t, "Your blog description goes here", cfg.Blog.Description)
	assert.Equal(t, "Your Name", cfg.Blog.Author)
	assert.Equal(t, "your.email@example.com", cfg.Blog.AuthorEmail)
	assert.Equal(t, "en", cfg.Blog.Language)
	assert.Equal(t, "default", cfg.Blog.Theme)
	assert.Equal(t, 10, cfg.Blog.PostsPerPage)

	// Test upload config defaults
	assert.Equal(t, "./uploads", cfg.Upload.Path)
	assert.Equal(t, int64(10<<20), cfg.Upload.MaxSize)

	// Test AMA config defaults — verbatim pre-v3.11.0 copy must round-trip
	assert.Equal(t, "Ask me anything", cfg.AMA.PageHeading)
	assert.Equal(t, "Curious about something? Submit a question and I'll answer it publicly.", cfg.AMA.PageIntro)
	assert.Equal(t, "What would you like to know?", cfg.AMA.FormPlaceholder)
	assert.Equal(t, "Submit Question", cfg.AMA.SubmitLabel)
	assert.Equal(t, "Question submitted! It will appear once answered.", cfg.AMA.ThankyouCopy)
}

func TestLoadWithEnvironmentVariables(t *testing.T) {
	// Clear environment variables first
	clearEnvVars()

	// Create temporary directories for validation
	tmpDir, err := os.MkdirTemp("", "markgo-test-*")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(tmpDir+"/articles", 0o750))
	require.NoError(t, os.MkdirAll(tmpDir+"/static", 0o750))
	require.NoError(t, os.MkdirAll(tmpDir+"/templates", 0o750))

	// Set environment variables
	_ = os.Setenv("ENVIRONMENT", "production")
	_ = os.Setenv("PORT", "8080")
	_ = os.Setenv("ARTICLES_PATH", tmpDir+"/articles")
	_ = os.Setenv("STATIC_PATH", tmpDir+"/static")
	_ = os.Setenv("TEMPLATES_PATH", tmpDir+"/templates")
	_ = os.Setenv("BASE_URL", "https://example.com")

	_ = os.Setenv("SERVER_READ_TIMEOUT", "30s")
	_ = os.Setenv("SERVER_WRITE_TIMEOUT", "30s")
	_ = os.Setenv("SERVER_IDLE_TIMEOUT", "120s")
	_ = os.Setenv("SHUTDOWN_TIMEOUT", "15s")

	_ = os.Setenv("CACHE_TTL", "2h")
	_ = os.Setenv("CACHE_MAX_SIZE", "2000")
	_ = os.Setenv("CACHE_CLEANUP_INTERVAL", "20m")

	_ = os.Setenv("EMAIL_HOST", "smtp.example.com")
	_ = os.Setenv("EMAIL_PORT", "465")
	_ = os.Setenv("EMAIL_USERNAME", "user@example.com")
	_ = os.Setenv("EMAIL_PASSWORD", "password")
	_ = os.Setenv("EMAIL_FROM", "noreply@example.com")
	_ = os.Setenv("EMAIL_TO", "admin@example.com")
	_ = os.Setenv("EMAIL_USE_SSL", "false")

	_ = os.Setenv("RATE_LIMIT_GENERAL_REQUESTS", "200")
	_ = os.Setenv("RATE_LIMIT_GENERAL_WINDOW", "30m")
	_ = os.Setenv("RATE_LIMIT_CONTACT_REQUESTS", "10")
	_ = os.Setenv("RATE_LIMIT_CONTACT_WINDOW", "2h")
	_ = os.Setenv("RATE_LIMIT_UPLOAD_REQUESTS", "30")
	_ = os.Setenv("RATE_LIMIT_UPLOAD_WINDOW", "10m")

	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com,https://api.example.com")
	_ = os.Setenv("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE")
	_ = os.Setenv("CORS_ALLOWED_HEADERS", "Content-Type,Authorization")

	_ = os.Setenv("ADMIN_USERNAME", "superuser")
	_ = os.Setenv("ADMIN_PASSWORD", "secretpassword")

	_ = os.Setenv("BLOG_TITLE", "My Custom Blog")
	_ = os.Setenv("BLOG_DESCRIPTION", "A custom blog description")
	_ = os.Setenv("BLOG_AUTHOR", "John Doe")
	_ = os.Setenv("BLOG_AUTHOR_EMAIL", "john@example.com")
	_ = os.Setenv("BLOG_LANGUAGE", "es")
	_ = os.Setenv("BLOG_THEME", "dark")
	_ = os.Setenv("BLOG_POSTS_PER_PAGE", "20")

	_ = os.Setenv("AMA_PAGE_HEADING", "Reach out")
	_ = os.Setenv("AMA_PAGE_INTRO", "Drop a note and I'll get back when I can.")
	_ = os.Setenv("AMA_FORM_PLACEHOLDER", "Type your question…")
	_ = os.Setenv("AMA_SUBMIT_LABEL", "Send")
	_ = os.Setenv("AMA_THANKYOU_COPY", "Got it. Thanks.")

	defer clearEnvVars()
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Test environment variable values
	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, tmpDir+"/articles", cfg.ArticlesPath)
	assert.Equal(t, tmpDir+"/static", cfg.StaticPath)
	assert.Equal(t, tmpDir+"/templates", cfg.TemplatesPath)
	assert.Equal(t, "https://example.com", cfg.BaseURL)

	// Test server config
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)
	assert.Equal(t, 120*time.Second, cfg.Server.IdleTimeout)
	assert.Equal(t, 15*time.Second, cfg.Server.ShutdownTimeout)

	// Test cache config
	assert.Equal(t, 2*time.Hour, cfg.Cache.TTL)
	assert.Equal(t, 2000, cfg.Cache.MaxSize)
	assert.Equal(t, 20*time.Minute, cfg.Cache.CleanupInterval)

	// Test email config
	assert.Equal(t, "smtp.example.com", cfg.Email.Host)
	assert.Equal(t, 465, cfg.Email.Port)
	assert.Equal(t, "user@example.com", cfg.Email.Username)
	assert.Equal(t, "password", cfg.Email.Password)
	assert.Equal(t, "noreply@example.com", cfg.Email.From)
	assert.Equal(t, "admin@example.com", cfg.Email.To)
	assert.False(t, cfg.Email.UseSSL)

	// Test rate limit config
	assert.Equal(t, 200, cfg.RateLimit.General.Requests)
	assert.Equal(t, 30*time.Minute, cfg.RateLimit.General.Window)
	assert.Equal(t, 10, cfg.RateLimit.Contact.Requests)
	assert.Equal(t, 2*time.Hour, cfg.RateLimit.Contact.Window)
	assert.Equal(t, 30, cfg.RateLimit.Upload.Requests)
	assert.Equal(t, 10*time.Minute, cfg.RateLimit.Upload.Window)

	// Test CORS config
	assert.Contains(t, cfg.CORS.AllowedOrigins, "https://example.com")
	assert.Contains(t, cfg.CORS.AllowedOrigins, "https://api.example.com")
	assert.Contains(t, cfg.CORS.AllowedMethods, "GET")
	assert.Contains(t, cfg.CORS.AllowedMethods, "PUT")
	assert.Contains(t, cfg.CORS.AllowedHeaders, "Content-Type")
	assert.Contains(t, cfg.CORS.AllowedHeaders, "Authorization")

	// Test admin config
	assert.Equal(t, "superuser", cfg.Admin.Username)
	assert.Equal(t, "secretpassword", cfg.Admin.Password)

	// Test blog config
	assert.Equal(t, "My Custom Blog", cfg.Blog.Title)
	assert.Equal(t, "A custom blog description", cfg.Blog.Description)
	assert.Equal(t, "John Doe", cfg.Blog.Author)
	assert.Equal(t, "john@example.com", cfg.Blog.AuthorEmail)
	assert.Equal(t, "es", cfg.Blog.Language)
	assert.Equal(t, "dark", cfg.Blog.Theme)
	assert.Equal(t, 20, cfg.Blog.PostsPerPage)

	// Test AMA config override
	assert.Equal(t, "Reach out", cfg.AMA.PageHeading)
	assert.Equal(t, "Drop a note and I'll get back when I can.", cfg.AMA.PageIntro)
	assert.Equal(t, "Type your question…", cfg.AMA.FormPlaceholder)
	assert.Equal(t, "Send", cfg.AMA.SubmitLabel)
	assert.Equal(t, "Got it. Thanks.", cfg.AMA.ThankyouCopy)
}

func TestEnvironmentVariableParsing(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		setValue string
		expected interface{}
		testFunc func(string, interface{}) interface{}
	}{
		// String parsing tests
		{"string with value", "TEST_STR", "test_value", "test_value", func(key string, def interface{}) interface{} { return getEnv(key, def.(string)) }},
		{"string with default", "NON_EXISTING_STR", "", "default", func(key string, def interface{}) interface{} { return getEnv(key, def.(string)) }},

		// Integer parsing tests
		{"int with valid value", "TEST_INT", "42", 42, func(key string, def interface{}) interface{} { return getEnvInt(key, def.(int)) }},
		{"int with invalid value", "TEST_INVALID_INT", "not_a_number", 10, func(key string, def interface{}) interface{} { return getEnvInt(key, def.(int)) }},
		{"int with default", "NON_EXISTING_INT", "", 10, func(key string, def interface{}) interface{} { return getEnvInt(key, def.(int)) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setValue != "" {
				_ = os.Setenv(tt.envVar, tt.setValue)
				defer func() { _ = os.Unsetenv(tt.envVar) }()
			}

			result := tt.testFunc(tt.envVar, tt.expected)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	testCases := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"FALSE", false},
		{"t", true},
		{"f", false},
		{"T", true},
		{"F", false},
	}

	for _, tc := range testCases {
		t.Run(tc.value, func(t *testing.T) {
			_ = os.Setenv("TEST_BOOL", tc.value)
			defer func() { _ = os.Unsetenv("TEST_BOOL") }()

			result := getEnvBool("TEST_BOOL", false)
			assert.Equal(t, tc.expected, result)
		})
	}

	// Test with invalid boolean
	_ = os.Setenv("TEST_INVALID_BOOL", "not_a_bool")
	defer func() { _ = os.Unsetenv("TEST_INVALID_BOOL") }()

	result := getEnvBool("TEST_INVALID_BOOL", true)
	assert.Equal(t, true, result)

	// Test with non-existing environment variable
	result = getEnvBool("NON_EXISTING_BOOL", true)
	assert.Equal(t, true, result)
}

func TestStringSliceParsing(t *testing.T) {
	tests := []struct {
		name      string
		envVar    string
		setValue  string
		expected  []string
		delimiter string
	}{
		{"valid comma-separated values", "TEST_SLICE", "value1,value2,value3", []string{"value1", "value2", "value3"}, ","},
		{"values with spaces", "TEST_SLICE_SPACES", "value1, value2 , value3", []string{"value1", "value2", "value3"}, ","},
		{"empty values filtered", "TEST_SLICE_EMPTY", "value1,,value3", []string{"value1", "value3"}, ","},
		{"non-existing returns default", "NON_EXISTING_SLICE", "", []string{"default"}, ","},
		{"empty string returns default", "TEST_EMPTY_SLICE", "", []string{"default"}, ","},
		{"semicolon delimiter", "TEST_SEMICOLON", "a;b;c", []string{"a", "b", "c"}, ";"},
		{"single value", "TEST_SINGLE", "single", []string{"single"}, ","},
		{"whitespace handling", "TEST_WHITESPACE", "   a   ,   b   ,   c   ", []string{"a", "b", "c"}, ","},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setValue != "" {
				_ = os.Setenv(tt.envVar, tt.setValue)
				defer func() { _ = os.Unsetenv(tt.envVar) }()
			}

			if tt.name == "non-existing returns default" || tt.name == "empty string returns default" {
				result := getEnvSlice(tt.envVar, tt.expected)
				assert.Equal(t, tt.expected, result)
			} else {
				result := splitString(tt.setValue, tt.delimiter)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// Helper function to clear environment variables
func clearEnvVars() {
	vars := []string{
		"ENVIRONMENT", "PORT", "ARTICLES_PATH", "STATIC_PATH", "TEMPLATES_PATH", "BASE_URL",
		"SERVER_READ_TIMEOUT", "SERVER_WRITE_TIMEOUT", "SERVER_IDLE_TIMEOUT", "SHUTDOWN_TIMEOUT",
		"AMA_PAGE_HEADING", "AMA_PAGE_INTRO", "AMA_FORM_PLACEHOLDER", "AMA_SUBMIT_LABEL", "AMA_THANKYOU_COPY",
		"CACHE_TTL", "CACHE_MAX_SIZE", "CACHE_CLEANUP_INTERVAL",
		"EMAIL_HOST", "EMAIL_PORT", "EMAIL_USERNAME", "EMAIL_PASSWORD", "EMAIL_FROM", "EMAIL_TO", "EMAIL_USE_SSL",
		"RATE_LIMIT_GENERAL_REQUESTS", "RATE_LIMIT_GENERAL_WINDOW", "RATE_LIMIT_CONTACT_REQUESTS", "RATE_LIMIT_CONTACT_WINDOW", "RATE_LIMIT_UPLOAD_REQUESTS", "RATE_LIMIT_UPLOAD_WINDOW",
		"CORS_ALLOWED_ORIGINS", "CORS_ALLOWED_METHODS", "CORS_ALLOWED_HEADERS",
		"ADMIN_USERNAME", "ADMIN_PASSWORD",
		"BLOG_TITLE", "BLOG_DESCRIPTION", "BLOG_AUTHOR", "BLOG_AUTHOR_EMAIL", "BLOG_LANGUAGE", "BLOG_THEME", "BLOG_POSTS_PER_PAGE",
		"UPLOAD_PATH", "UPLOAD_MAX_SIZE",
	}

	for _, env := range vars {
		_ = os.Unsetenv(env)
	}
}

// TestEnvironmentAwareRateLimiting tests that different environments get different rate limiting defaults
func TestEnvironmentAwareRateLimiting(t *testing.T) {
	tests := []struct {
		name                    string
		environment             string
		expectedGeneralRequests int
		expectedContactRequests int
		expectedUploadRequests  int
	}{
		{
			name:                    "production environment",
			environment:             "production",
			expectedGeneralRequests: 100,
			expectedContactRequests: 5,
			expectedUploadRequests:  20,
		},
		{
			name:                    "development environment",
			environment:             "development",
			expectedGeneralRequests: 3000,
			expectedContactRequests: 20,
			expectedUploadRequests:  100,
		},
		{
			name:                    "test environment",
			environment:             "test",
			expectedGeneralRequests: 5000,
			expectedContactRequests: 50,
			expectedUploadRequests:  500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment
			_ = os.Setenv("ENVIRONMENT", tt.environment)
			defer func() { _ = os.Unsetenv("ENVIRONMENT") }()

			// Load config
			cfg, err := Load()
			assert.NoError(t, err)
			assert.NotNil(t, cfg)

			// Check environment-specific rate limits
			assert.Equal(t, tt.expectedGeneralRequests, cfg.RateLimit.General.Requests)
			assert.Equal(t, tt.expectedContactRequests, cfg.RateLimit.Contact.Requests)
			assert.Equal(t, tt.expectedUploadRequests, cfg.RateLimit.Upload.Requests)
			assert.Equal(t, 15*time.Minute, cfg.RateLimit.General.Window)
			assert.Equal(t, 1*time.Hour, cfg.RateLimit.Contact.Window)
			assert.Equal(t, 5*time.Minute, cfg.RateLimit.Upload.Window)
		})
	}
}

func TestGetEnvInt64(t *testing.T) {
	t.Run("valid value", func(t *testing.T) {
		_ = os.Setenv("TEST_INT64", "10485760")
		defer func() { _ = os.Unsetenv("TEST_INT64") }()
		assert.Equal(t, int64(10485760), getEnvInt64("TEST_INT64", 0))
	})

	t.Run("invalid value returns default", func(t *testing.T) {
		_ = os.Setenv("TEST_INT64_BAD", "not-a-number")
		defer func() { _ = os.Unsetenv("TEST_INT64_BAD") }()
		assert.Equal(t, int64(42), getEnvInt64("TEST_INT64_BAD", 42))
	})

	t.Run("missing value returns default", func(t *testing.T) {
		assert.Equal(t, int64(10<<20), getEnvInt64("NONEXISTENT_INT64", 10<<20))
	})
}

func TestUploadConfigValidation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &UploadConfig{Path: "./uploads", MaxSize: 10 << 20}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("zero max size rejected", func(t *testing.T) {
		cfg := &UploadConfig{Path: "./uploads", MaxSize: 0}
		assert.Error(t, cfg.Validate())
	})

	t.Run("negative max size rejected", func(t *testing.T) {
		cfg := &UploadConfig{Path: "./uploads", MaxSize: -1}
		assert.Error(t, cfg.Validate())
	})

	t.Run("exceeds 100MB cap rejected", func(t *testing.T) {
		cfg := &UploadConfig{Path: "./uploads", MaxSize: 200 << 20}
		assert.Error(t, cfg.Validate())
	})
}

func TestUploadConfigBoundary(t *testing.T) {
	t.Run("exactly 100MB passes validation", func(t *testing.T) {
		cfg := &UploadConfig{Path: "./uploads", MaxSize: 100 << 20}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("100MB plus one byte fails validation", func(t *testing.T) {
		cfg := &UploadConfig{Path: "./uploads", MaxSize: 100<<20 + 1}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "100MB")
	})

	t.Run("empty path fails validation", func(t *testing.T) {
		cfg := &UploadConfig{Path: "", MaxSize: 10 << 20}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("non-empty path passes validation", func(t *testing.T) {
		cfg := &UploadConfig{Path: "/tmp/uploads", MaxSize: 10 << 20}
		assert.NoError(t, cfg.Validate())
	})
}

func TestUploadConfigFromEnv(t *testing.T) {
	clearEnvVars()

	_ = os.Setenv("UPLOAD_PATH", "/tmp/custom-uploads")
	_ = os.Setenv("UPLOAD_MAX_SIZE", "20971520")
	defer func() {
		_ = os.Unsetenv("UPLOAD_PATH")
		_ = os.Unsetenv("UPLOAD_MAX_SIZE")
	}()

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/custom-uploads", cfg.Upload.Path)
	assert.Equal(t, int64(20971520), cfg.Upload.MaxSize)
}
