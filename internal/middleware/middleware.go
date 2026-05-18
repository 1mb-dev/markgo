// Package middleware provides HTTP middleware for the MarkGo blog engine.
// It includes security, logging, CORS, rate limiting, and request tracking middleware.
package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/config"
	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

// foucScriptHash is the SHA-256 (base64) of the inline FOUC-prevention script in
// web/templates/base.html. Required by the CSP script-src directive because the
// script reads localStorage before stylesheets load — it cannot move to an
// external file without reintroducing the flash. The hash is locked by
// TestSecurity_FOUCScriptHashMatches; editing the inline script without
// updating this constant fails the test before reaching users' browsers.
const foucScriptHash = "sha256-0pz7XU3iscvI1rWHhJ8OyLJ4xXNoivNIt1N5xpF6GUg="

// cspPolicy is the Content-Security-Policy emitted by Security(). Keep
// directives alphabetically ordered to make diffs reviewable.
//
// connect-src 'self' covers same-origin fetches: search, compose, AMA submit,
// offline-queue replay on reconnect, contact form, login. Adding analytics or
// a third-party error reporter requires extending this directive.
//
// img-src includes https: because article banner fields accept absolute URLs
// to externally-hosted images (operator's choice, see compose banner-path
// forms). data: covers favicons and inline preview thumbnails.
var cspPolicy = strings.Join([]string{
	"default-src 'self'",
	"base-uri 'self'",
	"connect-src 'self'",
	"font-src 'self'",
	"form-action 'self'",
	"frame-ancestors 'none'",
	"img-src 'self' data: https:",
	"object-src 'none'",
	"script-src 'self' '" + foucScriptHash + "'",
	"style-src 'self'",
}, "; ")

// permissionsPolicy denies access to powerful features the app does not use,
// including FLoC opt-out (interest-cohort).
const permissionsPolicy = "camera=(), microphone=(), geolocation=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=(), interest-cohort=()"

// hstsValue ships without preload — preload-list registration is irreversible
// without months of pain. Opt-in via v3.17+ after one cycle of observed stability.
const hstsValue = "max-age=31536000; includeSubDomains"

// Security adds security headers: X-Content-Type-Options, X-Frame-Options,
// Referrer-Policy, Strict-Transport-Security, Content-Security-Policy,
// Permissions-Policy. CSP can be disabled via the CSP_DISABLE env var for
// operators whose edge proxy emits its own policy.
func Security(cfg *config.Config) gin.HandlerFunc {
	cspEnabled := cfg == nil || !cfg.Security.CSPDisable
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Strict-Transport-Security", hstsValue)
		c.Header("Permissions-Policy", permissionsPolicy)
		if cspEnabled {
			c.Header("Content-Security-Policy", cspPolicy)
		}
		c.Next()
	}
}

// Performance logs request timing and basic metrics
func Performance(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)

		// Log slow requests (over 1 second)
		if duration > time.Second {
			logger.Warn("Slow request",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"duration", duration,
				"status", c.Writer.Status(),
			)
		}

		// Add timing header
		c.Header("X-Response-Time", duration.String())
	}
}

