package feed

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
)

type mockArticleService struct {
	articles []*models.Article
	pages    []*models.Article
}

func (m *mockArticleService) GetAllArticles() []*models.Article { return m.articles }
func (m *mockArticleService) GetPages() []*models.Article       { return m.pages }
func (m *mockArticleService) GetArticleBySlug(_ string) (*models.Article, error) {
	return nil, nil
}
func (m *mockArticleService) GetArticlesByTag(_ string) []*models.Article           { return nil }
func (m *mockArticleService) GetArticlesByCategory(_ string) []*models.Article      { return nil }
func (m *mockArticleService) GetArticlesForFeed(_ int) []*models.Article            { return nil }
func (m *mockArticleService) GetFeaturedArticles(_ int) []*models.Article           { return nil }
func (m *mockArticleService) GetRecentArticles(_ int) []*models.Article             { return nil }
func (m *mockArticleService) GetAllTags() []string                                  { return nil }
func (m *mockArticleService) GetAllCategories() []string                            { return nil }
func (m *mockArticleService) GetTagCounts() []models.TagCount                       { return nil }
func (m *mockArticleService) GetCategoryCounts() []models.CategoryCount             { return nil }
func (m *mockArticleService) SearchArticles(_ string, _ int) []*models.SearchResult { return nil }
func (m *mockArticleService) GetStats() *models.Stats                               { return nil }
func (m *mockArticleService) ReloadArticles() error                                 { return nil }
func (m *mockArticleService) GetDraftArticles() []*models.Article                   { return nil }
func (m *mockArticleService) GetDraftBySlug(_ string) (*models.Article, error)      { return nil, nil }
func (m *mockArticleService) IsHealthy() bool                                       { return true }

func testConfig() *config.Config {
	return &config.Config{
		BaseURL: "http://localhost:3000",
		Blog: config.BlogConfig{
			Title:       "Test Blog",
			Description: "A test blog",
			Author:      "Test Author",
			Language:    "en",
		},
	}
}

func testArticles() []*models.Article {
	return []*models.Article{
		{
			Slug:        "hello-world",
			Title:       "Hello World",
			Description: "First post",
			Date:        time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			Tags:        []string{"intro"},
		},
		{
			Slug:        "second-post",
			Title:       "Second Post",
			Description: "Another post",
			Date:        time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC),
			Tags:        []string{"golang", "tutorial"},
		},
		{
			Slug:  "draft",
			Title: "Draft",
			Draft: true,
		},
	}
}

func TestGenerateRSS(t *testing.T) {
	svc := NewService(&mockArticleService{articles: testArticles()}, testConfig())

	rss, err := svc.GenerateRSS()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(rss, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, rss, "<title>Test Blog</title>")
	assert.Contains(t, rss, "<title>Hello World</title>")
	assert.Contains(t, rss, "<title>Second Post</title>")
	assert.NotContains(t, rss, "Draft")
	assert.Contains(t, rss, "http://localhost:3000/writing/hello-world")

	// Verify valid XML
	var doc rssDoc
	err = xml.Unmarshal([]byte(strings.TrimPrefix(rss, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")), &doc)
	require.NoError(t, err)
	assert.Equal(t, "2.0", doc.Version)
	assert.Equal(t, 2, len(doc.Channel.Items))
}

func TestGenerateRSSXMLSafe(t *testing.T) {
	articles := []*models.Article{
		{
			Slug:        "xss-test",
			Title:       `Title with <script>alert("xss")</script>`,
			Description: `Desc with <b>bold</b> & "quotes"`,
			Date:        time.Now(),
		},
	}
	svc := NewService(&mockArticleService{articles: articles}, testConfig())

	rss, err := svc.GenerateRSS()
	require.NoError(t, err)

	// Should NOT contain raw HTML/script tags — xml.Marshal escapes them
	assert.NotContains(t, rss, `<script>`)
	assert.Contains(t, rss, `&lt;script&gt;`)
}

func TestGenerateJSONFeed(t *testing.T) {
	svc := NewService(&mockArticleService{articles: testArticles()}, testConfig())

	jsonStr, err := svc.GenerateJSONFeed()
	require.NoError(t, err)

	var feed map[string]any
	err = json.Unmarshal([]byte(jsonStr), &feed)
	require.NoError(t, err)

	assert.Equal(t, "https://jsonfeed.org/version/1.1", feed["version"])
	assert.Equal(t, "Test Blog", feed["title"])
	assert.Equal(t, "http://localhost:3000", feed["home_page_url"])
	assert.Equal(t, "http://localhost:3000/feed.json", feed["feed_url"])

	items, ok := feed["items"].([]any)
	require.True(t, ok)
	assert.Equal(t, 2, len(items)) // draft excluded
}

func TestGenerateJSONFeed_BannerImage(t *testing.T) {
	articles := []*models.Article{
		{Slug: "with-relative", Title: "Rel", Date: time.Now(), Banner: "hero.jpg"},
		{Slug: "with-absolute", Title: "Abs", Date: time.Now(), Banner: "https://cdn.example.com/hero.jpg"},
		{Slug: "with-server-absolute", Title: "SrvAbs", Date: time.Now(), Banner: "/static/img/banners/hero.png"},
		{Slug: "no-banner", Title: "None", Date: time.Now()},
	}
	// Use a BaseURL with a trailing slash to assert we don't emit double-slashed URLs.
	cfg := testConfig()
	cfg.BaseURL = "http://localhost:3000/"
	svc := NewService(&mockArticleService{articles: articles}, cfg)

	jsonStr, err := svc.GenerateJSONFeed()
	require.NoError(t, err)

	var feed map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &feed))
	items := feed["items"].([]any)
	require.Len(t, items, 4)

	byID := map[string]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		byID[m["id"].(string)] = m
	}

	rel := byID["http://localhost:3000//writing/with-relative"]
	assert.Equal(t, "http://localhost:3000/uploads/with-relative/hero.jpg", rel["image"],
		"banner URL must collapse the trailing slash in BaseURL")

	abs := byID["http://localhost:3000//writing/with-absolute"]
	assert.Equal(t, "https://cdn.example.com/hero.jpg", abs["image"])

	srv := byID["http://localhost:3000//writing/with-server-absolute"]
	assert.Equal(t, "http://localhost:3000/static/img/banners/hero.png", srv["image"],
		"server-absolute banner must prepend BaseURL with trailing slash collapsed")

	none := byID["http://localhost:3000//writing/no-banner"]
	_, hasImage := none["image"]
	assert.False(t, hasImage, "article without banner should not emit image field")
}

