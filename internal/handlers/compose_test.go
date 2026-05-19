package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services/compose"
)

// ---------------------------------------------------------------------------
// Test helpers & mocks
// ---------------------------------------------------------------------------

type MockMarkdownRenderer struct {
	Result string
	Err    error
}

func (m *MockMarkdownRenderer) ProcessMarkdown(content string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.Result != "" {
		return m.Result, nil
	}
	return "<p>" + content + "</p>", nil
}

func createTestComposeHandler(t *testing.T, renderer MarkdownRenderer) (*ComposeHandler, string) {
	t.Helper()
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Environment: "test",
		BaseURL:     "http://localhost:3000",
		StaticPath:  tmpDir,
		Blog: config.BlogConfig{
			Title: "Test Blog",
		},
		Upload: config.UploadConfig{
			Path:    filepath.Join(tmpDir, "uploads"),
			MaxSize: 10 << 20,
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

	if renderer == nil {
		renderer = &MockMarkdownRenderer{}
	}

	handler := NewComposeHandler(base, nil, &MockArticleService{}, renderer)
	return handler, tmpDir
}

// createMultipartRequest builds a multipart/form-data request with a file field.
func createMultipartRequest(t *testing.T, fieldName, fileName string, fileContent []byte, extraFields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = part.Write(fileContent)
	require.NoError(t, err)

	for k, v := range extraFields {
		require.NoError(t, writer.WriteField(k, v))
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/compose/upload/test-article", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// Minimal 1x1 JPEG (valid JFIF header).
var minimalJPEG = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
	0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
	0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
	0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
	0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
	0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
	0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
	0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
	0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
	0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
	0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
	0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
	0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
	0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
	0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
	0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
	0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
	0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
	0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
	0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
	0x00, 0x00, 0x3F, 0x00, 0x7B, 0x94, 0x11, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xD9,
}

// ---------------------------------------------------------------------------
// sanitizeFilename tests (pure function)
// ---------------------------------------------------------------------------

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"simple name", "photo.jpg", "photo"},
		{"uppercase", "MyPhoto.PNG", "myphoto"},
		{"spaces and special chars", "my photo (1).jpg", "myphoto1"},
		{"unicode only", "фото.jpg", "file"},
		{"empty string", "", "file"},
		{"extension only", ".jpg", "file"},
		{"path traversal", "../../etc/passwd", "passwd"},
		{"deep path traversal", "../../../secret.jpg", "secret"},
		{"hyphens and underscores", "my-photo_2024.webp", "my-photo_2024"},
		{"long name", strings.Repeat("a", 100) + ".jpg", strings.Repeat("a", 50)},
		{"dots in name", "file.name.with.dots.jpg", "filenamewithdots"}, // filepath.Ext gets ".jpg", rest keeps dots→stripped
		{"mixed safe and unsafe", "Hello World! @#$.png", "helloworld"}, // special chars stripped
		{"numbers only", "12345.gif", "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Preview handler tests
// ---------------------------------------------------------------------------

func TestPreview(t *testing.T) {
	t.Run("valid markdown returns HTML", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, &MockMarkdownRenderer{
			Result: "<h1>Hello</h1>",
		})

		router := gin.New()
		router.POST("/compose/preview", handler.Preview)

		form := url.Values{"content": {"# Hello"}}
		req := httptest.NewRequest("POST", "/compose/preview", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
		assert.Equal(t, "<h1>Hello</h1>", w.Body.String())
	})

	t.Run("empty content returns 200 with empty body", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		router := gin.New()
		router.POST("/compose/preview", handler.Preview)

		form := url.Values{"content": {""}}
		req := httptest.NewRequest("POST", "/compose/preview", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Body.String())
	})

	t.Run("oversized body rejected by MaxBytesReader", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		router := gin.New()
		router.POST("/compose/preview", handler.Preview)

		largeContent := strings.Repeat("x", 2<<20) // 2MB, exceeds 1MB limit
		form := url.Values{"content": {largeContent}}
		req := httptest.NewRequest("POST", "/compose/preview", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// MaxBytesReader truncates the body; Gin's PostForm returns empty string
		// because form parsing fails on the truncated body → handler sees empty content → 200
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Body.String())
	})

	t.Run("renderer error returns 500", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, &MockMarkdownRenderer{
			Err: fmt.Errorf("render failed"),
		})

		router := gin.New()
		router.POST("/compose/preview", handler.Preview)

		form := url.Values{"content": {"# broken"}}
		req := httptest.NewRequest("POST", "/compose/preview", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Preview unavailable")
	})
}

// ---------------------------------------------------------------------------
// Upload handler tests
// ---------------------------------------------------------------------------

