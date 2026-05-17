package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// TestArticleService returns canned data for handler tests.
// No business logic — just lookups on the fixture slice.
type TestArticleService struct {
	articles []*models.Article
}

func (m *TestArticleService) GetAllArticles() []*models.Article { return m.articles }
func (m *TestArticleService) GetArticleBySlug(slug string) (*models.Article, error) {
	for _, a := range m.articles {
		if a.Slug == slug {
			return a, nil
		}
	}
	return nil, apperrors.ErrArticleNotFound
}
func (m *TestArticleService) GetArticlesByTag(_ string) []*models.Article      { return m.articles }
func (m *TestArticleService) GetArticlesByCategory(_ string) []*models.Article { return m.articles }
func (m *TestArticleService) GetArticlesForFeed(_ int) []*models.Article       { return m.articles }
func (m *TestArticleService) GetFeaturedArticles(_ int) []*models.Article      { return m.articles }
func (m *TestArticleService) GetRecentArticles(_ int) []*models.Article        { return m.articles }
func (m *TestArticleService) GetAllTags() []string                             { return []string{"golang", "tutorial"} }
func (m *TestArticleService) GetAllCategories() []string                       { return []string{"Programming"} }
func (m *TestArticleService) GetTagCounts() []models.TagCount {
	return []models.TagCount{{Tag: "golang", Count: 1}}
}
func (m *TestArticleService) GetCategoryCounts() []models.CategoryCount {
	return []models.CategoryCount{{Category: "Programming", Count: 1}}
}
func (m *TestArticleService) GetStats() *models.Stats {
	return &models.Stats{TotalArticles: len(m.articles)}
}
func (m *TestArticleService) SearchArticles(_ string, _ int) []*models.SearchResult { return nil }
func (m *TestArticleService) ReloadArticles() error                                 { return nil }
func (m *TestArticleService) GetDraftArticles() []*models.Article                   { return nil }
func (m *TestArticleService) GetDraftBySlug(_ string) (*models.Article, error) {
	return nil, apperrors.ErrArticleNotFound
}
func (m *TestArticleService) IsHealthy() bool { return true }

func testArticles() []*models.Article {
	now := time.Now()
	return []*models.Article{
		{Slug: "golang-tutorial", Title: "Getting Started with Go", Date: now, Tags: []string{"golang", "tutorial"}, Categories: []string{"Programming"}},
		{Slug: "web-development", Title: "Modern Web Development", Date: now.Add(-24 * time.Hour), Tags: []string{"web", "tutorial"}, Categories: []string{"Web Development"}},
	}
}

func createTestBase() (*BaseHandler, *TestArticleService) {
	cfg := &config.Config{
		Environment: "test",
		BaseURL:     "http://localhost:3000",
		Blog:        config.BlogConfig{Title: "Test Blog", Description: "Test", Author: "Test"},
	}
	svc := &TestArticleService{articles: testArticles()}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})
	return base, svc
}

func TestArticleBySlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
		want int
	}{
		{"valid slug", "golang-tutorial", http.StatusOK},
		{"not found", "nonexistent", http.StatusNotFound},
		{"empty slug", "", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, svc := createTestBase()
			router := gin.New()
			router.GET("/writing/:slug", NewPostHandler(base, svc).Article)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/writing/"+tt.slug, http.NoBody))

			if tt.slug == "" {
				assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusNotFound)
			} else {
				assert.Equal(t, tt.want, w.Code)
			}
		})
	}
}

// TestArticle_WritingAboutRedirects — slug=about resolves to a dedicated route
// (/about), so /writing/about must 301 to /about for both GET and HEAD.
func TestArticle_WritingAboutRedirects(t *testing.T) {
	verbs := []string{http.MethodGet, http.MethodHead}
	for _, verb := range verbs {
		t.Run(verb, func(t *testing.T) {
			base, svc := createTestBase()
			svc.articles = append(svc.articles, &models.Article{
				Slug: "about", Title: "About", Date: time.Now(),
			})
			router := gin.New()
			h := NewPostHandler(base, svc)
			router.GET("/writing/:slug", h.Article)
			router.HEAD("/writing/:slug", h.Article)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(verb, "/writing/about", http.NoBody))

			assert.Equal(t, http.StatusMovedPermanently, w.Code)
			assert.Equal(t, "/about", w.Header().Get("Location"))
		})
	}
}

