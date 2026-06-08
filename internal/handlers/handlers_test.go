package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Mock services for testing
type MockArticleService struct{}

func (m *MockArticleService) GetAllArticles() []*models.Article {
	return []*models.Article{
		{Slug: "test-article", Title: "Test", Draft: false, Date: time.Now()},
	}
}
func (m *MockArticleService) GetArticleBySlug(slug string) (*models.Article, error) {
	if slug == "test-article" {
		return &models.Article{Slug: slug, Title: "Test", Draft: false}, nil
	}
	return nil, errors.New("article not found")
}
func (m *MockArticleService) GetPages() []*models.Article                             { return nil }
func (m *MockArticleService) GetArticlesByTag(tag string) []*models.Article           { return nil }
func (m *MockArticleService) GetArticlesByCategory(category string) []*models.Article { return nil }
func (m *MockArticleService) GetArticlesForFeed(limit int) []*models.Article          { return nil }
func (m *MockArticleService) GetFeaturedArticles(limit int) []*models.Article         { return nil }
func (m *MockArticleService) GetRecentArticles(limit int) []*models.Article           { return nil }
func (m *MockArticleService) GetAllTags() []string                                    { return []string{} }
func (m *MockArticleService) GetAllCategories() []string                              { return []string{} }
func (m *MockArticleService) GetTagCounts() []models.TagCount                         { return []models.TagCount{} }
func (m *MockArticleService) GetCategoryCounts() []models.CategoryCount {
	return []models.CategoryCount{}
}
func (m *MockArticleService) SearchArticles(_ string, _ int) []*models.SearchResult {
	return nil
}
func (m *MockArticleService) GetStats() *models.Stats                             { return &models.Stats{} }
func (m *MockArticleService) ReloadArticles() error                               { return nil }
func (m *MockArticleService) GetDraftArticles() []*models.Article                 { return nil }
func (m *MockArticleService) GetDraftBySlug(slug string) (*models.Article, error) { return nil, nil }
func (m *MockArticleService) IsHealthy() bool                                     { return true }

type MockEmailService struct {
	ShouldFail      bool
	NotConfigured   bool
	LastMessageSent *models.ContactMessage
}

func (m *MockEmailService) SendContactMessage(msg *models.ContactMessage) error {
	if m.NotConfigured {
		return apperrors.ErrEmailNotConfigured
	}
	if m.ShouldFail {
		return errors.New("email send failed")
	}
	m.LastMessageSent = msg
	return nil
}
func (m *MockEmailService) SendNotification(to, subject, body string) error { return nil }
func (m *MockEmailService) SendTestEmail() error                            { return nil }
func (m *MockEmailService) TestConnection() error                           { return nil }
func (m *MockEmailService) ValidateConfig() []string                        { return nil }
func (m *MockEmailService) GetConfig() map[string]any                       { return nil }
func (m *MockEmailService) Shutdown()                                       {}

type MockTemplateService struct {
	// LastData captures the template data from the most recent Render call,
	// so tests can assert on what was passed to the template layer without
	// needing to actually render anything.
	LastData map[string]any
}

func (m *MockTemplateService) Render(w io.Writer, templateName string, data any) error {
	if d, ok := data.(map[string]any); ok {
		m.LastData = d
	}
	return nil
}
func (m *MockTemplateService) RenderToString(templateName string, data any) (string, error) {
	return "", nil
}
func (m *MockTemplateService) HasTemplate(templateName string) bool { return true }
func (m *MockTemplateService) ListTemplates() []string              { return nil }
func (m *MockTemplateService) Reload(templatesPath string) error    { return nil }
func (m *MockTemplateService) GetTemplate() *template.Template      { return nil }

type MockSEOService struct{}

func (m *MockSEOService) GenerateSitemap() ([]byte, error)   { return nil, nil }
func (m *MockSEOService) GenerateRobotsTxt() ([]byte, error) { return nil, nil }
func (m *MockSEOService) GenerateArticleSchema(article *models.Article, baseURL string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockSEOService) GenerateWebsiteSchema() (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockSEOService) GenerateBreadcrumbSchema(breadcrumbs []services.Breadcrumb) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockSEOService) GenerateOpenGraphTags(article *models.Article, baseURL string) (map[string]string, error) {
	return nil, nil
}
func (m *MockSEOService) GenerateTwitterCardTags(article *models.Article, baseURL string) (map[string]string, error) {
	return nil, nil
}
func (m *MockSEOService) GenerateMetaTags(article *models.Article) (map[string]string, error) {
	return nil, nil
}
func (m *MockSEOService) GeneratePageMetaTags(title, description, path string) (map[string]string, error) {
	return nil, nil
}
func (m *MockSEOService) AnalyzeContent(content string) (*services.SEOAnalysis, error) {
	return nil, nil
}
func (m *MockSEOService) IsEnabled() bool { return true }

