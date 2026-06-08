// Package slug provides canonical slug generation and path-containment
// primitives shared by the article repository and the compose service.
//
// It is a leaf package (depends only on internal/errors), so both higher
// layers import it directly. This replaces a duplicated generateSlug and a
// backwards article→compose import edge that previously existed solely to
// share ContainSlugPath.
package slug

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

// MaxLength caps operator-supplied slugs at creation time — a filesystem-safety
// and URL-readability ceiling.
const MaxLength = 100

// wellFormedMaxLength is the generous ceiling applied when guarding untrusted
// or already-stored slugs (route params, edit/lookup). It exceeds MaxLength so a
// legacy slug created before the create-time cap still validates for lookup.
const wellFormedMaxLength = 200

// charClass matches a structurally valid slug: lowercase ASCII letters and
// digits, with hyphens permitted only between them (no leading/trailing hyphen).
// Length is enforced separately by the caller, not encoded here.
var charClass = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// reserved blocks slugs that would shadow feed-like names if served from
// /p/<slug>. Deliberately minimal — only content-discovery collisions; route
// prefixes (/about, /writing, /admin) live at different paths.
var reserved = map[string]struct{}{
	"index": {},
	"feed":  {},
	"rss":   {},
	"atom":  {},
}

// Generate converts a title into a URL slug: lowercased, spaces to hyphens,
// characters outside [a-z0-9-] dropped, consecutive hyphens collapsed, and
// leading/trailing hyphens trimmed. An empty or fully-dropped title yields "".
func Generate(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")

	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}

	s = b.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// ContainPath joins basePath and slug, resolves to an absolute path, and
// verifies the result is strictly contained within basePath. Returns the
// cleaned absolute joined path on success.
//
// Returns apperrors.ErrPathEscape (wrapped) when the joined path escapes
// basePath. Returns other errors (wrapped) for filepath.Abs failures
// (configuration issues).
func ContainPath(basePath, slug string) (string, error) {
	joined := filepath.Join(basePath, slug)
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("resolve absolute joined path: %w", err)
	}
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute base path: %w", err)
	}
	if !strings.HasPrefix(absJoined, absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("slug %q: %w", slug, apperrors.ErrPathEscape)
	}
	return absJoined, nil
}

// WellFormed reports whether slug is a structurally valid, URL-safe slug within
// the well-formed ceiling. It is the permissive contract: it guards untrusted
// route-param input and already-stored slugs (which may predate the stricter
// create-time rules), so it checks charset and length only — not reserved names
// or consecutive hyphens. Any slug accepted by Validate is also WellFormed.
func WellFormed(slug string) bool {
	return len(slug) <= wellFormedMaxLength && charClass.MatchString(slug)
}

// Validate enforces the strict contract for a NEW operator-supplied slug — the
// CLI `new` command and the compose new-page form. Stricter than WellFormed:
// charset, no path traversal, ≤MaxLength, no consecutive hyphens, and not a
// reserved feed-like name. Errors are recovery-oriented so callers can surface
// them directly. Already-stored slugs are guarded by WellFormed, not this.
func Validate(slug string) error {
	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("slug cannot be empty")
	}
	if strings.Contains(slug, "..") || strings.ContainsAny(slug, `/\`) {
		return fmt.Errorf("slug contains path-traversal characters: %q", slug)
	}
	if len(slug) > MaxLength {
		return fmt.Errorf("slug exceeds %d characters: %d", MaxLength, len(slug))
	}
	if !charClass.MatchString(slug) {
		return fmt.Errorf("slug must be lowercase letters, digits, and interior hyphens: %q", slug)
	}
	if strings.Contains(slug, "--") {
		return fmt.Errorf("slug cannot contain consecutive hyphens: %q", slug)
	}
	if _, blocked := reserved[slug]; blocked {
		return fmt.Errorf("slug is reserved: %q", slug)
	}
	return nil
}
