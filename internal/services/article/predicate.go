package article

import "github.com/1mb-dev/markgo/internal/models"

// DedicatedRouteArticle reports whether an article is served by its own
// dedicated route rather than /writing/:slug. Such articles are excluded
// from /writing, RSS, JSONFeed, sitemap (article section), tag, and
// category listings. They remain directly accessible via GetBySlug and
// still indexed for search (so readers can find them by content).
func DedicatedRouteArticle(a *models.Article) bool {
	return a.Slug == "about"
}

// CanonicalURLFor returns the canonical URL path for an article. Used by
// the /writing/:slug redirect, Schema.org @id/url, and sitemap <loc> to
// keep one source of truth across redirect, SEO, and discovery surfaces.
func CanonicalURLFor(a *models.Article) string {
	if a.Slug == "about" {
		return "/about"
	}
	return "/writing/" + a.Slug
}
