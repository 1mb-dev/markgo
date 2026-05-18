package serve

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
)

func TestOverlayFS_Open(t *testing.T) {
	local := fstest.MapFS{
		"override.txt": &fstest.MapFile{Data: []byte("LOCAL")},
		"both.txt":     &fstest.MapFile{Data: []byte("LOCAL-WINS")},
	}
	embedded := fstest.MapFS{
		"embedded-only.txt": &fstest.MapFile{Data: []byte("EMBEDDED")},
		"both.txt":          &fstest.MapFile{Data: []byte("EMBEDDED-LOSES")},
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"local-only path serves local", "override.txt", "LOCAL", false},
		{"embedded-only path falls back", "embedded-only.txt", "EMBEDDED", false},
		{"both exist, local wins", "both.txt", "LOCAL-WINS", false},
		{"missing in both returns error", "nowhere.txt", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := newOverlayFS(http.FS(local), http.FS(embedded), discardLogger())
			f, err := o.Open(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Open(%q): want error, got nil", tc.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open(%q): unexpected error: %v", tc.path, err)
			}
			defer f.Close()
			body, _ := io.ReadAll(f)
			if string(body) != tc.want {
				t.Errorf("Open(%q): got %q, want %q", tc.path, string(body), tc.want)
			}
		})
	}
}

// TestOverlayFS_PathologicalPaths pins behavior for hostile inputs: no panic,
// any clean error/fallback result is acceptable.
func TestOverlayFS_PathologicalPaths(t *testing.T) {
	local := fstest.MapFS{}
	embedded := fstest.MapFS{"ok.txt": &fstest.MapFile{Data: []byte("E")}}
	o := newOverlayFS(http.FS(local), http.FS(embedded), discardLogger())

	cases := []string{
		"",
		"/",
		"../etc/passwd",
		"./ok.txt",
		"\x00null",
		strings.Repeat("a", 4096),
		"ok.txt/../ok.txt",
	}
	for _, p := range cases {
		t.Run("path="+truncate(p, 40), func(t *testing.T) {
			f, err := o.Open(p)
			if f != nil {
				_ = f.Close()
			}
			_ = err // any outcome (file or err) is OK; no panic is the contract
		})
	}
}

// TestDirExists pins the existing helper's behavior; we're touching the call
// site, so close the zero-coverage gap.
func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "afile")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"existing dir", dir, true},
		{"existing file (not dir)", filePath, false},
		{"missing path", filepath.Join(dir, "nope"), false},
		{"empty string", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dirExists(tc.path); got != tc.want {
				t.Errorf("dirExists(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestStaticMount_OverlayIntegration(t *testing.T) {
	embedded := fstest.MapFS{
		"css/embedded-only.css": &fstest.MapFile{Data: []byte("body{}")},
		"js/shared.js":          &fstest.MapFile{Data: []byte("EMBEDDED-JS")},
		"sw.js":                 &fstest.MapFile{Data: []byte("EMBEDDED-SW")},
	}
	localDir := t.TempDir()
	mustWrite(t, filepath.Join(localDir, "js", "shared.js"), "LOCAL-JS")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	overlay := newOverlayFS(http.Dir(localDir), http.FS(embedded), discardLogger())
	r.StaticFS("/static", &gin.OnlyFilesFS{FileSystem: overlay})
	// New signature: localFS for raw-passthrough, substituted body fallback.
	// Test substituted body uses literal "EMBEDDED-SW" so existing assertions hold.
	swHandler := serveSwJs(http.Dir(localDir), []byte("EMBEDDED-SW"), time.Now())
	r.GET("/sw.js", swHandler)
	r.HEAD("/sw.js", swHandler)

	srv := httptest.NewServer(r)
	defer srv.Close()

	type want struct {
		status            int
		body              string // "" = skip body assert (HEAD)
		contentTypeSubstr string // "" = skip; accept either text/javascript or application/javascript
	}
	cases := []struct {
		method string
		path   string
		want   want
	}{
		{"GET", "/static/js/shared.js", want{200, "LOCAL-JS", ""}},
		{"GET", "/static/css/embedded-only.css", want{200, "body{}", ""}},
		{"GET", "/sw.js", want{200, "EMBEDDED-SW", "javascript"}},
		{"HEAD", "/static/js/shared.js", want{200, "", ""}},
		{"HEAD", "/static/css/embedded-only.css", want{200, "", ""}},
		{"HEAD", "/sw.js", want{200, "", "javascript"}},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, srv.URL+tc.path, http.NoBody)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want.status {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.want.status)
			}
			if tc.want.contentTypeSubstr != "" {
				ct := resp.Header.Get("Content-Type")
				if !strings.Contains(ct, tc.want.contentTypeSubstr) {
					t.Errorf("Content-Type %q, want substring %q", ct, tc.want.contentTypeSubstr)
				}
			}
			if tc.want.body != "" {
				body, _ := io.ReadAll(resp.Body)
				if string(body) != tc.want.body {
					t.Errorf("body %q, want %q", string(body), tc.want.body)
				}
			}
		})
	}

	// After-the-fact: drop a local sw.js and verify it wins.
	mustWrite(t, filepath.Join(localDir, "sw.js"), "LOCAL-SW")
	resp, err := http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatalf("GET /sw.js (after local drop): %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "LOCAL-SW" {
		t.Errorf("after dropping local sw.js: got %q, want LOCAL-SW", string(body))
	}
}

