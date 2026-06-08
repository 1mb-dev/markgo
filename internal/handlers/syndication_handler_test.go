package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// T4: SyndicationHandler.HumansTxt()
// ---------------------------------------------------------------------------

func TestHumansTxt(t *testing.T) {
	t.Run("returns author and site info", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "1.0.0"}, &MockSEOService{})
		handler := NewSyndicationHandler(base, &MockFeedService{})

		router := gin.New()
		router.GET("/humans.txt", handler.HumansTxt)

		req := httptest.NewRequest("GET", "/humans.txt", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")

		body := w.Body.String()
		assert.Contains(t, body, "Test Author")
		assert.Contains(t, body, "http://localhost:3000")
		assert.Contains(t, body, "MarkGo 1.0.0")
		assert.Contains(t, body, "/* TEAM */")
		assert.Contains(t, body, "/* SITE */")
	})

	t.Run("nil buildInfo uses unknown version", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, nil, &MockSEOService{})
		handler := NewSyndicationHandler(base, &MockFeedService{})

		router := gin.New()
		router.GET("/humans.txt", handler.HumansTxt)

		req := httptest.NewRequest("GET", "/humans.txt", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "MarkGo unknown")
	})

	t.Run("empty version uses unknown", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: ""}, &MockSEOService{})
		handler := NewSyndicationHandler(base, &MockFeedService{})

		router := gin.New()
		router.GET("/humans.txt", handler.HumansTxt)

		req := httptest.NewRequest("GET", "/humans.txt", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "MarkGo unknown")
	})
}

// TestSyndicationCacheControl verifies the feeds and sitemap set a shared
// 1-hour Cache-Control so crawlers/CDNs don't force a full regeneration each hit.
func TestSyndicationCacheControl(t *testing.T) {
	cfg := createTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})
	handler := NewSyndicationHandler(base, &MockFeedService{})

	cases := []struct {
		name, path string
		h          gin.HandlerFunc
	}{
		{"rss", "/feed.xml", handler.RSS},
		{"json", "/feed.json", handler.JSONFeed},
		{"sitemap", "/sitemap.xml", handler.Sitemap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.GET(tc.path, tc.h)
			req := httptest.NewRequest("GET", tc.path, http.NoBody)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
		})
	}
}
