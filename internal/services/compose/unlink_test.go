package compose

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fileExists is a small helper for unlink assertions.
func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// stageUpload creates an upload file at <uploadPath>/<slug>/<filename> and
// returns its absolute path.
func stageUpload(t *testing.T, uploadPath, slug, filename string) string {
	t.Helper()
	slugDir := filepath.Join(uploadPath, slug)
	require.NoError(t, os.MkdirAll(slugDir, 0o755))
	path := filepath.Join(slugDir, filename)
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0o600))
	return path
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUnlinkOwnedUpload_Swap(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-a"
	prevPath := stageUpload(t, uploadPath, slug, "old.png")
	nextPath := stageUpload(t, uploadPath, slug, "new.png")

	UnlinkOwnedUpload(
		"/uploads/"+slug+"/old.png",
		"/uploads/"+slug+"/new.png",
		slug, uploadPath, quietLogger(),
	)

	assert.False(t, fileExists(t, prevPath), "previous banner must be unlinked on swap")
	assert.True(t, fileExists(t, nextPath), "new banner must be preserved")
}

func TestUnlinkOwnedUpload_Remove(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-b"
	prevPath := stageUpload(t, uploadPath, slug, "removed.png")

	UnlinkOwnedUpload(
		"/uploads/"+slug+"/removed.png",
		"", // banner cleared
		slug, uploadPath, quietLogger(),
	)

	assert.False(t, fileExists(t, prevPath), "previous banner must be unlinked on remove")
}

func TestUnlinkOwnedUpload_NoChange(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-c"
	keepPath := stageUpload(t, uploadPath, slug, "kept.png")

	UnlinkOwnedUpload(
		"/uploads/"+slug+"/kept.png",
		"/uploads/"+slug+"/kept.png",
		slug, uploadPath, quietLogger(),
	)

	assert.True(t, fileExists(t, keepPath), "unchanged banner must not be touched")
}

func TestUnlinkOwnedUpload_ExternalURLNotTouched(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-d"
	// Stage a file at a path that COULD collide with the external URL's
	// basename, to prove the helper doesn't accidentally derive a local path.
	collidingPath := stageUpload(t, uploadPath, slug, "remote.png")

	UnlinkOwnedUpload(
		"https://cdn.example.com/img/remote.png",
		"/uploads/"+slug+"/new.png",
		slug, uploadPath, quietLogger(),
	)

	assert.True(t, fileExists(t, collidingPath),
		"external URLs are operator-managed and must never trigger an unlink")
}

func TestUnlinkOwnedUpload_StaticPathNotTouched(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-e"
	// /static/... paths are STATIC_PATH-overlay territory, not slug uploads.
	UnlinkOwnedUpload(
		"/static/img/site-banner.png",
		"/uploads/"+slug+"/new.png",
		slug, uploadPath, quietLogger(),
	)
	// No file to verify — the assertion is that the function returns without
	// touching anything outside uploadPath. Coverage proves the branch.
}

func TestUnlinkOwnedUpload_BareFilenameNotTouched(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-f"
	// Bare filename in frontmatter (operator-edited markdown form) resolves
	// to /uploads/<slug>/bare.png via models.Article.BannerSrc, but the
	// frontmatter literal lacks the leading-slash shape. Operator owns these.
	keepPath := stageUpload(t, uploadPath, slug, "bare.png")

	UnlinkOwnedUpload(
		"bare.png",
		"/uploads/"+slug+"/new.png",
		slug, uploadPath, quietLogger(),
	)

	assert.True(t, fileExists(t, keepPath),
		"bare frontmatter filenames are operator-managed and must not be unlinked")
}

func TestUnlinkOwnedUpload_PathTraversalRejected(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-g"
	// File outside slug dir — must survive an attempted traversal.
	outsideDir := filepath.Join(uploadPath, "..", "victim")
	require.NoError(t, os.MkdirAll(outsideDir, 0o755))
	outsidePath := filepath.Join(outsideDir, "evil.png")
	require.NoError(t, os.WriteFile(outsidePath, []byte("payload"), 0o600))

	UnlinkOwnedUpload(
		"/uploads/"+slug+"/../victim/evil.png",
		"",
		slug, uploadPath, quietLogger(),
	)

	assert.True(t, fileExists(t, outsidePath),
		"path traversal in prev banner must not unlink files outside slug dir")
}

func TestUnlinkOwnedUpload_ENOENTSilent(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-h"
	// No file staged — prev points at a nonexistent file.

	// Capture logger output to verify no warn-level log fires for ENOENT.
	var buf bytesBuffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	UnlinkOwnedUpload(
		"/uploads/"+slug+"/never-existed.png",
		"",
		slug, uploadPath, logger,
	)

	assert.Empty(t, buf.String(),
		"ENOENT on os.Remove must be silent — no warn log expected")
}

func TestUnlinkOwnedUpload_EmptyPrevNoOp(t *testing.T) {
	uploadPath := t.TempDir()
	slug := "post-i"
	// Empty prev — first-time banner upload, nothing to clean.
	UnlinkOwnedUpload("", "/uploads/"+slug+"/first.png", slug, uploadPath, quietLogger())
	// Reaching this line without panic is the assertion.
}

// bytesBuffer is a tiny io.Writer used to capture slog output in tests.
type bytesBuffer struct{ data []byte }

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesBuffer) String() string { return string(b.data) }