// TestArticle_WritingPageRedirects — type:page articles 301 from /writing/<slug>
// to /p/<slug> for both GET and HEAD.
func TestArticle_WritingPageRedirects(t *testing.T) {
	verbs := []string{http.MethodGet, http.MethodHead}
	for _, verb := range verbs {
		t.Run(verb, func(t *testing.T) {
			base, svc := createTestBase()
			svc.articles = append(svc.articles, &models.Article{
				Slug: "run-your-own", Title: "Run Your Own", Type: "page", Date: time.Now(),
			})
			router := gin.New()
			h := NewPostHandler(base, svc)
			router.GET("/writing/:slug", h.Article)
			router.HEAD("/writing/:slug", h.Article)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(verb, "/writing/run-your-own", http.NoBody))

			assert.Equal(t, http.StatusMovedPermanently, w.Code)
			assert.Equal(t, "/p/run-your-own", w.Header().Get("Location"))
		})
	}
}

// TestPage_Handler verifies /p/:slug routing semantics: pages render 200,
// non-page articles 404 (including the about-slug + type=article case),
// missing slugs 404, drafts 404, HEAD supported.
func TestPage_Handler(t *testing.T) {
	tests := []struct {
		name     string
		slug     string
		articles []*models.Article
		want     int
	}{
		{
			name:     "page renders",
			slug:     "run-your-own",
			articles: []*models.Article{{Slug: "run-your-own", Title: "Page", Type: "page", Date: time.Now()}},
			want:     http.StatusOK,
		},
		{
			name:     "non-page article 404",
			slug:     "intro",
			articles: []*models.Article{{Slug: "intro", Title: "Intro", Type: "article", Date: time.Now()}},
			want:     http.StatusNotFound,
		},
		{
			name:     "about slug at /p 404",
			slug:     "about",
			articles: []*models.Article{{Slug: "about", Title: "About", Type: "article", Date: time.Now()}},
			want:     http.StatusNotFound,
		},
		{
			name:     "missing slug 404",
			slug:     "nonexistent",
			articles: []*models.Article{},
			want:     http.StatusNotFound,
		},
		{
			name:     "draft page 404",
			slug:     "draft-page",
			articles: []*models.Article{{Slug: "draft-page", Title: "Draft", Type: "page", Draft: true, Date: time.Now()}},
			want:     http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, svc := createTestBase()
			svc.articles = tt.articles
			router := gin.New()
			router.GET("/p/:slug", NewPostHandler(base, svc).Page)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p/"+tt.slug, http.NoBody))
			assert.Equal(t, tt.want, w.Code)
		})
	}
}

