package article

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
)

// --- Mock implementations ---

type mockRepository struct {
	articles  []*models.Article
	loadErr   error
	reloadErr error
}

func (m *mockRepository) LoadAll(_ context.Context) ([]*models.Article, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.articles, nil
}

func (m *mockRepository) GetBySlug(slug string) (*models.Article, error) {
	for _, a := range m.articles {
		if a.Slug == slug {
			return a, nil
		}
	}
	return nil, fmt.Errorf("article not found: %s: %w", slug, apperrors.ErrArticleNotFound)
}

func (m *mockRepository) GetByTag(tag string) []*models.Article {
	var result []*models.Article
	for _, a := range m.articles {
		if !a.Draft && slices.Contains(a.Tags, tag) {
			result = append(result, a)
		}
	}
	return result
}

func (m *mockRepository) GetByCategory(category string) []*models.Article {
	var result []*models.Article
	for _, a := range m.articles {
		if !a.Draft && slices.Contains(a.Categories, category) {
			result = append(result, a)
		}
	}
	return result
}

func (m *mockRepository) GetPublished() []*models.Article {
	var result []*models.Article
	for _, a := range m.articles {
		if !a.Draft {
			result = append(result, a)
		}
	}
	return result
}

func (m *mockRepository) GetPages() []*models.Article {
	var result []*models.Article
	for _, a := range m.articles {
		if !a.Draft && a.Type == TypePage {
			result = append(result, a)
		}
	}
	return result
}

func (m *mockRepository) GetDrafts() []*models.Article {
	var result []*models.Article
	for _, a := range m.articles {
		if a.Draft {
			result = append(result, a)
		}
	}
	return result
}

