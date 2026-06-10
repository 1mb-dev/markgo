package serve

import (
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/middleware"
)

// bigCSS is large and repetitive so gzip/brotli both beat identity; tinyCSS is
// small enough that compression overhead exceeds the savings (no variant kept).
var (
	bigCSS  = strings.Repeat(".cls{color:red;background:blue}\n", 300)
	tinyCSS = "a{}"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"css/main.css":  {Data: []byte(bigCSS)},
		"css/tiny.css":  {Data: []byte(tinyCSS)},
		"js/app.js":     {Data: []byte(bigCSS)},
		"fonts/x.woff2": {Data: []byte("FONTBYTES")},
		"img/y.png":     {Data: []byte("PNGBYTES")},
	}
}

func newStaticRouter(fsys fstest.MapFS, version string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// SmartCacheHeaders mirrors production: it sets Cache-Control: max-age=3600
	// globally before the handler runs, so every Cache-Control: no-cache
	// assertion below proves precompressedStatic *overrides* it, not just sets it.
	r.Use(middleware.SmartCacheHeaders())
	r.Use(middleware.DiscardBodyOnHEAD())
	table := buildPrecompressTable(fsys, version, discardLogger())
	g := r.Group("/static")
	g.Use(precompressedStatic(table, nil)) // embedded-only: no overlay
	g.StaticFS("/", &gin.OnlyFilesFS{FileSystem: http.FS(fsys)})
	return r
}

// newOverlayStaticRouter mirrors mountStatic's overlay mode: an embedded table
// plus an overlayFS(local, embedded) used both for the shadow check (in the
// handler) and the StaticFS fall-through — the same instance, so the shadow
// decision and the served bytes can't disagree.
func newOverlayStaticRouter(embedded, local fstest.MapFS, version string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.SmartCacheHeaders())
	r.Use(middleware.DiscardBodyOnHEAD())
	table := buildPrecompressTable(embedded, version, discardLogger())
	overlay := newOverlayFS(http.FS(local), http.FS(embedded), discardLogger())
	g := r.Group("/static")
	g.Use(precompressedStatic(table, overlay))
	g.StaticFS("/", &gin.OnlyFilesFS{FileSystem: overlay})
	return r
}

func req(r *gin.Engine, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	rq := httptest.NewRequest(method, path, http.NoBody)
	for k, v := range headers {
		rq.Header.Set(k, v)
	}
	r.ServeHTTP(w, rq)
	return w
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return out
}

func unbrotli(t *testing.T, b []byte) []byte {
	t.Helper()
	out, err := io.ReadAll(brotli.NewReader(strings.NewReader(string(b))))
	if err != nil {
		t.Fatalf("unbrotli: %v", err)
	}
	return out
}

func TestPrecompressTable_Build(t *testing.T) {
	table := buildPrecompressTable(testFS(), "3.25.0", discardLogger())

	if _, ok := table["fonts/x.woff2"]; ok {
		t.Error("non-css/js must not be in the precompress table")
	}
	if _, ok := table["img/y.png"]; ok {
		t.Error("images must not be in the precompress table")
	}

	main, ok := table["css/main.css"]
	if !ok {
		t.Fatal("css/main.css missing from table")
	}
	if main.contentType != "text/css; charset=utf-8" {
		t.Errorf("css content-type=%q", main.contentType)
	}
	if string(main.identity.body) != bigCSS {
		t.Error("identity body mismatch")
	}
	if len(main.encoded) != 2 {
		t.Fatalf("want 2 encoded variants (br,gzip), got %d", len(main.encoded))
	}
	// best-first: brotli then gzip.
	if main.encoded[0].encoding != "br" || main.encoded[1].encoding != "gzip" {
		t.Errorf("variant order=%q,%q want br,gzip", main.encoded[0].encoding, main.encoded[1].encoding)
	}

	// Distinct per-variant strong ETags.
	if main.identity.etag != `"3.25.0"` {
		t.Errorf("identity etag=%q", main.identity.etag)
	}
	if main.encoded[0].etag != `"3.25.0-br"` {
		t.Errorf("br etag=%q", main.encoded[0].etag)
	}
	if main.encoded[1].etag != `"3.25.0-gzip"` {
		t.Errorf("gzip etag=%q", main.encoded[1].etag)
	}

	// Stored compressed bytes must round-trip to the original.
	if got := unbrotli(t, main.encoded[0].body); string(got) != bigCSS {
		t.Error("brotli variant does not decompress to identity")
	}
	if got := gunzip(t, main.encoded[1].body); string(got) != bigCSS {
		t.Error("gzip variant does not decompress to identity")
	}

	js := table["js/app.js"]
	if js == nil || js.contentType != "text/javascript; charset=utf-8" {
		t.Errorf("js content-type wrong: %+v", js)
	}
}

