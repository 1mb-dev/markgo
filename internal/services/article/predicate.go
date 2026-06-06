package article

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/1mb-dev/markgo/internal/models"
)

// TypePage is the explicit frontmatter type value for evergreen pages
// served by the /p/:slug route.
const TypePage = "page"

// TypeAMA is the frontmatter type value for reader Q&A posts. The question
// is the card hook (frontmatter); the answer is the body.
const TypeAMA = "ama"

// aboutSlug is the reserved slug whose markdown body is rendered by the
// dedicated /about handler. The dedicated-route predicate excludes it from
// the writing-feed graph; tests and the predicate reference this constant
// rather than the literal.
const aboutSlug = "about"

// DedicatedRouteArticle reports whether an article is served by its own
// dedicated route rather than /writing/:slug. Such articles are excluded
// from /writing, RSS, JSONFeed, sitemap (article section), tag, and
// category listings. They remain directly accessible via GetBySlug and
// still indexed for search (so readers can find them by content).
//
// Cases:
//   - slug == "about" → served by /about handler
//   - type == "page"  → served by /p/:slug handler
func DedicatedRouteArticle(a *models.Article) bool {
	return a.Slug == aboutSlug || a.Type == TypePage
}

// CanonicalURLFor returns the canonical URL path for an article. Used by
// the /writing/:slug redirect, Schema.org @id/url, and sitemap <loc> to
// keep one source of truth across redirect, SEO, and discovery surfaces.
//
//   - slug == "about" → /about
//   - type == "page"  → /p/<slug>
//   - otherwise       → /writing/<slug>
func CanonicalURLFor(a *models.Article) string {
	if a.Slug == aboutSlug {
		return "/about"
	}
	if a.Type == TypePage {
		return "/p/" + a.Slug
	}
	return "/writing/" + a.Slug
}

// SlugMaxLength caps slug length for operator-supplied slugs. Filesystem
// safety + URL-readability ceiling.
const SlugMaxLength = 100

// slugCharClass enforces lowercase ASCII letters, digits, and hyphens
// with no leading or trailing hyphen. Mirrors the codebase-wide
// validSlug gate in compose handlers so a slug accepted at create-time
// is also accepted at edit/publish-draft time.
var slugCharClass = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// reservedSlugs blocks slugs that would confuse readers if served from
// /p/<slug> by shadowing feed-like names. The set is deliberately minimal:
// only names that collide with content-discovery conventions. Reserved
// route prefixes (/about, /writing, /admin, etc.) live at different paths
// and don't need explicit blocking here.
var reservedSlugs = map[string]struct{}{
	"index": {},
	"feed":  {},
	"rss":   {},
	"atom":  {},
}

// ValidateSlug enforces the strict contract for operator-supplied slugs
// at the compose/new-page authoring boundary. Rejects anything that
// wouldn't be a clean URL component or would shadow a feed-like name.
//
// This is intentionally stricter than FileSystemRepository.validateSlug,
// which guards already-stored slugs from path traversal at admin-only
// surfaces. Historical articles may have looser slugs (e.g. uppercase
// from pre-validation eras) — those still work via the repository's
// permissive check. New writes through the compose form must satisfy
// this stricter contract.
//
// Distinct from commands/new.ValidateSlug, the CLI scaffolding contract:
// that one forbids consecutive hyphens but checks neither reserved names
// nor path traversal. Two contracts by design — do not merge.
func ValidateSlug(slug string) error {
	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("slug cannot be empty")
	}
	if strings.Contains(slug, "..") || strings.Contains(slug, "/") || strings.Contains(slug, "\\") {
		return fmt.Errorf("slug contains path-traversal characters: %s", slug)
	}
	if len(slug) > SlugMaxLength {
		return fmt.Errorf("slug exceeds %d characters: %d", SlugMaxLength, len(slug))
	}
	if !slugCharClass.MatchString(slug) {
		return fmt.Errorf("slug must match %s: %s", slugCharClass.String(), slug)
	}
	if _, blocked := reservedSlugs[slug]; blocked {
		return fmt.Errorf("slug is reserved: %s", slug)
	}
	return nil
}
