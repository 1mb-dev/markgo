// Package seo provides SEO functionality including meta tags, schema markup, and sitemap generation.
// This is a stateless utility package - no lifecycle management or caching needed for small-scale blogs.
package seo

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

// Helper represents a simple SEO utility
type Helper struct {
	articleService services.ArticleServiceInterface
	siteConfig     services.SiteConfig
	robotsConfig   services.RobotsConfig
	logger         *slog.Logger
	enabled        bool
}

// URLSet represents the root sitemap XML structure
type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	Xmlns   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

// URL represents a single URL entry in sitemap
type URL struct {
	Location     string `xml:"loc"`
	LastModified string `xml:"lastmod,omitempty"`
	ChangeFreq   string `xml:"changefreq,omitempty"`
	Priority     string `xml:"priority,omitempty"`
}

// NewHelper creates a new SEO helper instance
func NewHelper(
	articleService services.ArticleServiceInterface,
	siteConfig *services.SiteConfig,
	robotsConfig *services.RobotsConfig,
	logger *slog.Logger,
	enabled bool,
) *Helper {
	return &Helper{
		articleService: articleService,
		siteConfig:     *siteConfig,
		robotsConfig:   *robotsConfig,
		logger:         logger,
		enabled:        enabled,
	}
}

// IsEnabled returns whether SEO features are enabled
func (h *Helper) IsEnabled() bool {
	return h.enabled
}

// GenerateSitemap creates an XML sitemap from all published articles
func (h *Helper) GenerateSitemap() ([]byte, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	h.logger.Debug("Generating sitemap")

	// Get all published articles
	articles := h.articleService.GetAllArticles()
	if articles == nil {
		return nil, fmt.Errorf("failed to retrieve articles")
	}

	// Filter out drafts and sort by date
	publishedArticles := make([]*models.Article, 0, len(articles))
	for _, article := range articles {
		if !article.Draft {
			publishedArticles = append(publishedArticles, article)
		}
	}

	// Sort by date (newest first)
	sort.Slice(publishedArticles, func(i, j int) bool {
		return publishedArticles[i].Date.After(publishedArticles[j].Date)
	})

	// Create URL set
	urlSet := URLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  make([]URL, 0, len(publishedArticles)+3),
	}

	// Add homepage
	urlSet.URLs = append(urlSet.URLs, URL{
		Location:   h.siteConfig.BaseURL,
		ChangeFreq: "daily",
		Priority:   "1.0",
	})

	// Add articles
	for _, article := range publishedArticles {
		articleURL, err := h.buildArticleURL(article.Slug)
		if err != nil {
			h.logger.Warn("Failed to build URL for article", "slug", article.Slug, "error", err)
			continue
		}

		priority := "0.8"
		if article.Featured {
			priority = "0.9"
		}

		changeFreq := "monthly"
		if article.Date.After(time.Now().AddDate(0, -1, 0)) {
			changeFreq = "weekly"
		}

		urlSet.URLs = append(urlSet.URLs, URL{
			Location:     articleURL,
			LastModified: article.Date.Format("2006-01-02"),
			ChangeFreq:   changeFreq,
			Priority:     priority,
		})
	}

	// Generate XML
	xmlData, err := xml.MarshalIndent(urlSet, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sitemap XML: %w", err)
	}

	// Add XML header
	result := append([]byte(xml.Header), xmlData...)

	h.logger.Info("Sitemap generated",
		"articles", len(publishedArticles),
		"total_urls", len(urlSet.URLs),
		"size_bytes", len(result))

	return result, nil
}

// GenerateRobotsTxt creates a robots.txt file
func (h *Helper) GenerateRobotsTxt() ([]byte, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	var builder strings.Builder

	// User-agent directive
	userAgent := h.robotsConfig.UserAgent
	if userAgent == "" {
		userAgent = "*"
	}
	fmt.Fprintf(&builder, "User-agent: %s\n", userAgent)

	// Allow directives
	for _, allow := range h.robotsConfig.Allow {
		fmt.Fprintf(&builder, "Allow: %s\n", allow)
	}

	// Disallow directives
	for _, disallow := range h.robotsConfig.Disallow {
		fmt.Fprintf(&builder, "Disallow: %s\n", disallow)
	}

	// Crawl delay
	if h.robotsConfig.CrawlDelay > 0 {
		fmt.Fprintf(&builder, "Crawl-delay: %d\n", h.robotsConfig.CrawlDelay)
	}

	// Sitemap URL
	if h.robotsConfig.SitemapURL != "" {
		fmt.Fprintf(&builder, "\nSitemap: %s\n", h.robotsConfig.SitemapURL)
	}

	h.logger.Debug("Generated robots.txt", "size", builder.Len())
	return []byte(builder.String()), nil
}

