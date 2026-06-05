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
	"strings"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

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