func TestPrecompressTable_TinyFileKeepsIdentityOnly(t *testing.T) {
	table := buildPrecompressTable(testFS(), "3.25.0", discardLogger())
	tiny, ok := table["css/tiny.css"]
	if !ok {
		t.Fatal("tiny.css should still be in the table (served identity)")
	}
	if len(tiny.encoded) != 0 {
		t.Errorf("tiny file should keep no compressed variant, got %d", len(tiny.encoded))
	}
}

func TestPrecompressedStatic_Negotiation(t *testing.T) {
	r := newStaticRouter(testFS(), "3.25.0")
	cases := []struct {
		name       string
		accept     string
		wantEnc    string // expected Content-Encoding ("" = identity)
		wantETag   string
		wantDecode func(*testing.T, []byte) []byte
	}{
		{"no accept-encoding → identity", "", "", `"3.25.0"`, nil},
		{"br,gzip → br (best-first)", "br, gzip", "br", `"3.25.0-br"`, unbrotli},
		{"gzip only → gzip", "gzip", "gzip", `"3.25.0-gzip"`, gunzip},
		{"br;q=0,gzip → gzip (br refused)", "br;q=0, gzip", "gzip", `"3.25.0-gzip"`, gunzip},
		{"gzip;q=0 alone → identity", "gzip;q=0", "", `"3.25.0"`, nil},
		{"identity only → identity", "identity", "", `"3.25.0"`, nil},
		{"wildcard → br (best-first)", "*", "br", `"3.25.0-br"`, unbrotli},
		{"deflate (unsupported) → identity", "deflate", "", `"3.25.0"`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := map[string]string{}
			if tc.accept != "" {
				h["Accept-Encoding"] = tc.accept
			}
			w := req(r, http.MethodGet, "/static/css/main.css", h)
			if w.Code != http.StatusOK {
				t.Fatalf("code=%d", w.Code)
			}
			if got := w.Header().Get("Content-Encoding"); got != tc.wantEnc {
				t.Errorf("Content-Encoding=%q want %q", got, tc.wantEnc)
			}
			if got := w.Header().Get("ETag"); got != tc.wantETag {
				t.Errorf("ETag=%q want %q", got, tc.wantETag)
			}
			if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary=%q want Accept-Encoding", got)
			}
			if got := w.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control=%q want no-cache", got)
			}
			body := w.Body.Bytes()
			if cl := w.Header().Get("Content-Length"); cl != strconv.Itoa(len(body)) {
				t.Errorf("Content-Length=%q want %d", cl, len(body))
			}
			if tc.wantDecode != nil {
				if got := tc.wantDecode(t, body); string(got) != bigCSS {
					t.Error("served compressed body does not decode to original")
				}
			} else if string(body) != bigCSS {
				t.Error("identity body mismatch")
			}
		})
	}
}

func TestPrecompressedStatic_TinyFileServesIdentityEvenWhenGzipAccepted(t *testing.T) {
	r := newStaticRouter(testFS(), "3.25.0")
	w := req(r, http.MethodGet, "/static/css/tiny.css", map[string]string{"Accept-Encoding": "br, gzip"})
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("tiny file must serve identity, got Content-Encoding=%q", got)
	}
	if w.Body.String() != tinyCSS {
		t.Errorf("body=%q", w.Body.String())
	}
}