// GenerateOpenGraphTags creates Open Graph meta tags
func (h *Helper) GenerateOpenGraphTags(article *models.Article, baseURL string) (map[string]string, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	if article == nil {
		return nil, fmt.Errorf("article cannot be nil")
	}

	tags := make(map[string]string)

	// Basic Open Graph tags
	tags["og:type"] = "article"
	tags["og:title"] = article.Title
	tags["og:site_name"] = h.siteConfig.Name

	// URL
	articleURL, err := h.buildArticleURL(article.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed to build article URL: %w", err)
	}
	tags["og:url"] = articleURL

	// Description
	description := article.Description
	if description == "" {
		description = generateExcerpt(article.Content, 160)
	}
	if description != "" {
		tags["og:description"] = description
	}

	// Image: 3-tier precedence — banner (declarative) > first inline image > static default.
	tags["og:image"] = h.resolveOGImage(article, baseURL)
	tags["og:image:alt"] = fmt.Sprintf("Featured image for %s", article.Title)

	// Article-specific tags
	tags["article:published_time"] = article.Date.Format("2006-01-02T15:04:05Z07:00")
	if !article.LastModified.IsZero() {
		tags["article:modified_time"] = article.LastModified.Format("2006-01-02T15:04:05Z07:00")
	}

	// Author
	authorName := article.Author
	if authorName == "" {
		authorName = h.siteConfig.Author
	}
	if authorName != "" {
		tags["article:author"] = authorName
	}

	// Tags and categories
	if len(article.Tags) > 0 {
		for _, tag := range article.Tags {
			tags["article:tag"] = tag
		}
	}

	if len(article.Categories) > 0 {
		tags["article:section"] = article.Categories[0]
	}

	// Locale
	if h.siteConfig.Language != "" {
		tags["og:locale"] = h.siteConfig.Language
	}

	h.logger.Debug("Generated Open Graph tags", "slug", article.Slug, "tags_count", len(tags))
	return tags, nil
}

// GenerateTwitterCardTags creates Twitter Card meta tags
func (h *Helper) GenerateTwitterCardTags(article *models.Article, baseURL string) (map[string]string, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	if article == nil {
		return nil, fmt.Errorf("article cannot be nil")
	}

	tags := make(map[string]string)

	// Image: 3-tier precedence matches OG (banner > inline > default).
	// Always emit summary_large_image since we always have *some* image now.
	tags["twitter:card"] = "summary_large_image"
	tags["twitter:image"] = h.resolveOGImage(article, baseURL)
	tags["twitter:image:alt"] = fmt.Sprintf("Featured image for %s", article.Title)

	// Basic Twitter tags
	tags["twitter:title"] = article.Title

	// Description
	description := article.Description
	if description == "" {
		description = generateExcerpt(article.Content, 160)
	}
	if description != "" {
		tags["twitter:description"] = description
	}

	// Site information
	tags["twitter:site"] = h.siteConfig.Name

	// Author (if available as Twitter handle)
	if article.Author != "" {
		authorHandle := extractTwitterHandle(article.Author)
		if authorHandle != "" {
			tags["twitter:creator"] = authorHandle
		}
	}

	h.logger.Debug("Generated Twitter Card tags", "slug", article.Slug, "card_type", tags["twitter:card"])
	return tags, nil
}

