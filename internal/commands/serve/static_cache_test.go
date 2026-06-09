package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestStaticRevalidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fsys := fstest.MapFS{
		"css/main.css":  {Data: []byte("body{}")},
		"js/app.js":     {Data: []byte("console.log(1)")},
		"fonts/x.woff2": {Data: []byte("FONT")},
		"img/y.png":     {Data: []byte("PNG")},
	}
	r := gin.New()
	g := r.Group("/static", staticRevalidate("3.24.0"))
	g.StaticFS("/", &gin.OnlyFilesFS{FileSystem: http.FS(fsys)})

	do := func(path, ifNoneMatch string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("css gets version ETag + no-cache + body", func(t *testing.T) {
		w := do("/static/css/main.css", "")
		if w.Code != http.StatusOK {
			t.Fatalf("code=%d", w.Code)
		}
		if got := w.Header().Get("ETag"); got != `"3.24.0"` {
			t.Errorf("ETag=%q, want \"3.24.0\"", got)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("Cache-Control=%q, want no-cache", got)
		}
		if w.Body.String() != "body{}" {
			t.Errorf("body=%q", w.Body.String())
		}
	})

	t.Run("js gets version ETag", func(t *testing.T) {
		if got := do("/static/js/app.js", "").Header().Get("ETag"); got != `"3.24.0"` {
			t.Errorf("ETag=%q", got)
		}
	})

	t.Run("matching If-None-Match returns 304 with no body", func(t *testing.T) {
		w := do("/static/css/main.css", `"3.24.0"`)
		if w.Code != http.StatusNotModified {
			t.Fatalf("code=%d, want 304", w.Code)
		}
		if w.Body.Len() != 0 {
			t.Errorf("304 must have no body, got %q", w.Body.String())
		}
	})

	t.Run("stale If-None-Match returns full 200", func(t *testing.T) {
		w := do("/static/css/main.css", `"3.23.0"`)
		if w.Code != http.StatusOK {
			t.Fatalf("code=%d, want 200", w.Code)
		}
		if w.Body.String() != "body{}" {
			t.Errorf("body=%q", w.Body.String())
		}
	})

	t.Run("fonts and images are untouched (no version ETag, still served)", func(t *testing.T) {
		for _, p := range []string{"/static/fonts/x.woff2", "/static/img/y.png"} {
			w := do(p, "")
			if w.Code != http.StatusOK {
				t.Errorf("%s code=%d", p, w.Code)
			}
			if got := w.Header().Get("ETag"); got == `"3.24.0"` {
				t.Errorf("%s must not get the version ETag", p)
			}
		}
	})
}
