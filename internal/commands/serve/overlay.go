// Package serve — overlay file system for static assets.
//
// overlayFS implements http.FileSystem with filesystem-first, embedded-fallback
// semantics. When STATIC_PATH is set and the directory exists, the server
// mounts statics through this overlay: requests check the local tree first,
// then fall back to the embedded FS. This lets forks ship source-controlled
// assets (e.g. per-article banners under static/img/banners/) without
// mirroring the entire embedded tree.
//
// Successful local hits log at debug level for operator verification. Non-
// ENOENT local errors (e.g. EACCES from a mid-run chmod) emit a warning
// before falling through to embedded so the regression is not silent.
package serve

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type overlayFS struct {
	local    http.FileSystem
	embedded http.FileSystem
	logger   *slog.Logger
}

func newOverlayFS(local, embedded http.FileSystem, logger *slog.Logger) *overlayFS {
	return &overlayFS{local: local, embedded: embedded, logger: logger}
}

// Open returns the local file if present, otherwise the embedded file.
func (o *overlayFS) Open(name string) (http.File, error) {
	f, err := o.local.Open(name)
	if err == nil {
		o.logger.Debug("static overlay hit", "path", name)
		return f, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		o.logger.Warn("static overlay local read failed, falling through to embedded",
			"path", name, "error", err)
	}
	return o.embedded.Open(name)
}

// serveSwJs serves sw.js with overlay precedence: an operator-supplied
// <STATIC_PATH>/sw.js (localFS) wins as raw bytes; otherwise the embedded
// copy with __MARKGO_CACHE_VERSION__ substituted at startup is served from
// the cached substitutedBody. localFS may be nil when STATIC_PATH is unset
// or missing.
//
// Operator-overlay sw.js bypasses the build-version auto-bump (the operator
// owns their cache version). This is intentional; documented in
// docs/configuration.md.
//
// Failure-mode mirror with overlayFS.Open: ENOENT is silent (operator chose
// not to override), but non-ENOENT errors and operator misconfig (e.g. a
// directory at the overlay path) emit slog.Warn before fall-through so the
// regression isn't silent. The IsDir guard also avoids the directory
// redirect loop that c.FileFromFS / http.FileServer would trigger via
// gin's RedirectTrailingSlash.
func serveSwJs(localFS http.FileSystem, substitutedBody []byte, modTime time.Time, logger *slog.Logger) func(c *gin.Context) {
	return func(c *gin.Context) {
		if localFS != nil {
			f, err := localFS.Open("sw.js")
			switch {
			case err == nil:
				defer f.Close()
				stat, sErr := f.Stat()
				switch {
				case sErr != nil:
					logger.Warn("sw.js overlay stat failed; falling back to embedded", "error", sErr)
				case stat.IsDir():
					logger.Warn("sw.js overlay path is a directory; falling back to embedded")
				default:
					http.ServeContent(c.Writer, c.Request, "sw.js", stat.ModTime(), f)
					return
				}
			case errors.Is(err, fs.ErrNotExist):
				// Operator chose not to override — silent fall-through.
			default:
				logger.Warn("sw.js overlay open failed; falling back to embedded", "error", err)
			}
		}
		http.ServeContent(c.Writer, c.Request, "sw.js", modTime, bytes.NewReader(substitutedBody))
	}
}