// CORS handles cross-origin requests with secure origin validation
func CORS(allowedOrigins []string, isDevelopment bool) gin.HandlerFunc {
	// Build a map of allowed origins for O(1) lookup
	allowedMap := make(map[string]bool)
	for _, origin := range allowedOrigins {
		allowedMap[origin] = true
	}

	// In development, add localhost variants explicitly
	if isDevelopment {
		allowedMap["http://localhost:3000"] = true
		allowedMap["http://127.0.0.1:3000"] = true
		allowedMap["http://localhost:3001"] = true
		allowedMap["http://127.0.0.1:3001"] = true
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Only allow explicitly configured origins (exact match - no substring)
		if origin != "" && allowedMap[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin") // Important for caching
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID, X-CSRF-Token")
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// rateLimiterRegistry tracks stop channels for all rate-limiter cleanup
// goroutines so they can be terminated together at graceful-shutdown time.
var (
	rateLimiterRegistry   []chan struct{}
	rateLimiterRegistryMu sync.Mutex
	rateLimiterRegistryWG sync.WaitGroup
)

// runRateLimitCleanup periodically prunes expired entries from a rate
// limiter's client map. Exits when stop is closed.
func runRateLimitCleanup(stop <-chan struct{}, mu *sync.RWMutex, clients map[string][]time.Time, window time.Duration) {
	defer rateLimiterRegistryWG.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			pruneRateLimitClients(mu, clients, window)
		}
	}
}

// pruneRateLimitClients drops timestamps older than window and removes
// clients whose history is fully expired.
func pruneRateLimitClients(mu *sync.RWMutex, clients map[string][]time.Time, window time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	for ip, times := range clients {
		var validTimes []time.Time
		for _, t := range times {
			if now.Sub(t) <= window {
				validTimes = append(validTimes, t)
			}
		}
		if len(validTimes) == 0 {
			delete(clients, ip)
		} else {
			clients[ip] = validTimes
		}
	}
}

// ShutdownRateLimiters stops the cleanup goroutines for every RateLimit
// instance created during the process lifetime and waits for them to exit.
// Safe to call multiple times.
func ShutdownRateLimiters() {
	rateLimiterRegistryMu.Lock()
	stops := rateLimiterRegistry
	rateLimiterRegistry = nil
	rateLimiterRegistryMu.Unlock()
	for _, stop := range stops {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
	rateLimiterRegistryWG.Wait()
}

// RateLimit provides sliding window rate limiting with bounded memory
func RateLimit(requests int, window time.Duration) gin.HandlerFunc {
	const maxClients = 10000 // Prevent memory exhaustion attacks

	clients := make(map[string][]time.Time)
	var mu sync.RWMutex

	stop := make(chan struct{})
	rateLimiterRegistryMu.Lock()
	rateLimiterRegistry = append(rateLimiterRegistry, stop)
	rateLimiterRegistryMu.Unlock()

	// Background cleanup goroutine to prevent unbounded growth. Exits on
	// ShutdownRateLimiters() so rolling restarts don't leak goroutines.
	rateLimiterRegistryWG.Add(1)
	go runRateLimitCleanup(stop, &mu, clients, window)

	return func(c *gin.Context) {
		// Skip rate limiting for static assets — a single page load
		// requests 20+ static files; counting them exhausts the budget.
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/static/") || path == "/favicon.ico" || path == "/sw.js" {
			c.Next()
			return
		}

		// Use RemoteAddr for security (ClientIP can be spoofed via X-Forwarded-For)
		ip := c.Request.RemoteAddr
		// Strip port from RemoteAddr (format is "IP:port")
		if idx := len(ip) - 1; idx >= 0 {
			for i := idx; i >= 0; i-- {
				if ip[i] == ':' {
					ip = ip[:i]
					break
				}
			}
		}

		now := time.Now()

		mu.Lock()
		defer mu.Unlock()

		// Prevent memory exhaustion: reject if too many unique IPs
		if len(clients) >= maxClients && clients[ip] == nil {
			c.Header("Retry-After", "3600")
			abortWithError(c, http.StatusTooManyRequests, "Too many requests — please wait")
			return
		}

		// Clean old entries for this IP
		if times, exists := clients[ip]; exists {
			var validTimes []time.Time
			for _, t := range times {
				if now.Sub(t) <= window {
					validTimes = append(validTimes, t)
				}
			}
			clients[ip] = validTimes
		}

		// Check rate limit
		if len(clients[ip]) >= requests {
			retryAfter := int(window.Seconds())
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			abortWithError(c, http.StatusTooManyRequests, "Too many requests — please wait")
			return
		}

		// Add current request
		if clients[ip] == nil {
			clients[ip] = make([]time.Time, 0, requests)
		}
		clients[ip] = append(clients[ip], now)
		c.Next()
	}
}

// generateRequestID generates a simple request ID
func generateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		slog.Warn("Request ID generation failed, using timestamp fallback", "error", err)
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(bytes)
}

