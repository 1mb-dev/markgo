package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
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

// TestRateLimit_ProxyKeying verifies the limiter keys on the real client once
// SetTrustedProxies is wired like production: with the proxy trusted, two XFF
// clients behind one proxy get separate buckets; with no proxy trusted, the
// forwarded header is ignored and they collapse onto the direct peer. The test
// router MUST call SetTrustedProxies exactly as the server does — otherwise the
// assertion would pass for the wrong reason (gin trusts all proxies by default).
func TestRateLimit_ProxyKeying(t *testing.T) {
	const proxy = "192.0.2.1" // RFC 5737 TEST-NET-1 — the trusted reverse proxy
	const clientA = "203.0.113.10"
	const clientB = "203.0.113.20"

	send := func(router *gin.Engine, remotePort, xff string) int {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.RemoteAddr = proxy + ":" + remotePort
		req.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	t.Run("trusted proxy keys on forwarded client", func(t *testing.T) {
		router := gin.New()
		require.NoError(t, router.SetTrustedProxies([]string{proxy + "/32"}))
		router.Use(RateLimit(1, time.Minute))
		router.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

		assert.Equal(t, 200, send(router, "1111", clientA), "clientA first request")
		assert.Equal(t, http.StatusTooManyRequests, send(router, "2222", clientA), "clientA over limit")
		assert.Equal(t, 200, send(router, "3333", clientB), "clientB has its own bucket")
	})

	t.Run("untrusted proxy ignores forwarded header", func(t *testing.T) {
		router := gin.New()
		// Prod-unset posture: only loopback is trusted. proxy (192.0.2.1) is an
		// off-host public IP outside the trusted set, so its XFF is ignored.
		require.NoError(t, router.SetTrustedProxies(EffectiveTrustedProxies(nil)))
		router.Use(RateLimit(1, time.Minute))
		router.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

		// Distinct forwarded clients, but XFF is ignored → both key on the proxy IP.
		assert.Equal(t, 200, send(router, "1111", clientA), "first request keyed on proxy IP")
		assert.Equal(t, http.StatusTooManyRequests, send(router, "2222", clientB),
			"second request collapses onto the same proxy IP bucket")
	})
}

// TestEffectiveTrustedProxies verifies the unset case auto-trusts loopback while
// an operator-set list is used verbatim, and that the returned slice is a fresh
// copy (mutating it must not corrupt the shared default).
func TestEffectiveTrustedProxies(t *testing.T) {
	t.Run("unset trusts loopback", func(t *testing.T) {
		assert.Equal(t, []string{"127.0.0.0/8", "::1/128"}, EffectiveTrustedProxies(nil))
		assert.Equal(t, []string{"127.0.0.0/8", "::1/128"}, EffectiveTrustedProxies([]string{}))
	})

	t.Run("operator list used verbatim", func(t *testing.T) {
		operator := []string{"10.0.0.0/8", "192.168.1.5/32"}
		assert.Equal(t, operator, EffectiveTrustedProxies(operator))
	})

	t.Run("returns a fresh slice", func(t *testing.T) {
		got := EffectiveTrustedProxies(nil)
		got[0] = "0.0.0.0/0" // must not poison the package default
		assert.Equal(t, []string{"127.0.0.0/8", "::1/128"}, EffectiveTrustedProxies(nil))
	})
}

// TestRateLimit_LoopbackAutoTrust is the #119 regression matrix: with the
// prod-unset posture (EffectiveTrustedProxies(nil) → loopback trusted), a
// same-host proxy on loopback keys the limiter on the real forwarded client,
// while a forged X-Forwarded-For from an untrusted public peer is ignored.
func TestRateLimit_LoopbackAutoTrust(t *testing.T) {
	newRouter := func() *gin.Engine {
		router := gin.New()
		require.NoError(t, router.SetTrustedProxies(EffectiveTrustedProxies(nil)))
		router.Use(RateLimit(1, time.Minute))
		router.GET("/test", func(c *gin.Context) { c.String(200, "ok") })
		return router
	}
	send := func(router *gin.Engine, remoteAddr, xff string) int {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.RemoteAddr = remoteAddr
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		return func() int {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w.Code
		}()
	}

	t.Run("loopback peer keys on forwarded client (two buckets)", func(t *testing.T) {
		r := newRouter()
		assert.Equal(t, 200, send(r, "127.0.0.1:5000", "203.0.113.10"), "clientA first")
		assert.Equal(t, http.StatusTooManyRequests, send(r, "127.0.0.1:5001", "203.0.113.10"), "clientA over limit")
		assert.Equal(t, 200, send(r, "127.0.0.1:5002", "203.0.113.20"), "clientB own bucket")
	})

	t.Run("public peer with forged XFF keys on the peer (one bucket)", func(t *testing.T) {
		r := newRouter()
		// 203.0.113.5 is not loopback → untrusted → XFF ignored. A remote
		// attacker rotating XFF cannot escape its own per-IP bucket.
		assert.Equal(t, 200, send(r, "203.0.113.5:5000", "1.2.3.4"), "peer first request")
		assert.Equal(t, http.StatusTooManyRequests, send(r, "203.0.113.5:5001", "9.9.9.9"),
			"same peer, different forged XFF, still one bucket")
	})

	t.Run("IPv6 loopback peer keys on forwarded client", func(t *testing.T) {
		r := newRouter()
		assert.Equal(t, 200, send(r, "[::1]:5000", "203.0.113.30"), "clientC first")
		assert.Equal(t, http.StatusTooManyRequests, send(r, "[::1]:5001", "203.0.113.30"), "clientC over limit")
		assert.Equal(t, 200, send(r, "[::1]:5002", "203.0.113.40"), "clientD own bucket")
	})

	t.Run("IPv4-mapped loopback peer is trusted (covered by 127.0.0.0/8)", func(t *testing.T) {
		r := newRouter()
		assert.Equal(t, 200, send(r, "[::ffff:127.0.0.1]:5000", "203.0.113.50"), "clientE first")
		assert.Equal(t, http.StatusTooManyRequests, send(r, "[::ffff:127.0.0.1]:5001", "203.0.113.50"),
			"clientE over limit → XFF honored, so the mapped peer was trusted")
	})
}

// TestProxyTrustWarning verifies the one-shot advisory fires only when a
// forwarded header arrives from an untrusted off-host peer (proxied-but-unset),
// and stays silent for a direct public client and for an auto-trusted loopback
// proxy (which no longer collapses under #119).
func TestProxyTrustWarning(t *testing.T) {
	// Wire the prod-unset posture (EffectiveTrustedProxies(nil) → loopback only;
	// ProxyTrustWarning is mounted only when TRUSTED_PROXIES is empty) —
	// otherwise gin's trust-all default makes ClientIP() return the XFF client
	// and the detector never sees a collapse, passing the test for the wrong
	// reason. Off-host peers (10.x, public) stay untrusted and still collapse.
	newRouter := func(buf *bytes.Buffer) *gin.Engine {
		logger := slog.New(slog.NewTextHandler(buf, nil))
		router := gin.New()
		require.NoError(t, router.SetTrustedProxies(EffectiveTrustedProxies(nil)))
		router.Use(ProxyTrustWarning(logger))
		router.GET("/test", func(c *gin.Context) { c.String(200, "ok") })
		return router
	}
	hit := func(router *gin.Engine, remoteAddr, xff string) {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.RemoteAddr = remoteAddr
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	t.Run("warns once when many clients collapse onto one proxy IP", func(t *testing.T) {
		var buf bytes.Buffer
		router := newRouter(&buf)
		// 6 distinct forwarded clients behind one private proxy IP → collapse.
		for i := range 6 {
			hit(router, "10.1.2.3:5000", fmt.Sprintf("203.0.113.%d", 10+i))
		}
		assert.Equal(t, 1, strings.Count(buf.String(), "set TRUSTED_PROXIES"), "fires exactly once")
	})

	t.Run("warns for a public-IP proxy too (old heuristic missed this)", func(t *testing.T) {
		var buf bytes.Buffer
		router := newRouter(&buf)
		for i := range 5 {
			hit(router, "198.51.100.7:5000", fmt.Sprintf("203.0.113.%d", 20+i))
		}
		assert.Contains(t, buf.String(), "set TRUSTED_PROXIES", "a proxy on a public IP must still warn")
	})

	t.Run("silent below the collapse threshold", func(t *testing.T) {
		var buf bytes.Buffer
		router := newRouter(&buf)
		for i := range 2 { // 2 distinct forwarded clients < threshold
			hit(router, "10.1.2.3:5000", fmt.Sprintf("203.0.113.%d", 40+i))
		}
		assert.NotContains(t, buf.String(), "set TRUSTED_PROXIES")
	})

	t.Run("silent for direct client (no forwarded header)", func(t *testing.T) {
		var buf bytes.Buffer
		router := newRouter(&buf)
		hit(router, "203.0.113.50:443", "")
		assert.NotContains(t, buf.String(), "set TRUSTED_PROXIES")
	})

	t.Run("silent for auto-trusted loopback proxy (no collapse under #119)", func(t *testing.T) {
		var buf bytes.Buffer
		router := newRouter(&buf)
		// Same-host proxy on loopback: gin resolves ClientIP() to the forwarded
		// client, so fwd == key and the detector never counts a collapse.
		for i := range 6 {
			hit(router, "127.0.0.1:5000", fmt.Sprintf("203.0.113.%d", 60+i))
		}
		assert.NotContains(t, buf.String(), "set TRUSTED_PROXIES",
			"loopback is trusted, so there is no collapse to warn about")
	})
}

// TestProxyTrustWarning_Concurrent is the regression guard for the data races in
// the detector: the nil-map write, and the collapse-count read (the warn path
// must log a count snapshotted under the lock, not re-read len(set) from the
// shared map). Many requests cross the warn transition at once. The test router
// has no Recovery middleware, so a nil-map panic would crash the goroutine and
// fail the test; under -race it catches unsynchronized map access (use -count to
// raise the odds the interleaving surfaces — a single run can miss it).
func TestProxyTrustWarning_Concurrent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(EffectiveTrustedProxies(nil)))
	router.Use(ProxyTrustWarning(logger))
	router.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			req.RemoteAddr = "10.9.9.9:5000" // one proxy IP
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", n))
			router.ServeHTTP(httptest.NewRecorder(), req)
		}(i)
	}
	wg.Wait()

	// 50 distinct forwarded clients on one proxy IP → collapse detected, once.
	assert.GreaterOrEqual(t, strings.Count(buf.String(), "set TRUSTED_PROXIES"), 1, "collapse must warn")
}

