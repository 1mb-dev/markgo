package article

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/1mb-dev/markgo/internal/models"
)

// uploadPathRE matches /uploads/<slug>/<filename> shapes that may appear in
// article bodies (markdown images, raw HTML img/src, or plain links).
// Stops at whitespace, quotes, brackets, query-string, and fragment chars
// so that ![alt](url?v=2) yields filename "url" — matching what's on disk.
var uploadPathRE = regexp.MustCompile(`/uploads/([^/\s"'>)<?#]+)/([^/\s"'>)<?#]+)`)

// OrphanSweep walks uploadPath and removes files that no article references
// — either via frontmatter `banner` field or by body content. Conservative
// on every ambiguous edge: false positives (preserving a file that isn't
// actually referenced) are acceptable; false negatives (removing a file an
// article still uses) are not.
//
// articles must include published AND drafts; drafts own uploads too. Pass
// the union of repository.LoadAll(ctx) and repository.GetDrafts() if those
// surfaces are disjoint in your implementation.
//
// Returns the count of removed files and any non-fatal errors encountered.
// A nonexistent upload directory is not an error — the operator may not
// have uploaded anything yet.
func OrphanSweep(ctx context.Context, articles []*models.Article, uploadPath string, logger *slog.Logger) (cleaned int, errs []error) {
	absBase, err := filepath.Abs(uploadPath)
	if err != nil {
		return 0, []error{err}
	}
	refs := buildReferencedSet(articles, absBase)
	return sweepUnreferenced(ctx, refs, absBase, logger)
}

// buildReferencedSet returns the absolute paths of every upload file
// referenced by the given articles. absBase is the absolute upload root.
func buildReferencedSet(articles []*models.Article, absBase string) map[string]struct{} {
	refs := make(map[string]struct{})
	for _, a := range articles {
		addBannerRef(refs, a, absBase)
		addBodyRefs(refs, a.Content, absBase)
	}
	return refs
}

// addBannerRef adds the article's banner field, if it resolves to an
// upload-owned path, to the referenced set.
func addBannerRef(refs map[string]struct{}, a *models.Article, absBase string) {
	if a.Banner == "" {
		return
	}
	if strings.HasPrefix(a.Banner, "http://") || strings.HasPrefix(a.Banner, "https://") {
		return
	}
	rel := bannerRelativePath(a.Banner, a.Slug)
	if rel == "" {
		return
	}
	addRefFromRel(refs, rel, absBase)
}

// bannerRelativePath converts a frontmatter banner value into the
// upload-root-relative path "<slug>/<filename>", or "" when the banner
// is operator-managed (e.g. /static/...) and not eligible for tracking.
func bannerRelativePath(banner, slug string) string {
	switch {
	case strings.HasPrefix(banner, "/uploads/"):
		return strings.TrimPrefix(banner, "/uploads/")
	case strings.HasPrefix(banner, "uploads/"):
		return strings.TrimPrefix(banner, "uploads/")
	case strings.HasPrefix(banner, "/"):
		return "" // /static/... and other server-absolute non-uploads
	default:
		return slug + "/" + banner
	}
}

// addBodyRefs scans content for /uploads/<slug>/<filename> patterns and
// records each as a reference.
func addBodyRefs(refs map[string]struct{}, content, absBase string) {
	for _, m := range uploadPathRE.FindAllStringSubmatch(content, -1) {
		rel := m[1] + "/" + m[2]
		addRefFromRel(refs, rel, absBase)
	}
}

// addRefFromRel resolves a "<slug>/<filename>" path against absBase,
// URL-decodes it, rejects traversal, and records the result.
func addRefFromRel(refs map[string]struct{}, rel, absBase string) {
	if decoded, err := url.PathUnescape(rel); err == nil {
		rel = decoded
	}
	if strings.Contains(rel, "..") {
		return
	}
	abs, err := filepath.Abs(filepath.Join(absBase, rel))
	if err != nil {
		return
	}
	if !strings.HasPrefix(abs, absBase+string(os.PathSeparator)) {
		return
	}
	refs[abs] = struct{}{}
}

// sweepUnreferenced walks absBase's slug subdirectories and removes any
// file whose absolute path is not in refs.
func sweepUnreferenced(ctx context.Context, refs map[string]struct{}, absBase string, logger *slog.Logger) (cleaned int, errs []error) {
	entries, err := os.ReadDir(absBase)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, []error{err}
	}
	for _, slug := range entries {
		if ctx.Err() != nil {
			return cleaned, errs
		}
		if !slug.IsDir() {
			continue
		}
		c, e := sweepSlugDir(ctx, refs, filepath.Join(absBase, slug.Name()), logger)
		cleaned += c
		errs = append(errs, e...)
	}
	return cleaned, errs
}

// sweepSlugDir handles one slug directory. Split out from sweepUnreferenced
// to keep cyclomatic complexity bounded.
func sweepSlugDir(ctx context.Context, refs map[string]struct{}, slugDir string, logger *slog.Logger) (cleaned int, errs []error) {
	files, err := os.ReadDir(slugDir)
	if err != nil {
		logger.Warn("orphan sweep: read slug dir failed", "dir", slugDir, "error", err)
		return 0, []error{err}
	}
	for _, file := range files {
		if ctx.Err() != nil {
			return cleaned, errs
		}
		if file.IsDir() {
			continue
		}
		path := filepath.Join(slugDir, file.Name())
		if _, referenced := refs[path]; referenced {
			continue
		}
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			logger.Warn("orphan sweep: remove failed", "path", path, "error", rmErr)
			errs = append(errs, rmErr)
			continue
		}
		cleaned++
	}
	return cleaned, errs
}
