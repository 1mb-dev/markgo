package seo

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

// MockArticleService for testing
type MockArticleService struct {
	articles []*models.Article
}

func (m *MockArticleService) GetAllArticles() []*models.Article {
	return m.articles
}

func (m *MockArticleService) GetArticleBySlug(slug string) (*models.Article, error) {
	for _, article := range m.articles {
		if article.Slug == slug {
			return article, nil
		}
	}
	return nil, nil
}

// Implement other required methods as no-ops
func (m *MockArticleService) GetPages() []*models.Article                           { return nil }
func (m *MockArticleService) GetArticlesByTag(_ string) []*models.Article           { return nil }
func (m *MockArticleService) GetArticlesByCategory(_ string) []*models.Article      { return nil }
func (m *MockArticleService) GetArticlesForFeed(_ int) []*models.Article            { return nil }
func (m *MockArticleService) GetFeaturedArticles(_ int) []*models.Article           { return nil }
func (m *MockArticleService) GetRecentArticles(_ int) []*models.Article             { return nil }
func (m *MockArticleService) GetAllTags() []string                                  { return nil }
func (m *MockArticleService) GetAllCategories() []string                            { return nil }
func (m *MockArticleService) GetTagCounts() []models.TagCount                       { return nil }
func (m *MockArticleService) GetCategoryCounts() []models.CategoryCount             { return nil }
func (m *MockArticleService) SearchArticles(_ string, _ int) []*models.SearchResult { return nil }
func (m *MockArticleService) GetStats() *models.Stats                               { return nil }
func (m *MockArticleService) ReloadArticles() error                                 { return nil }
func (m *MockArticleService) GetDraftArticles() []*models.Article                   { return nil }
func (m *MockArticleService) GetDraftBySlug(_ string) (*models.Article, error)      { return nil, nil }
func (m *MockArticleService) IsHealthy() bool                                       { return true }