// TestRateLimit_CeilingWarnsOnce verifies the maxClients ceiling — which 429s
// every new client IP once the map is full — now logs once so the operator can
// diagnose the otherwise-silent collapse.
func TestRateLimit_CeilingWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(RateLimit(100, time.Minute))
	router.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	// Fill the 10000-client ceiling; the 10001st distinct IP trips it.
	for i := range 10001 {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req.RemoteAddr = fmt.Sprintf("10.0.%d.%d:1", i>>8&0xff, i&0xff)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, 1, strings.Count(buf.String(), "client ceiling reached"), "warns exactly once")
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
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "'sha256-a5WrXwKmbsymAk3URteorxaIUb1TkQMRH7x8FSsnOt8='")
	assert.Contains(t, w.Header().Get("Permissions-Policy"), "interest-cohort=()")
	assert.Empty(t, w.Header().Get("X-XSS-Protection"), "X-XSS-Protection is deprecated and must not be emitted")
}

// TestSecurity_CSPAllowsLazyHighlightScript guards the invariant that lazy-loaded
// highlight.min.js depends on: it's injected at runtime as a same-origin <script>
// (web/static/js/modules/highlighter-loader.js), so CSP must keep script-src 'self'
// and must NOT add 'strict-dynamic' — which would ignore 'self' and trust only
// nonce/hash-loaded scripts (and their descendants), silently breaking syntax
// highlighting on every code page. A future hardening pass adding strict-dynamic must
// revisit the loader (e.g. nonce-propagation), not just delete this test.
func TestSecurity_CSPAllowsLazyHighlightScript(t *testing.T) {
	assert.Contains(t, cspPolicy, "script-src 'self'",
		"lazy-loaded highlight.min.js is a same-origin script; script-src 'self' must stay")
	assert.NotContains(t, cspPolicy, "strict-dynamic",
		"strict-dynamic ignores 'self' and would silently break runtime-injected highlight.min.js")
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
