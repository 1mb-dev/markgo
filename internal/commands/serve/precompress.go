// Package serve — pre-compressed static asset serving.
//
// At startup (embedded-only deployments) the binary walks its embedded CSS/JS,
// compresses each file once with gzip and brotli, and holds the variants in
// memory. The precompressedStatic handler then negotiates Accept-Encoding per
// request and serves the matching variant — the work a reverse proxy would
// otherwise do, done once instead of per request, so the binary is
// self-sufficient even behind a proxy that doesn't compress (or compresses only
// gzip). HTML compression stays the proxy's job.
//
// This handler ALSO owns the build-version ETag/304 revalidation for CSS/JS
// (absorbed from the former staticRevalidate). Variant selection and ETag
// stamping are a single decision so the two can never disagree: each encoding
// carries its own strong validator ("<v>", "<v>-gzip", "<v>-br") alongside
// Vary: Accept-Encoding, which makes the cross-encoding cache-poisoning bug —
// a gzipped body sharing the identity ETag — unrepresentable.
//
// Gated to embedded-only deployments (mountStatic, !overlayActive): operator
// STATIC_PATH overlay files aren't known at startup and an operator can change
// one without a version bump, which a build-version ETag would 304 stale
// forever. Overlay deployments keep max-age=3600 and serve uncompressed via
// gin StaticFS — same gate the R2 revalidation used.
package serve

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/etag"
)

// staticURLPrefix is the route-group prefix /static is mounted under. The
// handler and the mountStatic route registration both key off this constant so
// they can never silently disagree (a renamed group with a stale TrimPrefix
// would drop every table lookup to the uncompressed fall-through).
const staticURLPrefix = "/static"

// compressibleTypes is the single source of truth for what precompressedStatic
// owns: map membership decides which extensions are pre-compressed, and the
// value is the Content-Type served. Hand-mapped, not mime.TypeByExtension —
// stdlib's table is platform-dependent and historically wrong for .js. Add an
// extension here to extend coverage.
var compressibleTypes = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
}

// variant is one pre-encoded representation of a static file.
type variant struct {
	body     []byte
	etag     string // strong validator, e.g. `"3.25.0-br"`
	encoding string // Content-Encoding token; "" for identity
}

// asset holds every encoding of one static file. encoded is ordered best-first
// (matching precompressEncoders) so negotiation returns the best acceptable
// match by linear scan.
type asset struct {
	contentType string
	identity    variant
	encoded     []variant
}

type precompressTable map[string]*asset

// encoder is one content-coding the binary pre-computes. The slice returned by
// precompressEncoders is the negotiation-preference order (best ratio first).
type encoder struct {
	name   string                      // Content-Encoding token: "br", "gzip"
	suffix string                      // ETag suffix inside the quotes: "-br", "-gzip"
	encode func([]byte) ([]byte, bool) // gzipEncode / brotliEncode
}

// precompressEncoders is the single registry of pre-computed codings, best
// first. To drop an encoder, remove its entry here and its implementation
// (brotli lives in precompress_brotli.go) — the serving path falls back to the
// remaining codings plus identity with no other change.
func precompressEncoders() []encoder {
	return []encoder{
		{name: "br", suffix: "-br", encode: brotliEncode},
		{name: "gzip", suffix: "-gzip", encode: gzipEncode},
	}
}

// gzipEncode compresses b at best compression. ok is false when the result is
// not smaller than the input, so the caller stores no variant that would only
// cost bytes to serve.
func gzipEncode(b []byte) (compressed []byte, ok bool) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, false
	}
	if _, err := w.Write(b); err != nil {
		return nil, false
	}
	if err := w.Close(); err != nil {
		return nil, false
	}
	if buf.Len() >= len(b) {
		return nil, false
	}
	return buf.Bytes(), true
}

// buildPrecompressTable walks fsys for compressible assets and pre-encodes each
// with every registered encoder. Runs synchronously at startup (before routes go
// live) so there is no half-warm window. Per-file resilient: a read or encode
// miss drops only that file from the table — it then falls through to gin
// StaticFS uncompressed rather than aborting the boot.
func buildPrecompressTable(fsys fs.FS, version string, logger *slog.Logger) precompressTable {
	table := precompressTable{}
	encoders := precompressEncoders()
	identityTag := strconv.Quote(version)

	walkErr := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Warn("precompress: walk error, skipping subtree", "path", path, "error", err)
			return nil
		}
		if d.IsDir() || !isCompressible(path) {
			return nil
		}
		body, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			logger.Warn("precompress: read failed; asset will serve uncompressed via StaticFS",
				"path", path, "error", readErr)
			return nil
		}
		a := &asset{
			contentType: contentTypeForStatic(path),
			identity:    variant{body: body, etag: identityTag},
		}
		for _, e := range encoders {
			if enc, ok := e.encode(body); ok {
				a.encoded = append(a.encoded, variant{
					body:     enc,
					etag:     strconv.Quote(version + e.suffix),
					encoding: e.name,
				})
			}
		}
		table[path] = a
		return nil
	})
	if walkErr != nil {
		logger.Warn("precompress: asset walk aborted; some assets serve uncompressed", "error", walkErr)
	}
	logPrecompressSummary(logger, table)
	return table
}

