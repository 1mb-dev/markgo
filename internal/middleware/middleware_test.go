package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/web"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter() *gin.Engine {
	router := gin.New()
	return router
}

// TestCORS tests the CORS middleware security
func TestCORS(t *testing.T) {
	t.Run("exact origin match - allowed", func(t *testing.T) {
		router := setupTestRouter()
		allowedOrigins := []string{"https://example.com", "https://api.example.com"}
		router.Use(CORS(allowedOrigins, false))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
	})

	t.Run("exact origin match - not allowed", func(t *testing.T) {
		router := setupTestRouter()
		allowedOrigins := []string{"https://example.com"}
		router.Use(CORS(allowedOrigins, false))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.Header.Set("Origin", "https://evil.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		// Should NOT set Access-Control-Allow-Origin for disallowed origin
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("localhost bypass prevented - evil domain", func(t *testing.T) {
		router := setupTestRouter()
		allowedOrigins := []string{"https://example.com"}
		router.Use(CORS(allowedOrigins, false))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		// Try to bypass with localhost.evil.com (should be rejected)
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.Header.Set("Origin", "http://localhost.evil.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		// Should NOT allow localhost.evil.com even though it contains "localhost"
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("development mode - localhost allowed", func(t *testing.T) {
		router := setupTestRouter()
		allowedOrigins := []string{"https://example.com"}
		router.Use(CORS(allowedOrigins, true)) // Development mode
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("OPTIONS preflight request", func(t *testing.T) {
		router := setupTestRouter()
		allowedOrigins := []string{"https://example.com"}
		router.Use(CORS(allowedOrigins, false))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		req := httptest.NewRequest("OPTIONS", "/test", http.NoBody)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("no origin header - no CORS headers", func(t *testing.T) {
		router := setupTestRouter()
		allowedOrigins := []string{"https://example.com"}
		router.Use(CORS(allowedOrigins, false))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		// No Origin header
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		// Should not set CORS headers when no Origin header present
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})
}

// TestRateLimit tests the rate limiting middleware
func TestRateLimit(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(RateLimit(5, 1*time.Minute))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		// Make 5 requests (within limit)
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code, "Request %d should succeed", i+1)
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(RateLimit(3, 1*time.Minute))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		// Make 3 requests (at limit)
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			req.RemoteAddr = "192.168.1.2:12345"
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code, "Request %d should succeed", i+1)
		}

		// 4th request should be blocked
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.RemoteAddr = "192.168.1.2:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.NotEmpty(t, w.Header().Get("Retry-After"))
	})

	t.Run("different IPs tracked separately", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(RateLimit(2, 1*time.Minute))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		// IP1: 2 requests (at limit)
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			req.RemoteAddr = "192.168.1.3:12345"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code)
		}

		// IP2: 2 requests (should also succeed - different IP)
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			req.RemoteAddr = "192.168.1.4:12345"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code)
		}
	})

	t.Run("strips port from RemoteAddr", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(RateLimit(2, 1*time.Minute))
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "ok")
		})

		// Same IP with different ports should be treated as same client
		req1 := httptest.NewRequest("GET", "/test", http.NoBody)
		req1.RemoteAddr = "192.168.1.5:11111"
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		assert.Equal(t, 200, w1.Code)

		req2 := httptest.NewRequest("GET", "/test", http.NoBody)
		req2.RemoteAddr = "192.168.1.5:22222" // Different port, same IP
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		assert.Equal(t, 200, w2.Code)

		// 3rd request should be blocked (same IP, at limit)
		req3 := httptest.NewRequest("GET", "/test", http.NoBody)
		req3.RemoteAddr = "192.168.1.5:33333"
		w3 := httptest.NewRecorder()
		router.ServeHTTP(w3, req3)
		assert.Equal(t, http.StatusTooManyRequests, w3.Code)
	})
}

