package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// abortWithError sends a content-negotiated error response and aborts the request.
// JSON for API clients (Accept: application/json), styled HTML for browsers.
// Every middleware error path must use this instead of bare AbortWithStatus().
func abortWithError(c *gin.Context, status int, message string) {
	if wantsJSON(c) {
		c.AbortWithStatusJSON(status, gin.H{"error": message})
		return
	}
	c.Data(status, "text/html; charset=utf-8", []byte(errorHTML(status, message)))
	c.Abort()
}

// wantsJSON returns true if the client prefers JSON responses.
func wantsJSON(c *gin.Context) bool {
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "application/json")
}

// acceptsHTML reports whether the caller explicitly asked for HTML.
//
// Deliberately not the negation of wantsJSON. Use this to decide whether a caller
// can be shown an HTML affordance such as the login overlay: the answer must be no
// for `Accept: */*`, an absent Accept header, or anything else non-committal, so
// that a caller which cannot render the affordance is refused rather than handed
// whatever the handler would have produced.
func acceptsHTML(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "text/html")
}

// errorHTML returns a minimal self-contained HTML error page.
// Does not use the template engine — middleware must be dependency-free.
func errorHTML(status int, message string) string {
	title := http.StatusText(status)
	if title == "" {
		title = "Error"
	}
	return fmt.Sprintf(`<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%d %s</title><style>body{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#fafafa;color:#333}@media(prefers-color-scheme:dark){body{background:#1a1a1a;color:#e0e0e0}}.e{text-align:center;padding:2rem}.e h1{font-size:1.5rem;margin:0 0 .5rem}.e p{color:#666;margin:0 0 1.5rem}@media(prefers-color-scheme:dark){.e p{color:#999}}.e a{color:#4a90d9;text-decoration:none;margin:0 .75rem}@media(prefers-color-scheme:dark){.e a{color:#6db3f2}}.e a:hover{text-decoration:underline}</style></head><body><div class="e"><h1>%d %s</h1><p>%s</p><a href="/">Back to feed</a></div></body></html>`,
		status, title, status, title, message)
}
