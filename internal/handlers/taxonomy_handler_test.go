package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/1mb-dev/markgo/internal/models"
)

// tagCapture records the tag/category string the handler passes to the service,
// so we can assert the route param is decoded exactly once.
type tagCapture struct {
	MockArticleService
	gotTag      string
	gotCategory string
}

func (m *tagCapture) GetArticlesByTag(tag string) []*models.Article {
	m.gotTag = tag
	return nil
}

func (m *tagCapture) GetArticlesByCategory(category string) []*models.Article {
	m.gotCategory = category
	return nil
}

func newTaxonomyTestHandler(svc *tagCapture) *TaxonomyHandler {
	cfg := createTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})
	return NewTaxonomyHandler(base, svc)
}

// TestArticlesByTag_PlusInSlugResolves guards the decode fix: gin already
// URL-decodes path params, so a tag containing '+' (e.g. the sitemap/canonical
// URL "/tags/c++") must resolve to "c++" — a second QueryUnescape would turn the
// '+' into a space and the page would match nothing.
func TestArticlesByTag_PlusInSlugResolves(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &tagCapture{}
	h := newTaxonomyTestHandler(capture)

	router := gin.New()
	router.GET("/tags/:tag", h.ArticlesByTag)

	req := httptest.NewRequest("GET", "/tags/c++", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "c++", capture.gotTag, "tag with '+' must resolve to c++, not 'c  '")
}

// TestArticlesByTag_LowercasesParam: tags are lowercased at load, so a request
// to a capitalized URL (e.g. an old indexed /tags/Go) must resolve to the
// lowercase canonical term, not a separate "Go".
func TestArticlesByTag_LowercasesParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &tagCapture{}
	h := newTaxonomyTestHandler(capture)

	router := gin.New()
	router.GET("/tags/:tag", h.ArticlesByTag)

	req := httptest.NewRequest("GET", "/tags/Go", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "go", capture.gotTag, "capitalized URL must resolve to the lowercase canonical term")
}

func TestArticlesByCategory_PlusInSlugResolves(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &tagCapture{}
	h := newTaxonomyTestHandler(capture)

	router := gin.New()
	router.GET("/categories/:category", h.ArticlesByCategory)

	req := httptest.NewRequest("GET", "/categories/c++", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "c++", capture.gotCategory, "category with '+' must resolve to c++, not 'c  '")
}
