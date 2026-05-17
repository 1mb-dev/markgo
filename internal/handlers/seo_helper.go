package handlers

import (
	"log/slog"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
	articlepkg "github.com/1mb-dev/markgo/internal/services/article"
)

// SEODataHelper generates SEO data for templates
type SEODataHelper struct {
	seoService services.SEOServiceInterface
	config     *config.Config
}

// NewSEODataHelper creates a new SEO data helper
func NewSEODataHelper(seoService services.SEOServiceInterface, cfg *config.Config) *SEODataHelper {
	return &SEODataHelper{
		seoService: seoService,
		config:     cfg,
	}
}

// GenerateArticleSEOData generates comprehensive SEO data for article pages
func (h *SEODataHelper) GenerateArticleSEOData(article *models.Article) map[string]interface{} {
	if h.seoService == nil || !h.seoService.IsEnabled() {
		return map[string]interface{}{}
	}

	seoData := make(map[string]interface{})
	baseURL := h.config.BaseURL

	// Generate meta tags
	if metaTags, err := h.seoService.GenerateMetaTags(article); err == nil {
		seoData["seoMetaTags"] = metaTags
	} else {
		slog.Debug("SEO meta tag generation failed", "slug", article.Slug, "error", err)
	}

	// Generate Open Graph tags
	if ogTags, err := h.seoService.GenerateOpenGraphTags(article, baseURL); err == nil {
		// Merge with existing meta tags
		if seoMetaTags, exists := seoData["seoMetaTags"].(map[string]string); exists {
			for key, value := range ogTags {
				seoMetaTags[key] = value
			}
		} else {
			seoData["seoMetaTags"] = ogTags
		}
	} else {
		slog.Debug("SEO OpenGraph generation failed", "slug", article.Slug, "error", err)
	}

	// Generate Twitter Card tags
	if twitterTags, err := h.seoService.GenerateTwitterCardTags(article, baseURL); err == nil {
		// Merge with existing meta tags
		if seoMetaTags, exists := seoData["seoMetaTags"].(map[string]string); exists {
			for key, value := range twitterTags {
				seoMetaTags[key] = value
			}
		}
	} else {
		slog.Debug("SEO Twitter Card generation failed", "slug", article.Slug, "error", err)
	}

	// Generate Article Schema
	if articleSchema, err := h.seoService.GenerateArticleSchema(article, baseURL); err == nil {
		seoData["articleSchema"] = articleSchema
	} else {
		slog.Debug("SEO article schema generation failed", "slug", article.Slug, "error", err)
	}

	// Generate Website Schema
	if websiteSchema, err := h.seoService.GenerateWebsiteSchema(); err == nil {
		seoData["websiteSchema"] = websiteSchema
	} else {
		slog.Debug("SEO website schema generation failed", "error", err)
	}

	// Generate Breadcrumb Schema. Pages live outside the writing graph
	// (Home -> Title) and use the /p/<slug> canonical; everything else gets
	// the standard Home -> Writing -> Title trail.
	var breadcrumbs []services.Breadcrumb
	if articlepkg.DedicatedRouteArticle(article) {
		breadcrumbs = []services.Breadcrumb{
			{Name: "Home", URL: baseURL},
			{Name: article.Title, URL: baseURL + articlepkg.CanonicalURLFor(article)},
		}
	} else {
		breadcrumbs = []services.Breadcrumb{
			{Name: "Home", URL: baseURL},
			{Name: "Writing", URL: baseURL + "/writing"},
			{Name: article.Title, URL: baseURL + articlepkg.CanonicalURLFor(article)},
		}
	}
	if breadcrumbSchema, err := h.seoService.GenerateBreadcrumbSchema(breadcrumbs); err == nil {
		seoData["breadcrumbSchema"] = breadcrumbSchema
	}

	return seoData
}

// GeneratePageSEOData generates SEO data for non-article pages
func (h *SEODataHelper) GeneratePageSEOData(title, description, path string) map[string]interface{} {
	if h.seoService == nil || !h.seoService.IsEnabled() {
		return map[string]interface{}{}
	}

	seoData := make(map[string]interface{})

	// Generate page meta tags
	if metaTags, err := h.seoService.GeneratePageMetaTags(title, description, path); err == nil {
		seoData["seoMetaTags"] = metaTags
	}

	// Generate Website Schema
	if websiteSchema, err := h.seoService.GenerateWebsiteSchema(); err == nil {
		seoData["websiteSchema"] = websiteSchema
	}

	// Generate Breadcrumb Schema for pages
	breadcrumbs := []services.Breadcrumb{
		{Name: "Home", URL: h.config.BaseURL},
	}
	if path != "/" && path != "" {
		breadcrumbs = append(breadcrumbs, services.Breadcrumb{
			Name: title,
			URL:  h.config.BaseURL + path,
		})
	}
	if breadcrumbSchema, err := h.seoService.GenerateBreadcrumbSchema(breadcrumbs); err == nil {
		seoData["breadcrumbSchema"] = breadcrumbSchema
	}

	return seoData
}

// AddSEODataToTemplateData adds SEO data to existing template data
func (h *SEODataHelper) AddSEODataToTemplateData(templateData, seoData map[string]interface{}) {
	for key, value := range seoData {
		templateData[key] = value
	}
}

// GenerateHomeSEOData generates SEO data for the home page
func (h *SEODataHelper) GenerateHomeSEOData() map[string]interface{} {
	return h.GeneratePageSEOData(
		h.config.Blog.Title,
		h.config.Blog.Description,
		"/",
	)
}