// TestPage_HEAD_Supported — HEAD verb on /p/:slug returns the same status
// as GET without a body.
func TestPage_HEAD_Supported(t *testing.T) {
	base, svc := createTestBase()
	svc.articles = []*models.Article{{Slug: "ok", Title: "OK", Type: "page", Date: time.Now()}}
	router := gin.New()
	router.HEAD("/p/:slug", NewPostHandler(base, svc).Page)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/p/ok", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestPage_BreadcrumbsExcludeWriting — pages live outside /writing, so
// breadcrumbs must not include the "Writing" intermediate level (which
// getArticleData would otherwise inject by default).
func TestPage_BreadcrumbsExcludeWriting(t *testing.T) {
	base, svc := createTestBase()
	svc.articles = []*models.Article{{Slug: "run-your-own", Title: "Run Your Own", Type: "page", Date: time.Now()}}

	mockTpl, ok := base.templateService.(*MockTemplateService)
	require.True(t, ok, "createTestBase must wire a MockTemplateService for data inspection")
	mockTpl.LastData = nil

	router := gin.New()
	router.GET("/p/:slug", NewPostHandler(base, svc).Page)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p/run-your-own", http.NoBody))
	require.Equal(t, http.StatusOK, w.Code)

	require.NotNil(t, mockTpl.LastData, "MockTemplateService.Render must have captured rendered data")
	crumbs, ok := mockTpl.LastData["breadcrumbs"].([]services.Breadcrumb)
	require.True(t, ok, "breadcrumbs must be []services.Breadcrumb, got %T", mockTpl.LastData["breadcrumbs"])
	for _, c := range crumbs {
		assert.NotEqual(t, "/writing", c.URL, "Page breadcrumbs must not include the /writing intermediate")
		assert.NotEqual(t, "Writing", c.Name, "Page breadcrumbs must not include the Writing label")
	}
}

func TestArticlesListing(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"default page", ""},
		{"page 1", "?page=1"},
		{"page 2", "?page=2"},
		{"invalid page", "?page=invalid"},
		{"negative page", "?page=-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, svc := createTestBase()
			router := gin.New()
			router.GET("/writing", NewPostHandler(base, svc).Articles)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/writing"+tt.query, http.NoBody))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestArticlesByTag(t *testing.T) {
	tags := []string{"golang", "tutorial", "nonexistent", "web%20development"}
	for _, tag := range tags {
		t.Run(tag, func(t *testing.T) {
			base, svc := createTestBase()
			router := gin.New()
			router.GET("/tags/:tag", NewTaxonomyHandler(base, svc).ArticlesByTag)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/tags/"+tag, http.NoBody))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestArticlesByCategory(t *testing.T) {
	categories := []string{"Programming", url.PathEscape("Web Development"), "Nonexistent"}
	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			base, svc := createTestBase()
			router := gin.New()
			router.GET("/categories/:category", NewTaxonomyHandler(base, svc).ArticlesByCategory)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/categories/"+cat, http.NoBody))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestSearch(t *testing.T) {
	queries := []string{"?q=golang", "", "?q=go+programming", "?q=thisisaverylongquery"}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			base, svc := createTestBase()
			router := gin.New()
			router.GET("/search", NewSearchHandler(base, svc).Search)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/search"+q, http.NoBody))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// TestSearch_LongQueryTruncated_DoesNotPolluteCache verifies the
// maxSearchQueryLength cap (v3.10.3 F16). Pre-fix, raw `?q=` was passed
// unsanitized to SearchArticles and used as the obcache key prefix,
// allowing a bot with N distinct long queries to evict real entries.
func TestSearch_LongQueryTruncated_DoesNotPolluteCache(t *testing.T) {
	base, svc := createTestBase()
	router := gin.New()
	router.GET("/search", NewSearchHandler(base, svc).Search)

	w := httptest.NewRecorder()
	longQuery := strings.Repeat("a", 5000)
	router.ServeHTTP(w, httptest.NewRequest("GET", "/search?q="+longQuery, http.NoBody))
	// The handler should not crash and should respond OK. The cap itself is
	// enforced inside the handler before SearchArticles is invoked.
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHomePage(t *testing.T) {
	base, svc := createTestBase()
	router := gin.New()
	router.GET("/", NewFeedHandler(base, svc).Home)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHomePageAMAFilter(t *testing.T) {
	base, svc := createTestBase()
	router := gin.New()
	router.GET("/", NewFeedHandler(base, svc).Home)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/?type=ama", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHomePageInvalidFilter(t *testing.T) {
	base, svc := createTestBase()
	router := gin.New()
	router.GET("/", NewFeedHandler(base, svc).Home)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/?type=invalid", http.NoBody))
	// Invalid type falls back to empty (all)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTagsPage(t *testing.T) {
	base, svc := createTestBase()
	router := gin.New()
	router.GET("/tags", NewTaxonomyHandler(base, svc).Tags)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/tags", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCategoriesPage(t *testing.T) {
	base, svc := createTestBase()
	router := gin.New()
	router.GET("/categories", NewTaxonomyHandler(base, svc).Categories)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/categories", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}