// TestSecurity tests the security headers middleware
func TestSecurity(t *testing.T) {
	router := setupTestRouter()
	router.Use(Security(&config.Config{}))
	router.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	assert.Equal(t, "max-age=31536000; includeSubDomains", w.Header().Get("Strict-Transport-Security"))
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "default-src 'self'")
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "'sha256-0pz7XU3iscvI1rWHhJ8OyLJ4xXNoivNIt1N5xpF6GUg='")
	assert.Contains(t, w.Header().Get("Permissions-Policy"), "interest-cohort=()")
	assert.Empty(t, w.Header().Get("X-XSS-Protection"), "X-XSS-Protection is deprecated and must not be emitted")
}

// TestSecurity_FOUCScriptHashMatches locks the CSP script-src hash to the
// actual contents of the inline <script> in base.html. Editing the inline
// script without updating foucScriptHash in middleware.go fails this test
// before reaching users' browsers (where CSP would silently block the script).
func TestSecurity_FOUCScriptHashMatches(t *testing.T) {
	data, err := web.Assets.ReadFile("templates/base.html")
	require.NoError(t, err)

	re := regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	match := re.FindSubmatch(data)
	require.NotNil(t, match, "expected exactly one inline <script> in base.html")

	sum := sha256.Sum256(match[1])
	expected := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
	assert.Equal(t, expected, foucScriptHash,
		"FOUC script content changed; update foucScriptHash in middleware.go to %s", expected)
}

// TestSecurity_ExactlyOneInlineJavaScript asserts only the FOUC script is
// inlined as executable JavaScript. JSON-LD blocks (type="application/ld+json")
// are excluded — browsers treat non-JS MIME types as data, not script, so CSP
// script-src does not gate them.
//
// Hash-based CSP fails open on template diff: any new inline executable
// <script> would be silently blocked in browsers. This test catches the drift
// before merge.
func TestSecurity_ExactlyOneInlineJavaScript(t *testing.T) {
	scriptOpen := regexp.MustCompile(`<script[^>]*>[^<]`) // inline content (excludes <script src="..."></script>)
	jsonLD := regexp.MustCompile(`<script[^>]*type="application/ld\+json"[^>]*>`)
	count := 0
	err := fs.WalkDir(web.Assets, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		data, err := web.Assets.ReadFile(path)
		if err != nil {
			return err
		}
		inlineJS := len(scriptOpen.FindAll(data, -1)) - len(jsonLD.FindAll(data, -1))
		if inlineJS > 0 {
			t.Logf("inline JS in %s: %d", path, inlineJS)
		}
		count += inlineJS
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"expected exactly one inline JavaScript across templates (the FOUC script in base.html); "+
			"any new inline executable script needs a CSP hash entry — see foucScriptHash in middleware.go")
}

// TestSecurity_CSPDisableSkipsCSPHeader verifies CSP_DISABLE=true skips the CSP
// header without affecting the other security headers.
func TestSecurity_CSPDisableSkipsCSPHeader(t *testing.T) {
	router := setupTestRouter()
	router.Use(Security(&config.Config{Security: config.SecurityConfig{CSPDisable: true}}))
	router.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "max-age=31536000; includeSubDomains", w.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, w.Header().Get("Permissions-Policy"), "interest-cohort=()")
}

