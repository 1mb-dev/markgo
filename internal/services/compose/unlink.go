package compose

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// UnlinkOwnedUpload removes an upload referenced by the server-absolute
// path shape /uploads/<slug>/<filename> when the banner has changed.
// Side-effect cleanup intended for callers who just persisted a new banner
// value to disk.
//
// No-op when:
//   - prev is empty (no previous upload to clean).
//   - prev equals next (banner unchanged).
//   - prev is not in the /uploads/<slug>/ shape (external URL, /static/...,
//     or operator-managed bare filename — those are not ours to remove).
//   - filename contains a path separator (defense against malformed input
//     that would land outside the slug directory).
//
// Path-containment is enforced via ContainSlugPath; a containment failure
// is logged and skipped. os.ErrNotExist is silent (the file was already
// gone). Any other os.Remove error is logged at warn level.
//
// Cleanup is best-effort and must never fail the caller; this function
// has no return value by design.
func UnlinkOwnedUpload(prev, next, slug, uploadPath string, logger *slog.Logger) {
	if prev == "" || prev == next {
		return
	}
	expectedPrefix := "/uploads/" + slug + "/"
	if !strings.HasPrefix(prev, expectedPrefix) {
		return
	}
	filename := strings.TrimPrefix(prev, expectedPrefix)
	if filename == "" || strings.ContainsRune(filename, '/') {
		return
	}
	slugDir, err := ContainSlugPath(uploadPath, slug)
	if err != nil {
		logger.Warn("unlink owned upload: containment failed",
			"slug", slug, "prev_banner", prev, "error", err)
		return
	}
	target := filepath.Join(slugDir, filename)
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("unlink owned upload: remove failed",
			"slug", slug, "path", target, "error", err)
	}
}
