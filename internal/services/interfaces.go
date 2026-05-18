package services

import (
	"html/template"
	"io"

	"github.com/1mb-dev/markgo/internal/models"
)

// ArticleServiceInterface defines the interface for article operations
type ArticleServiceInterface interface {
	// Article retrieval methods
	GetAllArticles() []*models.Article
	GetPages() []*models.Article
	GetArticleBySlug(slug string) (*models.Article, error)
	GetArticlesByTag(tag string) []*models.Article
	GetArticlesByCategory(category string) []*models.Article
	GetArticlesForFeed(limit int) []*models.Article

	// Featured and recent articles
	GetFeaturedArticles(limit int) []*models.Article
	GetRecentArticles(limit int) []*models.Article

	// Metadata methods
	GetAllTags() []string
	GetAllCategories() []string
	GetTagCounts() []models.TagCount
	GetCategoryCounts() []models.CategoryCount

	// Search
	SearchArticles(query string, limit int) []*models.SearchResult

	// Statistics and management
	GetStats() *models.Stats
	ReloadArticles() error

	// Draft read-only operations
	// Note: Drafts are articles with `draft: true` in YAML frontmatter.
	// Users manage draft status via Git (edit file, commit, push).
	// No publish/unpublish API needed - use your editor and Git workflow.
	GetDraftArticles() []*models.Article
	GetDraftBySlug(slug string) (*models.Article, error)

	// IsHealthy reports whether the article service is in a working state.
	// Used by /health to surface real degradation to uptime monitors.
	IsHealthy() bool
}

// EmailServiceInterface defines the interface for email operations
type EmailServiceInterface interface {
	// Email sending methods
	SendContactMessage(msg *models.ContactMessage) error
	SendNotification(to, subject, body string) error
	SendTestEmail() error

	// Configuration and validation
	TestConnection() error
	ValidateConfig() []string
	GetConfig() map[string]any

	// Lifecycle management
	Shutdown()
}

// TemplateServiceInterface defines the interface for template operations
type TemplateServiceInterface interface {
	// Template rendering
	Render(w io.Writer, templateName string, data any) error
	RenderToString(templateName string, data any) (string, error)

	// Template management
	HasTemplate(templateName string) bool
	ListTemplates() []string
	Reload(templatesPath string) error

	// Internal access (for Gin integration)
	GetTemplate() *template.Template
}

// FeedServiceInterface defines the interface for feed generation
type FeedServiceInterface interface {
	GenerateRSS() (string, error)
	GenerateJSONFeed() (string, error)
	GenerateSitemap() (string, error)
}

// SEOServiceInterface defines the interface for SEO utilities (stateless).
// Sitemap generation lives in feed.Service (served at /sitemap.xml); this
// interface covers per-request SEO surfaces (robots, Schema.org, OG/Twitter
// meta, content analysis).
type SEOServiceInterface interface {
	GenerateRobotsTxt() ([]byte, error)

	// Schema.org structured data
	GenerateArticleSchema(article *models.Article, baseURL string) (map[string]interface{}, error)
	GenerateWebsiteSchema() (map[string]interface{}, error)
	GenerateBreadcrumbSchema(breadcrumbs []Breadcrumb) (map[string]interface{}, error)

	// Open Graph and meta tag optimization
	GenerateOpenGraphTags(article *models.Article, baseURL string) (map[string]string, error)
	GenerateTwitterCardTags(article *models.Article, baseURL string) (map[string]string, error)
	GenerateMetaTags(article *models.Article) (map[string]string, error)
	GeneratePageMetaTags(title, description, path string) (map[string]string, error)

	// SEO analysis
	AnalyzeContent(content string) (*SEOAnalysis, error)

	// Status check
	IsEnabled() bool
}

// SiteConfig represents site-wide configuration for SEO
type SiteConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	BaseURL     string `json:"base_url"`
	Language    string `json:"language"`
	Author      string `json:"author"`
	Logo        string `json:"logo,omitempty"`
	Image       string `json:"image,omitempty"`
}

// Breadcrumb represents a breadcrumb item for structured data
type Breadcrumb struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SEOAnalysis represents content SEO analysis results
type SEOAnalysis struct {
	TitleLength  int      `json:"title_length"`
	DescLength   int      `json:"description_length"`
	WordCount    int      `json:"word_count"`
	ReadingTime  int      `json:"reading_time"`
	HeadingCount int      `json:"heading_count"`
	ImageCount   int      `json:"image_count"`
	LinkCount    int      `json:"link_count"`
	Keywords     []string `json:"keywords"`
	Suggestions  []string `json:"suggestions"`
	Score        float64  `json:"score"`
}

// RobotsConfig represents robots.txt configuration
type RobotsConfig struct {
	UserAgent  string   `json:"user_agent"`
	Allow      []string `json:"allow"`
	Disallow   []string `json:"disallow"`
	CrawlDelay int      `json:"crawl_delay,omitempty"`
	SitemapURL string   `json:"sitemap_url"`
}