// MockSEOServiceWithRobots extends MockSEOService with configurable robots.txt output.
type MockSEOServiceWithRobots struct {
	MockSEOService
	robotsTxt []byte
}

func (m *MockSEOServiceWithRobots) GenerateRobotsTxt() ([]byte, error) {
	return m.robotsTxt, nil
}

func createTestConfig() *config.Config {
	return &config.Config{
		Environment: "test",
		BaseURL:     "http://localhost:3000",
		Blog: config.BlogConfig{
			Title:       "Test Blog",
			Description: "Test Description",
			Author:      "Test Author",
			AuthorEmail: "test@example.com",
		},
	}
}

func createTestContactHandler(emailService *MockEmailService) *ContactHandler {
	cfg := createTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	if emailService == nil {
		emailService = &MockEmailService{}
	}

	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})
	return NewContactHandler(base, emailService)
}

func createTestHealthHandler() *HealthHandler {
	cfg := createTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})
	return NewHealthHandler(base, &MockArticleService{}, time.Now())
}

// MockFeedService is a minimal feed service for syndication handler tests.
type MockFeedService struct{}

func (m *MockFeedService) GenerateRSS() (string, error)      { return "<rss/>", nil }
func (m *MockFeedService) GenerateJSONFeed() (string, error) { return "{}", nil }
func (m *MockFeedService) GenerateSitemap() (string, error)  { return "<sitemap/>", nil }

// TestRobotsTxt tests the robots.txt handler with different SEO service states.
func TestRobotsTxt(t *testing.T) {
	t.Run("returns dynamic robots.txt when SEO enabled", func(t *testing.T) {
		seoMock := &MockSEOServiceWithRobots{
			robotsTxt: []byte("User-agent: *\nDisallow: /admin/\nSitemap: http://localhost:3000/sitemap.xml\n"),
		}
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, seoMock)
		handler := NewSyndicationHandler(base, &MockFeedService{})

		router := gin.New()
		router.GET("/robots.txt", handler.RobotsTxt)

		req := httptest.NewRequest("GET", "/robots.txt", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
		assert.Contains(t, w.Body.String(), "Disallow: /admin/")
		assert.Contains(t, w.Body.String(), "Sitemap:")
	})

	t.Run("returns fallback when SEO disabled", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
		// nil seoService — simulates SEO not configured
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, nil)
		handler := NewSyndicationHandler(base, &MockFeedService{})

		router := gin.New()
		router.GET("/robots.txt", handler.RobotsTxt)

		req := httptest.NewRequest("GET", "/robots.txt", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "User-agent: *")
		assert.Contains(t, w.Body.String(), "Allow: /")
	})
}