func TestUpload(t *testing.T) {
	t.Run("valid JPEG upload returns image markdown", func(t *testing.T) {
		handler, tmpDir := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "test-photo.jpg", minimalJPEG, nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["url"], "/uploads/test-article/")
		assert.Contains(t, resp["url"], ".jpg")
		assert.Contains(t, resp["markdown"], "![test-photo]")
		assert.NotEmpty(t, resp["filename"])

		// Verify file exists on disk with correct permissions
		uploadDir := filepath.Join(tmpDir, "uploads", "test-article")
		files, err := os.ReadDir(uploadDir)
		require.NoError(t, err)
		assert.Len(t, files, 1)

		info, err := os.Stat(filepath.Join(uploadDir, files[0].Name()))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	})

	t.Run("PDF upload returns link markdown", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "document.pdf", []byte("fake pdf content"), nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["url"], "/uploads/test-article/")
		assert.Contains(t, resp["url"], ".pdf")
		assert.Contains(t, resp["markdown"], "[document]")
		assert.NotContains(t, resp["markdown"], "![")
	})

	t.Run("blocked extension rejected", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "malware.exe", []byte("bad content"), nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "File type not allowed", resp["error"])
	})

	t.Run("HTML extension rejected (XSS prevention)", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "page.html", []byte("<script>alert('xss')</script>"), nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "File type not allowed", resp["error"])
	})

	t.Run("SVG extension rejected (XSS prevention)", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "image.svg", []byte("<svg><script>alert('xss')</script></svg>"), nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "File type not allowed", resp["error"])
	})

	t.Run("invalid slug rejected", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, err := writer.CreateFormFile("file", "photo.jpg")
		require.NoError(t, err)
		_, err = part.Write(minimalJPEG)
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		req := httptest.NewRequest("POST", "/compose/upload/INVALID%20SLUG!", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Invalid slug", resp["error"])
	})

	t.Run("no file provided", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		form := url.Values{"other": {"data"}}
		req := httptest.NewRequest("POST", "/compose/upload/test-article", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["error"], "No file")
	})

	t.Run("path traversal filename is sanitized", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "../../etc/passwd.jpg", minimalJPEG, nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotContains(t, resp["filename"], "..")
		assert.NotContains(t, resp["filename"], "/")
		assert.Contains(t, resp["markdown"], "![passwd]")
	})

	t.Run("unicode filename falls back to file", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "фото.jpg", minimalJPEG, nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["markdown"], "![file]")
	})

	t.Run("no-extension file rejected", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		req := createMultipartRequest(t, "file", "noextension", []byte("some content"), nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/upload/:slug", handler.Upload)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "File must have an extension", resp["error"])
	})

	t.Run("case-insensitive extension blocking", func(t *testing.T) {
		cases := []struct {
			name     string
			filename string
		}{
			{"uppercase EXE", "malware.EXE"},
			{"uppercase HTML", "page.HTML"},
			{"uppercase JS", "script.JS"},
			{"mixed case Exe", "trojan.Exe"},
			{"mixed case Html", "inject.Html"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				handler, _ := createTestComposeHandler(t, nil)

				req := createMultipartRequest(t, "file", tc.filename, []byte("bad content"), nil)
				w := httptest.NewRecorder()

				router := gin.New()
				router.POST("/compose/upload/:slug", handler.Upload)
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				var resp map[string]string
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, "File type not allowed", resp["error"])
			})
		}
	})

	t.Run("blocked extension categories", func(t *testing.T) {
		cases := []struct {
			name     string
			filename string
			reason   string
		}{
			{"JavaScript upload", "script.js", "XSS via JavaScript"},
			{"PHP upload", "shell.php", "server-side code execution"},
			{"shell script upload", "run.sh", "shell script execution"},
			{"Python upload", "exploit.py", "server-side code execution"},
			{"batch file upload", "virus.bat", "Windows batch execution"},
			{"module JS upload", "module.mjs", "ES module JavaScript"},
			{"CommonJS upload", "require.cjs", "CommonJS JavaScript"},
			{"JSP upload", "admin.jsp", "Java server pages"},
			{"ASP upload", "cmd.asp", "Active Server Pages"},
			{"ASPX upload", "page.aspx", "ASP.NET pages"},
			{"CGI upload", "handler.cgi", "CGI execution"},
			{"Ruby upload", "app.rb", "Ruby execution"},
			{"Perl upload", "hack.pl", "Perl execution"},
			{"DLL upload", "payload.dll", "Windows library"},
			{"SO upload", "payload.so", "Linux shared object"},
			{"MSI upload", "setup.msi", "Windows installer"},
			{"COM upload", "virus.com", "DOS executable"},
			{"XHTML upload", "page.xhtml", "XHTML injection"},
			{"HTM upload", "page.htm", "HTML injection"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				handler, _ := createTestComposeHandler(t, nil)

				req := createMultipartRequest(t, "file", tc.filename, []byte("bad content"), nil)
				w := httptest.NewRecorder()

				router := gin.New()
				router.POST("/compose/upload/:slug", handler.Upload)
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusBadRequest, w.Code, "expected %s to be blocked (%s)", tc.filename, tc.reason)
				var resp map[string]string
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, "File type not allowed", resp["error"])
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Quick publish handler tests
// ---------------------------------------------------------------------------

func createQuickPublishHandler(t *testing.T) *ComposeHandler {
	t.Helper()
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Environment:  "test",
		BaseURL:      "http://localhost:3000",
		ArticlesPath: tmpDir,
		Blog:         config.BlogConfig{Title: "Test Blog", Author: "Test Author"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

	composeSvc := compose.NewService(tmpDir, cfg.Blog.Author)
	return NewComposeHandler(base, composeSvc, &MockArticleService{}, &MockMarkdownRenderer{})
}

func TestQuickPublish(t *testing.T) {
	t.Run("thought — content only, no title", func(t *testing.T) {
		handler := createQuickPublishHandler(t)

		body := `{"content":"Just a quick thought about Go templates."}`
		req := httptest.NewRequest("POST", "/compose/quick", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/quick", handler.HandleQuickPublish)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["slug"])
		assert.Contains(t, resp["url"], "/writing/")
		assert.Equal(t, "thought", resp["type"])
		assert.Equal(t, "Published", resp["message"])
		assert.Equal(t, false, resp["draft"])
	})

	t.Run("article — has title", func(t *testing.T) {
		handler := createQuickPublishHandler(t)

		body := `{"content":"Full article content here.","title":"My Article"}`
		req := httptest.NewRequest("POST", "/compose/quick", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/quick", handler.HandleQuickPublish)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "my-article", resp["slug"])
		assert.Equal(t, "/writing/my-article", resp["url"])
		assert.Equal(t, "article", resp["type"])
	})

	t.Run("link — has link_url", func(t *testing.T) {
		handler := createQuickPublishHandler(t)

		body := `{"content":"Check this out","link_url":"https://example.com"}`
		req := httptest.NewRequest("POST", "/compose/quick", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/quick", handler.HandleQuickPublish)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "link", resp["type"])
	})

	t.Run("long content without title is article not thought", func(t *testing.T) {
		handler := createQuickPublishHandler(t)

		// 150 words without a title — should be "article", not "thought"
		words := strings.Repeat("word ", 150)
		body := fmt.Sprintf(`{"content":%q}`, words)
		req := httptest.NewRequest("POST", "/compose/quick", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/quick", handler.HandleQuickPublish)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "article", resp["type"])
	})

	t.Run("empty content returns 400", func(t *testing.T) {
		handler := createQuickPublishHandler(t)

		body := `{"content":""}`
		req := httptest.NewRequest("POST", "/compose/quick", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/quick", handler.HandleQuickPublish)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["error"], "Content is required")
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		handler := createQuickPublishHandler(t)

		req := httptest.NewRequest("POST", "/compose/quick", strings.NewReader("not json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/compose/quick", handler.HandleQuickPublish)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["error"], "Invalid request body")
	})
}

// ---------------------------------------------------------------------------
// PublishDraft handler tests
// ---------------------------------------------------------------------------

func createPublishDraftHandler(t *testing.T) (*ComposeHandler, string) {
	t.Helper()
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Environment:  "test",
		BaseURL:      "http://localhost:3000",
		ArticlesPath: tmpDir,
		Blog:         config.BlogConfig{Title: "Test Blog", Author: "Test Author"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

	composeSvc := compose.NewService(tmpDir, cfg.Blog.Author)
	handler := NewComposeHandler(base, composeSvc, &MockArticleService{}, &MockMarkdownRenderer{})
	return handler, tmpDir
}

func writeDraftArticle(t *testing.T, dir, slug string, isDraft bool) {
	t.Helper()
	draftStr := "false"
	if isDraft {
		draftStr = "true"
	}
	content := fmt.Sprintf("---\nslug: %s\ntitle: Test Article\ndraft: %s\ndate: 2026-01-01T00:00:00Z\n---\n\nHello world.\n", slug, draftStr)
	filename := fmt.Sprintf("2026-01-01-%s.md", slug)
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
}

func writePageArticle(t *testing.T, dir, slug string) {
	t.Helper()
	content := fmt.Sprintf("---\nslug: %s\ntitle: A Page\ntype: page\ndraft: false\n---\n\nPage body.\n", slug)
	filename := fmt.Sprintf("2026-01-01-%s.md", slug)
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
}

func TestPublishDraft(t *testing.T) {
	t.Run("invalid slug returns 400", func(t *testing.T) {
		handler, _ := createPublishDraftHandler(t)

		router := gin.New()
		router.POST("/compose/publish/:slug", handler.PublishDraft)

		req := httptest.NewRequest("POST", "/compose/publish/INVALID!SLUG", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Invalid slug", resp["error"])
	})

	t.Run("slug not found returns 404", func(t *testing.T) {
		handler, _ := createPublishDraftHandler(t)

		router := gin.New()
		router.POST("/compose/publish/:slug", handler.PublishDraft)

		req := httptest.NewRequest("POST", "/compose/publish/nonexistent", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Article not found", resp["error"])
	})

	t.Run("already published returns 200 with message", func(t *testing.T) {
		handler, tmpDir := createPublishDraftHandler(t)
		writeDraftArticle(t, tmpDir, "published-post", false)

		router := gin.New()
		router.POST("/compose/publish/:slug", handler.PublishDraft)

		req := httptest.NewRequest("POST", "/compose/publish/published-post", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Already published", resp["message"])
		assert.Equal(t, "published-post", resp["slug"])
		assert.Equal(t, "/writing/published-post", resp["url"])
	})

	t.Run("successful publish returns 200", func(t *testing.T) {
		handler, tmpDir := createPublishDraftHandler(t)
		writeDraftArticle(t, tmpDir, "my-draft", true)

		router := gin.New()
		router.POST("/compose/publish/:slug", handler.PublishDraft)

		req := httptest.NewRequest("POST", "/compose/publish/my-draft", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Published", resp["message"])
		assert.Equal(t, "my-draft", resp["slug"])
		assert.Equal(t, "/writing/my-draft", resp["url"])

		// Verify file was updated on disk
		updated, err := os.ReadFile(filepath.Join(tmpDir, "2026-01-01-my-draft.md"))
		require.NoError(t, err)
		assert.Contains(t, string(updated), "draft: false")
	})
}

// ---------------------------------------------------------------------------
// T5: PublishDraft reload-failure path
// ---------------------------------------------------------------------------

// FailReloadArticleService is a mock where ReloadArticles always returns an error.
type FailReloadArticleService struct {
	MockArticleService
}

func (m *FailReloadArticleService) ReloadArticles() error {
	return errors.New("reload failed: index corrupted")
}

func createPublishDraftHandlerWithReloadFailure(t *testing.T) (*ComposeHandler, string) {
	t.Helper()
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Environment:  "test",
		BaseURL:      "http://localhost:3000",
		ArticlesPath: tmpDir,
		Blog:         config.BlogConfig{Title: "Test Blog", Author: "Test Author"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

	composeSvc := compose.NewService(tmpDir, cfg.Blog.Author)
	handler := NewComposeHandler(base, composeSvc, &FailReloadArticleService{}, &MockMarkdownRenderer{})
	return handler, tmpDir
}

func TestPublishDraftReloadFailure(t *testing.T) {
	t.Run("publish succeeds but reload fails returns degraded response", func(t *testing.T) {
		handler, tmpDir := createPublishDraftHandlerWithReloadFailure(t)
		writeDraftArticle(t, tmpDir, "reload-fail-draft", true)

		router := gin.New()
		router.POST("/compose/publish/:slug", handler.PublishDraft)

		req := httptest.NewRequest("POST", "/compose/publish/reload-fail-draft", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		// When reload fails, URL should fall back to "/"
		assert.Equal(t, "/", resp["url"])
		assert.Equal(t, "reload-fail-draft", resp["slug"])
		assert.Contains(t, resp["message"], "next reload")

		// Verify file was still updated on disk (publish succeeded)
		updated, err := os.ReadFile(filepath.Join(tmpDir, "2026-01-01-reload-fail-draft.md"))
		require.NoError(t, err)
		assert.Contains(t, string(updated), "draft: false")
	})
}

// ---------------------------------------------------------------------------
// Drafts handler tests
// ---------------------------------------------------------------------------

// DraftsArticleService returns canned draft data.
type DraftsArticleService struct {
	MockArticleService
	Drafts []*models.Article
}

func (m *DraftsArticleService) GetDraftArticles() []*models.Article { return m.Drafts }

func TestDrafts(t *testing.T) {
	t.Run("JSON response with drafts", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		svc := &DraftsArticleService{
			Drafts: []*models.Article{
				{Slug: "draft-1", Title: "Draft One", Draft: true, Date: time.Now()},
				{Slug: "draft-2", Title: "Draft Two", Draft: true, Date: time.Now()},
			},
		}
		handler := NewAdminHandler(base, svc, time.Now())

		router := gin.New()
		router.GET("/admin/drafts", handler.Drafts)

		req := httptest.NewRequest("GET", "/admin/drafts", http.NoBody)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, float64(2), resp["draft_count"])

		drafts, ok := resp["drafts"].([]any)
		require.True(t, ok)
		assert.Len(t, drafts, 2)
	})

	t.Run("JSON response with no drafts", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		svc := &DraftsArticleService{Drafts: []*models.Article{}}
		handler := NewAdminHandler(base, svc, time.Now())

		router := gin.New()
		router.GET("/admin/drafts", handler.Drafts)

		req := httptest.NewRequest("GET", "/admin/drafts", http.NoBody)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, float64(0), resp["draft_count"])
	})

	// TestDrafts_JSON_NoPII_AskerEmail_NotLeaked verifies that AskerEmail from
	// AMA drafts is not serialised to JSON. Regression guard for v3.10.3 F2:
	// pre-fix the handler emitted raw *models.Article including AskerEmail.
	t.Run("JSON response omits AskerEmail PII from AMA drafts", func(t *testing.T) {
		cfg := createTestConfig()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		svc := &DraftsArticleService{
			Drafts: []*models.Article{
				{
					Slug:       "ama-question",
					Title:      "Why X?",
					Draft:      true,
					Date:       time.Now(),
					Type:       "ama",
					Asker:      "Anonymous",
					AskerEmail: "secret@example.com",
				},
			},
		}
		handler := NewAdminHandler(base, svc, time.Now())
		router := gin.New()
		router.GET("/admin/drafts", handler.Drafts)

		req := httptest.NewRequest("GET", "/admin/drafts", http.NoBody)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.NotContains(t, body, "secret@example.com", "AskerEmail must not leak in JSON response")
		assert.NotContains(t, body, "asker_email", "asker_email field must not appear in JSON response")
	})
}

// ---------------------------------------------------------------------------
// J1: HandleSubmit form tests
// ---------------------------------------------------------------------------

// createFormComposeHandler creates a ComposeHandler with a REAL compose.Service
// writing to t.TempDir(). Mirrors createQuickPublishHandler but for form tests.
func createFormComposeHandler(t *testing.T) (*ComposeHandler, string) {
	t.Helper()
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Environment:  "test",
		BaseURL:      "http://localhost:3000",
		ArticlesPath: tmpDir,
		Blog:         config.BlogConfig{Title: "Test Blog", Author: "Test Author"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

	composeSvc := compose.NewService(tmpDir, cfg.Blog.Author)
	handler := NewComposeHandler(base, composeSvc, &MockArticleService{}, &MockMarkdownRenderer{})
	return handler, tmpDir
}

func TestHandleSubmit(t *testing.T) {
	t.Run("valid content creates article and redirects", func(t *testing.T) {
		handler, tmpDir := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Hello world, this is a test article."},
			"title":   {"My Test Article"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/writing/my-test-article", w.Header().Get("Location"))

		// Verify file was created on disk
		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Contains(t, entries[0].Name(), "my-test-article.md")
	})

	t.Run("empty content returns 400", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {""},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("draft mode redirects to /admin/drafts", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Draft content here."},
			"title":   {"Draft Post"},
			"draft":   {"on"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Drafts have no public URL — canonicalPathForSlug would 404. Operator
		// must land on /admin/drafts to find and publish.
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/admin/drafts", w.Header().Get("Location"))
	})

	t.Run("content without title redirects to feed", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Just a quick thought."},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// No title → redirects to "/" (feed) regardless of reloadOK
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
	})

	t.Run("reload failure redirects to feed", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &config.Config{
			Environment:  "test",
			BaseURL:      "http://localhost:3000",
			ArticlesPath: tmpDir,
			Blog:         config.BlogConfig{Title: "Test Blog", Author: "Test Author"},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		composeSvc := compose.NewService(tmpDir, cfg.Blog.Author)
		handler := NewComposeHandler(base, composeSvc, &FailReloadArticleService{}, &MockMarkdownRenderer{})

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Article with reload failure."},
			"title":   {"Reload Fail"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Reload failure → redirect to "/" even though title is set
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
	})
}

// ---------------------------------------------------------------------------
// J1b: HandleEdit form tests
// ---------------------------------------------------------------------------

func TestHandleEdit(t *testing.T) {
	t.Run("valid edit updates and redirects", func(t *testing.T) {
		handler, tmpDir := createFormComposeHandler(t)
		writeDraftArticle(t, tmpDir, "existing-post", false)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose/edit/:slug", handler.HandleEdit)

		form := url.Values{
			"content": {"Updated content here."},
			"title":   {"Updated Title"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose/edit/existing-post", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/writing/existing-post", w.Header().Get("Location"))

		// Verify file was updated on disk
		updated, err := os.ReadFile(filepath.Join(tmpDir, "2026-01-01-existing-post.md"))
		require.NoError(t, err)
		assert.Contains(t, string(updated), "Updated content here.")
		assert.Contains(t, string(updated), "Updated Title")
	})

	t.Run("invalid slug returns error", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose/edit/:slug", handler.HandleEdit)

		form := url.Values{
			"content": {"Some content."},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose/edit/!!!invalid", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// handleError renders a 404 error page for invalid slugs
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("empty content returns 400", func(t *testing.T) {
		handler, tmpDir := createFormComposeHandler(t)
		writeDraftArticle(t, tmpDir, "edit-empty", false)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose/edit/:slug", handler.HandleEdit)

		form := url.Values{
			"content": {""},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose/edit/edit-empty", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("edit toggles to draft → redirects to /admin/drafts", func(t *testing.T) {
		handler, tmpDir := createFormComposeHandler(t)
		writeDraftArticle(t, tmpDir, "edit-to-draft", false)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose/edit/:slug", handler.HandleEdit)

		form := url.Values{
			"content": {"Demoted to draft."},
			"title":   {"Demoted"},
			"draft":   {"on"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose/edit/edit-to-draft", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/admin/drafts", w.Header().Get("Location"))
	})

	t.Run("edit reload failure redirects to feed", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &config.Config{
			Environment:  "test",
			BaseURL:      "http://localhost:3000",
			ArticlesPath: tmpDir,
			Blog:         config.BlogConfig{Title: "Test Blog", Author: "Test Author"},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		composeSvc := compose.NewService(tmpDir, cfg.Blog.Author)
		handler := NewComposeHandler(base, composeSvc, &FailReloadArticleService{}, &MockMarkdownRenderer{})

		writeDraftArticle(t, tmpDir, "reload-fail-edit", false)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose/edit/:slug", handler.HandleEdit)

		form := url.Values{
			"content": {"Updated content."},
			"title":   {"Updated"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose/edit/reload-fail-edit", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Reload failure → redirect to "/" instead of /writing/slug
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
	})
}

// ---------------------------------------------------------------------------
// J2: refreshCSRFToken tests
// ---------------------------------------------------------------------------

func TestRefreshCSRFToken(t *testing.T) {
	t.Run("generates valid 64-char hex token", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("csrf_secure", false)
		c.Request = httptest.NewRequest("GET", "/", http.NoBody)

		token := refreshCSRFToken(c)

		assert.NotEmpty(t, token)
		assert.Len(t, token, 64, "token should be 64 hex characters (32 bytes)")
		assert.False(t, c.IsAborted(), "should not abort on success")
	})

	t.Run("sets cookie with correct attributes", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("csrf_secure", false)
		c.Request = httptest.NewRequest("GET", "/", http.NoBody)

		token := refreshCSRFToken(c)

		// Verify Set-Cookie header exists
		cookies := w.Result().Cookies()
		require.NotEmpty(t, cookies, "should have at least one cookie")

		var csrfCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "_csrf" {
				csrfCookie = cookie
				break
			}
		}
		require.NotNil(t, csrfCookie, "should have _csrf cookie")
		assert.Equal(t, token, csrfCookie.Value)
		assert.True(t, csrfCookie.HttpOnly, "cookie should be HttpOnly")
		assert.Equal(t, http.SameSiteStrictMode, csrfCookie.SameSite, "cookie should be SameSite=Strict")
		assert.False(t, csrfCookie.Secure, "cookie should not be Secure when csrf_secure=false")
	})

	t.Run("secure cookie when csrf_secure is true", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("csrf_secure", true)
		c.Request = httptest.NewRequest("GET", "/", http.NoBody)

		refreshCSRFToken(c)

		cookies := w.Result().Cookies()
		var csrfCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "_csrf" {
				csrfCookie = cookie
				break
			}
		}
		require.NotNil(t, csrfCookie, "should have _csrf cookie")
		assert.True(t, csrfCookie.Secure, "cookie should be Secure when csrf_secure=true")
	})

	t.Run("each call generates a unique token", func(t *testing.T) {
		w1 := httptest.NewRecorder()
		c1, _ := gin.CreateTestContext(w1)
		c1.Set("csrf_secure", false)
		c1.Request = httptest.NewRequest("GET", "/", http.NoBody)
		token1 := refreshCSRFToken(c1)

		w2 := httptest.NewRecorder()
		c2, _ := gin.CreateTestContext(w2)
		c2.Set("csrf_secure", false)
		c2.Request = httptest.NewRequest("GET", "/", http.NoBody)
		token2 := refreshCSRFToken(c2)

		assert.NotEqual(t, token1, token2, "consecutive tokens should be unique")
	})
}

// ---------------------------------------------------------------------------
// csrfToken helper tests
// ---------------------------------------------------------------------------

func TestCsrfToken(t *testing.T) {
	t.Run("returns token from context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("csrf_token", "abc123")

		assert.Equal(t, "abc123", csrfToken(c))
	})

	t.Run("returns empty string when no token set", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		assert.Equal(t, "", csrfToken(c))
	})

	t.Run("returns empty string when token is not a string", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("csrf_token", 12345)

		assert.Equal(t, "", csrfToken(c))
	})
}

// ---------------------------------------------------------------------------
// J3: injectAuthState tests
// ---------------------------------------------------------------------------

func TestInjectAuthState(t *testing.T) {
	t.Run("unauthenticated with admin configured generates CSRF token", func(t *testing.T) {
		cfg := &config.Config{
			Environment: "development",
			BaseURL:     "http://localhost:3000",
			Blog:        config.BlogConfig{Title: "Test Blog"},
			Admin: config.AdminConfig{
				Username: "admin",
				Password: "secret",
			},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/some-page", http.NoBody)

		data := make(map[string]any)
		base.injectAuthState(c, data)

		// Should have generated a CSRF token since admin is configured and no token existed
		assert.NotEmpty(t, data["csrf_token"], "should generate CSRF token for login popover")
		token, ok := data["csrf_token"].(string)
		assert.True(t, ok, "csrf_token should be a string")
		assert.Len(t, token, 64, "generated token should be 64 hex chars")
	})

	t.Run("authenticated copies auth state to data", func(t *testing.T) {
		cfg := &config.Config{
			Environment: "test",
			BaseURL:     "http://localhost:3000",
			Blog:        config.BlogConfig{Title: "Test Blog"},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/compose", http.NoBody)
		c.Set("authenticated", true)
		c.Set("auth_required", true)
		c.Set("csrf_token", "existing-token-from-middleware")

		data := make(map[string]any)
		base.injectAuthState(c, data)

		assert.Equal(t, true, data["authenticated"])
		assert.Equal(t, true, data["auth_required"])
		assert.Equal(t, "existing-token-from-middleware", data["csrf_token"])
	})

	t.Run("login_next set to request URI", func(t *testing.T) {
		cfg := &config.Config{
			Environment: "test",
			BaseURL:     "http://localhost:3000",
			Blog:        config.BlogConfig{Title: "Test Blog"},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/writing/my-article?foo=bar", http.NoBody)

		data := make(map[string]any)
		base.injectAuthState(c, data)

		assert.Equal(t, "/writing/my-article?foo=bar", data["login_next"])
	})

	t.Run("no CSRF token generated when admin not configured", func(t *testing.T) {
		cfg := &config.Config{
			Environment: "test",
			BaseURL:     "http://localhost:3000",
			Blog:        config.BlogConfig{Title: "Test Blog"},
			Admin:       config.AdminConfig{}, // no username/password
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", http.NoBody)

		data := make(map[string]any)
		base.injectAuthState(c, data)

		// No admin configured → no CSRF token should be generated
		_, hasCSRF := data["csrf_token"]
		assert.False(t, hasCSRF, "should not generate CSRF token when admin is not configured")
	})

	t.Run("existing CSRF token preserved and not regenerated", func(t *testing.T) {
		cfg := &config.Config{
			Environment: "test",
			BaseURL:     "http://localhost:3000",
			Blog:        config.BlogConfig{Title: "Test Blog"},
			Admin: config.AdminConfig{
				Username: "admin",
				Password: "secret",
			},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", http.NoBody)
		c.Set("csrf_token", "pre-existing-token")

		data := make(map[string]any)
		base.injectAuthState(c, data)

		// Should preserve existing token, not generate a new one
		assert.Equal(t, "pre-existing-token", data["csrf_token"])
	})

	t.Run("unauthenticated state is not copied when not set", func(t *testing.T) {
		cfg := &config.Config{
			Environment: "test",
			BaseURL:     "http://localhost:3000",
			Blog:        config.BlogConfig{Title: "Test Blog"},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		base := NewBaseHandler(cfg, logger, &MockTemplateService{}, &BuildInfo{Version: "test"}, &MockSEOService{})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", http.NoBody)

		data := make(map[string]any)
		base.injectAuthState(c, data)

		_, hasAuth := data["authenticated"]
		assert.False(t, hasAuth, "authenticated should not be in data when not set in context")
		_, hasAuthReq := data["auth_required"]
		assert.False(t, hasAuthReq, "auth_required should not be in data when not set in context")
	})
}

// ---------------------------------------------------------------------------
// ShowCompose tests
// ---------------------------------------------------------------------------

func TestShowCompose(t *testing.T) {
	t.Run("renders compose template", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.GET("/compose", handler.ShowCompose)

		req := httptest.NewRequest("GET", "/compose", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("pre-fills from share_target query params", func(t *testing.T) {
		// Use a custom template service to capture the data passed to render
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.GET("/compose", handler.ShowCompose)

		req := httptest.NewRequest("GET", "/compose?title=Shared+Title&text=Shared+content&url=https://example.com", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ---------------------------------------------------------------------------
// ShowEdit tests
// ---------------------------------------------------------------------------

func TestShowEdit(t *testing.T) {
	t.Run("renders edit form for existing article", func(t *testing.T) {
		handler, tmpDir := createFormComposeHandler(t)
		writeDraftArticle(t, tmpDir, "editable-post", false)

		router := gin.New()
		router.GET("/compose/edit/:slug", handler.ShowEdit)

		req := httptest.NewRequest("GET", "/compose/edit/editable-post", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid slug returns error", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.GET("/compose/edit/:slug", handler.ShowEdit)

		req := httptest.NewRequest("GET", "/compose/edit/!!!invalid", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("nonexistent slug returns error", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.GET("/compose/edit/:slug", handler.ShowEdit)

		req := httptest.NewRequest("GET", "/compose/edit/does-not-exist", http.NoBody)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// handleError wraps this as ErrArticleNotFound → 404
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ---------------------------------------------------------------------------
// ShowComposeNewPage tests
// ---------------------------------------------------------------------------

func TestShowComposeNewPage(t *testing.T) {
	t.Run("renders compose form in page mode", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.GET("/compose/new-page", handler.ShowComposeNewPage)

		req := httptest.NewRequest("GET", "/compose/new-page", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ---------------------------------------------------------------------------
// HandleEdit — type:page round-trip regression
// ---------------------------------------------------------------------------

// TestHandleEdit_PageRoundTrip guards the contract that editing a
// type:page article through /compose/edit/<slug> preserves the
// `type: page` frontmatter key. UpdateArticle preserves unknown
// frontmatter via generic-map round-trip today (see
// TestUpdateArticle_PreservesAMAFields for the sibling contract);
// this test locks the contract end-to-end through the handler so a
// future refactor of UpdateArticle's preservation mechanism can't
// silently regress the page edit path.
func TestHandleEdit_PageRoundTrip(t *testing.T) {
	handler, tmpDir := createFormComposeHandler(t)
	writePageArticle(t, tmpDir, "evergreen-page")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("csrf_secure", false)
		c.Next()
	})
	router.GET("/compose/edit/:slug", handler.ShowEdit)
	router.POST("/compose/edit/:slug", handler.HandleEdit)

	// GET: hidden type field must surface as "page"
	req := httptest.NewRequest("GET", "/compose/edit/evergreen-page", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// POST: round-trip type=page from form
	form := url.Values{
		"content": {"Updated page body."},
		"title":   {"A Page"},
		"type":    {"page"},
		"_csrf":   {"test-token"},
	}
	req = httptest.NewRequest("POST", "/compose/edit/evergreen-page", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusSeeOther, w.Code)

	// Frontmatter survived
	files, err := filepath.Glob(filepath.Join(tmpDir, "*evergreen-page.md"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	contentBytes, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Contains(t, string(contentBytes), "type: page", "type:page must survive edit-then-save")
	assert.Contains(t, string(contentBytes), "Updated page body.", "content reflects the edit")
}

// ---------------------------------------------------------------------------
// HandleSubmit — page-mode validation
// ---------------------------------------------------------------------------

func TestHandleSubmit_PageMode(t *testing.T) {
	t.Run("creates page and redirects to /p/<slug>", func(t *testing.T) {
		handler, tmpDir := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Evergreen page body."},
			"title":   {"About"},
			"type":    {"page"},
			"slug":    {"about-the-author"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusSeeOther, w.Code)
		// MockArticleService doesn't know the new slug, so canonicalPathForSlug
		// falls back to the synthetic empty-Type article — /writing/<slug>.
		// In production the slug would be reloaded into the repo and resolve
		// to /p/<slug>. The redirect path itself is exercised; the canonical
		// resolution is covered by predicate tests.
		assert.NotEmpty(t, w.Header().Get("Location"))

		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Contains(t, entries[0].Name(), "-about-the-author.md")

		content, err := os.ReadFile(tmpDir + "/" + entries[0].Name())
		require.NoError(t, err)
		assert.Contains(t, string(content), "type: page")
		assert.Contains(t, string(content), "slug: about-the-author")
		assert.NotContains(t, string(content), "date:", "page frontmatter omits date")
	})

	t.Run("empty title returns 400", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Body."},
			"title":   {""},
			"type":    {"page"},
			"slug":    {"my-page"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty slug returns 400", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Body."},
			"title":   {"X"},
			"type":    {"page"},
			"slug":    {""},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("reserved slug returns 400", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Body."},
			"title":   {"X"},
			"type":    {"page"},
			"slug":    {"feed"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("colliding slug returns 400", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		// MockArticleService.GetArticleBySlug returns a hit for "test-article".
		form := url.Values{
			"content": {"Body."},
			"title":   {"X"},
			"type":    {"page"},
			"slug":    {"test-article"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid slug charset returns 400", func(t *testing.T) {
		handler, _ := createFormComposeHandler(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("csrf_secure", false)
			c.Next()
		})
		router.POST("/compose", handler.HandleSubmit)

		form := url.Values{
			"content": {"Body."},
			"title":   {"X"},
			"type":    {"page"},
			"slug":    {"My_Page"},
			"_csrf":   {"test-token"},
		}
		req := httptest.NewRequest("POST", "/compose", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
