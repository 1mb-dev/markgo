package handlers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/middleware"
)

const defaultRedirect = "/admin"

// maxLoginBodySize caps the login form body before parsing. Credentials are a
// handful of short fields, so 64KB is ample while bounding the body an
// unauthenticated caller can force us to read into memory.
const maxLoginBodySize = 64 << 10 // 64KB

// AuthHandler handles login and logout.
type AuthHandler struct {
	*BaseHandler
	username     string
	password     string
	sessionStore *middleware.SessionStore
	secureCookie bool
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(
	base *BaseHandler,
	username, password string,
	store *middleware.SessionStore,
	secureCookie bool,
) *AuthHandler {
	return &AuthHandler{
		BaseHandler:  base,
		username:     username,
		password:     password,
		sessionStore: store,
		secureCookie: secureCookie,
	}
}

// HandleLogin validates credentials and creates a session.
// Returns JSON when Accept: application/json is set (popover fetch),
// otherwise falls back to HTML redirect (graceful degradation).
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxLoginBodySize)
	wantJSON := strings.Contains(c.GetHeader("Accept"), "application/json")

	// Parse the bounded body up front: an oversized form must fail loudly, not
	// silently yield empty credentials and a misleading "invalid password".
	if err := c.Request.ParseForm(); err != nil {
		if wantJSON {
			code, msg := http.StatusBadRequest, "Invalid request."
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				code, msg = http.StatusRequestEntityTooLarge, "Request too large."
			}
			c.JSON(code, gin.H{"success": false, "error": msg})
			return
		}
		c.Redirect(http.StatusFound, defaultRedirect)
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")
	next := sanitizeNext(c.DefaultPostForm("next", defaultRedirect))

	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(h.username)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(h.password)) == 1

	if !usernameMatch || !passwordMatch {
		h.logger.Warn("Failed login attempt", "username", username)

		if wantJSON {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid username or password.",
			})
			return
		}

		// HTML fallback — redirect back with error (no login page to render)
		c.Redirect(http.StatusFound, next)
		return
	}

	token, err := h.sessionStore.Create(username)
	if err != nil {
		h.logger.Error("Session creation failed", "error", err)

		if wantJSON {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Unable to create session. Please try again.",
			})
			return
		}

		c.Redirect(http.StatusFound, next)
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("_session", token, 604800, "/", "", h.secureCookie, true)

	if wantJSON {
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"redirect": next,
		})
		return
	}

	c.Redirect(http.StatusFound, next)
}

// HandleLogout clears the session and redirects to /.
// Uses GET for simplicity — acceptable trade-off since logout has no destructive side effects
// and SameSite=Strict cookies prevent cross-site CSRF for same-site requests.
func (h *AuthHandler) HandleLogout(c *gin.Context) {
	if token, err := c.Cookie("_session"); err == nil {
		h.sessionStore.Delete(token)
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("_session", "", -1, "/", "", h.secureCookie, true)

	c.Redirect(http.StatusFound, "/")
}

// sanitizeNext validates the redirect target to prevent open redirects.
// Only allows relative paths starting with "/". Rejects protocol-relative URLs,
// absolute URLs, and anything with a scheme.
func sanitizeNext(next string) string {
	if next == "" || next[0] != '/' || strings.HasPrefix(next, "//") {
		return defaultRedirect
	}
	// Reject URLs with scheme (e.g., "/login?next=javascript:..." is blocked by prefix check,
	// but also guard against encoded forms)
	if strings.Contains(next, "://") {
		return defaultRedirect
	}
	return next
}
