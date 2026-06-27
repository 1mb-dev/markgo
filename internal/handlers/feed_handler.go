package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

// feedVisibleTypes is the set of content types that surface as filterable
// feed listings on the home page. Read-only after init — no mutex needed.
var feedVisibleTypes = map[string]bool{
	"thought": true,
	"link":    true,
	"article": true,
	"ama":     true,
}

// feedTypeDisplayNames maps type key → human-readable label for page titles.
var feedTypeDisplayNames = map[string]string{
	"thought": "Thoughts",
	"link":    "Links",
	"article": "Articles",
	"ama":     "AMA",
}

// isValidFeedType reports whether t is a recognized feed type filter value.
// Empty string is valid (represents the unfiltered All view).
func isValidFeedType(t string) bool {
	return t == "" || feedVisibleTypes[t]
}

// typeDisplayName returns the human-readable label for a feed type key.
func typeDisplayName(t string) string {
	if n, ok := feedTypeDisplayNames[t]; ok {
		return n
	}
	return t
}

// FeedHandler handles the home/feed page.
type FeedHandler struct {
	*BaseHandler
	articleService services.ArticleServiceInterface
}

// NewFeedHandler creates a new feed handler.
func NewFeedHandler(base *BaseHandler, articleService services.ArticleServiceInterface) *FeedHandler {
	return &FeedHandler{
		BaseHandler:    base,
		articleService: articleService,
	}
}

// Home handles the home page request. When a ?type= query param is present,
// Home 301-redirects to the clean canonical path (e.g. /?type=thought →
// /thought), preserving any non-type query params. For the bare / (All)
// view, it renders the feed directly.
func (h *FeedHandler) Home(c *gin.Context) {
	typeFilter := c.Query("type")
	if !isValidFeedType(typeFilter) {
		typeFilter = ""
	}
	// Query-param form is not canonical — redirect to clean path when present.
	if typeFilter != "" {
		q := c.Request.URL.Query()
		q.Del("type")
		dst := "/" + typeFilter
		if encoded := q.Encode(); encoded != "" {
			dst += "?" + encoded
		}
		c.Redirect(http.StatusMovedPermanently, dst)
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	data, err := h.getHomeData(page, "")
	if err != nil {
		h.handleError(c, err, "Failed to get home data")
		return
	}

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// Type handles a feed type listing at a clean path (/thought, /link,
// /article, /ama). Type is derived from the URL path; an unknown path
// segment returns 404 via handleError.
func (h *FeedHandler) Type(c *gin.Context) {
	typeFilter := strings.TrimPrefix(c.Request.URL.Path, "/")
	if !isValidFeedType(typeFilter) || typeFilter == "" {
		h.handleError(c, apperrors.ErrArticleNotFound, "Type not found")
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	data, err := h.getHomeData(page, typeFilter)
	if err != nil {
		h.handleError(c, err, "Failed to get home data")
		return
	}

	data["canonicalPath"] = "/" + typeFilter
	data["path"] = "/" + typeFilter
	data["feedPath"] = "/" + typeFilter
	data["title"] = typeDisplayName(typeFilter) + " — " + h.config.Blog.Title
	data["activeFilter"] = typeFilter

	h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
	h.renderHTML(c, http.StatusOK, "base.html", data)
}

func (h *FeedHandler) getHomeData(page int, typeFilter string) (map[string]any, error) {
	allArticles := h.articleService.GetAllArticles()

	postsPerPage := h.config.Blog.PostsPerPage
	if postsPerPage <= 0 {
		postsPerPage = 20
	}
	var posts []*models.Article
	for _, a := range allArticles {
		if !a.Draft && (typeFilter == "" || a.Type == typeFilter) {
			posts = append(posts, a)
		}
	}

	pagination := models.NewPagination(page, len(posts), postsPerPage)

	start := (pagination.CurrentPage - 1) * postsPerPage
	end := min(start+postsPerPage, len(posts))
	pagePosts := posts[start:end]

	data := h.buildBaseTemplateData(h.config.Blog.Title)
	data["description"] = h.config.Blog.Description
	data["posts"] = pagePosts
	data["pagination"] = pagination
	data["activeFilter"] = typeFilter
	data["template"] = "feed"
	data["canonicalPath"] = "/"

	return data, nil
}