// TestLogger tests the logger middleware
func TestLogger(t *testing.T) {
	router := setupTestRouter()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	router.Use(Logger(logger))
	router.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

// TestNoCache tests the NoCache middleware
func TestNoCache(t *testing.T) {
	router := setupTestRouter()
	router.Use(NoCache())
	router.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Equal(t, "0", w.Header().Get("Expires"))
}

// TestCSRF tests the CSRF middleware
func TestCSRF(t *testing.T) {
	t.Run("GET sets token cookie and context", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		var token string
		router.GET("/form", func(c *gin.Context) {
			if v, exists := c.Get("csrf_token"); exists {
				token, _ = v.(string)
			}
			c.String(200, "ok")
		})

		req := httptest.NewRequest("GET", "/form", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.NotEmpty(t, token)
		// Cookie should be set
		cookies := w.Result().Cookies()
		var csrfCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "_csrf" {
				csrfCookie = cookie
			}
		}
		require.NotNil(t, csrfCookie)
		assert.Equal(t, token, csrfCookie.Value)
		assert.True(t, csrfCookie.HttpOnly)
		assert.True(t, csrfCookie.Secure)
	})

	t.Run("POST with valid token succeeds", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		var token string
		router.GET("/form", func(c *gin.Context) {
			if v, exists := c.Get("csrf_token"); exists {
				token, _ = v.(string)
			}
			c.String(200, "ok")
		})
		router.POST("/form", func(c *gin.Context) {
			c.String(200, "posted")
		})

		// GET to get the token
		getReq := httptest.NewRequest("GET", "/form", http.NoBody)
		getW := httptest.NewRecorder()
		router.ServeHTTP(getW, getReq)
		require.Equal(t, 200, getW.Code)
		require.NotEmpty(t, token)

		// POST with the token
		form := url.Values{"_csrf": {token}}
		postReq := httptest.NewRequest("POST", "/form", strings.NewReader(form.Encode()))
		postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		postReq.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
		postW := httptest.NewRecorder()
		router.ServeHTTP(postW, postReq)

		assert.Equal(t, 200, postW.Code)
		assert.Equal(t, "posted", postW.Body.String())
	})

	t.Run("POST without token is rejected", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		router.POST("/form", func(c *gin.Context) {
			c.String(200, "posted")
		})

		postReq := httptest.NewRequest("POST", "/form", http.NoBody)
		postW := httptest.NewRecorder()
		router.ServeHTTP(postW, postReq)

		assert.Equal(t, http.StatusForbidden, postW.Code)
	})

	t.Run("POST with mismatched token is rejected", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		router.POST("/form", func(c *gin.Context) {
			c.String(200, "posted")
		})

		form := url.Values{"_csrf": {"wrong-token"}}
		postReq := httptest.NewRequest("POST", "/form", strings.NewReader(form.Encode()))
		postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		postReq.AddCookie(&http.Cookie{Name: "_csrf", Value: "different-token"})
		postW := httptest.NewRecorder()
		router.ServeHTTP(postW, postReq)

		assert.Equal(t, http.StatusForbidden, postW.Code)
	})

	t.Run("POST with valid X-CSRF-Token header succeeds", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		var token string
		router.GET("/api", func(c *gin.Context) {
			if v, exists := c.Get("csrf_token"); exists {
				token, _ = v.(string)
			}
			c.String(200, "ok")
		})
		router.POST("/api", func(c *gin.Context) {
			c.String(200, "posted")
		})

		// GET to get the token
		getReq := httptest.NewRequest("GET", "/api", http.NoBody)
		getW := httptest.NewRecorder()
		router.ServeHTTP(getW, getReq)
		require.Equal(t, 200, getW.Code)
		require.NotEmpty(t, token)

		// POST with token in header (JSON API pattern)
		postReq := httptest.NewRequest("POST", "/api", strings.NewReader(`{"content":"hello"}`))
		postReq.Header.Set("Content-Type", "application/json")
		postReq.Header.Set("X-CSRF-Token", token)
		postReq.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
		postW := httptest.NewRecorder()
		router.ServeHTTP(postW, postReq)

		assert.Equal(t, 200, postW.Code)
		assert.Equal(t, "posted", postW.Body.String())
	})
}

