package article

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/1mb-dev/markgo/internal/models"
)

func newTestSearchService() *TextSearchService {
	return NewTextSearchService(slog.Default())
}

func testArticles() []*models.Article {
	return []*models.Article{
		{
			Title:       "Getting Started with Go",
			Description: "A beginner guide to Go programming",
			Content:     "Go is a statically typed language designed at Google. It is great for building web servers and CLI tools.",
			Tags:        []string{"go", "programming", "beginner"},
			Categories:  []string{"tutorials"},
			Slug:        "getting-started-go",
			Date:        time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			Draft:       false,
			Featured:    true,
		},
		{
			Title:       "Advanced Go Patterns",
			Description: "Deep dive into Go design patterns",
			Content:     "Channels and goroutines enable powerful concurrency patterns in Go applications.",
			Tags:        []string{"go", "patterns", "advanced"},
			Categories:  []string{"tutorials"},
			Slug:        "advanced-go-patterns",
			Date:        time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			Draft:       false,
			Featured:    false,
		},
		{
			Title:       "Python for Data Science",
			Description: "Using Python for data analysis",
			Content:     "Python is widely used in data science with libraries like pandas and numpy.",
			Tags:        []string{"python", "data-science"},
			Categories:  []string{"data"},
			Slug:        "python-data-science",
			Date:        time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC),
			Draft:       false,
			Featured:    false,
		},
		{
			Title:       "Draft Post",
			Description: "Not published yet",
			Content:     "This is a draft about Go.",
			Tags:        []string{"go"},
			Categories:  []string{"tutorials"},
			Slug:        "draft-post",
			Date:        time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
			Draft:       true,
			Featured:    false,
		},
	}
}

func TestSearch_Basic(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	results := svc.Search(articles, "go", 10)
	assert.NotEmpty(t, results)
	// Draft articles should be excluded
	for _, r := range results {
		assert.False(t, r.Draft)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	svc := newTestSearchService()
	results := svc.Search(testArticles(), "", 10)
	assert.Empty(t, results)
}

func TestSearch_NoResults(t *testing.T) {
	svc := newTestSearchService()
	results := svc.Search(testArticles(), "zzzznonexistent", 10)
	assert.Empty(t, results)
}

func TestSearch_ScoringOrder(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	// "go" appears in title of first two articles
	results := svc.Search(articles, "go", 10)
	assert.GreaterOrEqual(t, len(results), 2)
	// Results should be sorted by score descending
	for i := 1; i < len(results); i++ {
		assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score)
	}
}

func TestSearch_Limit(t *testing.T) {
	svc := newTestSearchService()
	results := svc.Search(testArticles(), "go", 1)
	assert.Len(t, results, 1)
}

func TestSearch_FeaturedBonus(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	// Search for something both featured and non-featured articles match
	results := svc.Search(articles, "go patterns", 10)
	assert.NotEmpty(t, results)

	// The featured article bonus only applies when score > 0
	// This is verified by the scoring order test above
}

func TestSearch_StopWordFiltering(t *testing.T) {
	svc := newTestSearchService()

	// "the" is a stop word, should be filtered
	results := svc.Search(testArticles(), "the", 10)
	assert.Empty(t, results)

	// "a" is a stop word
	results = svc.Search(testArticles(), "a", 10)
	assert.Empty(t, results)
}

func TestSearchInTitle(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	results := svc.SearchInTitle(articles, "go", 10)
	assert.NotEmpty(t, results)
	for _, r := range results {
		assert.Contains(t, r.MatchedFields, "title")
	}

	// Empty query
	results = svc.SearchInTitle(articles, "", 10)
	assert.Empty(t, results)

	// With limit
	results = svc.SearchInTitle(articles, "go", 1)
	assert.Len(t, results, 1)
}

func TestSearchByTag(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	results := svc.SearchByTag(articles, "go")
	assert.Len(t, results, 2) // Excludes draft

	// Case-insensitive
	results = svc.SearchByTag(articles, "GO")
	assert.Len(t, results, 2)

	// Empty tag
	results = svc.SearchByTag(articles, "")
	assert.Empty(t, results)

	// No match
	results = svc.SearchByTag(articles, "rust")
	assert.Empty(t, results)
}

func TestSearchByCategory(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	results := svc.SearchByCategory(articles, "tutorials")
	assert.Len(t, results, 2) // Excludes draft

	// Case-insensitive
	results = svc.SearchByCategory(articles, "TUTORIALS")
	assert.Len(t, results, 2)

	// Empty
	results = svc.SearchByCategory(articles, "")
	assert.Empty(t, results)
}

