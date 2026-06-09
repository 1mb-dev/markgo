package handlers

import (
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
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

// TestRenderHTML_ETagRevalidation locks the HTML conditional-GET contract:
// successful pages emit a weak ETag + Cache-Control: no-cache; a matching
// If-None-Match returns 304 with no body; a stale one returns the full page;
// different content yields a different ETag.
func TestRenderHTML_ETagRevalidation(t *testing.T) {
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
		{Slug: "a", Title: "Article A", Date: now, Content: "x", ProcessedContent: "<p>x</p>"},
		{Slug: "b", Title: "Totally Different B", Date: now.Add(-time.Hour), Content: "y", ProcessedContent: "<p>y</p>"},
	}}
	router := gin.New()
	router.GET("/writing/:slug", NewPostHandler(base, svc).Article)

	get := func(slug, ifNoneMatch string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/writing/"+slug, http.NoBody)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		router.ServeHTTP(w, req)
		return w
	}

	// First load: full 200 with a weak ETag + no-cache.
	w1 := get("a", "")
	require.Equal(t, http.StatusOK, w1.Code)
	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag, "successful HTML must carry an ETag")
	assert.True(t, strings.HasPrefix(etag, `W/"`), "ETag must be weak, got %q", etag)
	assert.Equal(t, "no-cache", w1.Header().Get("Cache-Control"))
	require.NotEmpty(t, w1.Body.String())

	// Matching If-None-Match → 304, no body, ETag echoed.
	w2 := get("a", etag)
	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Empty(t, w2.Body.String(), "304 must not send a body")
	assert.Equal(t, etag, w2.Header().Get("ETag"))

	// Stale If-None-Match → full 200.
	w3 := get("a", `W/"deadbeef00000000"`)
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.NotEmpty(t, w3.Body.String())

	// Different article → different ETag (body-derived validator).
	w4 := get("b", "")
	assert.NotEqual(t, etag, w4.Header().Get("ETag"), "different content must yield a different ETag")
}

// TestRenderHTML_ErrorPageNoETag — non-200 (error) pages keep the streaming
// path and carry no ETag (nothing to revalidate).
func TestRenderHTML_ErrorPageNoETag(t *testing.T) {
	base, svc := createTestBase()
	router := gin.New()
	router.GET("/writing/:slug", NewPostHandler(base, svc).Article)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/writing/nonexistent", http.NoBody))
	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, w.Header().Get("ETag"), "error pages must not emit an ETag")
}