func createTestHelper() (*Helper, *MockArticleService) {
	mockArticles := &MockArticleService{
		articles: []*models.Article{
			{
				Slug:        "test-article",
				Title:       "Test Article",
				Description: "Test description",
				Date:        time.Now().Add(-24 * time.Hour),
				Tags:        []string{"test", "article"},
				Categories:  []string{"tech"},
				Draft:       false,
				Featured:    true,
				Author:      "Test Author",
				Content:     "This is test content with more than 150 words...",
				WordCount:   200,
				ReadingTime: 1,
			},
			{
				Slug:  "draft-article",
				Title: "Draft Article",
				Draft: true,
			},
		},
	}

	siteConfig := services.SiteConfig{
		Name:        "Test Blog",
		Description: "Test blog description",
		BaseURL:     "https://example.com",
		Language:    "en",
		Author:      "Test Author",
	}

	robotsConfig := services.RobotsConfig{
		UserAgent:  "*",
		Allow:      []string{"/"},
		Disallow:   []string{"/admin"},
		CrawlDelay: 1,
		SitemapURL: "https://example.com/sitemap.xml",
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	helper := NewHelper(mockArticles, &siteConfig, &robotsConfig, logger, true)
	return helper, mockArticles
}

func TestRobotsGeneration(t *testing.T) {
	helper, _ := createTestHelper()

	robots, err := helper.GenerateRobotsTxt()
	if err != nil {
		t.Fatalf("Failed to generate robots.txt: %v", err)
	}

	robotsStr := string(robots)

	// Check basic structure
	if !strings.Contains(robotsStr, "User-agent: *") {
		t.Error("robots.txt missing user-agent")
	}

	if !strings.Contains(robotsStr, "Allow: /") {
		t.Error("robots.txt missing allow directive")
	}

	if !strings.Contains(robotsStr, "Disallow: /admin") {
		t.Error("robots.txt missing disallow directive")
	}

	if !strings.Contains(robotsStr, "Sitemap: https://example.com/sitemap.xml") {
		t.Error("robots.txt missing sitemap URL")
	}
}

func TestContentAnalysis(t *testing.T) {
	helper, _ := createTestHelper()

	content := "# Test Article\n\nThis is a test article with multiple paragraphs and some **bold** text.\n\n![Image](image.jpg)\n\n[Link](https://example.com)"

	analysis, err := helper.AnalyzeContent(content)
	if err != nil {
		t.Fatalf("Failed to analyze content: %v", err)
	}

	if analysis.WordCount == 0 {
		t.Error("Word count should be greater than 0")
	}

	if analysis.HeadingCount != 1 {
		t.Errorf("Expected 1 heading, got %d", analysis.HeadingCount)
	}

	if analysis.ImageCount != 1 {
		t.Errorf("Expected 1 image, got %d", analysis.ImageCount)
	}

	if analysis.LinkCount != 1 {
		t.Errorf("Expected 1 link, got %d", analysis.LinkCount)
	}

	if analysis.Score <= 0 {
		t.Error("SEO score should be positive")
	}
}

func TestResolveOGImage_Tiers(t *testing.T) {
	helper, _ := createTestHelper()
	baseURL := "https://example.com"

	t.Run("banner takes precedence over inline and default", func(t *testing.T) {
		a := &models.Article{
			Slug:    "post",
			Banner:  "hero.jpg",
			Content: "![inline](/img/inline.png)\n\nbody",
		}
		got := helper.resolveOGImage(a, baseURL)
		want := "https://example.com/uploads/post/hero.jpg"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("absolute URL banner passes through", func(t *testing.T) {
		a := &models.Article{Slug: "post", Banner: "https://cdn.example.com/hero.jpg"}
		got := helper.resolveOGImage(a, baseURL)
		if got != "https://cdn.example.com/hero.jpg" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("server-absolute banner prepends base URL", func(t *testing.T) {
		a := &models.Article{Slug: "post", Banner: "/static/img/banners/hero.png"}
		got := helper.resolveOGImage(a, baseURL)
		want := "https://example.com/static/img/banners/hero.png"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("falls back to first inline image when no banner", func(t *testing.T) {
		a := &models.Article{
			Slug:    "post",
			Content: "intro\n![alt](/img/inline.png)\n\nbody",
		}
		got := helper.resolveOGImage(a, baseURL)
		if !strings.Contains(got, "/img/inline.png") {
			t.Errorf("expected inline image URL, got %q", got)
		}
	})

	t.Run("falls back to static default when no banner and no inline", func(t *testing.T) {
		a := &models.Article{Slug: "post", Content: "no images here"}
		got := helper.resolveOGImage(a, baseURL)
		want := "https://example.com/static/img/og-article-default.png"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestGenerateArticleSchema_BannerPrecedence(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{
		Slug:    "post",
		Title:   "Post",
		Banner:  "hero.jpg",
		Date:    time.Now(),
		Content: "![inline](/img/inline.png)\n\nbody",
	}
	schema, err := helper.GenerateArticleSchema(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	img, ok := schema["image"].(map[string]any)
	if !ok {
		t.Fatal("schema image must be an ImageObject map")
	}
	if !strings.Contains(img["url"].(string), "/uploads/post/hero.jpg") {
		t.Errorf("schema image should resolve to banner, got %q", img["url"])
	}
}

func TestGenerateOpenGraphTags_EmitsSingleOGImage(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{
		Slug:    "post",
		Title:   "Post",
		Banner:  "hero.jpg",
		Date:    time.Now(),
		Content: "body",
	}
	tags, err := helper.GenerateOpenGraphTags(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := tags["og:image"]; !ok {
		t.Fatal("og:image must be present")
	}
	// Map keys are unique by definition — but check the value matches banner precedence.
	if !strings.Contains(tags["og:image"], "/uploads/post/hero.jpg") {
		t.Errorf("og:image should resolve to banner, got %q", tags["og:image"])
	}
}

func TestGenerateOpenGraphTags_PageEmitsWebsiteType(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{
		Slug:    "run-your-own",
		Title:   "Run Your Own",
		Type:    "page",
		Date:    time.Now(),
		Content: "Guide body.",
	}
	tags, err := helper.GenerateOpenGraphTags(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags["og:type"] != "website" {
		t.Errorf("type=page must emit og:type=website, got %q", tags["og:type"])
	}
	if !strings.HasSuffix(tags["og:url"], "/p/run-your-own") {
		t.Errorf("page og:url must be /p/<slug>, got %q", tags["og:url"])
	}
}

func TestGenerateOpenGraphTags_ArticleEmitsArticleType(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{
		Slug:    "intro",
		Title:   "Intro",
		Type:    "article",
		Date:    time.Now(),
		Content: "Body.",
	}
	tags, err := helper.GenerateOpenGraphTags(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags["og:type"] != "article" {
		t.Errorf("type=article must emit og:type=article, got %q", tags["og:type"])
	}
}

func TestGenerateArticleSchema_PageEmitsWebPage(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{
		Slug:    "run-your-own",
		Title:   "Run Your Own",
		Type:    "page",
		Date:    time.Now(),
		Content: "Guide body.",
	}
	schema, err := helper.GenerateArticleSchema(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema["@type"] != "WebPage" {
		t.Errorf("type=page must emit @type=WebPage, got %q", schema["@type"])
	}
	urlStr, _ := schema["url"].(string)
	if !strings.HasSuffix(urlStr, "/p/run-your-own") {
		t.Errorf("page url must be /p/<slug>, got %q", urlStr)
	}
}

func TestGenerateArticleSchema_ArticleEmitsArticle(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{
		Slug:    "intro",
		Title:   "Intro",
		Type:    "article",
		Date:    time.Now(),
		Content: "Body.",
	}
	schema, err := helper.GenerateArticleSchema(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema["@type"] != "Article" {
		t.Errorf("type=article must emit @type=Article, got %q", schema["@type"])
	}
	urlStr, _ := schema["url"].(string)
	if !strings.HasSuffix(urlStr, "/writing/intro") {
		t.Errorf("article url must be /writing/<slug>, got %q", urlStr)
	}
}

func TestDisabledHelper(t *testing.T) {
	mockArticles := &MockArticleService{}
	siteConfig := services.SiteConfig{BaseURL: "https://example.com"}
	robotsConfig := services.RobotsConfig{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create disabled helper
	helper := NewHelper(mockArticles, &siteConfig, &robotsConfig, logger, false)

	if helper.IsEnabled() {
		t.Error("Helper should be disabled")
	}

	_, err := helper.GenerateRobotsTxt()
	if err == nil {
		t.Error("Disabled helper should return error for robots.txt generation")
	}
}

// TestGenerateArticleSchema_DatesUseFrontmatterNotMtime: dateModified (and
// datePublished) come from the frontmatter date, not the file mtime — mtime is
// reset by image builds/checkouts and would churn the dates on every deploy.
func TestGenerateArticleSchema_DatesUseFrontmatterNotMtime(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{
		Slug:         "post",
		Title:        "Post",
		Type:         "article",
		Date:         time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC),
		LastModified: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), // must be ignored
		Content:      "Body.",
	}
	schema, err := helper.GenerateArticleSchema(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = "2025-05-20T00:00:00Z"
	if schema["datePublished"] != want {
		t.Errorf("datePublished = %v, want %s", schema["datePublished"], want)
	}
	if schema["dateModified"] != want {
		t.Errorf("dateModified = %v, want %s (frontmatter date, not mtime)", schema["dateModified"], want)
	}
}

// TestGenerateArticleSchema_DatelessPageOmitsDates: a page carries no date:, so
// its schema must omit datePublished/dateModified, never emit "0001-01-01".
func TestGenerateArticleSchema_DatelessPageOmitsDates(t *testing.T) {
	helper, _ := createTestHelper()
	a := &models.Article{Slug: "colophon", Title: "Colophon", Type: "page", Content: "About."}
	schema, err := helper.GenerateArticleSchema(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := schema["datePublished"]; ok {
		t.Errorf("date-less page must omit datePublished, got %v", schema["datePublished"])
	}
	if _, ok := schema["dateModified"]; ok {
		t.Errorf("date-less page must omit dateModified, got %v", schema["dateModified"])
	}
}

// TestGenerateOpenGraphTags_Dates mirrors the schema behavior for og tags:
// article timestamps come from the frontmatter date (both published+modified),
// and a date-less page omits them entirely.
func TestGenerateOpenGraphTags_Dates(t *testing.T) {
	helper, _ := createTestHelper()

	a := &models.Article{
		Slug: "post", Title: "Post", Type: "article",
		Date:         time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC),
		LastModified: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), // ignored
		Content:      "Body.",
	}
	tags, err := helper.GenerateOpenGraphTags(a, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = "2025-05-20T00:00:00Z"
	if tags["article:published_time"] != want {
		t.Errorf("article:published_time = %q, want %s", tags["article:published_time"], want)
	}
	if tags["article:modified_time"] != want {
		t.Errorf("article:modified_time = %q, want %s (not mtime)", tags["article:modified_time"], want)
	}

	p := &models.Article{Slug: "colophon", Title: "Colophon", Type: "page", Content: "x"}
	ptags, err := helper.GenerateOpenGraphTags(p, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := ptags["article:published_time"]; ok {
		t.Error("date-less page must omit article:published_time")
	}
	if _, ok := ptags["article:modified_time"]; ok {
		t.Error("date-less page must omit article:modified_time")
	}
}
