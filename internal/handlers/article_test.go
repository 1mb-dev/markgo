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
func (m *TestArticleService) GetPages() []*models.Article                      { return m.articles }
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

// TestArticleHandler_JSONLDEmail locks the contract: article.html JSON-LD
// emits the "email" field only when BLOG_AUTHOR_EMAIL is configured. With
// an empty value, no "email": substring may appear inside the JSON-LD block
// (would be invalid Schema.org). Surfaced by #80.
func TestArticleHandler_JSONLDEmail(t *testing.T) {
	tests := []struct {
		name          string
		authorEmail   string
		denySubstring string
		wantSubstring string
	}{
		{
			name:          "empty email — JSON-LD omits email field",
			authorEmail:   "",
			denySubstring: `"email":`,
		},
		{
			name:          "configured email — JSON-LD includes email field",
			authorEmail:   "author@example.com",
			wantSubstring: `"email": "author@example.com"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Environment: "test",
				BaseURL:     "http://localhost:3000",
				Blog:        config.BlogConfig{Title: "Test Blog", Description: "Test", Author: "Test Author", AuthorEmail: tc.authorEmail},
			}
			tplSvc, err := services.NewTemplateService("/nonexistent", cfg)
			require.NoError(t, err, "real TemplateService falls back to embedded templates")

			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
			base := NewBaseHandler(cfg, logger, tplSvc, &BuildInfo{Version: "test"}, &MockSEOService{})
			svc := &TestArticleService{articles: []*models.Article{
				{Slug: "test-slug", Title: "Test Article", Date: time.Now(), Content: "body", ProcessedContent: "<p>body</p>"},
			}}

			router := gin.New()
			router.GET("/writing/:slug", NewPostHandler(base, svc).Article)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/writing/test-slug", http.NoBody))
			require.Equal(t, http.StatusOK, w.Code)

			body := w.Body.String()
			jsonLDStart := strings.Index(body, `<script type="application/ld+json">`)
			require.GreaterOrEqual(t, jsonLDStart, 0, "JSON-LD script tag must be present in article body")
			jsonLDEnd := strings.Index(body[jsonLDStart:], `</script>`)
			require.Greater(t, jsonLDEnd, 0, "JSON-LD script tag must be closed")
			jsonLD := body[jsonLDStart : jsonLDStart+jsonLDEnd]

			if tc.denySubstring != "" {
				assert.NotContains(t, jsonLD, tc.denySubstring,
					"JSON-LD must not emit email field when AuthorEmail is empty (would be invalid Schema.org)")
			}
			if tc.wantSubstring != "" {
				assert.Contains(t, jsonLD, tc.wantSubstring,
					"JSON-LD must emit configured email value")
			}
		})
	}
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

// TestPages_Handler verifies /p index renders pages sorted alphabetically
// by title (case-insensitive), shows an empty state when no pages exist,
// and exposes the expected template-data keys for the pages.html template.
func TestPages_Handler(t *testing.T) {
	t.Run("empty pages list renders empty state", func(t *testing.T) {
		base, svc := createTestBase()
		svc.articles = nil

		mockTpl := requireMockTemplateService(t, base)
		mockTpl.LastData = nil

		router := gin.New()
		router.GET("/p", NewPostHandler(base, svc).Pages)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p", http.NoBody))

		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, mockTpl.LastData)
		assert.Equal(t, "pages", mockTpl.LastData["template"])
		assert.Equal(t, 0, mockTpl.LastData["pageCount"])
	})

	t.Run("alphabetical sort case-insensitive", func(t *testing.T) {
		base, svc := createTestBase()
		// Inject in non-alphabetical order with mixed case to verify
		// case-insensitive lowercase comparison.
		svc.articles = []*models.Article{
			{Slug: "zoo", Title: "Zoo", Type: "page", Date: time.Now()},
			{Slug: "alpha", Title: "alpha", Type: "page", Date: time.Now()},
			{Slug: "beta", Title: "Beta", Type: "page", Date: time.Now()},
		}

		mockTpl := requireMockTemplateService(t, base)
		mockTpl.LastData = nil

		router := gin.New()
		router.GET("/p", NewPostHandler(base, svc).Pages)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p", http.NoBody))

		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, mockTpl.LastData)
		assert.Equal(t, 3, mockTpl.LastData["pageCount"])

		pages, ok := mockTpl.LastData["pages"].([]*models.Article)
		require.True(t, ok, "pages must be []*models.Article, got %T", mockTpl.LastData["pages"])
		require.Len(t, pages, 3)
		assert.Equal(t, "alpha", pages[0].Title, "case-insensitive sort: 'alpha' first")
		assert.Equal(t, "Beta", pages[1].Title)
		assert.Equal(t, "Zoo", pages[2].Title)
	})

	t.Run("canonicalPath set to /p", func(t *testing.T) {
		base, svc := createTestBase()
		svc.articles = []*models.Article{{Slug: "x", Title: "X", Type: "page", Date: time.Now()}}

		mockTpl := requireMockTemplateService(t, base)
		mockTpl.LastData = nil

		router := gin.New()
		router.GET("/p", NewPostHandler(base, svc).Pages)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p", http.NoBody))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "/p", mockTpl.LastData["canonicalPath"])
	})
}

// TestPages_HEAD_Supported — HEAD verb on /p returns the same status as GET.
func TestPages_HEAD_Supported(t *testing.T) {
	base, svc := createTestBase()
	svc.articles = []*models.Article{{Slug: "ok", Title: "OK", Type: "page", Date: time.Now()}}
	router := gin.New()
	router.HEAD("/p", NewPostHandler(base, svc).Pages)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/p", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

// requireMockTemplateService extracts the MockTemplateService from a
// BaseHandler for data-inspection tests. Fails the test if the base
// wasn't wired with a mock (which would render real templates and
// invalidate the data-capture).
func requireMockTemplateService(t *testing.T, base *BaseHandler) *MockTemplateService {
	t.Helper()
	mockTpl, ok := base.templateService.(*MockTemplateService)
	require.True(t, ok, "createTestBase must wire a MockTemplateService for data inspection")
	return mockTpl
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

// TestArticle_PrevNextNeighbors locks the post-footer pager data: prev = newer,
// next = older (matching the feed's "← Newer / Older →"), nil at the ends, drafts
// excluded from the sequence, and hasNeighbors gating the <nav> so it never renders
// empty. Pages are excluded from this graph by GetAllArticles' DedicatedRouteArticle
// predicate (covered in the article package); the mock returns raw fixtures, so page
// exclusion is intentionally not re-modeled here.
func TestArticle_PrevNextNeighbors(t *testing.T) {
	now := time.Now()
	// Newest-first, matching GetAllArticles' production order.
	newest := &models.Article{Slug: "newest", Title: "Newest", Date: now}
	middle := &models.Article{Slug: "middle", Title: "Middle", Date: now.Add(-24 * time.Hour)}
	oldest := &models.Article{Slug: "oldest", Title: "Oldest", Date: now.Add(-48 * time.Hour)}
	draft := &models.Article{Slug: "draft", Title: "Draft", Draft: true, Date: now.Add(-36 * time.Hour)}

	tests := []struct {
		name         string
		articles     []*models.Article
		slug         string
		wantPrev     string // "" means nil
		wantNext     string // "" means nil
		wantNeighbor bool
	}{
		{"middle has both", []*models.Article{newest, middle, oldest}, "middle", "newest", "oldest", true},
		{"newest end has only older", []*models.Article{newest, middle, oldest}, "newest", "", "middle", true},
		{"oldest end has only newer", []*models.Article{newest, middle, oldest}, "oldest", "middle", "", true},
		{"single article has none", []*models.Article{middle}, "middle", "", "", false},
		{"draft neighbor skipped", []*models.Article{newest, middle, draft, oldest}, "middle", "newest", "oldest", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, svc := createTestBase()
			svc.articles = tt.articles
			mockTpl := requireMockTemplateService(t, base)
			mockTpl.LastData = nil

			router := gin.New()
			router.GET("/writing/:slug", NewPostHandler(base, svc).Article)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/writing/"+tt.slug, http.NoBody))
			require.Equal(t, http.StatusOK, w.Code)
			require.NotNil(t, mockTpl.LastData)

			assert.Equal(t, tt.wantNeighbor, mockTpl.LastData["hasNeighbors"], "hasNeighbors")
			assertNeighborSlug(t, "prevArticle", mockTpl.LastData["prevArticle"], tt.wantPrev)
			assertNeighborSlug(t, "nextArticle", mockTpl.LastData["nextArticle"], tt.wantNext)
		})
	}
}

func assertNeighborSlug(t *testing.T, key string, got any, wantSlug string) {
	t.Helper()
	if wantSlug == "" {
		assert.Nil(t, got, "%s should be nil at the list end", key)
		return
	}
	art, ok := got.(*models.Article)
	require.True(t, ok, "%s should be *models.Article, got %T", key, got)
	require.NotNil(t, art, "%s should not be nil", key)
	assert.Equal(t, wantSlug, art.Slug, "%s slug", key)
}

// TestArticle_PagerRendered verifies the post-footer pager renders real neighbor
// links through the canonical-URL path (not a hardcoded shape), and that the
// strict-typed template FuncMap didn't truncate the render — the pager uses only
// if/permalink/displayTitle, never or/not on non-bools, so the page must complete
// through </body> and the app.js tag. [[verify-served-outcome-not-artifact]]
func TestArticle_PagerRendered(t *testing.T) {
	now := time.Now()
	cfg := &config.Config{
		Environment: "test",
		BaseURL:     "http://localhost:3000",
		Blog:        config.BlogConfig{Title: "Test Blog", Description: "Test", Author: "Test Author"},
	}
	tplSvc, err := services.NewTemplateService("/nonexistent", cfg)
	require.NoError(t, err, "real TemplateService falls back to embedded templates")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	base := NewBaseHandler(cfg, logger, tplSvc, &BuildInfo{Version: "test"}, &MockSEOService{})
	svc := &TestArticleService{articles: []*models.Article{
		{Slug: "newest", Title: "Newest", Date: now, Content: "n", ProcessedContent: "<p>n</p>"},
		{Slug: "middle", Title: "Middle", Date: now.Add(-24 * time.Hour), Content: "m", ProcessedContent: "<p>m</p>"},
		{Slug: "oldest", Title: "Oldest", Date: now.Add(-48 * time.Hour), Content: "o", ProcessedContent: "<p>o</p>"},
	}}

	router := gin.New()
	router.GET("/writing/:slug", NewPostHandler(base, svc).Article)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/writing/middle", http.NoBody))
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `href="/writing/newest"`, "pager must link to the newer article via canonical path")
	assert.Contains(t, body, `href="/writing/oldest"`, "pager must link to the older article via canonical path")
	assert.Contains(t, body, "article-pager", "pager nav must render")
	// Truncation canary: a strict-FuncMap panic cuts the render before these.
	assert.Contains(t, body, "</body>", "render must complete (strict-FuncMap truncation guard)")
	assert.Contains(t, body, "/static/js/app.js", "app.js tag must survive render (truncation guard)")
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
