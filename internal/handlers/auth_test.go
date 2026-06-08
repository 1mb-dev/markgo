package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/middleware"
)

func TestSanitizeNext(t *testing.T) {
	tests := []struct {
		name string
		next string
		want string
	}{
		{"valid relative path", "/compose", "/compose"},
		{"valid admin path", "/admin", "/admin"},
		{"valid path with query", "/admin?tab=stats", "/admin?tab=stats"},
		{"empty string", "", "/admin"},
		{"absolute URL", "https://evil.com", "/admin"},
		{"protocol-relative", "//evil.com", "/admin"},
		{"no leading slash", "evil.com", "/admin"},
		{"scheme in path", "/foo://bar", "/admin"},
		{"javascript scheme", "javascript:alert(1)", "/admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeNext(tt.next)
			if got != tt.want {
				t.Errorf("sanitizeNext(%q) = %q, want %q", tt.next, got, tt.want)
			}
		})
	}
}

func newTestAuthHandler() *AuthHandler {
	cfg := &config.Config{}
	logger := slog.Default()
	base := &BaseHandler{config: cfg, logger: logger}
	store := middleware.NewSessionStore()
	return NewAuthHandler(base, "admin", "secret", store, false)
}

// TestHandleLogin_RateLimited mirrors the prod wiring (RateLimit before the
// login handler) and asserts the burst-counter behavior: the first N invalid
// attempts reach the handler (401), the (N+1)th is throttled (429) within a
// coarse window. No time.Sleep — the limiter counts within the window. (#2)
func TestHandleLogin_RateLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestAuthHandler()

	const limit = 3
	router := gin.New()
	loginGroup := router.Group("/login")
	loginGroup.Use(middleware.RateLimit(limit, time.Minute))
	loginGroup.POST("", h.HandleLogin)

	post := func() int {
		form := url.Values{"username": {"admin"}, "password": {"wrong"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json") // get a 401, not the HTML 302
		req.RemoteAddr = "203.0.113.7:5555"          // same client → one bucket
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	for i := range limit {
		assert.Equal(t, http.StatusUnauthorized, post(), "attempt %d should reach the handler", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, post(), "attempt beyond the limit is throttled")
}

func TestHandleLogin_OversizedBody_Returns413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	form := url.Values{
		"username": {"admin"},
		"password": {strings.Repeat("a", 70000)}, // body exceeds the 64KB cap
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request.Header.Set("Accept", "application/json")

	h.HandleLogin(c)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestHandleLogin_JSON_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	form := url.Values{
		"username": {"admin"},
		"password": {"secret"},
		"next":     {"/compose"},
		"_csrf":    {"token"},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request.Header.Set("Accept", "application/json")

	h.HandleLogin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, true, resp["success"])
	assert.Equal(t, "/compose", resp["redirect"])
}

func TestHandleLogin_JSON_InvalidCreds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	form := url.Values{
		"username": {"admin"},
		"password": {"wrong"},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request.Header.Set("Accept", "application/json")

	h.HandleLogin(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, false, resp["success"])
	assert.Contains(t, resp["error"], "Invalid")
}

func TestHandleLogin_HTML_Redirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestAuthHandler()

	router := gin.New()
	router.POST("/login", h.HandleLogin)

	form := url.Values{
		"username": {"admin"},
		"password": {"secret"},
		"next":     {"/admin"},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No Accept: application/json — should redirect

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/admin", w.Header().Get("Location"))
}

func TestHandleLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestAuthHandler()

	// Create a session first
	token, err := h.sessionStore.Create("admin")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/logout", http.NoBody)
	c.Request.AddCookie(&http.Cookie{Name: "_session", Value: token})

	h.HandleLogout(c)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/", w.Header().Get("Location"))

	// Session should be deleted
	_, valid := h.sessionStore.Validate(token)
	assert.False(t, valid)
}