// TestCSRF_CookieStability covers the v3.18.1 fix: the middleware reuses a
// valid existing _csrf cookie on GET/HEAD instead of unconditionally
// regenerating it. SPA navigation only swaps <main>, so the meta tag in
// <head> goes stale if the cookie rotates underneath it — the subsequent
// XHR's X-CSRF-Token header (from stale meta) then mismatches the new
// cookie and the POST 403s. Cookie must be session-stable.
func TestCSRF_CookieStability(t *testing.T) {
	t.Run("GET preserves valid existing cookie", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		var contextToken string
		router.GET("/page", func(c *gin.Context) {
			if v, exists := c.Get("csrf_token"); exists {
				contextToken, _ = v.(string)
			}
			c.String(200, "ok")
		})

		existing := generateCSRFToken()
		require.NotEmpty(t, existing)

		req := httptest.NewRequest("GET", "/page", http.NoBody)
		req.AddCookie(&http.Cookie{Name: "_csrf", Value: existing})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, 200, w.Code)
		assert.Equal(t, existing, contextToken,
			"middleware must reuse the existing valid cookie, not generate a new token")

		// No rotating Set-Cookie should be emitted when the cookie is reused.
		for _, c := range w.Result().Cookies() {
			if c.Name == "_csrf" {
				assert.Equal(t, existing, c.Value,
					"if a Set-Cookie is emitted, it must echo the existing token, not a rotated one")
			}
		}
	})

	t.Run("GET generates when cookie absent", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		var contextToken string
		router.GET("/page", func(c *gin.Context) {
			if v, exists := c.Get("csrf_token"); exists {
				contextToken, _ = v.(string)
			}
			c.String(200, "ok")
		})

		req := httptest.NewRequest("GET", "/page", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, 200, w.Code)
		require.NotEmpty(t, contextToken)

		var csrfCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "_csrf" {
				csrfCookie = c
			}
		}
		require.NotNil(t, csrfCookie, "Set-Cookie must be emitted when no cookie was sent")
		assert.Equal(t, contextToken, csrfCookie.Value)
	})

	t.Run("GET regenerates when cookie malformed", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		var contextToken string
		router.GET("/page", func(c *gin.Context) {
			if v, exists := c.Get("csrf_token"); exists {
				contextToken, _ = v.(string)
			}
			c.String(200, "ok")
		})

		// Wrong length and non-hex characters — must be rejected, not trusted.
		req := httptest.NewRequest("GET", "/page", http.NoBody)
		req.AddCookie(&http.Cookie{Name: "_csrf", Value: "not-a-valid-hex-token"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, 200, w.Code)
		require.NotEmpty(t, contextToken)
		assert.NotEqual(t, "not-a-valid-hex-token", contextToken,
			"middleware must not accept a malformed cookie as-is")

		var csrfCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "_csrf" {
				csrfCookie = c
			}
		}
		require.NotNil(t, csrfCookie, "Set-Cookie must be emitted when malformed cookie is rejected")
		assert.Equal(t, contextToken, csrfCookie.Value)
	})

	t.Run("POST validates against preserved cookie", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(CSRF(true))
		router.GET("/api", func(c *gin.Context) { c.String(200, "ok") })
		router.POST("/api", func(c *gin.Context) { c.String(200, "posted") })

		existing := generateCSRFToken()
		require.NotEmpty(t, existing)

		// GET with the cookie — middleware should reuse it.
		getReq := httptest.NewRequest("GET", "/api", http.NoBody)
		getReq.AddCookie(&http.Cookie{Name: "_csrf", Value: existing})
		getW := httptest.NewRecorder()
		router.ServeHTTP(getW, getReq)
		require.Equal(t, 200, getW.Code)

		// POST with the original token — must succeed because cookie was preserved.
		postReq := httptest.NewRequest("POST", "/api", strings.NewReader(""))
		postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		postReq.Header.Set("X-CSRF-Token", existing)
		postReq.AddCookie(&http.Cookie{Name: "_csrf", Value: existing})
		postW := httptest.NewRecorder()
		router.ServeHTTP(postW, postReq)
		assert.Equal(t, 200, postW.Code, "POST must validate against the preserved cookie")
	})
}

// TestSmartCacheHeaders tests the smart cache headers middleware
func TestSmartCacheHeaders(t *testing.T) {
	router := setupTestRouter()
	router.Use(SmartCacheHeaders())
	router.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
}
