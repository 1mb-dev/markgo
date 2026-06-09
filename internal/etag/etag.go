// Package etag provides HTTP entity-tag generation and If-None-Match comparison,
// shared by the HTML render path (weak, body-derived validator) and static-asset
// serving (strong, build-version-derived validator).
package etag

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Weak returns a weak validator over body. Weak because the bytes may be
// transformed downstream (e.g. a proxy re-compressing) — If-None-Match only
// needs semantic equivalence, not byte-identity.
func Weak(body []byte) string {
	sum := sha256.Sum256(body)
	return `W/"` + hex.EncodeToString(sum[:16]) + `"`
}

// Matches reports whether an If-None-Match header value satisfies etag, using
// the weak comparison If-None-Match requires (RFC 9110 §8.8.3.2): the W/ prefix
// is ignored on both sides, and a comma-separated list or "*" is honored.
func Matches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == want {
			return true
		}
	}
	return false
}