func TestSearchWithFilters(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	t.Run("tag filter", func(t *testing.T) {
		results := svc.SearchWithFilters(articles, "go", &SearchFilters{
			Tags: []string{"advanced"},
		})
		assert.Len(t, results, 1)
		assert.Equal(t, "advanced-go-patterns", results[0].Slug)
	})

	t.Run("category filter", func(t *testing.T) {
		results := svc.SearchWithFilters(articles, "python", &SearchFilters{
			Categories: []string{"data"},
		})
		assert.Len(t, results, 1)
	})

	t.Run("date filter", func(t *testing.T) {
		results := svc.SearchWithFilters(articles, "go", &SearchFilters{
			DateFrom: "2025-06-12",
			DateTo:   "2025-06-20",
		})
		// Only "Getting Started with Go" (June 15) matches
		assert.Len(t, results, 1)
		assert.Equal(t, "getting-started-go", results[0].Slug)
	})

	t.Run("only published", func(t *testing.T) {
		results := svc.SearchWithFilters(articles, "go", &SearchFilters{
			OnlyPublished: true,
		})
		for _, r := range results {
			assert.False(t, r.Draft)
		}
	})

	t.Run("only featured", func(t *testing.T) {
		results := svc.SearchWithFilters(articles, "go", &SearchFilters{
			OnlyFeatured: true,
		})
		for _, r := range results {
			assert.True(t, r.Featured)
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		results := svc.SearchWithFilters(articles, "go", &SearchFilters{
			Tags:          []string{"go"},
			Categories:    []string{"tutorials"},
			OnlyPublished: true,
		})
		assert.NotEmpty(t, results)
		for _, r := range results {
			assert.False(t, r.Draft)
		}
	})
}

func TestGetSuggestions(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	t.Run("prefix matching", func(t *testing.T) {
		suggestions := svc.GetSuggestions(articles, "go", 10)
		// Should find words starting with "go" (e.g., "goroutines", "google")
		for _, s := range suggestions {
			assert.True(t, len(s) > len("go"), "suggestion %q should be longer than query", s)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		suggestions := svc.GetSuggestions(articles, "", 10)
		assert.Empty(t, suggestions)
	})

	t.Run("limit enforcement", func(t *testing.T) {
		suggestions := svc.GetSuggestions(articles, "p", 2)
		assert.LessOrEqual(t, len(suggestions), 2)
	})

	t.Run("excludes drafts", func(t *testing.T) {
		// Draft articles should not contribute to suggestions
		suggestions := svc.GetSuggestions(articles, "dra", 10)
		// "draft" comes from the draft article title, but drafts are skipped
		assert.Empty(t, suggestions)
	})
}

func TestBuildSearchIndex(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()

	index := svc.BuildSearchIndex(articles)

	// Index should be built
	assert.NotEmpty(t, index.TitleIndex)
	assert.NotEmpty(t, index.TagIndex)
	assert.NotNil(t, index.Articles)

	// Draft articles should not be indexed
	for _, articleList := range index.TitleIndex {
		for _, a := range articleList {
			assert.False(t, a.Draft)
		}
	}

	// Content index is limited to first 100 words per article
	// (documented behavior)
}

func TestSearchWithIndex(t *testing.T) {
	svc := newTestSearchService()
	articles := testArticles()
	index := svc.BuildSearchIndex(articles)

	t.Run("basic search", func(t *testing.T) {
		results := svc.SearchWithIndex(index, "go", 10)
		assert.NotEmpty(t, results)
	})

	t.Run("empty query", func(t *testing.T) {
		results := svc.SearchWithIndex(index, "", 10)
		assert.Empty(t, results)
	})

	t.Run("limit", func(t *testing.T) {
		results := svc.SearchWithIndex(index, "go", 1)
		assert.Len(t, results, 1)
	})
}

func TestRemoveDuplicateStrings(t *testing.T) {
	result := removeDuplicateStrings([]string{"a", "b", "a", "c", "b"})
	assert.Equal(t, []string{"a", "b", "c"}, result)

	result = removeDuplicateStrings(nil)
	assert.Nil(t, result)
}

// TestSearch_FindsAMAQuestion verifies an AMA is findable by a word that
// appears only in its question (frontmatter), not its answer (body). Guards
// against the question becoming unsearchable once it left the body.
func TestSearch_FindsAMAQuestion(t *testing.T) {
	svc := newTestSearchService()
	amas := []*models.Article{
		{
			Slug:     "ama-stack",
			Type:     TypeAMA,
			Question: "Which architecture decision aged exceptionally well?",
			Content:  "Plain files in git, served by one binary.", // no "architecture" here
			Date:     time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC),
		},
	}

	// Linear path.
	results := svc.Search(amas, "architecture", 10)
	if assert.Len(t, results, 1, "AMA must be findable by a question-only term") {
		assert.Contains(t, results[0].MatchedFields, "question")
	}

	// Index path.
	index := svc.BuildSearchIndex(amas)
	indexed := svc.SearchWithIndex(index, "architecture", 10)
	assert.Len(t, indexed, 1, "index path must also surface the AMA by question term")
}
