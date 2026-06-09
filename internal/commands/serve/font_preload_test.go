package serve

import (
	"io/fs"
	"log/slog"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/1mb-dev/markgo/web"
)

// embeddedStatic mirrors the embedded tree's relevant slice: the default Inter
// face is always present.
func embeddedStatic() http.FileSystem {
	return http.FS(fstest.MapFS{
		"fonts/inter/inter-latin.woff2": &fstest.MapFile{Data: []byte("INTER")},
	})
}

func TestVerifyFontPreloadResolves(t *testing.T) {
	const interURL = "/static/fonts/inter/inter-latin.woff2"
	const monoURL = "/static/fonts/space-mono/space-mono.woff2"

	tests := []struct {
		name      string
		staticFS  func() http.FileSystem
		url       string
		wantWarns int
	}{
		{
			name:      "default Inter resolves in embedded",
			staticFS:  embeddedStatic,
			url:       interURL,
			wantWarns: 0,
		},
		{
			// The crux: an overlay that swaps fonts.css but keeps Inter still
			// carries the default URL; the woff2 falls through to embedded, so
			// no false-positive warning (Linus's objection to intent-guessing).
			name: "overlay keeps Inter — resolves via embedded fall-through",
			staticFS: func() http.FileSystem {
				local := fstest.MapFS{"css/fonts.css": &fstest.MapFile{Data: []byte("@font-face{}")}}
				return newOverlayFS(http.FS(local), embeddedStatic(), discardLogger())
			},
			url:       interURL,
			wantWarns: 0,
		},
		{
			name: "swapped font present in overlay",
			staticFS: func() http.FileSystem {
				local := fstest.MapFS{"fonts/space-mono/space-mono.woff2": &fstest.MapFile{Data: []byte("MONO")}}
				return newOverlayFS(http.FS(local), embeddedStatic(), discardLogger())
			},
			url:       monoURL,
			wantWarns: 0,
		},
		{
			name:      "swapped font missing everywhere — warns",
			staticFS:  embeddedStatic,
			url:       monoURL,
			wantWarns: 1,
		},
		{
			name:      "external CDN URL — not our FS, silent",
			staticFS:  embeddedStatic,
			url:       "https://cdn.example.com/font.woff2",
			wantWarns: 0,
		},
		{
			name:      "empty disables preload — nothing to check",
			staticFS:  embeddedStatic,
			url:       "",
			wantWarns: 0,
		},
		{
			name:      "non-/static absolute path — proxy-served, operator-owned, silent",
			staticFS:  embeddedStatic,
			url:       "/assets/font.woff2",
			wantWarns: 0,
		},
		{
			// Relative URL resolves against the page path → 404s off the
			// homepage; silently reintroduces #124. Skips the /static check.
			name:      "relative URL (no leading slash) — warns",
			staticFS:  embeddedStatic,
			url:       "static/fonts/inter/inter-latin.woff2",
			wantWarns: 1,
		},
		{
			// Open succeeds on a dir, but gin.OnlyFilesFS 404s it at serve.
			name:      "resolves to a directory — warns",
			staticFS:  embeddedStatic,
			url:       "/static/fonts/inter",
			wantWarns: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &captureHandler{}
			verifyFontPreloadResolves(tt.staticFS(), tt.url, slog.New(capture))
			if got := len(capture.warnings()); got != tt.wantWarns {
				t.Errorf("warnings = %d, want %d (warns: %v)", got, tt.wantWarns, capture.warnings())
			}
		})
	}
}

// TestVerifyFontPreloadResolves_RealEmbeddedDefault pins the flagship invariant:
// the shipped default FONT_PRELOAD_URL resolves against the *real* embedded
// static FS, so a stock deployment never emits a spurious startup warning.
func TestVerifyFontPreloadResolves_RealEmbeddedDefault(t *testing.T) {
	staticSub, err := fs.Sub(web.Assets, "static")
	if err != nil {
		t.Fatalf("fs.Sub(web.Assets, static): %v", err)
	}
	capture := &captureHandler{}
	verifyFontPreloadResolves(http.FS(staticSub), "/static/fonts/inter/inter-latin.woff2", slog.New(capture))
	if got := capture.warnings(); len(got) != 0 {
		t.Errorf("default URL must resolve in embedded assets; got warnings: %v", got)
	}
}