// logPrecompressSummary emits one boot line so an operator can confirm
// precompression ran and see the resident cost. Keyed by encoding name; a
// removed encoder simply reports 0.
func logPrecompressSummary(logger *slog.Logger, table precompressTable) {
	identityBytes := 0
	byEncoding := map[string]int{}
	for _, a := range table {
		identityBytes += len(a.identity.body)
		for _, v := range a.encoded {
			byEncoding[v.encoding] += len(v.body)
		}
	}
	logger.Info("Static precompression ready",
		"files", len(table),
		"identity_kb", identityBytes/1024,
		"gzip_kb", byEncoding["gzip"]/1024,
		"br_kb", byEncoding["br"]/1024)
}

// precompressedStatic serves a pre-encoded variant for any CSS/JS file in the
// table, negotiated against Accept-Encoding, with a per-variant strong ETag and
// 304 support. Requests for files not in the table (fonts, images, or a CSS/JS
// file that failed precompute) fall through untouched to gin StaticFS.
//
// overlay is the live *overlayFS when STATIC_PATH is active, else nil. When set,
// a path the operator overlay claims is served from the operator's file (via the
// StaticFS fall-through) rather than the embedded variant — the table holds only
// embedded bytes, so serving a variant for a shadowed path would serve stale
// content. The check is per request, mirroring overlayFS.Open, so an overlay
// file added after startup is honored on the next hit. Unshadowed embedded
// css/js still get full compression + the version ETag even in overlay mode.
func precompressedStatic(table precompressTable, overlay *overlayFS) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimPrefix(c.Request.URL.Path, staticURLPrefix+"/")
		a, ok := table[key]
		if !ok {
			return // not a precompressed asset — gin StaticFS handles it
		}
		if overlay != nil && overlay.localClaims("/"+key) {
			return // operator overrides this path — StaticFS serves their file
		}
		v := negotiateVariant(a, c.GetHeader("Accept-Encoding"))

		h := c.Writer.Header()
		h.Set("Vary", "Accept-Encoding")
		h.Set("Cache-Control", "no-cache")
		h.Set("ETag", v.etag)
		if etag.Matches(c.GetHeader("If-None-Match"), v.etag) {
			c.AbortWithStatus(http.StatusNotModified)
			return
		}
		if v.encoding != "" {
			h.Set("Content-Encoding", v.encoding)
		}
		// Encoded variants are served whole — no byte-range into a compressed
		// stream. Content-Length is set explicitly so HEAD reports parity (the
		// HEAD wrapper discards the body, not the header). c.Data sets
		// Content-Type and writes the body (error-handled internally).
		h.Set("Content-Length", strconv.Itoa(len(v.body)))
		c.Data(http.StatusOK, a.contentType, v.body)
		c.Abort() // fully handled; do not fall through to StaticFS
	}
}

// negotiateVariant returns the best acceptable encoded variant, falling back to
// identity. encoded is best-first, so the first acceptable match wins.
func negotiateVariant(a *asset, acceptEncoding string) variant {
	accept := parseAcceptEncoding(acceptEncoding)
	for _, v := range a.encoded {
		if accept.accepts(v.encoding) {
			return v
		}
	}
	return a.identity
}

func isCompressible(p string) bool {
	_, ok := compressibleTypes[path.Ext(p)]
	return ok
}

func contentTypeForStatic(p string) string {
	if ct, ok := compressibleTypes[path.Ext(p)]; ok {
		return ct
	}
	return "application/octet-stream"
}

// acceptSet is a parsed Accept-Encoding header: explicit per-coding q-values
// plus an optional "*" wildcard default.
type acceptSet struct {
	q        map[string]float64
	wildcard float64
	hasWild  bool
}

// accepts reports whether the client will take the named coding. RFC 9110
// §12.5.3: an explicit q=0 refuses a coding; "*" supplies the default for
// codings not listed; absent the coding and any wildcard, it is not acceptable.
func (s acceptSet) accepts(coding string) bool {
	if q, ok := s.q[coding]; ok {
		return q > 0
	}
	if s.hasWild {
		return s.wildcard > 0
	}
	return false
}

func parseAcceptEncoding(header string) acceptSet {
	set := acceptSet{q: map[string]float64{}}
	for part := range strings.SplitSeq(header, ",") {
		token, q := parseEncodingPart(part)
		switch token {
		case "":
			continue
		case "*":
			set.wildcard, set.hasWild = q, true
		default:
			set.q[token] = q
		}
	}
	return set
}

func parseEncodingPart(part string) (token string, q float64) {
	part = strings.TrimSpace(part)
	if part == "" {
		return "", 0
	}
	if base, params, found := strings.Cut(part, ";"); found {
		return strings.ToLower(strings.TrimSpace(base)), parseQValue(params)
	}
	return strings.ToLower(part), 1.0
}

// parseQValue extracts the q-value from the parameter segment of an
// Accept-Encoding element. A missing or malformed q defaults to 1.0
// (acceptable): the client listed the coding, so default to honoring it.
func parseQValue(params string) float64 {
	for p := range strings.SplitSeq(params, ";") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return 1.0
			}
			return f
		}
	}
	return 1.0
}