// GenerateMetaTags creates general SEO meta tags
func (h *Helper) GenerateMetaTags(article *models.Article) (map[string]string, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	if article == nil {
		return nil, fmt.Errorf("article cannot be nil")
	}

	tags := make(map[string]string)

	// Title
	tags["title"] = article.Title

	// Description
	description := article.Description
	if description == "" {
		description = generateExcerpt(article.Content, 160)
	}
	if description != "" {
		tags["description"] = description
	}

	// Keywords from tags
	if len(article.Tags) > 0 {
		tags["keywords"] = strings.Join(article.Tags, ", ")
	}

	// Author
	authorName := article.Author
	if authorName == "" {
		authorName = h.siteConfig.Author
	}
	if authorName != "" {
		tags["author"] = authorName
	}

	// Canonical URL
	articleURL, err := h.buildArticleURL(article.Slug)
	if err == nil {
		tags["canonical"] = articleURL
	}

	// Language
	if h.siteConfig.Language != "" {
		tags["language"] = h.siteConfig.Language
	}

	// Publication date
	tags["article:published_time"] = article.Date.Format("2006-01-02")

	// Robots meta tag
	robotsValue := "index, follow"
	if article.Draft {
		robotsValue = "noindex, nofollow"
	}
	tags["robots"] = robotsValue

	// Reading time
	if article.ReadingTime > 0 {
		tags["reading_time"] = strconv.Itoa(article.ReadingTime)
	}

	// Word count
	if article.WordCount > 0 {
		tags["word_count"] = strconv.Itoa(article.WordCount)
	}

	h.logger.Debug("Generated meta tags", "slug", article.Slug)
	return tags, nil
}

// GeneratePageMetaTags creates meta tags for non-article pages
func (h *Helper) GeneratePageMetaTags(title, description, path string) (map[string]string, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	tags := make(map[string]string)

	// Basic meta tags
	if title != "" {
		if title != h.siteConfig.Name {
			tags["title"] = fmt.Sprintf("%s - %s", title, h.siteConfig.Name)
		} else {
			tags["title"] = title
		}
	}

	if description != "" {
		tags["description"] = description
	} else if h.siteConfig.Description != "" {
		tags["description"] = h.siteConfig.Description
	}

	// Canonical URL
	if path != "" && h.siteConfig.BaseURL != "" {
		canonicalURL := strings.TrimRight(h.siteConfig.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")
		tags["canonical"] = canonicalURL
	}

	// Language
	if h.siteConfig.Language != "" {
		tags["language"] = h.siteConfig.Language
	}

	// Robots
	tags["robots"] = "index, follow"

	// Open Graph for pages
	tags["og:type"] = "website"
	if title != "" {
		tags["og:title"] = title
	}
	if description != "" {
		tags["og:description"] = description
	}
	tags["og:site_name"] = h.siteConfig.Name

	// Twitter Card for pages
	tags["twitter:card"] = "summary"
	if title != "" {
		tags["twitter:title"] = title
	}
	if description != "" {
		tags["twitter:description"] = description
	}

	h.logger.Debug("Generated page meta tags", "title", title, "path", path)
	return tags, nil
}

// GenerateArticleSchema creates Schema.org Article structured data
func (h *Helper) GenerateArticleSchema(article *models.Article, baseURL string) (map[string]interface{}, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	if article == nil {
		return nil, fmt.Errorf("article cannot be nil")
	}

	articleURL, err := h.buildArticleURL(article.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed to build article URL: %w", err)
	}

	// Extract first image from content if available
	imageURL := extractFirstImage(article.Content, baseURL, &h.siteConfig)

	schema := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Article",
		"headline": article.Title,
		"url":      articleURL,
	}

	// Description
	if article.Description != "" {
		schema["description"] = article.Description
	} else {
		schema["description"] = generateExcerpt(article.Content, 160)
	}

	// Author
	authorName := article.Author
	if authorName == "" {
		authorName = h.siteConfig.Author
	}

	schema["author"] = map[string]interface{}{
		"@type": "Person",
		"name":  authorName,
	}

	// Publisher (organization)
	publisher := map[string]interface{}{
		"@type": "Organization",
		"name":  h.siteConfig.Name,
		"url":   h.siteConfig.BaseURL,
	}

	if h.siteConfig.Logo != "" {
		publisher["logo"] = map[string]interface{}{
			"@type": "ImageObject",
			"url":   buildImageURL(h.siteConfig.Logo, baseURL),
		}
	}

	schema["publisher"] = publisher

	// Dates
	schema["datePublished"] = article.Date.Format(time.RFC3339)
	if !article.LastModified.IsZero() {
		schema["dateModified"] = article.LastModified.Format(time.RFC3339)
	} else {
		schema["dateModified"] = article.Date.Format(time.RFC3339)
	}

	// Main entity of page
	schema["mainEntityOfPage"] = map[string]interface{}{
		"@type": "WebPage",
		"@id":   articleURL,
	}

	// Image
	if imageURL != "" {
		schema["image"] = map[string]interface{}{
			"@type": "ImageObject",
			"url":   imageURL,
		}
	}

	// Keywords from tags
	if len(article.Tags) > 0 {
		schema["keywords"] = strings.Join(article.Tags, ", ")
	}

	// Article section (categories)
	if len(article.Categories) > 0 {
		schema["articleSection"] = article.Categories[0]
	}

	// Word count
	if article.WordCount > 0 {
		schema["wordCount"] = article.WordCount
	}

	h.logger.Debug("Generated article schema", "slug", article.Slug)
	return schema, nil
}