func TestPrecompressedStatic_ETag304PerVariant(t *testing.T) {
	r := newStaticRouter(testFS(), "3.25.0")
	cases := []struct {
		name   string
		accept string
		etag   string
	}{
		{"identity 304", "identity", `"3.25.0"`},
		{"gzip 304", "gzip", `"3.25.0-gzip"`},
		{"brotli 304", "br", `"3.25.0-br"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := req(r, http.MethodGet, "/static/css/main.css", map[string]string{
				"Accept-Encoding": tc.accept,
				"If-None-Match":   tc.etag,
			})
			if w.Code != http.StatusNotModified {
				t.Fatalf("code=%d want 304", w.Code)
			}
			if w.Body.Len() != 0 {
				t.Errorf("304 must have empty body, got %q", w.Body.String())
			}
			if got := w.Header().Get("ETag"); got != tc.etag {
				t.Errorf("304 ETag=%q want %q", got, tc.etag)
			}
			if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("304 Vary=%q", got)
			}
		})
	}
}

// A client that cached the gzip variant but now refuses gzip must NOT get a 304
// off the gzip ETag — it must get a fresh identity 200. The per-variant ETag is
// what makes this correct.
func TestPrecompressedStatic_CrossEncodingNo304(t *testing.T) {
	r := newStaticRouter(testFS(), "3.25.0")
	w := req(r, http.MethodGet, "/static/css/main.css", map[string]string{
		"Accept-Encoding": "identity",
		"If-None-Match":   `"3.25.0-gzip"`,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d want 200 (gzip ETag must not 304 an identity response)", w.Code)
	}
	if got := w.Header().Get("ETag"); got != `"3.25.0"` {
		t.Errorf("ETag=%q want identity", got)
	}
	if w.Body.String() != bigCSS {
		t.Error("identity body mismatch")
	}
}

func TestPrecompressedStatic_HEAD(t *testing.T) {
	r := newStaticRouter(testFS(), "3.25.0")
	w := req(r, http.MethodHead, "/static/css/main.css", map[string]string{"Accept-Encoding": "gzip"})
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("HEAD must have no body, got %d bytes", w.Body.Len())
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("HEAD Content-Encoding=%q want gzip", got)
	}
	if got := w.Header().Get("ETag"); got != `"3.25.0-gzip"` {
		t.Errorf("HEAD ETag=%q", got)
	}
	// Content-Length reflects the encoded variant even though the body is discarded.
	if cl := w.Header().Get("Content-Length"); cl == "" || cl == "0" {
		t.Errorf("HEAD Content-Length=%q want the gzip variant length", cl)
	}
}

func TestPrecompressedStatic_FallThroughForNonTableFiles(t *testing.T) {
	r := newStaticRouter(testFS(), "3.25.0")
	for _, p := range []string{"/static/fonts/x.woff2", "/static/img/y.png"} {
		w := req(r, http.MethodGet, p, map[string]string{"Accept-Encoding": "br, gzip"})
		if w.Code != http.StatusOK {
			t.Errorf("%s code=%d", p, w.Code)
		}
		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("%s must not be compressed, got Content-Encoding=%q", p, got)
		}
		if got := w.Header().Get("ETag"); strings.HasPrefix(got, `"3.25.0`) {
			t.Errorf("%s must not get the version ETag, got %q", p, got)
		}
		if got := w.Header().Get("Vary"); got == "Accept-Encoding" {
			t.Errorf("%s must not get Vary from the precompress handler", p)
		}
		// Fall-through keeps the global SmartCacheHeaders value (not overridden).
		if got := w.Header().Get("Cache-Control"); got != "public, max-age=3600" {
			t.Errorf("%s Cache-Control=%q want global max-age=3600", p, got)
		}
	}
}

func TestParseAcceptEncoding(t *testing.T) {
	cases := []struct {
		header string
		coding string
		want   bool
	}{
		{"", "gzip", false},
		{"gzip", "gzip", true},
		{"gzip", "br", false},
		{"br, gzip", "br", true},
		{"gzip;q=0", "gzip", false},
		{"gzip;q=0.0", "gzip", false},
		{"gzip;q=0.001", "gzip", true},
		{"*", "br", true},
		{"*;q=0", "br", false},
		{"br;q=0, *", "br", false},         // explicit beats wildcard
		{"identity, *;q=0", "gzip", false}, // wildcard refuses unlisted
		{"GZIP", "gzip", true},             // case-insensitive token
		{" br ; q=1.0 ", "br", true},       // whitespace tolerance
		{"gzip;q=bogus", "gzip", true},     // malformed q → acceptable
	}
	for _, tc := range cases {
		got := parseAcceptEncoding(tc.header).accepts(tc.coding)
		if got != tc.want {
			t.Errorf("accepts(%q, %q)=%v want %v", tc.header, tc.coding, got, tc.want)
		}
	}
}

// --- overlay mode (#128): per-file shadow check ---

