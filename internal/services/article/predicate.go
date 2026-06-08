package article

import (
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