// GenerateWebsiteSchema creates Schema.org WebSite structured data
func (h *Helper) GenerateWebsiteSchema() (map[string]interface{}, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	schema := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "WebSite",
		"name":     h.siteConfig.Name,
		"url":      h.siteConfig.BaseURL,
	}

	if h.siteConfig.Description != "" {
		schema["description"] = h.siteConfig.Description
	}

	// Publisher information
	if h.siteConfig.Author != "" {
		schema["publisher"] = map[string]interface{}{
			"@type": "Person",
			"name":  h.siteConfig.Author,
		}
	}

	// Potential search action
	searchURL, err := url.Parse(h.siteConfig.BaseURL)
	if err == nil {
		searchURL.Path = "/search"
		schema["potentialAction"] = map[string]interface{}{
			"@type":       "SearchAction",
			"target":      searchURL.String() + "?q={search_term_string}",
			"query-input": "required name=search_term_string",
		}
	}

	h.logger.Debug("Generated website schema")
	return schema, nil
}

// GenerateBreadcrumbSchema creates Schema.org BreadcrumbList structured data
func (h *Helper) GenerateBreadcrumbSchema(breadcrumbs []services.Breadcrumb) (map[string]interface{}, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	if len(breadcrumbs) == 0 {
		return nil, fmt.Errorf("breadcrumbs cannot be empty")
	}

	itemList := make([]map[string]interface{}, len(breadcrumbs))

	for i, crumb := range breadcrumbs {
		itemList[i] = map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     crumb.Name,
			"item":     crumb.URL,
		}
	}

	schema := map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": itemList,
	}

	h.logger.Debug("Generated breadcrumb schema", "items", len(breadcrumbs))
	return schema, nil
}

// AnalyzeContent performs basic SEO analysis on article content
func (h *Helper) AnalyzeContent(content string) (*services.SEOAnalysis, error) {
	if !h.enabled {
		return nil, fmt.Errorf("SEO not enabled")
	}

	analysis := &services.SEOAnalysis{
		Keywords:    make([]string, 0),
		Suggestions: make([]string, 0),
	}

	// Word count and reading time
	words := strings.Fields(content)
	analysis.WordCount = len(words)
	analysis.ReadingTime = analysis.WordCount / 200 // 200 WPM average

	// Count headings
	headingRegex := regexp.MustCompile(`(?m)^#{1,6}\s+.+$`)
	headings := headingRegex.FindAllString(content, -1)
	analysis.HeadingCount = len(headings)

	// Count images
	imageRegex := regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	images := imageRegex.FindAllString(content, -1)
	analysis.ImageCount = len(images)

	// Count links
	linkRegex := regexp.MustCompile(`\[.*?\]\(.*?\)`)
	links := linkRegex.FindAllString(content, -1)
	analysis.LinkCount = len(links) - analysis.ImageCount

	// Basic SEO scoring
	score := 0.0

	if analysis.WordCount >= 300 {
		score += 20.0
	} else if analysis.WordCount >= 150 {
		score += 10.0
	}

	if analysis.HeadingCount >= 2 {
		score += 15.0
	} else if analysis.HeadingCount >= 1 {
		score += 8.0
	}

	if analysis.ImageCount >= 1 {
		score += 10.0
	}

	if analysis.ReadingTime >= 2 && analysis.ReadingTime <= 10 {
		score += 15.0
	}

	analysis.Score = score

	// Generate suggestions
	if analysis.WordCount < 300 {
		analysis.Suggestions = append(analysis.Suggestions, "Consider adding more content (minimum 300 words recommended)")
	}
	if analysis.HeadingCount == 0 {
		analysis.Suggestions = append(analysis.Suggestions, "Add headings to improve content structure")
	}
	if analysis.ImageCount == 0 {
		analysis.Suggestions = append(analysis.Suggestions, "Consider adding images to enhance visual appeal")
	}
	if analysis.ReadingTime > 15 {
		analysis.Suggestions = append(analysis.Suggestions, "Article might be too long; consider breaking into multiple parts")
	}

	h.logger.Debug("Content analysis completed", "word_count", analysis.WordCount, "score", analysis.Score)
	return analysis, nil
}

