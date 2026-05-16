package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName = "_session"
	sessionTokenBytes = 32
	sessionMaxAge     = 7 * 24 * time.Hour // 7 days
)

type session struct {
	username  string
	expiresAt time.Time
}

// SessionStore manages in-memory sessions for admin authentication.
type SessionStore struct {
	sessions sync.Map
}

// NewSessionStore creates a new session store and starts background cleanup.
func NewSessionStore() *SessionStore {
	s := &SessionStore{}

	// Background cleanup of expired sessions (prevents unbounded memory growth)
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.sessions.Range(func(key, value any) bool {
				if sess, ok := value.(*session); ok {
					if time.Now().After(sess.expiresAt) {
						s.sessions.Delete(key)
					}
				}
				return true
			})
		}
	}()

	return s
}

// Create generates a new session token and stores it.
func (s *SessionStore) Create(username string) (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	s.sessions.Store(token, &session{
		username:  username,
		expiresAt: time.Now().Add(sessionMaxAge),
	})
	return token, nil
}

// Validate checks if a token is valid and not expired.
func (s *SessionStore) Validate(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	val, ok := s.sessions.Load(token)
	if !ok {
		return "", false
	}
	sess, ok := val.(*session)
	if !ok {
		return "", false
	}
	if time.Now().After(sess.expiresAt) {
		s.sessions.Delete(token)
		return "", false
	}
	return sess.username, true
}

// Delete removes a session.
func (s *SessionStore) Delete(token string) {
	s.sessions.Delete(token)
}

// SessionAware checks for a valid session cookie and sets authenticated=true
// in the gin context if found. For unauthenticated GET/HEAD requests, generates
// a CSRF token so the login popover can render on public pages.
// Never blocks — passes through regardless.
func SessionAware(store *SessionStore, secureCookie bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err == nil {
			if username, valid := store.Validate(token); valid {
				c.Set("admin_user", username)
				c.Set("authenticated", true)
				c.Next()
				return
			}
		}
		// Not authenticated — ensure CSRF token exists for login popover.
		// Reuse existing cookie to avoid SPA desync (fetch responses overwrite
		// the cookie, but the login popover hidden input keeps the old value).
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			if existing, cookieErr := c.Cookie(csrfCookieName); cookieErr == nil && isValidCSRFToken(existing) {
				c.Set("csrf_token", existing)
				// Refresh cookie max-age so it doesn't silently expire while user is browsing
				c.SetSameSite(http.SameSiteStrictMode)
				c.SetCookie(csrfCookieName, existing, 3600, "", "", secureCookie, true)
			} else {
				GenerateCSRFToken(c, secureCookie)
			}
		}
		c.Next()
	}
}

// SessionAuth provides session-based authentication middleware.
// Returns 401 for all unauthenticated requests.
func SessionAuth(store *SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err == nil {
			if username, valid := store.Validate(token); valid {
				c.Set("admin_user", username)
				c.Next()
				return
			}
			// Stale/invalid cookie — clear it to prevent repeated failed lookups
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
		}

		// Not authenticated — return 401 for all methods.
		// Debug/API routes use this middleware; there is no dedicated login page to redirect to.
		slog.Warn("Unauthenticated request", "method", c.Request.Method, "path", c.Request.URL.Path)
		abortWithError(c, http.StatusUnauthorized, "Authentication required")
	}
}

// SoftSessionAuth provides session-based authentication that allows handlers to run
// even when not authenticated. On valid session: sets admin_user + authenticated=true.
// On invalid GET: sets auth_required=true and generates a CSRF token so the login
// popover can render. On invalid POST: returns 401 JSON.
func SoftSessionAuth(store *SessionStore, secureCookie bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err == nil {
			if username, valid := store.Validate(token); valid {
				c.Set("admin_user", username)
				c.Set("authenticated", true)
				c.Next()
				return
			}
			// Stale/invalid cookie — clear it
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie(sessionCookieName, "", -1, "/", "", secureCookie, true)
		}

		// Not authenticated.
		//
		// Asymmetric handling on GET/HEAD is intentional:
		//   HTML callers → soft-fail with the login overlay (auth_required flag),
		//                  preserving SPA UX where the same URL serves owner + reader.
		//   JSON callers → hard 401, because no overlay path applies and silent
		//                  fall-through would leak handler data (see GHSA / #42).
		// Do NOT collapse this asymmetry without revisiting the bypass class.
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			if wantsJSON(c) {
				slog.Warn("Unauthenticated JSON request", "method", c.Request.Method, "path", c.Request.URL.Path)
				abortWithError(c, http.StatusUnauthorized, "Authentication required")
				return
			}
			c.Set("auth_required", true)
			// Generate CSRF token for the login popover form
			GenerateCSRFToken(c, secureCookie)
			if c.IsAborted() {
				return
			}
			c.Next()
			return
		}

		slog.Warn("Unauthenticated non-GET request", "method", c.Request.Method, "path", c.Request.URL.Path)
		abortWithError(c, http.StatusUnauthorized, "Authentication required")
	}
}

// GenerateCSRFToken creates a CSRF token, sets it as a cookie, and stores it in gin context.
// Aborts with 500 if token generation fails (crypto/rand failure is a system emergency).
func GenerateCSRFToken(c *gin.Context, secureCookie bool) {
	token := generateCSRFToken()
	if token == "" {
		slog.Error("CSRF token generation failed — aborting request")
		abortWithError(c, http.StatusInternalServerError, "Internal server error")
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(csrfCookieName, token, 3600, "", "", secureCookie, true)
	c.Set("csrf_token", token)
}

// isValidCSRFToken checks that a CSRF token has the expected format (64 hex chars = 32 bytes).
// Rejects corrupted, truncated, or injected cookie values.
func isValidCSRFToken(token string) bool {
	if len(token) != csrfTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}
