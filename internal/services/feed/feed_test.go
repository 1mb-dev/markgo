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

	// Verify valid XML
	var sm models.Sitemap
	xmlContent := strings.TrimPrefix(sitemap, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	err = xml.Unmarshal([]byte(xmlContent), &sm)
	require.NoError(t, err)
	// 6 static URLs (home, /writing, /tags, /categories, /about, /p) + 2 published articles = 8
	assert.Equal(t, 8, len(sm.URLs))
}

// TestGenerateSitemap_IncludesPages verifies that type:page articles surface
// in sitemap.xml at their canonical /p/<slug> URL via GetPages(). Pages are
// excluded from the article section (via the predicate in GetPublished) so
// they must be emitted via the separate pages loop.
func TestGenerateSitemap_IncludesPages(t *testing.T) {
	articles := []*models.Article{
		{Slug: "regular-post", Title: "Post", Date: time.Now()},
	}
	pages := []*models.Article{
		{Slug: "run-your-own", Title: "Run Your Own", Type: "page", Date: time.Now()},
		{Slug: "colophon", Title: "Colophon", Type: "page", Date: time.Now()},
	}
	svc := NewService(&mockArticleService{articles: articles, pages: pages}, testConfig())

	sitemap, err := svc.GenerateSitemap()
	require.NoError(t, err)

	assert.Contains(t, sitemap, "http://localhost:3000/writing/regular-post")
	assert.Contains(t, sitemap, "http://localhost:3000/p/run-your-own", "page must surface at /p/<slug>")
	assert.Contains(t, sitemap, "http://localhost:3000/p/colophon")
	// No /writing/<page-slug> leak.
	assert.NotContains(t, sitemap, "/writing/run-your-own")
	assert.NotContains(t, sitemap, "/writing/colophon")
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