// Private helper functions

// resolveOGImage applies the 3-tier OG image precedence:
//  1. article.Banner (declarative; writer's explicit choice)
//  2. first inline image found in content (graceful auto-fallback)
//  3. /static/img/og-article-default.png (static default)
//
// Returns an absolute URL. The banner tier preserves writer intent; the
// inline-image tier preserves pre-banner behavior for articles that didn't
// opt into the new field.
func (h *Helper) resolveOGImage(article *models.Article, baseURL string) string {
	if src := article.BannerSrc(); src != "" {
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			return src
		}
		return strings.TrimRight(baseURL, "/") + src
	}
	if inline := extractFirstImage(article.Content, baseURL, &h.siteConfig); inline != "" {
		return inline
	}
	return strings.TrimRight(baseURL, "/") + "/static/img/og-article-default.png"
}

func (h *Helper) buildArticleURL(slug string) (string, error) {
	baseURL, err := url.Parse(h.siteConfig.BaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	articleURL, err := baseURL.Parse("/writing/" + slug)
	if err != nil {
		return "", fmt.Errorf("failed to build article URL: %w", err)
	}

	return articleURL.String(), nil
}

// Standalone utility functions (stateless)

func extractFirstImage(content, baseURL string, siteConfig *services.SiteConfig) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "![") && strings.Contains(line, "](") {
			start := strings.Index(line, "](") + 2
			end := strings.Index(line[start:], ")")
			if end > 0 {
				imagePath := line[start : start+end]
				return buildImageURL(imagePath, baseURL)
			}
		}
	}

	// Fallback to site default image
	if siteConfig.Image != "" {
		return buildImageURL(siteConfig.Image, baseURL)
	}

	return ""
}

func buildImageURL(imagePath, baseURL string) string {
	if strings.HasPrefix(imagePath, "http://") || strings.HasPrefix(imagePath, "https://") {
		return imagePath
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return imagePath
	}

	imageURL, err := base.Parse(imagePath)
	if err != nil {
		return imagePath
	}

	return imageURL.String()
}

func generateExcerpt(content string, maxLength int) string {
	// Remove markdown formatting
	text := content
	text = strings.ReplaceAll(text, "#", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", "")
	text = strings.ReplaceAll(text, "`", "")

	// Clean up whitespace
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	// Build excerpt
	var excerpt strings.Builder
	for _, word := range words {
		if excerpt.Len()+len(word)+1 > maxLength {
			break
		}
		if excerpt.Len() > 0 {
			excerpt.WriteString(" ")
		}
		excerpt.WriteString(word)
	}

	result := excerpt.String()
	if len(result) < len(strings.Join(words, " ")) {
		result += "..."
	}

	return result
}

func extractTwitterHandle(author string) string {
	if strings.Contains(author, "@") {
		parts := strings.Fields(author)
		for _, part := range parts {
			if strings.HasPrefix(part, "@") {
				handle := strings.TrimPrefix(part, "@")
				if handle != "" && len(handle) <= 15 {
					return "@" + handle
				}
			}
		}
	}
	return ""
}
