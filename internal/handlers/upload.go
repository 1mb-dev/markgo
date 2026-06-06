package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	slugutil "github.com/1mb-dev/markgo/internal/slug"
)

var (
	blockedExtensions = map[string]bool{
		".exe": true, ".bat": true, ".cmd": true, ".sh": true,
		".com": true, ".msi": true, ".dll": true, ".so": true,
		".html": true, ".htm": true, ".xhtml": true,
		".js": true, ".mjs": true, ".cjs": true,
		".svg": true,
		".php": true, ".py": true, ".rb": true, ".pl": true,
		".cgi": true, ".jsp": true, ".asp": true, ".aspx": true,
	}
	imageExtensions = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true,
	}
	safeFilenameRe = regexp.MustCompile(`[^a-z0-9_-]`)
)

// sanitizeFilename strips the extension, lowercases, removes unsafe chars,
// and caps length. Returns "file" if the result is empty.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)
	name = strings.ToLower(name)
	name = safeFilenameRe.ReplaceAllString(name, "")
	if name == "" {
		name = "file"
	}
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// cleanupTempFile removes a temporary file, logging on failure.
func (h *ComposeHandler) cleanupTempFile(path string) {
	if removeErr := os.Remove(path); removeErr != nil {
		h.logger.Error("Failed to remove temp file", "path", path, "error", removeErr)
	}
}

// Upload handles file upload for the compose page.
// Files are scoped to a slug directory: {config.Upload.Path}/{slug}/{filename}.
// Extension-based blocklist rejects executables, scripts, and HTML-renderable types; all others are allowed.
func (h *ComposeHandler) Upload(c *gin.Context) {
	slug := c.Param("slug")
	if !validSlug.MatchString(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug"})
		return
	}

	maxSize := h.config.Upload.MaxSize
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.logger.Error("Upload FormFile error", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided or file too large"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File must have an extension"})
		return
	}
	if blockedExtensions[ext] {
		h.logger.Warn("Upload rejected: blocked extension",
			"extension", ext,
			"filename", header.Filename,
			"size", header.Size,
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}

	safeName := sanitizeFilename(header.Filename)
	randBytes := make([]byte, 4)
	if _, randErr := rand.Read(randBytes); randErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}
	filename := fmt.Sprintf("%d-%s-%s%s", time.Now().UnixMilli(), safeName, hex.EncodeToString(randBytes), ext)

	uploadDir, containErr := slugutil.ContainPath(h.config.Upload.Path, slug)
	if containErr != nil {
		if errors.Is(containErr, apperrors.ErrPathEscape) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug"})
			return
		}
		h.logger.Error("Upload containment check failed", "error", containErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Server configuration error"})
		return
	}

	if mkdirErr := os.MkdirAll(uploadDir, 0o755); mkdirErr != nil { //nolint:gosec // upload dir needs to be accessible
		h.logger.Error("Failed to create upload directory", "error", mkdirErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	destPath := filepath.Join(uploadDir, filename)

	// Atomic write: temp file + rename
	tmpFile, err := os.CreateTemp(uploadDir, "upload-*")
	if err != nil {
		h.logger.Error("Failed to create temp file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}
	tmpPath := tmpFile.Name()

	if _, copyErr := io.Copy(tmpFile, file); copyErr != nil {
		_ = tmpFile.Close()
		h.cleanupTempFile(tmpPath)
		h.logger.Error("Failed to write file", "error", copyErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	if closeErr := tmpFile.Close(); closeErr != nil {
		h.cleanupTempFile(tmpPath)
		h.logger.Error("Failed to close temp file", "error", closeErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	if renameErr := os.Rename(tmpPath, destPath); renameErr != nil {
		h.cleanupTempFile(tmpPath)
		h.logger.Error("Failed to rename uploaded file", "error", renameErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	if chmodErr := os.Chmod(destPath, 0o644); chmodErr != nil { //nolint:gosec // uploaded files must be readable by web server
		h.logger.Error("Failed to chmod uploaded file", "path", destPath, "error", chmodErr)
		h.cleanupTempFile(destPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	url := fmt.Sprintf("/uploads/%s/%s", slug, filename)
	var markdown string
	if imageExtensions[ext] {
		markdown = fmt.Sprintf("![%s](%s)", safeName, url)
	} else {
		markdown = fmt.Sprintf("[%s](%s)", safeName, url)
	}

	c.JSON(http.StatusOK, gin.H{
		"url":      url,
		"markdown": markdown,
		"filename": filename,
	})
}
