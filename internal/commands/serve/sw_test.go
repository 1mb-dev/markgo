package serve

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
)

// captureHandler is a slog.Handler that records each entry's level + message
// for assertions. Race-safe so it works under t.Parallel and httptest.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle takes slog.Record by value per the slog.Handler interface contract;
// the heavyParam lint warning here is a false positive for an interface impl.
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) warnings() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.records))
	for i := range h.records {
		if h.records[i].Level == slog.LevelWarn {
			out = append(out, h.records[i].Message)
		}
	}
	return out
}

// TestSwCacheVersion: the three cases that determine cache-name shape —
// stamped build, "dev" (or empty) build, and leading-v strip.
func TestSwCacheVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"semver with leading v", "v3.17.0", "3.17.0"},
		{"semver without leading v", "3.17.0", "3.17.0"},
		{"empty falls back to dev", "", "dev"},
		{"literal v alone strips to empty then dev", "v", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := swCacheVersion(tc.in); got != tc.want {
				t.Errorf("swCacheVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestLoadServiceWorker_SubstitutesPlaceholder: the happy path — embedded
// sw.js has the placeholder, it gets substituted to the version string.
func TestLoadServiceWorker_SubstitutesPlaceholder(t *testing.T) {
	embedded := fstest.MapFS{
		"sw.js": &fstest.MapFile{Data: []byte("const CACHE_VERSION = '__MARKGO_CACHE_VERSION__';")},
	}
	body, _, err := loadServiceWorker(embedded, "3.17.0")
	if err != nil {
		t.Fatalf("loadServiceWorker: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "const CACHE_VERSION = '3.17.0';") {
		t.Errorf("body did not contain substituted version: %q", got)
	}
	if strings.Contains(got, "__MARKGO_CACHE_VERSION__") {
		t.Errorf("placeholder still present after substitution: %q", got)
	}
}

// TestLoadServiceWorker_FailsLoudOnMissingPlaceholder: build invariant —
// if a future templates refactor drops the placeholder, startup fails
// rather than silently shipping un-versioned cache names.
func TestLoadServiceWorker_FailsLoudOnMissingPlaceholder(t *testing.T) {
	embedded := fstest.MapFS{
		"sw.js": &fstest.MapFile{Data: []byte("const CACHE_VERSION = 7;")},
	}
	_, _, err := loadServiceWorker(embedded, "3.17.0")
	if err == nil {
		t.Fatal("loadServiceWorker: want error for missing placeholder, got nil")
	}
	if !strings.Contains(err.Error(), "__MARKGO_CACHE_VERSION__") {
		t.Errorf("error %q should mention placeholder name", err)
	}
}

// TestLoadServiceWorker_FailsOnMissingFile: embedded read failure is a
// startup-only error condition (build invariant).
func TestLoadServiceWorker_FailsOnMissingFile(t *testing.T) {
	embedded := fstest.MapFS{}
	_, _, err := loadServiceWorker(embedded, "3.17.0")
	if err == nil {
		t.Fatal("loadServiceWorker: want error for missing sw.js, got nil")
	}
}

// TestServeSwJs_EmbeddedSubstitutesVersion: with no overlay, served body
// is the substituted embedded body. Equivalent to a deployment without
// STATIC_PATH set.
func TestServeSwJs_EmbeddedSubstitutesVersion(t *testing.T) {
	substituted := []byte("const CACHE_VERSION = '3.17.0';")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sw.js", serveSwJs(nil, substituted, time.Now(), discardLogger()))
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatalf("GET /sw.js: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, substituted) {
		t.Errorf("body %q, want %q", string(body), substituted)
	}
}

// TestServeSwJs_DevFallbackInCacheNames: with empty version → cache names
// embed "dev" (via swCacheVersion fallback). Composes the two helpers.
func TestServeSwJs_DevFallbackInCacheNames(t *testing.T) {
	embedded := fstest.MapFS{
		"sw.js": &fstest.MapFile{Data: []byte("const CACHE_VERSION = '__MARKGO_CACHE_VERSION__';")},
	}
	body, modTime, err := loadServiceWorker(embedded, swCacheVersion(""))
	if err != nil {
		t.Fatalf("loadServiceWorker: %v", err)
	}
	if !strings.Contains(string(body), "'dev'") {
		t.Errorf("expected 'dev' fallback in body, got %q", string(body))
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sw.js", serveSwJs(nil, body, modTime, discardLogger()))
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatalf("GET /sw.js: %v", err)
	}
	defer resp.Body.Close()
	served, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(served), "'dev'") {
		t.Errorf("served body missing 'dev': %q", string(served))
	}
}

// TestServeSwJs_OverlayDirectoryEmitsWarning: operator misconfig (directory
// at <STATIC_PATH>/sw.js) falls back to embedded AND emits slog.Warn so
// the regression isn't silent — mirroring overlayFS.Open's contract at
// lines 41-46 and the brand-logo template-overlay pattern.
func TestServeSwJs_OverlayDirectoryEmitsWarning(t *testing.T) {
	localDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localDir, "sw.js"), 0o755); err != nil {
		t.Fatal(err)
	}
	capture := &captureHandler{}
	logger := slog.New(capture)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sw.js", serveSwJs(http.Dir(localDir), []byte("FALLBACK"), time.Now(), logger))
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatalf("GET /sw.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status %d, want 200 (falls back to embedded)", resp.StatusCode)
	}

	warns := capture.warnings()
	if len(warns) == 0 {
		t.Fatal("expected slog.Warn on directory overlay; got 0 warnings")
	}
	if !strings.Contains(warns[0], "directory") {
		t.Errorf("warning %q should mention directory", warns[0])
	}
}

// TestServeSwJs_OverlayBypassesSubstitution: when an operator drops
// <STATIC_PATH>/sw.js, raw bytes serve verbatim — no substitution
// attempted on operator-owned content. Operator owns their cache version.
func TestServeSwJs_OverlayBypassesSubstitution(t *testing.T) {
	localDir := t.TempDir()
	operatorContent := "// Operator's own sw.js with their own CACHE_VERSION = 42"
	mustWrite(t, filepath.Join(localDir, "sw.js"), operatorContent)

	substituted := []byte("EMBEDDED-WITH-SUBSTITUTION-MUST-NOT-LEAK")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sw.js", serveSwJs(http.Dir(localDir), substituted, time.Now(), discardLogger()))
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatalf("GET /sw.js: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != operatorContent {
		t.Errorf("operator overlay not honored: body %q, want %q", string(body), operatorContent)
	}
}
