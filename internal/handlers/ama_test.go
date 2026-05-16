package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/middleware"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services/compose"
)

// AMAArticleService returns canned drafts for AMA handler tests.
type AMAArticleService struct {
	MockArticleService
	Drafts []*models.Article
}

func (m *AMAArticleService) GetDraftArticles() []*models.Article { return m.Drafts }

func createTestAMAHandler(t *testing.T) (*AMAHandler, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := createTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

	composeService := compose.NewService(dir, "Test Author")
	articleService := &AMAArticleService{}

	handler := NewAMAHandler(base, composeService, articleService)
	return handler, dir
}

func TestAMASubmit(t *testing.T) {
	t.Run("valid submission creates file", func(t *testing.T) {
		handler, _ := createTestAMAHandler(t)

		router := gin.New()
		router.POST("/ama/submit", handler.Submit)

		body, _ := json.Marshal(map[string]string{
			"name":     "Alice",
			"email":    "alice@example.com",
			"question": "What is your favorite programming language and why do you prefer it?",
		})
		req := httptest.NewRequest("POST", "/ama/submit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "success", resp["status"])
	})

	t.Run("honeypot triggers silent success", func(t *testing.T) {
		handler, dir := createTestAMAHandler(t)

		router := gin.New()
		router.POST("/ama/submit", handler.Submit)

		body, _ := json.Marshal(map[string]string{
			"name":     "Bot",
			"question": "This is definitely a real question from a human being, trust me.",
			"website":  "http://spam.example.com",
		})
		req := httptest.NewRequest("POST", "/ama/submit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Returns success (to not alert the bot) but no file created
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "success", resp["status"])

		// Verify no file was created
		entries, _ := readDir(dir)
		assert.Empty(t, entries)
	})

	t.Run("question too short returns 400", func(t *testing.T) {
		handler, _ := createTestAMAHandler(t)

		router := gin.New()
		router.POST("/ama/submit", handler.Submit)

		body, _ := json.Marshal(map[string]string{
			"name":     "Alice",
			"question": "Too short",
		})
		req := httptest.NewRequest("POST", "/ama/submit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		handler, _ := createTestAMAHandler(t)

		router := gin.New()
		router.POST("/ama/submit", handler.Submit)

		body, _ := json.Marshal(map[string]string{
			"email": "alice@example.com",
		})
		req := httptest.NewRequest("POST", "/ama/submit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAMAListPending(t *testing.T) {
	t.Run("returns pending AMAs as JSON", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		svc := &AMAArticleService{
			Drafts: []*models.Article{
				{Slug: "ama-1", Type: "ama", Content: "Question 1?", Asker: "Alice", Draft: true, Date: time.Now()},
				{Slug: "ama-2", Type: "ama", Content: "Question 2?", Asker: "Bob", Draft: true, Date: time.Now()},
				{Slug: "regular-draft", Type: "article", Title: "WIP Article", Draft: true, Date: time.Now()},
			},
		}

		composeService := compose.NewService(t.TempDir(), "Test Author")
		handler := NewAMAHandler(base, composeService, svc)

		router := gin.New()
		router.GET("/admin/ama", handler.ListPending)

		req := httptest.NewRequest("GET", "/admin/ama", http.NoBody)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		// Only AMA drafts, not the regular article draft
		assert.Equal(t, float64(2), resp["pending_count"])
	})
}

func TestAMAAnswer(t *testing.T) {
	t.Run("publishes answer", func(t *testing.T) {
		dir := t.TempDir()
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		composeService := compose.NewService(dir, "Test Author")
		articleService := &AMAArticleService{}

		// Create an AMA post first
		slug, err := composeService.CreatePost(&compose.Input{
			Content:    "What is your favorite language?",
			Draft:      true,
			Asker:      "Alice",
			AskerEmail: "alice@example.com",
			Type:       "ama",
		})
		require.NoError(t, err)

		handler := NewAMAHandler(base, composeService, articleService)

		router := gin.New()
		router.POST("/admin/ama/:slug/answer", handler.Answer)

		body, _ := json.Marshal(map[string]string{
			"answer": "Go, because it's simple and powerful.",
		})
		req := httptest.NewRequest("POST", "/admin/ama/"+slug+"/answer", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "success", resp["status"])

		// Verify the article was updated (draft=false, answer appended)
		input, err := composeService.LoadArticle(slug)
		require.NoError(t, err)
		assert.False(t, input.Draft)
		assert.Contains(t, input.Content, "Go, because it's simple and powerful.")
	})
}

func TestAMAAnswer_NotFound(t *testing.T) {
	handler, _ := createTestAMAHandler(t)

	router := gin.New()
	router.POST("/admin/ama/:slug/answer", handler.Answer)

	body, _ := json.Marshal(map[string]string{
		"answer": "This question does not exist.",
	})
	req := httptest.NewRequest("POST", "/admin/ama/nonexistent-slug/answer", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusInternalServerError)
}

func TestAMAAnswer_InvalidSlug(t *testing.T) {
	handler, _ := createTestAMAHandler(t)

	router := gin.New()
	router.POST("/admin/ama/:slug/answer", handler.Answer)

	body, _ := json.Marshal(map[string]string{
		"answer": "My answer.",
	})
	req := httptest.NewRequest("POST", "/admin/ama/INVALID-UPPERCASE/answer", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAMADelete_NotFound(t *testing.T) {
	handler, _ := createTestAMAHandler(t)

	router := gin.New()
	router.POST("/admin/ama/:slug/delete", handler.Delete)

	req := httptest.NewRequest("POST", "/admin/ama/nonexistent-slug/delete", http.NoBody)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusInternalServerError)
}

func TestAMAListPending_Empty(t *testing.T) {
	cfg := createTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

	svc := &AMAArticleService{
		Drafts: []*models.Article{},
	}

	composeService := compose.NewService(t.TempDir(), "Test Author")
	handler := NewAMAHandler(base, composeService, svc)

	router := gin.New()
	router.GET("/admin/ama", handler.ListPending)

	req := httptest.NewRequest("GET", "/admin/ama", http.NoBody)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["pending_count"])
}

func TestAMADelete(t *testing.T) {
	t.Run("removes file", func(t *testing.T) {
		dir := t.TempDir()
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		composeService := compose.NewService(dir, "Test Author")
		articleService := &AMAArticleService{}

		slug, err := composeService.CreatePost(&compose.Input{
			Content: "What is your favorite color?",
			Draft:   true,
			Asker:   "Bob",
			Type:    "ama",
		})
		require.NoError(t, err)

		handler := NewAMAHandler(base, composeService, articleService)

		router := gin.New()
		router.POST("/admin/ama/:slug/delete", handler.Delete)

		req := httptest.NewRequest("POST", "/admin/ama/"+slug+"/delete", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify file is gone
		entries, _ := readDir(dir)
		assert.Empty(t, entries)
	})
}

// ---------------------------------------------------------------------------
// Happy-path E2E: public submit → admin moderates with auth → file holds answer
//
// Pause-notes "Next Session #2". Exercises the AMA flow across the auth
// boundary in one pass: submission is public, moderation requires a session
// cookie. Asserts the moderation chain rejects unauthenticated calls AND
// succeeds with a valid session. File-system side effects verified by
// re-reading the article file.
// ---------------------------------------------------------------------------

func TestAMA_HappyPath_PublicSubmitAdminAnswer(t *testing.T) {
	handler, dir := createTestAMAHandler(t)

	// Real session store; admin session minted directly (login form has its
	// own tests — this exercises the AMA chain, not the login UI).
	store := middleware.NewSessionStore()
	sessionToken, err := store.Create("admin")
	require.NoError(t, err)

	router := gin.New()
	router.POST("/ama/submit", handler.Submit)
	adminGroup := router.Group("/admin", middleware.SoftSessionAuth(store, false))
	adminGroup.POST("/ama/:slug/answer", handler.Answer)

	// 1. Public submission — no auth, no cookie
	question := "What is the value of a small, focused engineering practice?"
	body, _ := json.Marshal(map[string]string{
		"name":     "Alice",
		"email":    "alice@example.com",
		"question": question,
	})
	req := httptest.NewRequest(http.MethodPost, "/ama/submit", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "public submit must succeed")

	// 2. Discover the slug from the tempdir — Submit writes a file.
	// Filename is "YYYY-MM-DD-<slug>.md"; codebase strips the date prefix
	// when resolving URL slugs (see compose.slugFromFilename).
	entries, err := readDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "submit must create exactly one file")
	fileSlug := strings.TrimSuffix(entries[0], ".md")
	require.Greater(t, len(fileSlug), 11, "filename must have date prefix")
	slug := fileSlug[11:] // strip "YYYY-MM-DD-"
	require.NotEmpty(t, slug)

	// Verify the submitted question is in the file
	original, err := os.ReadFile(filepath.Join(dir, entries[0]))
	require.NoError(t, err)
	require.Contains(t, string(original), question, "file must contain submitted question")

	// 3. Unauthenticated moderation must be rejected — regression net for #42-class issues
	answerBody, _ := json.Marshal(map[string]string{"answer": "A useful answer."})
	req = httptest.NewRequest(http.MethodPost, "/admin/ama/"+slug+"/answer", bytes.NewBuffer(answerBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code, "moderation without session must be 401")

	// 4. Authenticated moderation succeeds
	answer := "A reliable answer beats a clever one."
	answerBody, _ = json.Marshal(map[string]string{"answer": answer})
	req = httptest.NewRequest(http.MethodPost, "/admin/ama/"+slug+"/answer", bytes.NewBuffer(answerBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "_session", Value: sessionToken})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "authenticated moderation must succeed (body=%s)", w.Body.String())

	// 5. The file on disk now contains both question and answer, and is no longer a draft
	updated, err := os.ReadFile(filepath.Join(dir, entries[0]))
	require.NoError(t, err)
	updatedStr := string(updated)
	assert.Contains(t, updatedStr, question, "original question preserved")
	assert.Contains(t, updatedStr, answer, "answer written to file")
	assert.Contains(t, updatedStr, "draft: false", "draft flag flipped to false on publish")
}

// readDir reads directory entries, filtering out non-markdown files.
func readDir(dir string) ([]string, error) {
	entries, err := dirEntries(dir)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, e := range entries {
		if !e.IsDir() {
			result = append(result, e.Name())
		}
	}
	return result, nil
}

var dirEntries = os.ReadDir