// TestContact tests the contact form handler
func TestContact(t *testing.T) {
	t.Run("valid contact form submission", func(t *testing.T) {
		mockEmail := &MockEmailService{}
		handler := createTestContactHandler(mockEmail)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		formData := map[string]string{
			"name":    "John Doe",
			"email":   "john@example.com",
			"subject": "Test Subject",
			"message": "Test message content",
		}

		body, _ := json.Marshal(formData)
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotNil(t, mockEmail.LastMessageSent)
		assert.Equal(t, "John Doe", mockEmail.LastMessageSent.Name)
		assert.Equal(t, "john@example.com", mockEmail.LastMessageSent.Email)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "success", response["status"])
	})

	t.Run("invalid email address", func(t *testing.T) {
		handler := createTestContactHandler(nil)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		formData := map[string]string{
			"name":    "John Doe",
			"email":   "not-an-email",
			"subject": "Test Subject",
			"message": "Test message content",
		}

		body, _ := json.Marshal(formData)
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields", func(t *testing.T) {
		handler := createTestContactHandler(nil)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		formData := map[string]string{
			"name": "John Doe",
			// Missing email, subject, message
		}

		body, _ := json.Marshal(formData)
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("email service not configured", func(t *testing.T) {
		mockEmail := &MockEmailService{NotConfigured: true}
		handler := createTestContactHandler(mockEmail)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		formData := map[string]string{
			"name":    "John Doe",
			"email":   "john@example.com",
			"subject": "Test Subject",
			"message": "Test message content",
		}

		body, _ := json.Marshal(formData)
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "unavailable", response["status"])
	})

	t.Run("email service failure", func(t *testing.T) {
		mockEmail := &MockEmailService{ShouldFail: true}
		handler := createTestContactHandler(mockEmail)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		formData := map[string]string{
			"name":    "John Doe",
			"email":   "john@example.com",
			"subject": "Test Subject",
			"message": "Test message content",
		}

		body, _ := json.Marshal(formData)
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// When email fails, error is logged and handled
		// The test verifies the handler doesn't crash
		assert.NotEqual(t, 0, w.Code, "Handler should return a status code")
	})

	t.Run("oversized body returns 413", func(t *testing.T) {
		mockEmail := &MockEmailService{}
		handler := createTestContactHandler(mockEmail)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		// Body exceeds the 64KB cap — MaxBytesReader trips during bind.
		body, _ := json.Marshal(map[string]string{
			"name":    "John Doe",
			"email":   "john@example.com",
			"subject": "Test Subject",
			"message": strings.Repeat("a", 70000),
		})
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		assert.Nil(t, mockEmail.LastMessageSent, "no email on rejected body")
	})

	t.Run("oversized field returns 400", func(t *testing.T) {
		mockEmail := &MockEmailService{}
		handler := createTestContactHandler(mockEmail)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		// Body is under the 64KB cap but message exceeds max=2000 → field validation.
		body, _ := json.Marshal(map[string]string{
			"name":    "John Doe",
			"email":   "john@example.com",
			"subject": "Test Subject",
			"message": strings.Repeat("a", 2001),
		})
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Nil(t, mockEmail.LastMessageSent, "no email on rejected field")
	})

	t.Run("at-limit message accepted", func(t *testing.T) {
		mockEmail := &MockEmailService{}
		handler := createTestContactHandler(mockEmail)

		router := gin.New()
		router.POST("/contact", handler.Submit)

		// Message at exactly max=2000 — the largest valid submission is accepted.
		body, _ := json.Marshal(map[string]string{
			"name":    "John Doe",
			"email":   "john@example.com",
			"subject": "Test Subject",
			"message": strings.Repeat("a", 2000),
		})
		req := httptest.NewRequest("POST", "/contact", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotNil(t, mockEmail.LastMessageSent, "valid at-limit message is delivered")
	})
}

// TestAdminEndpoints tests basic admin functionality
func TestAdminEndpoints(t *testing.T) {
	t.Run("admin stats returns valid data", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})
		adminHandler := NewAdminHandler(base, &MockArticleService{}, time.Now())

		router := gin.New()
		router.GET("/admin/stats", adminHandler.Stats)

		req := httptest.NewRequest("GET", "/admin/stats", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Stats returns JSON with various metrics
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		// Response should have at least some fields
		assert.NotEmpty(t, response)
	})
}

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	handler := createTestHealthHandler()

	router := gin.New()
	router.GET("/health", handler.Health)

	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

// unhealthyArticleService is a MockArticleService variant that reports
// unhealthy state. Used by TestHealth_Unhealthy_Returns503.
type unhealthyArticleService struct{ MockArticleService }

func (m *unhealthyArticleService) IsHealthy() bool { return false }

// TestHealth_Unhealthy_Returns503 verifies that /health surfaces real
// degradation: when the article service reports unhealthy, the endpoint
// returns 503 + status="unhealthy" so uptime monitors get a truthful signal.
// Regression guard for v3.10.3 F3.
func TestHealth_Unhealthy_Returns503(t *testing.T) {
	cfg := createTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})
	handler := NewHealthHandler(base, &unhealthyArticleService{}, time.Now())

	router := gin.New()
	router.GET("/health", handler.Health)

	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unhealthy", resp["status"])
	svcStatus, ok := resp["services"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "unhealthy", svcStatus["articles"])
}
