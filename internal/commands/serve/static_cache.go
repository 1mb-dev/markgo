package serve

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/etag"
)

// staticRevalidate makes CSS/JS revalidate instead of caching for a fixed window
// — fixing stale assets after a deploy without versioned URLs. The ETag is the
// build version: it changes exactly when a release ships (when embedded CSS/JS
// can change), so a client gets 304 within a version and fresh bytes after an
// upgrade. Comparison is per-URL, so a single version value across files is
// correct — a client only sends If-None-Match for a URL it has cached.
//
// Fonts and images are left untouched (they keep the upstream cache headers):
// they aren't churned by a deploy and aren't the staleness defect. Runs after
// the global SmartCacheHeaders, so for CSS/JS its no-cache replaces the blanket
// max-age; for everything else the blanket header stands.
func staticRevalidate(version string) gin.HandlerFunc {
	tag := `"` + version + `"` // strong: embedded bytes are fixed within a release
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if !strings.HasSuffix(p, ".css") && !strings.HasSuffix(p, ".js") {
			return // fonts/img/etc. — leave upstream headers untouched
		}
		c.Header("ETag", tag)
		c.Header("Cache-Control", "no-cache")
		if etag.Matches(c.GetHeader("If-None-Match"), tag) {
			c.AbortWithStatus(http.StatusNotModified)
		}
	}
}
