package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestDiscardBodyOnHEAD_PassthroughGET — GET requests unaffected.
func TestDiscardBodyOnHEAD_PassthroughGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(DiscardBodyOnHEAD())
	r.GET("/probe", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ok":true`, "GET body must be present")
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
}

// TestDiscardBodyOnHEAD_OmitsBody — HEAD returns status+headers, no body.
func TestDiscardBodyOnHEAD_OmitsBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(DiscardBodyOnHEAD())
	handler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	r.GET("/probe", handler)
	r.HEAD("/probe", handler)

	req := httptest.NewRequest(http.MethodHead, "/probe", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "HEAD status must mirror GET")
	assert.Empty(t, w.Body.String(), "HEAD body must be discarded")
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"),
		"HEAD must propagate Content-Type from handler")
}

// TestDiscardBodyOnHEAD_PropagatesNonOKStatus — HEAD reflects actual handler status.
func TestDiscardBodyOnHEAD_PropagatesNonOKStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(DiscardBodyOnHEAD())
	handler := func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
	}
	r.GET("/probe", handler)
	r.HEAD("/probe", handler)

	req := httptest.NewRequest(http.MethodHead, "/probe", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, w.Body.String())
}

// TestDiscardBodyOnHEAD_RouteNotRegistered — HEAD on a GET-only route still
// returns 404 because gin's router is method-strict. Documents the contract:
// the middleware does not auto-route HEAD; route definitions must include it
// (use the registerGET helper in serve/command.go).
func TestDiscardBodyOnHEAD_RouteNotRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(DiscardBodyOnHEAD())
	r.GET("/get-only", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodHead, "/get-only", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"HEAD must 404 when route registered as GET-only — register HEAD explicitly via registerGET")
}
