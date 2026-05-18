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
// Explicit directory rejection on the local path mirrors the prior shape:
// c.FileFromFS routes through http.FileServer which redirects on
// directories, and gin's RedirectTrailingSlash would loop that back. The
// /static/* mount avoids this because gin's createStaticHandler has an
// explicit *OnlyFilesFS type-check; the /sw.js route bypasses that path.
func serveSwJs(localFS http.FileSystem, substitutedBody []byte, modTime time.Time) func(c *gin.Context) {
	return func(c *gin.Context) {
		if localFS != nil {
			if f, err := localFS.Open("sw.js"); err == nil {
				defer f.Close()
				if stat, sErr := f.Stat(); sErr == nil && !stat.IsDir() {
					http.ServeContent(c.Writer, c.Request, "sw.js", stat.ModTime(), f)
					return
				}
			}
		}
		http.ServeContent(c.Writer, c.Request, "sw.js", modTime, bytes.NewReader(substitutedBody))
	}
}
