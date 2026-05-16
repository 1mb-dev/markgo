package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_CreateAndValidate(t *testing.T) {
	store := NewSessionStore()

	token, err := store.Create("admin")
	require.NoError(t, err)
	assert.Len(t, token, 64) // 32 bytes = 64 hex chars

	username, valid := store.Validate(token)
	assert.True(t, valid)
	assert.Equal(t, "admin", username)
}

func TestSessionStore_ValidateEmpty(t *testing.T) {
	store := NewSessionStore()

	_, valid := store.Validate("")
	assert.False(t, valid)
}

func TestSessionStore_ValidateUnknownToken(t *testing.T) {
	store := NewSessionStore()

	_, valid := store.Validate("nonexistent")
	assert.False(t, valid)
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore()

	token, err := store.Create("admin")
	require.NoError(t, err)

	store.Delete(token)

	_, valid := store.Validate(token)
	assert.False(t, valid)
}

func TestSessionStore_Expired(t *testing.T) {
	store := NewSessionStore()

	// Manually store an expired session
	store.sessions.Store("expired-token", &session{
		username:  "admin",
		expiresAt: time.Now().Add(-1 * time.Hour),
	})

	_, valid := store.Validate("expired-token")
	assert.False(t, valid)

	// Confirm it was cleaned up
	_, exists := store.sessions.Load("expired-token")
	assert.False(t, exists)
}

func TestSessionAuth_ValidSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()
	token, _ := store.Create("admin")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: token})

	handler := SessionAuth(store)
	handler(c)

	assert.False(t, c.IsAborted())
	user, exists := c.Get("admin_user")
	assert.True(t, exists)
	assert.Equal(t, "admin", user)
}

func TestSessionAuth_NoSession_GET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin", http.NoBody)

	handler := SessionAuth(store)
	handler(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSessionAuth_NoSession_POST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/cache/clear", http.NoBody)

	handler := SessionAuth(store)
	handler(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSessionAuth_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/compose", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: "bogus"})

	handler := SessionAuth(store)
	handler(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- SessionAware tests ---

func TestSessionAware_ValidSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()
	token, _ := store.Create("admin")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: token})

	handler := SessionAware(store, false)
	handler(c)

	assert.False(t, c.IsAborted())
	user, exists := c.Get("admin_user")
	assert.True(t, exists)
	assert.Equal(t, "admin", user)
	authenticated, exists := c.Get("authenticated")
	assert.True(t, exists)
	assert.Equal(t, true, authenticated)

	// Should NOT generate CSRF token for authenticated users
	_, hasCSRF := c.Get("csrf_token")
	assert.False(t, hasCSRF)
}

func TestSessionAware_NoSession_GET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	handler := SessionAware(store, false)
	handler(c)

	assert.False(t, c.IsAborted())

	// Should NOT set authenticated
	_, exists := c.Get("authenticated")
	assert.False(t, exists)

	// Should generate CSRF token for login popover
	csrfToken, exists := c.Get("csrf_token")
	assert.True(t, exists)
	assert.NotEmpty(t, csrfToken)
}

func TestSessionAware_ReusesExistingCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	// Valid 64-char hex token (32 bytes encoded)
	validToken := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/writing", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_csrf", Value: validToken})

	handler := SessionAware(store, false)
	handler(c)

	assert.False(t, c.IsAborted())

	// Should reuse existing cookie value, not generate a new one
	csrfToken, exists := c.Get("csrf_token")
	assert.True(t, exists)
	assert.Equal(t, validToken, csrfToken)
}

func TestSessionAware_RejectsInvalidCSRFCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/writing", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_csrf", Value: "malformed-not-hex"})

	handler := SessionAware(store, false)
	handler(c)

	assert.False(t, c.IsAborted())

	// Should generate a fresh token instead of reusing the malformed one
	csrfToken, exists := c.Get("csrf_token")
	assert.True(t, exists)
	assert.NotEqual(t, "malformed-not-hex", csrfToken)
	assert.Len(t, csrfToken, 64) // Fresh 32-byte hex token
}

func TestSessionAware_NoSession_POST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/contact", http.NoBody)

	handler := SessionAware(store, false)
	handler(c)

	// Should NOT abort — passes through
	assert.False(t, c.IsAborted())

	// Should NOT generate CSRF token on POST
	_, exists := c.Get("csrf_token")
	assert.False(t, exists)
}

// --- SoftSessionAuth tests ---

func TestSoftSessionAuth_ValidSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()
	token, _ := store.Create("admin")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/compose", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: token})

	handler := SoftSessionAuth(store, false)
	handler(c)

	assert.False(t, c.IsAborted())
	user, exists := c.Get("admin_user")
	assert.True(t, exists)
	assert.Equal(t, "admin", user)
	authenticated, exists := c.Get("authenticated")
	assert.True(t, exists)
	assert.Equal(t, true, authenticated)
	_, authRequired := c.Get("auth_required")
	assert.False(t, authRequired)
}

func TestSoftSessionAuth_NoSession_GET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/compose", http.NoBody)

	handler := SoftSessionAuth(store, false)
	handler(c)

	// Should NOT abort — allows handler to run
	assert.False(t, c.IsAborted())

	// Should set auth_required
	authRequired, exists := c.Get("auth_required")
	assert.True(t, exists)
	assert.Equal(t, true, authRequired)

	// Should generate CSRF token
	csrfToken, exists := c.Get("csrf_token")
	assert.True(t, exists)
	assert.NotEmpty(t, csrfToken)

	// Should NOT set authenticated
	_, exists = c.Get("authenticated")
	assert.False(t, exists)
}

