package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
)

func createTestAboutHandler(cfg *config.Config) (*AboutHandler, *MockTemplateService) {
	if cfg == nil {
		cfg = createTestConfig()
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	mockTpl := &MockTemplateService{}
	base := NewBaseHandler(cfg, logger, mockTpl, &BuildInfo{Version: "test"}, &MockSEOService{})
	return NewAboutHandler(base, &MockArticleService{}, &MockMarkdownRenderer{}), mockTpl
}

// TestAboutHandler_TemplateData verifies the ShowAbout data contract
// (v3.14.0+ closes #75). The AMA copy keys must reach the template
// verbatim from the v3.11.0 AMA_PAGE_* env vars (operator voice),
// and has_contact must gate the reach-section mailto half. The AMA
// half always renders — matches pre-v3.14.0 behavior; getEnv falls
// back to non-empty defaults so there's no .env path to hide it.
func TestAboutHandler_TemplateData(t *testing.T) {
	baseCfg := func() *config.Config {
		return &config.Config{
			Environment: "test",
			BaseURL:     "http://localhost:3000",
			Blog: config.BlogConfig{
				Title:       "Test Blog",
				Author:      "Test Author",
				AuthorEmail: "author@example.com",
			},
			AMA: config.AMAConfig{
				PageHeading: "Ask me anything",
				PageIntro:   "Curious about something?",
				SubmitLabel: "Submit Question",
			},
		}
	}

	tests := []struct {
		name           string
		mutate         func(*config.Config)
		wantHasContact bool
		wantHeading    string
		wantIntro      string
		wantLabel      string
	}{
		{
			name:           "defaults: AMA + mailto",
			mutate:         func(*config.Config) {},
			wantHasContact: true,
			wantHeading:    "Ask me anything",
			wantIntro:      "Curious about something?",
			wantLabel:      "Submit Question",
		},
		{
			name: "no email — AMA still renders solo",
			mutate: func(c *config.Config) {
				c.Blog.AuthorEmail = ""
			},
			wantHasContact: false,
			wantHeading:    "Ask me anything",
			wantIntro:      "Curious about something?",
			wantLabel:      "Submit Question",
		},
		{
			name: "custom AMA copy reaches template (operator voice)",
			mutate: func(c *config.Config) {
				c.AMA.PageHeading = "Hit me up"
				c.AMA.PageIntro = "Got something on your mind?"
				c.AMA.SubmitLabel = "Send it"
			},
			wantHasContact: true,
			wantHeading:    "Hit me up",
			wantIntro:      "Got something on your mind?",
			wantLabel:      "Send it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseCfg()
			tt.mutate(cfg)
			handler, mockTpl := createTestAboutHandler(cfg)

			router := gin.New()
			router.GET("/about", handler.ShowAbout)

			req := httptest.NewRequest(http.MethodGet, "/about", http.NoBody)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			require.NotNil(t, mockTpl.LastData, "ShowAbout must invoke the template renderer")

			assert.Equal(t, tt.wantHasContact, mockTpl.LastData["has_contact"], "has_contact gates the reach-section mailto half")
			assert.Equal(t, tt.wantHeading, mockTpl.LastData["about_ama_heading"], "operator voice — AMA heading reaches the template verbatim")
			assert.Equal(t, tt.wantIntro, mockTpl.LastData["about_ama_intro"])
			assert.Equal(t, tt.wantLabel, mockTpl.LastData["about_ama_label"])
		})
	}
}

// TestAboutHandler_Identity verifies the identity/social/bio composition
// path still produces expected data after the v3.14.0 reach addition.
func TestAboutHandler_Identity(t *testing.T) {
	cfg := &config.Config{
		Environment: "test",
		BaseURL:     "http://localhost:3000",
		Blog:        config.BlogConfig{Title: "Test Blog", Author: "Jane Doe"},
		About: config.AboutConfig{
			Avatar:   "img/avatar.jpg",
			Tagline:  "Building things",
			Location: "San Francisco, CA",
			GitHub:   "janedoe",
		},
	}
	handler, mockTpl := createTestAboutHandler(cfg)

	router := gin.New()
	router.GET("/about", handler.ShowAbout)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/about", http.NoBody))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, mockTpl.LastData)
	assert.Equal(t, "img/avatar.jpg", mockTpl.LastData["about_avatar"])
	assert.Equal(t, "Building things", mockTpl.LastData["about_tagline"])
	assert.Equal(t, "San Francisco, CA", mockTpl.LastData["about_location"])
	assert.Equal(t, true, mockTpl.LastData["has_social"])
}

func TestBuildSocialLinks(t *testing.T) {
	t.Run("no social links configured", func(t *testing.T) {
		handler, _ := createTestAboutHandler(nil)
		links := handler.buildSocialLinks()
		assert.Empty(t, links)
	})

	t.Run("normalizes github username to full URL", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.About.GitHub = "testuser"
		handler, _ := createTestAboutHandler(cfg)
		links := handler.buildSocialLinks()

		assert.Len(t, links, 1)
		assert.Equal(t, "github", links[0].Platform)
		assert.Equal(t, "https://github.com/testuser", links[0].URL)
	})

	t.Run("preserves full URLs", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.About.GitHub = "https://github.com/testuser"
		handler, _ := createTestAboutHandler(cfg)
		links := handler.buildSocialLinks()

		assert.Len(t, links, 1)
		assert.Equal(t, "https://github.com/testuser", links[0].URL)
	})

	t.Run("normalizes twitter handle", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.About.Twitter = "@janedoe"
		handler, _ := createTestAboutHandler(cfg)
		links := handler.buildSocialLinks()

		assert.Len(t, links, 1)
		assert.Equal(t, "https://x.com/janedoe", links[0].URL)
	})

	t.Run("all platforms configured", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.About.GitHub = "user"
		cfg.About.Twitter = "user"
		cfg.About.LinkedIn = "https://linkedin.com/in/user"
		cfg.About.Mastodon = "https://mastodon.social/@user"
		cfg.About.Website = "example.com"
		handler, _ := createTestAboutHandler(cfg)
		links := handler.buildSocialLinks()

		assert.Len(t, links, 5)
		assert.Equal(t, "github", links[0].Platform)
		assert.Equal(t, "twitter", links[1].Platform)
		assert.Equal(t, "linkedin", links[2].Platform)
		assert.Equal(t, "mastodon", links[3].Platform)
		assert.Equal(t, "website", links[4].Platform)
	})
}

func TestNormalizeURL(t *testing.T) {
	assert.Equal(t, "https://github.com/user", normalizeURL("user", "https://github.com/"))
	assert.Equal(t, "https://github.com/user", normalizeURL("https://github.com/user", "https://github.com/"))
	assert.Equal(t, "https://example.com", normalizeURL("example.com", "https://"))
}