// Logger provides basic request logging.
// Static asset requests are demoted to debug level to reduce log noise.
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		if strings.HasPrefix(param.Path, "/static/") || param.Path == "/favicon.ico" {
			logger.Debug("Request",
				"method", param.Method,
				"path", param.Path,
				"status", param.StatusCode,
				"duration", param.Latency,
			)
			return ""
		}
		logger.Info("Request",
			"method", param.Method,
			"path", param.Path,
			"status", param.StatusCode,
			"duration", param.Latency,
		)
		return ""
	})
}

// SmartCacheHeaders adds basic cache headers
func SmartCacheHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=3600")
		c.Next()
	}
}

// RequestTracker adds request tracking
func RequestTracker() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := generateRequestID()
		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)
		c.Next()
	}
}

// NoCache adds no-cache headers
func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

// RecoveryWithErrorHandler provides recovery with error handling for all panic types
func RecoveryWithErrorHandler(logger *slog.Logger) gin.HandlerFunc {
	return gin.RecoveryWithWriter(gin.DefaultWriter, func(c *gin.Context, recovered any) {
		switch v := recovered.(type) {
		case string:
			logger.Error("Panic recovered", "error", v)
		case error:
			logger.Error("Panic recovered", "error", v.Error())
		default:
			logger.Error("Panic recovered", "error", fmt.Sprintf("%v", v))
		}
		if !c.Writer.Written() {
			c.Data(http.StatusInternalServerError, "text/html; charset=utf-8",
				[]byte(apperrors.FallbackErrorHTML))
		}
		c.Abort()
	})
}

// ErrorHandler provides centralized error handling
func ErrorHandler(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			logger.Error("Request error", "errors", c.Errors.String())
		}
	}
}

const (
	csrfCookieName = "_csrf"
	csrfFormField  = "_csrf"
	csrfHeaderName = "X-CSRF-Token"
	csrfTokenBytes = 32
)

// CSRF implements double-submit cookie CSRF protection.
// On GET/HEAD: generates a token, sets it as an HttpOnly cookie, and stores it in gin context as "csrf_token".
// On other methods (POST, PUT, DELETE): verifies the form field or X-CSRF-Token header matches the cookie value.
// secureCookie controls the Secure flag — set false for localhost/HTTP development.
func CSRF(secureCookie bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("csrf_secure", secureCookie)

		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			// Skip if SoftSessionAuth already set a valid CSRF token
			if token, exists := c.Get("csrf_token"); exists {
				if s, ok := token.(string); ok && s != "" {
					c.Next()
					return
				}
			}
			token := generateCSRFToken()
			if token == "" {
				abortWithError(c, http.StatusInternalServerError, "Internal server error")
				return
			}
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie(csrfCookieName, token, 3600, "", "", secureCookie, true)
			c.Set("csrf_token", token)
			c.Next()
			return
		}

		cookieToken, err := c.Cookie(csrfCookieName)
		if err != nil || cookieToken == "" {
			abortWithError(c, http.StatusForbidden, "Session expired — please reload")
			return
		}

		// Check form field first (HTML forms), then header (JSON API requests)
		submittedToken := c.PostForm(csrfFormField)
		if submittedToken == "" {
			submittedToken = c.GetHeader(csrfHeaderName)
		}
		if submittedToken == "" || subtle.ConstantTimeCompare([]byte(submittedToken), []byte(cookieToken)) != 1 {
			abortWithError(c, http.StatusForbidden, "Request validation failed — please retry")
			return
		}

		c.Next()
	}
}

func generateCSRFToken() string {
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		slog.Error("CSRF token generation failed", "error", err)
		return ""
	}
	return hex.EncodeToString(b)
}