// TestStaticMount_NoDirectoryListings_BothBranches: overlay AND embedded-only
// must suppress directory listings post-fix.
func TestStaticMount_NoDirectoryListings_BothBranches(t *testing.T) {
	embedded := fstest.MapFS{"css/a.css": &fstest.MapFile{Data: []byte("a")}}
	embFS := http.FS(embedded)

	t.Run("overlay branch", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		overlay := newOverlayFS(http.Dir(t.TempDir()), embFS, discardLogger())
		r.StaticFS("/static", &gin.OnlyFilesFS{FileSystem: overlay})
		srv := httptest.NewServer(r)
		defer srv.Close()

		for _, method := range []string{"GET", "HEAD"} {
			req, _ := http.NewRequest(method, srv.URL+"/static/css/", http.NoBody)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s /static/css/: %v", method, err)
			}
			resp.Body.Close()
			if resp.StatusCode != 404 {
				t.Errorf("%s /static/css/: status %d, want 404 (no listing)", method, resp.StatusCode)
			}
		}
	})

	t.Run("embedded-only branch", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.StaticFS("/static", &gin.OnlyFilesFS{FileSystem: embFS})
		srv := httptest.NewServer(r)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/static/css/")
		if err != nil {
			t.Fatalf("GET /static/css/: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Errorf("embedded-only /static/css/: status %d, want 404", resp.StatusCode)
		}
	})
}

// TestStaticMount_SwJsDirectoryFallsBackToEmbedded: if an operator
// misconfigures <STATIC_PATH>/sw.js as a directory rather than a file,
// the sw.js handler falls back to the substituted embedded body rather
// than 404'ing. More resilient than the pre-v3.17.0 behavior (which
// returned 404) — operator misconfig no longer breaks SW registration.
func TestStaticMount_SwJsDirectoryFallsBackToEmbedded(t *testing.T) {
	localDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localDir, "sw.js"), 0o755); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sw.js", serveSwJs(http.Dir(localDir), []byte("EMBEDDED-FALLBACK"), time.Now()))

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatalf("GET /sw.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET /sw.js (dir at <STATIC_PATH>/sw.js): status %d, want 200 (falls back to embedded)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "EMBEDDED-FALLBACK" {
		t.Errorf("body %q, want EMBEDDED-FALLBACK", string(body))
	}
}

// TestStaticMount_SymlinkTraversal pins http.Dir's symlink behavior (follows
// by default). Documents current state for future containment hardening.
func TestStaticMount_SymlinkTraversal(t *testing.T) {
	localDir := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.txt"), "SECRET")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(localDir, "escape.txt")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	embedded := fstest.MapFS{}
	overlay := newOverlayFS(http.Dir(localDir), http.FS(embedded), discardLogger())
	r.StaticFS("/static", &gin.OnlyFilesFS{FileSystem: overlay})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/escape.txt")
	if err != nil {
		t.Fatalf("GET /static/escape.txt: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	t.Logf("symlink target served: status=%d body=%q (http.Dir follows symlinks; pin for future hardening)",
		resp.StatusCode, string(body))
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