func TestGenerateSitemap(t *testing.T) {
	svc := NewService(&mockArticleService{articles: testArticles()}, testConfig())

	sitemap, err := svc.GenerateSitemap()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(sitemap, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, sitemap, "http://localhost:3000")
	assert.Contains(t, sitemap, "http://localhost:3000/writing/hello-world")
	assert.Contains(t, sitemap, "http://localhost:3000/writing/second-post")
	assert.Contains(t, sitemap, "http://localhost:3000/about", "static /about entry")
	assert.Contains(t, sitemap, "http://localhost:3000/p", "static /p index entry")
	assert.NotContains(t, sitemap, "draft")

	// testArticles' tags are all single-use → excluded by the ≥2-article gate.
	assert.NotContains(t, sitemap, "/tags/intro", "single-article term is gated out")
	assert.NotContains(t, sitemap, "/tags/golang")

	var sm models.Sitemap
	xmlContent := strings.TrimPrefix(sitemap, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	require.NoError(t, xml.Unmarshal([]byte(xmlContent), &sm))
	// 6 static + 2 published articles; no term pages (all tags single-use) = 8.
	assert.Equal(t, 8, len(sm.URLs))

	byLoc := map[string]models.SitemapURL{}
	for _, u := range sm.URLs {
		byLoc[u.Loc] = u
	}
	// Static index entries omit <lastmod> (a synthetic "now" churns and is distrusted).
	assert.Empty(t, byLoc["http://localhost:3000"].LastMod, "static home omits lastmod")
	assert.Empty(t, byLoc["http://localhost:3000/writing"].LastMod, "static /writing omits lastmod")
	// Articles carry their frontmatter date.
	assert.Equal(t, "2025-01-15T00:00:00Z", byLoc["http://localhost:3000/writing/hello-world"].LastMod)
}

// TestGenerateSitemap_IncludesPages verifies that type:page articles surface
// in sitemap.xml at their canonical /p/<slug> URL via GetPages(). Pages are
// excluded from the article section (via the predicate in GetPublished) so
// they must be emitted via the separate pages loop.
func TestGenerateSitemap_IncludesPages(t *testing.T) {
	articles := []*models.Article{
		{Slug: "regular-post", Title: "Post", Date: time.Now()},
	}
	// Real pages carry NO date: frontmatter (v3.13.0) — their LastModified comes
	// from the file mtime at load. The mock leaves both zero to exercise the path
	// that previously emitted "0001-01-01T00:00:00Z".
	pages := []*models.Article{
		{Slug: "run-your-own", Title: "Run Your Own", Type: "page"},
		{Slug: "colophon", Title: "Colophon", Type: "page"},
	}
	svc := NewService(&mockArticleService{articles: articles, pages: pages}, testConfig())

	sitemap, err := svc.GenerateSitemap()
	require.NoError(t, err)

	assert.Contains(t, sitemap, "http://localhost:3000/p/run-your-own", "page must surface at /p/<slug>")
	assert.Contains(t, sitemap, "http://localhost:3000/p/colophon")
	// No /writing/<page-slug> leak.
	assert.NotContains(t, sitemap, "/writing/run-your-own")
	assert.NotContains(t, sitemap, "/writing/colophon")
	// The bug: a date-less page must NOT emit the bogus zero-time lastmod.
	assert.NotContains(t, sitemap, "0001-01-01", "date-less page must omit <lastmod>, not emit zero time")
}

// TestGenerateSitemap_AllURLsAreCanonical verifies no <loc> entry contains
// a hardcoded /writing/<slug> for content that should canonicalize to /p
// or /about. Guard against future regressions of the v3.14.0 sweep.
func TestGenerateSitemap_AllURLsAreCanonical(t *testing.T) {
	// Fixture: article + page + about-slug article. The mock's GetAllArticles
	// returns ALL of them (no predicate filtering at mock layer per CLAUDE.md);
	// but in production GetAllArticles → repo.GetPublished excludes pages and
	// /about. To simulate real behavior, separate them into articles vs pages.
	articles := []*models.Article{
		{Slug: "real-post", Title: "Real Post", Date: time.Now()},
	}
	pages := []*models.Article{
		{Slug: "about-pages", Title: "About Pages", Type: "page", Date: time.Now()},
	}
	svc := NewService(&mockArticleService{articles: articles, pages: pages}, testConfig())

	sitemap, err := svc.GenerateSitemap()
	require.NoError(t, err)

	// /writing/<page-slug> must NEVER appear in sitemap
	assert.NotContains(t, sitemap, "/writing/about-pages", "type:page must canonicalize to /p, not /writing")
	assert.Contains(t, sitemap, "/p/about-pages")
}

// TestGenerateSitemap_LastModUsesFrontmatterDate verifies <lastmod> comes from
// the frontmatter date (deploy-stable), NOT the file mtime — which image builds
// and checkouts reset, churning every date and getting the signal discounted.
func TestGenerateSitemap_LastModUsesFrontmatterDate(t *testing.T) {
	articles := []*models.Article{{
		Slug:         "post",
		Title:        "Post",
		Date:         time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC),
		LastModified: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), // must be ignored
	}}
	svc := NewService(&mockArticleService{articles: articles}, testConfig())

	sitemap, err := svc.GenerateSitemap()
	require.NoError(t, err)

	assert.Contains(t, sitemap, "<lastmod>2025-05-20T00:00:00Z</lastmod>", "lastmod is the frontmatter date")
	assert.NotContains(t, sitemap, "2026-03-01", "file mtime must not be used (it churns on deploy)")
}

