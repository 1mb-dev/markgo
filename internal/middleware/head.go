package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DiscardBodyOnHEAD wraps the response writer for HEAD requests so handlers
// can run the GET path unchanged while the body bytes are discarded before
// flush. Status code, headers, and Content-Type are preserved.
//
// Mount globally with engine.Use(). For routes that should accept HEAD,
// register HEAD alongside GET (see registerGET in serve.command). HEAD on
// a route without a HEAD registration still returns 404 — gin's router is
// method-strict, by design.
//
// RFC 9110 §9.3.2: a HEAD response SHOULD be identical to GET minus body.
// Note: this implementation reports written bytes as len(b) but does not
// forward them, so Content-Length auto-detection on HEAD returns 0 rather
// than the GET-equivalent size. Uptime probes and conditional requests
// (If-Modified-Since against Last-Modified, ETag) work as expected;
// applications that rely on HEAD Content-Length parity will need extra
// handling per-route.
func DiscardBodyOnHEAD() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodHead {
			c.Writer = &headResponseWriter{ResponseWriter: c.Writer}
		}
		c.Next()
	}
}

// headResponseWriter discards body bytes while passing everything else
// through to the underlying gin.ResponseWriter (status code, headers,
// flush, hijack, push).
//
// Write and WriteString must call WriteHeaderNow on the underlying writer
// before returning. Gin caches the status via WriteHeader and flushes it
// to the network on the first actual Write — if we discard bytes without
// triggering the flush, the cached status never reaches the client and
// the response defaults to 200 even on errors (e.g. NoRoute's 404).
type headResponseWriter struct {
	gin.ResponseWriter
}

func (w *headResponseWriter) Write(b []byte) (int, error) {
	w.WriteHeaderNow()
	return len(b), nil
}

func (w *headResponseWriter) WriteString(s string) (int, error) {
	w.WriteHeaderNow()
	return len(s), nil
}
