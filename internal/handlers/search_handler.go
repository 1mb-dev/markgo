package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

// SearchHandler handles search requests.
type SearchHandler struct {
	*BaseHandler
	articleService services.ArticleServiceInterface
}

// NewSearchHandler creates a new search handler.
func NewSearchHandler(base *BaseHandler, articleService services.ArticleServiceInterface) *SearchHandler {
	return &SearchHandler{
		BaseHandler:    base,
		articleService: articleService,
	}
}

// maxSearchQueryLength caps the search query at the handler entry to keep
// search-cache keys bounded. Without this, a bot probing 100K distinct long
// query strings could saturate the obcache with megabyte-scale keys.
const maxSearchQueryLength = 200

// Search handles search requests.
func (h *SearchHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if len(query) > maxSearchQueryLength {
		query = query[:maxSearchQueryLength]
	}

	data, err := h.getSearchResults(query)
	if err != nil {
		h.handleError(c, err, "Failed to perform search")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

func (h *SearchHandler) getSearchResults(query string) (map[string]any, error) {
	var results []*models.SearchResult
	var title, description string
	var totalCount int

	allArticles := h.articleService.GetAllArticles()
	for _, a := range allArticles {
		if !a.Draft {
			totalCount++
		}
	}

	if query == "" {
		results = []*models.SearchResult{}
		title = "Search - " + h.config.Blog.Title
		description = "Search through articles on " + h.config.Blog.Title
	} else {
		results = h.articleService.SearchArticles(query, 50)
		title = "Search results for \"" + query + "\" - " + h.config.Blog.Title
		description = "Search results for \"" + query + "\""
	}

	data := h.buildBaseTemplateData(title)
	data["description"] = description
	data["results"] = results
	data["query"] = query
	data["count"] = len(results)
	data["totalCount"] = totalCount
	data["template"] = "search"
	data["canonicalPath"] = "/search"

	return data, nil
}