func (m *mockRepository) GetFeatured(limit int) []*models.Article {
	var result []*models.Article
	for _, a := range m.articles {
		if a.Featured && !a.Draft {
			result = append(result, a)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

func (m *mockRepository) GetRecent(limit int) []*models.Article {
	published := m.GetPublished()
	if limit > len(published) {
		limit = len(published)
	}
	return published[:limit]
}

func (m *mockRepository) Reload(_ context.Context) error {
	return m.reloadErr
}

func (m *mockRepository) GetLastModified() time.Time {
	return time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
}

func (m *mockRepository) UpdateDraftStatus(_ string, _ bool) error {
	return nil
}

func (m *mockRepository) GetStats() *models.Stats {
	published := m.GetPublished()
	drafts := m.GetDrafts()
	return &models.Stats{
		TotalArticles:  len(m.articles),
		PublishedCount: len(published),
		DraftCount:     len(drafts),
	}
}

type mockContentProcessor struct {
	processErr error
}

func (m *mockContentProcessor) ProcessMarkdown(content string) (string, error) {
	if m.processErr != nil {
		return "", m.processErr
	}
	return "<p>" + content + "</p>", nil
}

func (m *mockContentProcessor) GenerateExcerpt(content string, maxLength int) string {
	if len(content) > maxLength {
		return content[:maxLength] + "..."
	}
	return content
}

func (m *mockContentProcessor) ProcessDuplicateTitles(_, htmlContent string) string {
	return htmlContent
}

func (m *mockContentProcessor) CalculateReadingTime(content string) int {
	words := len(content) / 5
	minutes := words / 200
	if minutes == 0 {
		return 1
	}
	return minutes
}

func (m *mockContentProcessor) ExtractImageURLs(_ string) []string { return nil }
func (m *mockContentProcessor) ExtractLinks(_ string) []string     { return nil }
func (m *mockContentProcessor) ValidateContent(_ string) []string  { return nil }

type mockSearchService struct{}

func (m *mockSearchService) Search(articles []*models.Article, query string, limit int) []*models.SearchResult {
	var results []*models.SearchResult
	for _, a := range articles {
		if len(results) >= limit {
			break
		}
		if contains(a.Title, query) || contains(a.Content, query) {
			results = append(results, &models.SearchResult{Article: a, Score: 1.0})
		}
	}
	return results
}

func (m *mockSearchService) SearchInTitle(articles []*models.Article, query string, limit int) []*models.SearchResult {
	var results []*models.SearchResult
	for _, a := range articles {
		if len(results) >= limit {
			break
		}
		if contains(a.Title, query) {
			results = append(results, &models.SearchResult{Article: a, Score: 1.0})
		}
	}
	return results
}

func (m *mockSearchService) SearchByTag(_ []*models.Article, _ string) []*models.Article {
	return nil
}

func (m *mockSearchService) SearchByCategory(_ []*models.Article, _ string) []*models.Article {
	return nil
}

func (m *mockSearchService) SearchWithFilters(_ []*models.Article, _ string, _ *SearchFilters) []*models.SearchResult {
	return nil
}

func (m *mockSearchService) GetSuggestions(_ []*models.Article, _ string, _ int) []string {
	return []string{"suggestion1"}
}

func (m *mockSearchService) BuildSearchIndex(articles []*models.Article) SearchIndex {
	return SearchIndex{Articles: articles}
}

func (m *mockSearchService) SearchWithIndex(index SearchIndex, query string, limit int) []*models.SearchResult {
	return m.Search(index.Articles, query, limit)
}

func contains(s, substr string) bool {
	return substr != "" && s != "" && (s == substr || len(s) >= len(substr) && searchString(s, substr))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Test fixtures ---

func serviceTestArticles() []*models.Article {
	return []*models.Article{
		{
			Slug: "hello-world", Title: "Hello World", Content: "First post content",
			Tags: []string{"go", "blog"}, Categories: []string{"programming"},
			Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Featured: true,
		},
		{
			Slug: "second-post", Title: "Second Post", Content: "Another post about testing",
			Tags: []string{"go", "testing"}, Categories: []string{"programming", "tutorial"},
			Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			Slug: "draft-post", Title: "Draft Post", Content: "This is a draft",
			Tags: []string{"draft"}, Draft: true,
			Date: time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC),
		},
	}
}

func newTestService(articles []*models.Article) *CompositeService {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewCompositeService(
		&mockRepository{articles: articles},
		&mockContentProcessor{},
		&mockSearchService{},
		logger,
	)
}

// --- Tests ---

func TestCompositeService_StartStop(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	err := svc.Start(context.Background())
	require.NoError(t, err)
	assert.True(t, svc.started)

	// Double start should error
	err = svc.Start(context.Background())
	assert.Error(t, err)

	err = svc.Stop()
	require.NoError(t, err)
	assert.False(t, svc.started)

	// Double stop is a no-op
	err = svc.Stop()
	assert.NoError(t, err)
}

func TestCompositeService_StartLoadError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := NewCompositeService(
		&mockRepository{loadErr: fmt.Errorf("disk error")},
		&mockContentProcessor{},
		&mockSearchService{},
		logger,
	)

	err := svc.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disk error")
	assert.False(t, svc.started)
}

func TestCompositeService_GetAllArticles(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	articles := svc.GetAllArticles()
	// Should exclude drafts
	assert.Len(t, articles, 2)
}

func TestCompositeService_GetArticleBySlug(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	t.Run("published article", func(t *testing.T) {
		article, err := svc.GetArticleBySlug("hello-world")
		require.NoError(t, err)
		assert.Equal(t, "Hello World", article.Title)
	})

	t.Run("draft article is not found", func(t *testing.T) {
		_, err := svc.GetArticleBySlug("draft-post")
		assert.Error(t, err)
	})

	t.Run("nonexistent slug", func(t *testing.T) {
		_, err := svc.GetArticleBySlug("no-such-article")
		assert.Error(t, err)
	})
}

func TestCompositeService_GetDraftBySlug(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	t.Run("draft article", func(t *testing.T) {
		article, err := svc.GetDraftBySlug("draft-post")
		require.NoError(t, err)
		assert.Equal(t, "Draft Post", article.Title)
	})

	t.Run("published article is not a draft", func(t *testing.T) {
		_, err := svc.GetDraftBySlug("hello-world")
		assert.Error(t, err)
	})
}

func TestCompositeService_GetArticlesByTag(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	articles := svc.GetArticlesByTag("go")
	assert.Len(t, articles, 2)

	articles = svc.GetArticlesByTag("testing")
	assert.Len(t, articles, 1)

	articles = svc.GetArticlesByTag("nonexistent")
	assert.Empty(t, articles)
}

func TestCompositeService_GetArticlesByCategory(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	articles := svc.GetArticlesByCategory("programming")
	assert.Len(t, articles, 2)

	articles = svc.GetArticlesByCategory("tutorial")
	assert.Len(t, articles, 1)
}

func TestCompositeService_GetArticlesForFeed(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	articles := svc.GetArticlesForFeed(1)
	assert.Len(t, articles, 1)

	articles = svc.GetArticlesForFeed(10)
	assert.Len(t, articles, 2) // only 2 published
}

func TestCompositeService_GetFeaturedArticles(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	articles := svc.GetFeaturedArticles(10)
	assert.Len(t, articles, 1)
	assert.Equal(t, "hello-world", articles[0].Slug)
}

func TestCompositeService_GetRecentArticles(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	articles := svc.GetRecentArticles(1)
	assert.Len(t, articles, 1)
}

func TestCompositeService_GetDraftArticles(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	drafts := svc.GetDraftArticles()
	assert.Len(t, drafts, 1)
	assert.Equal(t, "draft-post", drafts[0].Slug)
}

func TestCompositeService_SearchArticles(t *testing.T) {
	t.Run("without index uses direct search", func(t *testing.T) {
		svc := newTestService(serviceTestArticles())
		results := svc.SearchArticles("Hello", 10)
		assert.Len(t, results, 1)
		assert.Equal(t, "hello-world", results[0].Slug)
	})

	t.Run("with index after start", func(t *testing.T) {
		svc := newTestService(serviceTestArticles())
		require.NoError(t, svc.Start(context.Background()))
		defer svc.Stop() //nolint:errcheck

		results := svc.SearchArticles("testing", 10)
		assert.Len(t, results, 1)
		assert.Equal(t, "second-post", results[0].Slug)
	})
}

func TestCompositeService_SearchInTitle(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	results := svc.SearchInTitle("Hello", 10)
	assert.Len(t, results, 1)

	results = svc.SearchInTitle("content", 10) // in body, not title
	assert.Empty(t, results)
}

func TestCompositeService_GetSearchSuggestions(t *testing.T) {
	svc := newTestService(serviceTestArticles())
	suggestions := svc.GetSearchSuggestions("test", 5)
	assert.NotEmpty(t, suggestions)
}

func TestCompositeService_ProcessArticleContent(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	article := &models.Article{Content: "Some markdown content"}
	err := svc.ProcessArticleContent(article)
	require.NoError(t, err)
	assert.Contains(t, article.ProcessedContent, "Some markdown content")
	assert.NotEmpty(t, article.Excerpt)
	assert.Greater(t, article.ReadingTime, 0)
	assert.Greater(t, article.WordCount, 0)
}

func TestCompositeService_ProcessArticleContent_Error(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := NewCompositeService(
		&mockRepository{articles: serviceTestArticles()},
		&mockContentProcessor{processErr: fmt.Errorf("parse error")},
		&mockSearchService{},
		logger,
	)

	article := &models.Article{Content: "bad content"}
	err := svc.ProcessArticleContent(article)
	assert.Error(t, err)
}

func TestCompositeService_GetStats(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	stats := svc.GetStats()
	assert.Equal(t, 3, stats.TotalArticles)
	assert.Equal(t, 2, stats.PublishedCount)
	assert.Equal(t, 1, stats.DraftCount)
}

func TestCompositeService_GetAllTags(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	tags := svc.GetAllTags()
	// hello-world has ["go","blog"], second-post has ["go","testing"] = unique: go, blog, testing
	assert.Len(t, tags, 3)
}

func TestCompositeService_GetAllCategories(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	categories := svc.GetAllCategories()
	// hello-world: ["programming"], second-post: ["programming","tutorial"] = unique: programming, tutorial
	assert.Len(t, categories, 2)
}

func TestCompositeService_GetTagCounts(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	tagCounts := svc.GetTagCounts()
	assert.NotEmpty(t, tagCounts)

	countMap := make(map[string]int)
	for _, tc := range tagCounts {
		countMap[tc.Tag] = tc.Count
	}
	assert.Equal(t, 2, countMap["go"])   // both published articles
	assert.Equal(t, 1, countMap["blog"]) // only hello-world
}

func TestCompositeService_GetCategoryCounts(t *testing.T) {
	svc := newTestService(serviceTestArticles())

	categoryCounts := svc.GetCategoryCounts()
	assert.NotEmpty(t, categoryCounts)

	countMap := make(map[string]int)
	for _, cc := range categoryCounts {
		countMap[cc.Category] = cc.Count
	}
	assert.Equal(t, 2, countMap["programming"]) // both published
	assert.Equal(t, 1, countMap["tutorial"])    // only second-post
}

func TestCompositeService_ReloadArticles(t *testing.T) {
	svc := newTestService(serviceTestArticles())
	require.NoError(t, svc.Start(context.Background()))
	defer svc.Stop() //nolint:errcheck

	err := svc.ReloadArticles()
	assert.NoError(t, err)
}

func TestCompositeService_ReloadArticles_Error(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := NewCompositeService(
		&mockRepository{articles: serviceTestArticles(), reloadErr: fmt.Errorf("reload failed")},
		&mockContentProcessor{},
		&mockSearchService{},
		logger,
	)

	err := svc.ReloadArticles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reload failed")
}

func TestCompositeService_ReloadArticles_WithoutStart(t *testing.T) {
	svc := newTestService(serviceTestArticles())
	// Reload without Start should use context.Background()
	err := svc.ReloadArticles()
	assert.NoError(t, err)
}

func TestCompositeService_GetLastReloadTime(t *testing.T) {
	svc := newTestService(serviceTestArticles())
	reloadTime := svc.GetLastReloadTime()
	assert.Equal(t, 2026, reloadTime.Year())
}

func TestSplitLegacyAMA(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantQ   string
		wantAns string
	}{
		{
			name:    "answered: question and answer split on first separator",
			body:    "What aged well?\n\n---\n\nPlain files did.",
			wantQ:   "What aged well?",
			wantAns: "Plain files did.",
		},
		{
			name:    "unanswered: no separator, whole body is the question",
			body:    "What is your take on plain files?",
			wantQ:   "What is your take on plain files?",
			wantAns: "",
		},
		{
			name:    "answer containing its own thematic break: split only on the first",
			body:    "Short question?\n\n---\n\nFirst part.\n\n---\n\nSecond part.",
			wantQ:   "Short question?",
			wantAns: "First part.\n\n---\n\nSecond part.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, ans := SplitLegacyAMA(tt.body)
			assert.Equal(t, tt.wantQ, q)
			assert.Equal(t, tt.wantAns, ans)
		})
	}
}

