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
	"errors"
	"io/fs"
	"log/slog"
	"net/http"

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

// serveSwJs serves sw.js from the given FS, with explicit directory rejection.
// Necessary because c.FileFromFS routes through http.FileServer, which redirects
// on directories — and gin's RedirectTrailingSlash would loop that back. The
// /static/* mount avoids this because gin's createStaticHandler has an explicit
// *OnlyFilesFS type-check; the /sw.js route bypasses that path.
func serveSwJs(staticFS http.FileSystem) func(c *gin.Context) {
	return func(c *gin.Context) {
		f, err := staticFS.Open("sw.js")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}
		http.ServeContent(c.Writer, c.Request, "sw.js", stat.ModTime(), f)
	}
}
