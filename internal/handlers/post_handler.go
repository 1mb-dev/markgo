package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
	"github.com/1mb-dev/markgo/internal/services/article"
)

// PostHandler handles individual article and article listing pages.
type PostHandler struct {
	*BaseHandler
	articleService services.ArticleServiceInterface
}

// NewPostHandler creates a new post handler.
func NewPostHandler(base *BaseHandler, articleService services.ArticleServiceInterface) *PostHandler {
	return &PostHandler{
		BaseHandler:    base,
		articleService: articleService,
	}
}

// Articles handles the articles listing page.
func (h *PostHandler) Articles(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	data, err := h.getArticlesPage(page)
	if err != nil {
		h.handleError(c, err, "Failed to get articles page")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// Article handles individual article requests.
func (h *PostHandler) Article(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		h.handleError(c, apperrors.NewValidationError("slug", "", "slug is required", nil), "Invalid article slug")
		return
	}

	// Dedicated-route articles (e.g. slug=about) redirect to their canonical URL.
	if art, err := h.articleService.GetArticleBySlug(slug); err == nil && article.DedicatedRouteArticle(art) {
		c.Redirect(http.StatusMovedPermanently, article.CanonicalURLFor(art))
		return
	}

	data, err := h.getArticleData(slug)
	if err != nil {
		h.handleError(c, err, "Failed to get article")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

func (h *PostHandler) getArticleData(slug string) (map[string]any, error) {
	art, err := h.articleService.GetArticleBySlug(slug)
	if err != nil {
		return nil, err
	}

	if art.Draft {
		return nil, apperrors.ErrArticleNotFound
	}

	// Get recent articles for sidebar
	allArticles := h.articleService.GetAllArticles()
	var recent []*models.Article
	for _, a := range allArticles {
		if !a.Draft && a.Slug != slug && len(recent) < 5 {
			recent = append(recent, a)
		}
	}

	data := h.buildArticlePageData(art.Title+" - "+h.config.Blog.Title, recent)
	data["article"] = art
	data["description"] = art.Description
	data["template"] = templateArticle
	data["canonicalPath"] = "/writing/" + art.Slug
	data["breadcrumbs"] = []services.Breadcrumb{
		{Name: "Home", URL: "/"},
		{Name: "Writing", URL: "/writing"},
		{Name: art.Title},
	}

	return data, nil
}

func (h *PostHandler) getArticlesPage(page int) (map[string]any, error) {
	allArticles := h.articleService.GetAllArticles()

	var published []*models.Article
	for _, article := range allArticles {
		if !article.Draft {
			published = append(published, article)
		}
	}

	postsPerPage := h.config.Blog.PostsPerPage
	if postsPerPage <= 0 {
		postsPerPage = 10
	}

	pagination := models.NewPagination(page, len(published), postsPerPage)

	start := (pagination.CurrentPage - 1) * postsPerPage
	end := start + postsPerPage
	if end > len(published) {
		end = len(published)
	}

	articles := published[start:end]

	data := h.buildBaseTemplateData("Writing - " + h.config.Blog.Title)
	data["description"] = "Writing from " + h.config.Blog.Title
	data["articles"] = articles
	data["pagination"] = pagination
	data["template"] = "articles"
	data["canonicalPath"] = "/writing"

	if baseURL := h.config.BaseURL; baseURL != "" {
		items := make([]map[string]any, len(articles))
		for i, a := range articles {
			item := map[string]any{
				"@type":         "BlogPosting",
				"position":      i + 1,
				"headline":      a.DisplayTitle(),
				"description":   a.Excerpt,
				"url":           baseURL + "/writing/" + a.Slug,
				"datePublished": a.Date.Format("2006-01-02T15:04:05Z07:00"),
				"author": map[string]any{
					"@type": "Person",
					"name":  h.config.Blog.Author,
				},
			}
			if len(a.Tags) > 0 {
				item["keywords"] = strings.Join(a.Tags, ", ")
			}
			items[i] = item
		}
		data["collectionSchema"] = map[string]any{
			"@context":    "https://schema.org",
			"@type":       "CollectionPage",
			"name":        "Writing",
			"description": "Writing from " + h.config.Blog.Title,
			"url":         baseURL + "/writing",
			"mainEntity": map[string]any{
				"@type":           "ItemList",
				"numberOfItems":   pagination.TotalItems,
				"itemListElement": items,
			},
		}
	}

	return data, nil
}