// TestGenerateSitemap_TermPages verifies term pages are emitted only for tags/
// categories carried by ≥2 articles (single-article terms are thin content and
// gated out), with lastmod = the newest article date, in sorted order.
func TestGenerateSitemap_TermPages(t *testing.T) {
	articles := []*models.Article{
		{Slug: "a", Title: "A", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Tags: []string{"go", "alpha", "solo"}, Categories: []string{"eng"}},
		{Slug: "b", Title: "B", Date: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), Tags: []string{"go", "alpha"}, Categories: []string{"eng"}},
	}
	svc := NewService(&mockArticleService{articles: articles}, testConfig())

	sitemap, err := svc.GenerateSitemap()
	require.NoError(t, err)

	var sm models.Sitemap
	require.NoError(t, xml.Unmarshal([]byte(strings.TrimPrefix(sitemap, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")), &sm))

	byLoc := map[string]models.SitemapURL{}
	idxAlpha, idxGo := -1, -1
	for i, u := range sm.URLs {
		byLoc[u.Loc] = u
		switch u.Loc {
		case "http://localhost:3000/tags/alpha":
			idxAlpha = i
		case "http://localhost:3000/tags/go":
			idxGo = i
		}
	}

	// "go" and "alpha" each span both articles → emitted; lastmod = newest (Feb 2).
	goTag, ok := byLoc["http://localhost:3000/tags/go"]
	require.True(t, ok, "multi-article tag must be emitted")
	assert.Equal(t, "2026-02-02T00:00:00Z", goTag.LastMod, "term lastmod = newest article date")

	_, ok = byLoc["http://localhost:3000/categories/eng"]
	assert.True(t, ok, "multi-article category must be emitted")

	// "solo" is on one article → gated out as thin content.
	_, ok = byLoc["http://localhost:3000/tags/solo"]
	assert.False(t, ok, "single-article term must be excluded")

	// Deterministic, sorted output: alpha before go.
	require.True(t, idxAlpha >= 0 && idxGo >= 0)
	assert.Less(t, idxAlpha, idxGo, "term pages are emitted in sorted order")
}

// TestGenerateSitemap_TermAccumulationIsClean verifies a tag listed twice on one
// article doesn't inflate the ≥2 gate, and empty/whitespace tags emit no URL.
func TestGenerateSitemap_TermAccumulationIsClean(t *testing.T) {
	articles := []*models.Article{
		{Slug: "a", Title: "A", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Tags: []string{"dup", "dup", "  ", ""}},
	}
	svc := NewService(&mockArticleService{articles: articles}, testConfig())

	sitemap, err := svc.GenerateSitemap()
	require.NoError(t, err)

	assert.NotContains(t, sitemap, "/tags/dup", "a tag on one article (even if listed twice) stays gated out")
	assert.NotContains(t, sitemap, "<loc>http://localhost:3000/tags/</loc>", "empty tag must not emit a URL")
	assert.NotContains(t, sitemap, "/tags/%20", "whitespace tag must not emit a URL")
}
