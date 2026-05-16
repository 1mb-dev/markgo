package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

// ContainSlugPath joins basePath and slug, resolves to an absolute path, and
// verifies the result is strictly contained within basePath. Returns the
// cleaned absolute joined path on success.
//
// Returns apperrors.ErrPathEscape (wrapped) when the joined path escapes basePath.
// Returns other errors (wrapped) for filepath.Abs failures (configuration issues).
func ContainSlugPath(basePath, slug string) (string, error) {
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
