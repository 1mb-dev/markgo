package serve

import (
	"net/http"
	"testing"
	"testing/fstest"

	"log/slog"
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
			name:      "non-/static path — operator-owned, silent",
			staticFS:  embeddedStatic,
			url:       "/assets/font.woff2",
			wantWarns: 0,
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
