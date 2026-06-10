// Package web embeds templates and static assets into the binary.
package web

import "embed"

// Assets contains all embedded web templates and static files.
//
// No `all:` prefix: dot- and underscore-prefixed entries are excluded so a
// stray local .DS_Store never bakes into a dev build (CI builds from a clean
// checkout never had it). There are no dot/underscore files in the trees that
// need embedding; add the `all:` form back if that ever changes.
//
//go:embed templates static
var Assets embed.FS
