package feed

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"time"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
	articlepkg "github.com/1mb-dev/markgo/internal/services/article"
)

// Service generates RSS, JSON Feed, and Sitemap content.
type Service struct {
	articleService services.ArticleServiceInterface
	config         *config.Config
}

// NewService creates a new feed service.
func NewService(articleService services.ArticleServiceInterface, cfg *config.Config) *Service {
	return &Service{
		articleService: articleService,
		config:         cfg,
	}
}

// RSS structs for safe XML serialization

type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

// GenerateRSS produces an RSS 2.0 XML feed.
func (s *Service) GenerateRSS() (string, error) {
	published := s.publishedArticles(20)

	items := make([]rssItem, 0, len(published))
	for _, a := range published {
		canonical := s.config.BaseURL + articlepkg.CanonicalURLFor(a)
		items = append(items, rssItem{
			Title:       a.DisplayTitle(),
			Link:        canonical,
			Description: a.Description,
			PubDate:     a.Date.Format(time.RFC1123Z),
			GUID:        canonical,
		})
	}

	doc := rssDoc{
		Version: "2.0",
		Channel: rssChannel{
			Title:       s.config.Blog.Title,
			Link:        s.config.BaseURL,
			Description: s.config.Blog.Description,
			Language:    s.config.Blog.Language,
			Items:       items,
		},
	}

	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + string(out), nil
}

// GenerateJSONFeed produces a JSON Feed 1.1 document.
func (s *Service) GenerateJSONFeed() (string, error) {
	published := s.publishedArticles(20)

	items := make([]map[string]any, 0, len(published))
	for _, a := range published {
		canonical := s.config.BaseURL + articlepkg.CanonicalURLFor(a)
		item := map[string]any{
			"id":             canonical,
			"url":            canonical,
			"title":          a.DisplayTitle(),
			"summary":        a.Description,
			"date_published": a.Date.Format(time.RFC3339),
		}
		if len(a.Tags) > 0 {
			item["tags"] = a.Tags
		}
		if src := a.BannerSrc(); src != "" {
			if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
				item["image"] = src
			} else {
				item["image"] = strings.TrimRight(s.config.BaseURL, "/") + src
			}
		}
		items = append(items, item)
	}

	feed := map[string]any{
		"version":       "https://jsonfeed.org/version/1.1",
		"title":         s.config.Blog.Title,
		"home_page_url": s.config.BaseURL,
		"feed_url":      s.config.BaseURL + "/feed.json",
		"description":   s.config.Blog.Description,
		"authors": []map[string]string{
			{
				"name": s.config.Blog.Author,
			},
		},
		"items": items,
	}

	out, err := json.Marshal(feed)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GenerateSitemap produces an XML sitemap. Includes:
//   - Site-wide static entries (home, /writing, /tags, /categories, /about, /p)
//   - Published articles (from GetAllArticles, which is GetPublished — already
//     excludes dedicated-route content via the predicate)
//   - Pages (from GetPages, the only list-shaped accessor that returns
//     dedicated-route type:page content)
//
// All URLs route through articlepkg.CanonicalURLFor — no hardcoded paths.
// /about and /p surface as static entries since neither maps to a single
// article: /about is its own handler, /p is the pages index.
func (s *Service) GenerateSitemap() (string, error) {
	allArticles := s.articleService.GetAllArticles()
	pages := s.articleService.GetPages()

	urls := []models.SitemapURL{
		{Loc: s.config.BaseURL, LastMod: time.Now(), ChangeFreq: "weekly", Priority: 1.0},
		{Loc: s.config.BaseURL + "/writing", LastMod: time.Now(), ChangeFreq: "daily", Priority: 0.8},
		{Loc: s.config.BaseURL + "/tags", LastMod: time.Now(), ChangeFreq: "weekly", Priority: 0.6},
		{Loc: s.config.BaseURL + "/categories", LastMod: time.Now(), ChangeFreq: "weekly", Priority: 0.6},
		{Loc: s.config.BaseURL + "/about", LastMod: time.Now(), ChangeFreq: "yearly", Priority: 0.5},
		{Loc: s.config.BaseURL + "/p", LastMod: time.Now(), ChangeFreq: "monthly", Priority: 0.5},
	}

	for _, a := range allArticles {
		if !a.Draft {
			urls = append(urls, models.SitemapURL{
				Loc:        s.config.BaseURL + articlepkg.CanonicalURLFor(a),
				LastMod:    a.Date,
				ChangeFreq: "monthly",
				Priority:   0.7,
			})
		}
	}

	for _, p := range pages {
		urls = append(urls, models.SitemapURL{
			Loc:        s.config.BaseURL + articlepkg.CanonicalURLFor(p),
			LastMod:    p.Date,
			ChangeFreq: "yearly",
			Priority:   0.5,
		})
	}

	sitemap := models.Sitemap{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}

	out, err := xml.MarshalIndent(sitemap, "", "  ")
	if err != nil {
		return "", err
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + string(out), nil
}

func (s *Service) publishedArticles(limit int) []*models.Article {
	all := s.articleService.GetAllArticles()
	var published []*models.Article
	for _, a := range all {
		if !a.Draft && len(published) < limit {
			published = append(published, a)
		}
	}
	return published
}
