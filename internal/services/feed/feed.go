package feed

import (
	"encoding/json"
	"encoding/xml"
	"net/url"
	"sort"
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

	// Static index/nav entries carry no <lastmod>: it is optional, and a synthetic
	// "now" on every generation is a churning value search engines distrust — and
	// discount across the whole sitemap. The content entries below carry real,
	// stable dates.
	urls := []models.SitemapURL{
		{Loc: s.config.BaseURL, ChangeFreq: "weekly", Priority: 1.0},
		{Loc: s.config.BaseURL + "/writing", ChangeFreq: "daily", Priority: 0.8},
		{Loc: s.config.BaseURL + "/tags", ChangeFreq: "weekly", Priority: 0.6},
		{Loc: s.config.BaseURL + "/categories", ChangeFreq: "weekly", Priority: 0.6},
		{Loc: s.config.BaseURL + "/about", ChangeFreq: "yearly", Priority: 0.5},
		{Loc: s.config.BaseURL + "/p", ChangeFreq: "monthly", Priority: 0.5},
		{Loc: s.config.BaseURL + "/thought", ChangeFreq: "weekly", Priority: 0.6},
		{Loc: s.config.BaseURL + "/link", ChangeFreq: "weekly", Priority: 0.6},
		{Loc: s.config.BaseURL + "/article", ChangeFreq: "weekly", Priority: 0.6},
		{Loc: s.config.BaseURL + "/ama", ChangeFreq: "weekly", Priority: 0.6},
	}

	// Accumulate taxonomy terms from the same published articles we emit, so the
	// sitemap's term pages always correspond to its content.
	tags := map[string]*termStat{}
	cats := map[string]*termStat{}
	for _, a := range allArticles {
		if a.Draft {
			continue
		}
		urls = append(urls, models.SitemapURL{
			Loc:        s.config.BaseURL + articlepkg.CanonicalURLFor(a),
			LastMod:    sitemapLastMod(a),
			ChangeFreq: "monthly",
			Priority:   0.7,
		})
		for _, tag := range distinctNonEmpty(a.Tags) {
			accumulateTerm(tags, tag, a.Date)
		}
		for _, cat := range distinctNonEmpty(a.Categories) {
			accumulateTerm(cats, cat, a.Date)
		}
	}

	for _, p := range pages {
		urls = append(urls, models.SitemapURL{
			Loc:        s.config.BaseURL + articlepkg.CanonicalURLFor(p),
			LastMod:    sitemapLastMod(p),
			ChangeFreq: "yearly",
			Priority:   0.5,
		})
	}

	urls = append(urls, termURLs(s.config.BaseURL+"/tags/", tags)...)
	urls = append(urls, termURLs(s.config.BaseURL+"/categories/", cats)...)

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

// termStat tracks, for one taxonomy term, how many articles carry it and the
// newest publish date among them.
type termStat struct {
	count  int
	latest time.Time
}

// distinctNonEmpty trims, drops empty, and de-duplicates a term list so each
// article counts once per term (a duplicate tag must not inflate the gate) and a
// stray empty tag never becomes a "/tags/" or "/tags/%20" sitemap URL.
func distinctNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}

func accumulateTerm(m map[string]*termStat, term string, date time.Time) {
	st := m[term]
	if st == nil {
		st = &termStat{}
		m[term] = st
	}
	st.count++
	if date.After(st.latest) {
		st.latest = date
	}
}

// formatSitemapTime renders a W3C datetime, or "" to omit <lastmod>. lastmod is
// optional, and a zero time.Time would render the bogus "0001-01-01T00:00:00Z"
// that search engines reject. We key on the frontmatter Date (deploy-stable) —
// file mtime is reset by image builds and checkouts, which would churn every
// date and get the whole sitemap's lastmod signal discounted.
func formatSitemapTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// sitemapLastMod is the formatted <lastmod> for an article/page entry, from its
// frontmatter date (omitted for date-less entries such as pages).
func sitemapLastMod(a *models.Article) string {
	return formatSitemapTime(a.Date)
}

// termURLs builds sorted sitemap entries for taxonomy term pages carried by at
// least two articles — a single-article term page is thin, near-duplicate
// content that dilutes crawl quality (those terms stay reachable via the
// article's own links). Terms are path-escaped to match the canonical handler
// URL; lastmod is the newest article date for the term (omitted if unknown).
func termURLs(prefix string, terms map[string]*termStat) []models.SitemapURL {
	const minArticles = 2

	names := make([]string, 0, len(terms))
	for name, st := range terms {
		if st.count >= minArticles {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	out := make([]models.SitemapURL, 0, len(names))
	for _, name := range names {
		out = append(out, models.SitemapURL{
			Loc:        prefix + url.PathEscape(name),
			LastMod:    formatSitemapTime(terms[name].latest),
			ChangeFreq: "weekly",
			Priority:   0.4,
		})
	}
	return out
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