// THE load-bearing guard. A path the operator overlays must serve the OPERATOR's
// bytes (which differ from the embedded file), uncompressed, with no version
// ETag — never the embedded precompressed variant. Routes through the real gin
// group + StaticFS + overlayFS, so it also proves the shadow-check name form
// matches what StaticFS feeds overlayFS.Open. If it regresses, this fails loud.
func TestPrecompressedStatic_OverlayShadowedServesOperatorBytes(t *testing.T) {
	const operatorCSS = "/* OPERATOR OVERRIDE */body{color:hotpink}"
	embedded := fstest.MapFS{"css/main.css": {Data: []byte(bigCSS)}}
	local := fstest.MapFS{"css/main.css": {Data: []byte(operatorCSS)}}
	r := newOverlayStaticRouter(embedded, local, "3.25.1")

	w := req(r, http.MethodGet, "/static/css/main.css", map[string]string{"Accept-Encoding": "br, gzip"})
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if w.Body.String() != operatorCSS {
		t.Errorf("shadowed path served wrong bytes:\n got %q\nwant operator override", w.Body.String())
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("operator file must be served uncompressed, got Content-Encoding=%q", got)
	}
	if got := w.Header().Get("ETag"); strings.HasPrefix(got, `"3.25.1`) {
		t.Errorf("operator file must not carry the version ETag, got %q", got)
	}
}

// An embedded css/js the operator did NOT override still gets compression + the
// version ETag in overlay mode — the whole point of #128.
func TestPrecompressedStatic_OverlayUnshadowedServesVariant(t *testing.T) {
	embedded := fstest.MapFS{"css/main.css": {Data: []byte(bigCSS)}}
	local := fstest.MapFS{"img/logo.svg": {Data: []byte("<svg/>")}} // overrides something else
	r := newOverlayStaticRouter(embedded, local, "3.25.1")

	w := req(r, http.MethodGet, "/static/css/main.css", map[string]string{"Accept-Encoding": "br"})
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "br" {
		t.Errorf("unshadowed embedded css must stay brotli in overlay mode, got %q", got)
	}
	if got := w.Header().Get("ETag"); got != `"3.25.1-br"` {
		t.Errorf("ETag=%q want version br ETag", got)
	}
	if got := unbrotli(t, w.Body.Bytes()); string(got) != bigCSS {
		t.Error("served brotli variant does not decode to embedded css")
	}
}

// The shadow check is per request, not a startup snapshot: an overlay file added
// after the table is built is honored on the next hit.
func TestPrecompressedStatic_OverlayLiveAddDetected(t *testing.T) {
	const operatorCSS = "body{color:hotpink}"
	embedded := fstest.MapFS{"css/main.css": {Data: []byte(bigCSS)}}
	local := fstest.MapFS{} // empty at startup
	r := newOverlayStaticRouter(embedded, local, "3.25.1")

	if got := req(r, http.MethodGet, "/static/css/main.css",
		map[string]string{"Accept-Encoding": "br"}).Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("pre-add: want brotli variant, got %q", got)
	}

	local["css/main.css"] = &fstest.MapFile{Data: []byte(operatorCSS)} // operator adds override live

	w := req(r, http.MethodGet, "/static/css/main.css", map[string]string{"Accept-Encoding": "br"})
	if w.Body.String() != operatorCSS {
		t.Errorf("post-add: live overlay not detected, got %q want operator bytes", w.Body.String())
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("post-add: operator file must serve uncompressed, got %q", got)
	}
}

// permErrFS returns a non-ENOENT error for every Open (simulates EACCES).
type permErrFS struct{}

func (permErrFS) Open(string) (http.File, error) { return nil, fs.ErrPermission }

// A non-ENOENT overlay error counts as a claim: the handler must NOT serve the
// embedded variant; it falls through so overlayFS.Open emits its single warning
// and serves the embedded file uncompressed.
func TestPrecompressedStatic_OverlayNonENOENTFallsThrough(t *testing.T) {
	embedded := fstest.MapFS{"css/main.css": {Data: []byte(bigCSS)}}
	table := buildPrecompressTable(embedded, "3.25.1", discardLogger())
	overlay := newOverlayFS(permErrFS{}, http.FS(embedded), discardLogger())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.SmartCacheHeaders())
	g := r.Group("/static")
	g.Use(precompressedStatic(table, overlay))
	g.StaticFS("/", &gin.OnlyFilesFS{FileSystem: overlay})

	w := req(r, http.MethodGet, "/static/css/main.css", map[string]string{"Accept-Encoding": "br"})
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("non-ENOENT overlay error must fall through to uncompressed, got Content-Encoding=%q", got)
	}
	if w.Body.String() != bigCSS {
		t.Error("expected embedded bytes served uncompressed on overlay read error")
	}
}