func TestProcessArticleContent_AMANormalization(t *testing.T) {
	svc := newTestService(nil)

	t.Run("legacy answered: body split, answer in Content, question excluded", func(t *testing.T) {
		a := &models.Article{Type: TypeAMA, Content: "Which decision aged well?\n\n---\n\nMarkdown files in git."}
		require.NoError(t, svc.ProcessArticleContent(a))
		assert.Equal(t, "Which decision aged well?", a.Question)
		assert.Equal(t, "Markdown files in git.", a.Content)
		// ProcessedContent (the card/detail answer body) must not leak the question.
		assert.Contains(t, a.ProcessedContent, "Markdown files in git.")
		assert.NotContains(t, a.ProcessedContent, "Which decision aged well?")
	})

	t.Run("new shape: Question set in frontmatter, answer with --- left intact", func(t *testing.T) {
		a := &models.Article{
			Type:     TypeAMA,
			Question: "Which decision aged well?",
			Content:  "Markdown files.\n\n---\n\nStill true today.",
		}
		require.NoError(t, svc.ProcessArticleContent(a))
		assert.Equal(t, "Which decision aged well?", a.Question)
		// Not split — the author's own thematic break survives.
		assert.Equal(t, "Markdown files.\n\n---\n\nStill true today.", a.Content)
	})

	t.Run("legacy unanswered: question only, empty answer", func(t *testing.T) {
		a := &models.Article{Type: TypeAMA, Content: "Just a pending question, no answer yet."}
		require.NoError(t, svc.ProcessArticleContent(a))
		assert.Equal(t, "Just a pending question, no answer yet.", a.Question)
		assert.Empty(t, a.Content)
	})

	t.Run("non-ama untouched", func(t *testing.T) {
		a := &models.Article{Type: "thought", Content: "A thought with a\n\n---\n\nbreak in it."}
		require.NoError(t, svc.ProcessArticleContent(a))
		assert.Empty(t, a.Question)
		assert.Equal(t, "A thought with a\n\n---\n\nbreak in it.", a.Content)
	})
}