func TestSoftSessionAuth_NoSession_POST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/compose", http.NoBody)

	handler := SoftSessionAuth(store, false)
	handler(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSoftSessionAuth_InvalidToken_GET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: "bogus"})

	handler := SoftSessionAuth(store, false)
	handler(c)

	// Should NOT abort — allows handler to run
	assert.False(t, c.IsAborted())

	// Should set auth_required (stale cookie cleared, treated as unauthenticated)
	authRequired, exists := c.Get("auth_required")
	assert.True(t, exists)
	assert.Equal(t, true, authRequired)
}

func TestSoftSessionAuth_HEAD_Request(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodHead, "/compose", http.NoBody)

	handler := SoftSessionAuth(store, false)
	handler(c)

	// HEAD should behave same as GET — not abort
	assert.False(t, c.IsAborted())
	authRequired, exists := c.Get("auth_required")
	assert.True(t, exists)
	assert.Equal(t, true, authRequired)
}

// --- SoftSessionAuth: JSON-bypass regression tests (GHSA / #42) ---
//
// Before this fix, an unauthenticated GET with `Accept: application/json`
// fell through SoftSessionAuth (auth_required set, c.Next() called) and any
// downstream JSON-branching handler returned admin data with 200. These tests
// pin the corrected behavior: JSON callers get 401, no leaky body keys.
//
// HTML callers continue through the soft-fail path (TestSoftSessionAuth_NoSession_GET).

func TestSoftSessionAuth_NoSession_JSON_GET_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/drafts", http.NoBody)
	c.Request.Header.Set("Accept", "application/json")

	handler := SoftSessionAuth(store, false)
	handler(c)

	assert.True(t, c.IsAborted(), "JSON request without session must abort")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Belt-and-braces: ensure no admin payload leaked into the body.
	// Bypass class is "silent fall-through serves handler data" — any of these
	// keys in the response means the regression is back.
	body := w.Body.String()
	for _, leakedKey := range []string{`"drafts"`, `"articles"`, `"goroutines"`, `"submissions"`, `"draft_count"`} {
		assert.NotContains(t, body, leakedKey, "401 body must not contain leaky admin key %q", leakedKey)
	}

	// auth_required must NOT be set — JSON callers don't get the soft-fail path
	_, authRequired := c.Get("auth_required")
	assert.False(t, authRequired, "JSON callers bypass the soft-fail path")
}

func TestSoftSessionAuth_NoSession_JSON_HEAD_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodHead, "/admin/drafts", http.NoBody)
	c.Request.Header.Set("Accept", "application/json")

	handler := SoftSessionAuth(store, false)
	handler(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSoftSessionAuth_InvalidToken_JSON_GET_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/stats", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: "bogus"})
	c.Request.Header.Set("Accept", "application/json")

	handler := SoftSessionAuth(store, false)
	handler(c)

	// Stale cookie + JSON Accept → same hard fail
	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	body := w.Body.String()
	for _, leakedKey := range []string{`"memory"`, `"uptime"`, `"environment"`} {
		assert.NotContains(t, body, leakedKey)
	}
}

func TestSoftSessionAuth_ValidSession_JSON_GET_Passes(t *testing.T) {
	// Confirms the fix is targeted: authenticated JSON callers are unaffected.
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()
	token, _ := store.Create("admin")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/drafts", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: token})
	c.Request.Header.Set("Accept", "application/json")

	handler := SoftSessionAuth(store, false)
	handler(c)

	assert.False(t, c.IsAborted(), "valid session JSON request must pass through")
	authenticated, _ := c.Get("authenticated")
	assert.Equal(t, true, authenticated)
}

// Accept header variants — proves we're matching loosely enough to catch
// real-world headers (browsers send long Accept lists; SDKs send q-weighted lists).
func TestSoftSessionAuth_NoSession_JSON_AcceptVariants(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		accept     string
		wantAbort  bool
		wantStatus int
	}{
		{"bare application/json", "application/json", true, http.StatusUnauthorized},
		{"with charset", "application/json; charset=utf-8", true, http.StatusUnauthorized},
		{"q-weighted multi", "text/html;q=0.9, application/json;q=1.0", true, http.StatusUnauthorized},
		{"html only", "text/html", false, 0}, // falls through to soft-fail
		{"empty", "", false, 0},              // falls through to soft-fail
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewSessionStore()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/admin/drafts", http.NoBody)
			if tc.accept != "" {
				c.Request.Header.Set("Accept", tc.accept)
			}

			handler := SoftSessionAuth(store, false)
			handler(c)

			if tc.wantAbort {
				assert.True(t, c.IsAborted(), "Accept=%q must abort", tc.accept)
				assert.Equal(t, tc.wantStatus, w.Code)
			} else {
				assert.False(t, c.IsAborted(), "Accept=%q must fall through to soft-fail", tc.accept)
				_, authRequired := c.Get("auth_required")
				assert.True(t, authRequired)
			}
		})
	}
}

// --- isValidCSRFToken tests ---

func TestIsValidCSRFToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"valid 64-char hex", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", true},
		{"valid uppercase hex", "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789", true},
		{"empty string", "", false},
		{"too short", "abcdef", false},
		{"too long", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef01234567890", false},
		{"non-hex chars", "xyz_ef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
		{"contains hyphens", "abcdef01-3456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidCSRFToken(tt.token))
		})
	}
}
