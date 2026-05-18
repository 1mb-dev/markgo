package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/services"
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
// verbatim from the v3.11.0 AMA_PAGE_* env vars (operator voice), and
// has_contact / has_contact_form together gate the mailto card inside
// about-reach. The AMA card in the template always renders regardless
// of has_contact_form — matches pre-v3.14.0 behavior where /about
// always showed an AMA section.
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
			About: config.AboutConfig{
				// v3.15.0 reach copy: tests assert pipes-through behavior;
				// byte-exact defaults are guarded at config-load time
				// (see internal/config/config_test.go).
				ReachHeading: "Reach out",
				EmailHeading: "Email",
				EmailIntro:   "Or drop a line directly.",
			},
		}
	}

	tests := []struct {
		name               string
		mutate             func(*config.Config)
		wantHasContact     bool
		wantHasContactForm bool
		wantHeading        string
		wantIntro          string
		wantLabel          string
		wantReachHeading   string
		wantEmailHeading   string
		wantEmailIntro     string
	}{
		{
			name:             "defaults: AMA + mailto",
			mutate:           func(*config.Config) {},
			wantHasContact:   true,
			wantHeading:      "Ask me anything",
			wantIntro:        "Curious about something?",
			wantLabel:        "Submit Question",
			wantReachHeading: "Reach out",
			wantEmailHeading: "Email",
			wantEmailIntro:   "Or drop a line directly.",
		},
		{
			name: "no email — AMA still renders solo",
			mutate: func(c *config.Config) {
				c.Blog.AuthorEmail = ""
			},
			wantHasContact:   false,
			wantHeading:      "Ask me anything",
			wantIntro:        "Curious about something?",
			wantLabel:        "Submit Question",
			wantReachHeading: "Reach out",
			wantEmailHeading: "Email",
			wantEmailIntro:   "Or drop a line directly.",
		},
		{
			name: "custom AMA copy reaches template (operator voice)",
			mutate: func(c *config.Config) {
				c.AMA.PageHeading = "Hit me up"
				c.AMA.PageIntro = "Got something on your mind?"
				c.AMA.SubmitLabel = "Send it"
			},
			wantHasContact:   true,
			wantHeading:      "Hit me up",
			wantIntro:        "Got something on your mind?",
			wantLabel:        "Send it",
			wantReachHeading: "Reach out",
			wantEmailHeading: "Email",
			wantEmailIntro:   "Or drop a line directly.",
		},
		{
			// v3.15.0 #78: operator overrides for the reach section's
			// section heading + email card heading/intro must reach the
			// template verbatim, alongside the AMA copy.
			name: "custom reach copy reaches template (operator voice)",
			mutate: func(c *config.Config) {
				c.About.ReachHeading = "Get in touch"
				c.About.EmailHeading = "Mail me"
				c.About.EmailIntro = "I read everything."
			},
			wantHasContact:   true,
			wantHeading:      "Ask me anything",
			wantIntro:        "Curious about something?",
			wantLabel:        "Submit Question",
			wantReachHeading: "Get in touch",
			wantEmailHeading: "Mail me",
			wantEmailIntro:   "I read everything.",
		},
		{
			// Regression guard for the v3.14.0 PR #76 finding: AMA card
			// must keep rendering even when SMTP is fully configured.
			// has_contact_form=true gates the mailto card but the
			// template (about.html) renders AMA unconditionally.
			name: "SMTP configured — AMA still rendered, mailto suppressed",
			mutate: func(c *config.Config) {
				c.Email.Host = "smtp.example.com"
				c.Email.Username = "user"
			},
			wantHasContact:     true,
			wantHasContactForm: true,
			wantHeading:        "Ask me anything",
			wantIntro:          "Curious about something?",
			wantLabel:          "Submit Question",
			wantReachHeading:   "Reach out",
			wantEmailHeading:   "Email",
			wantEmailIntro:     "Or drop a line directly.",
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

			assert.Equal(t, tt.wantHasContact, mockTpl.LastData["has_contact"], "has_contact (BLOG_AUTHOR_EMAIL set) feeds the mailto card visibility")
			assert.Equal(t, tt.wantHasContactForm, mockTpl.LastData["has_contact_form"], "has_contact_form (SMTP configured) gates the full contact form section")
			assert.Equal(t, tt.wantHeading, mockTpl.LastData["about_ama_heading"], "operator voice — AMA heading reaches the template verbatim")
			assert.Equal(t, tt.wantIntro, mockTpl.LastData["about_ama_intro"])
			assert.Equal(t, tt.wantLabel, mockTpl.LastData["about_ama_label"])
			assert.Equal(t, tt.wantReachHeading, mockTpl.LastData["about_reach_heading"], "operator voice — reach section heading reaches the template verbatim")
			assert.Equal(t, tt.wantEmailHeading, mockTpl.LastData["about_email_heading"])
			assert.Equal(t, tt.wantEmailIntro, mockTpl.LastData["about_email_intro"])
		})
	}
}

// TestAboutHandler_JSONLDEmail locks the contract: about.html JSON-LD emits
// the "email" field only when BLOG_AUTHOR_EMAIL is configured. With an empty
// value, no "email": substring may appear inside the JSON-LD block.
// Sibling of TestArticleHandler_JSONLDEmail — locks the same contract on
// the second emission site surfaced by #80.
func TestAboutHandler_JSONLDEmail(t *testing.T) {
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
			handler := NewAboutHandler(base, &MockArticleService{}, &MockMarkdownRenderer{})

			router := gin.New()
			router.GET("/about", handler.ShowAbout)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/about", http.NoBody))
			require.Equal(t, http.StatusOK, w.Code)

			body := w.Body.String()
			jsonLDStart := strings.Index(body, `<script type="application/ld+json">`)
			require.GreaterOrEqual(t, jsonLDStart, 0, "JSON-LD script tag must be present in about body")
			jsonLDEnd := strings.Index(body[jsonLDStart:], `</script>`)
			require.Greater(t, jsonLDEnd, 0, "JSON-LD script tag must be closed")
			jsonLD := body[jsonLDStart : jsonLDStart+jsonLDEnd]

			if tc.denySubstring != "" {
				assert.NotContains(t, jsonLD, tc.denySubstring,
					"JSON-LD must not emit email field when AuthorEmail is empty")
			}
			if tc.wantSubstring != "" {
				assert.Contains(t, jsonLD, tc.wantSubstring,
					"JSON-LD must emit configured email value")
			}
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
