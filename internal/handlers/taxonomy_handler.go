package handlers

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
	articlepkg "github.com/1mb-dev/markgo/internal/services/article"
)

// TaxonomyHandler handles tag and category pages.
type TaxonomyHandler struct {
	*BaseHandler
	articleService services.ArticleServiceInterface
}

// NewTaxonomyHandler creates a new taxonomy handler.
func NewTaxonomyHandler(base *BaseHandler, articleService services.ArticleServiceInterface) *TaxonomyHandler {
	return &TaxonomyHandler{
		BaseHandler:    base,
		articleService: articleService,
	}
}

// Tags handles the tags listing page.
func (h *TaxonomyHandler) Tags(c *gin.Context) {
	data, err := h.getTagsPage()
	if err != nil {
		h.handleError(c, err, "Failed to get tags")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// Categories handles the categories listing page.
func (h *TaxonomyHandler) Categories(c *gin.Context) {
	data, err := h.getCategoriesPage()
	if err != nil {
		h.handleError(c, err, "Failed to get categories")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// ArticlesByTag handles articles filtered by tag.
func (h *TaxonomyHandler) ArticlesByTag(c *gin.Context) {
	tag := c.Param("tag")
	if tag == "" {
		h.handleError(c, apperrors.NewValidationError("tag", "", "tag is required", nil), "Invalid tag")
		return
	}

	decodedTag, err := url.QueryUnescape(tag)
	if err != nil {
		h.handleError(c, fmt.Errorf("invalid tag format: %v", err), "Invalid tag")
		return
	}
	tag = decodedTag

	data, err := h.getTagArticles(tag)
	if err != nil {
		h.handleError(c, err, "Failed to get articles by tag")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// ArticlesByCategory handles articles filtered by category.
func (h *TaxonomyHandler) ArticlesByCategory(c *gin.Context) {
	category := c.Param("category")
	if category == "" {
		h.handleError(c, apperrors.NewValidationError("category", "", "category is required", nil), "Invalid category")
		return
	}

	decodedCategory, err := url.QueryUnescape(category)
	if err != nil {
		h.handleError(c, fmt.Errorf("invalid category format: %v", err), "Invalid category")
		return
	}
	category = decodedCategory

	data, err := h.getCategoryArticles(category)
	if err != nil {
		h.handleError(c, err, "Failed to get articles by category")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

func (h *TaxonomyHandler) getTagArticles(tag string) (map[string]any, error) {
	articles := h.articleService.GetArticlesByTag(tag)

	var published []*models.Article
	for _, article := range articles {
		if !article.Draft {
			published = append(published, article)
		}
	}

	data := h.buildBaseTemplateData("Articles tagged with " + tag + " - " + h.config.Blog.Title)
	data["description"] = "Articles tagged with " + tag
	data["articles"] = published
	data["tag"] = tag
	data["totalCount"] = len(published)
	data["template"] = "tag"
	data["canonicalPath"] = "/tags/" + url.PathEscape(tag)
	data["breadcrumbs"] = []services.Breadcrumb{
		{Name: "Home", URL: "/"},
		{Name: "Tags", URL: "/tags"},
		{Name: tag},
	}

	if baseURL := h.config.BaseURL; baseURL != "" {
		items := make([]map[string]any, len(published))
		for i, a := range published {
			items[i] = map[string]any{
				"@type":    "ListItem",
				"position": i + 1,
				"url":      baseURL + articlepkg.CanonicalURLFor(a),
			}
		}
		data["collectionSchema"] = map[string]any{
			"@context": "https://schema.org",
			"@type":    "CollectionPage",
			"name":     "Articles tagged with " + tag,
			"url":      baseURL + "/tags/" + url.PathEscape(tag),
			"mainEntity": map[string]any{
				"@type":           "ItemList",
				"numberOfItems":   len(published),
				"itemListElement": items,
			},
		}
	}

	return data, nil
}

func (h *TaxonomyHandler) getCategoryArticles(category string) (map[string]any, error) {
	articles := h.articleService.GetArticlesByCategory(category)

	var published []*models.Article
	for _, article := range articles {
		if !article.Draft {
			published = append(published, article)
		}
	}

	data := h.buildBaseTemplateData("Articles in " + category + " - " + h.config.Blog.Title)
	data["description"] = "Articles in " + category
	data["articles"] = published
	data["category"] = category
	data["totalCount"] = len(published)
	data["template"] = "category"
	data["canonicalPath"] = "/categories/" + url.PathEscape(category)
	data["breadcrumbs"] = []services.Breadcrumb{
		{Name: "Home", URL: "/"},
		{Name: "Categories", URL: "/categories"},
		{Name: category},
	}

	if baseURL := h.config.BaseURL; baseURL != "" {
		items := make([]map[string]any, len(published))
		for i, a := range published {
			items[i] = map[string]any{
				"@type":    "ListItem",
				"position": i + 1,
				"url":      baseURL + articlepkg.CanonicalURLFor(a),
			}
		}
		data["collectionSchema"] = map[string]any{
			"@context": "https://schema.org",
			"@type":    "CollectionPage",
			"name":     "Articles in " + category,
			"url":      baseURL + "/categories/" + url.PathEscape(category),
			"mainEntity": map[string]any{
				"@type":           "ItemList",
				"numberOfItems":   len(published),
				"itemListElement": items,
			},
		}
	}

	return data, nil
}

func (h *TaxonomyHandler) getTagsPage() (map[string]any, error) {
	tagCounts := h.articleService.GetTagCounts()

	data := h.buildBaseTemplateData("Tags - " + h.config.Blog.Title)
	data["description"] = "All tags from " + h.config.Blog.Title
	data["tags"] = tagCounts
	data["count"] = len(tagCounts)
	data["template"] = "tags"
	data["canonicalPath"] = "/tags"

	if baseURL := h.config.BaseURL; baseURL != "" {
		items := make([]map[string]any, len(tagCounts))
		for i, tc := range tagCounts {
			items[i] = map[string]any{
				"@type":    "ListItem",
				"position": i + 1,
				"item": map[string]any{
					"@type": "Thing",
					"name":  tc.Tag,
					"url":   baseURL + "/tags/" + url.PathEscape(tc.Tag),
				},
			}
		}
		data["collectionSchema"] = map[string]any{
			"@context":    "https://schema.org",
			"@type":       "CollectionPage",
			"name":        "Tags - " + h.config.Blog.Title,
			"description": "Browse posts by tag on " + h.config.Blog.Title,
			"url":         baseURL + "/tags",
			"mainEntity": map[string]any{
				"@type":           "ItemList",
				"numberOfItems":   len(tagCounts),
				"itemListElement": items,
			},
		}
	}

	return data, nil
}

func (h *TaxonomyHandler) getCategoriesPage() (map[string]any, error) {
	categoryCounts := h.articleService.GetCategoryCounts()

	data := h.buildBaseTemplateData("Categories - " + h.config.Blog.Title)
	data["description"] = "All categories from " + h.config.Blog.Title
	data["categories"] = categoryCounts
	data["count"] = len(categoryCounts)
	data["template"] = "categories"
	data["canonicalPath"] = "/categories"

	if baseURL := h.config.BaseURL; baseURL != "" {
		items := make([]map[string]any, len(categoryCounts))
		for i, cc := range categoryCounts {
			items[i] = map[string]any{
				"@type":    "ListItem",
				"position": i + 1,
				"item": map[string]any{
					"@type": "Thing",
					"name":  cc.Category,
					"url":   baseURL + "/categories/" + url.PathEscape(cc.Category),
				},
			}
		}
		data["collectionSchema"] = map[string]any{
			"@context":    "https://schema.org",
			"@type":       "CollectionPage",
			"name":        "Categories - " + h.config.Blog.Title,
			"description": "Browse posts by category on " + h.config.Blog.Title,
			"url":         baseURL + "/categories",
			"mainEntity": map[string]any{
				"@type":           "ItemList",
				"numberOfItems":   len(categoryCounts),
				"itemListElement": items,
			},
		}
	}

	return data, nil
}
