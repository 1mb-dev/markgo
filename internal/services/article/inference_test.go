package article

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/1mb-dev/markgo/internal/models"
	slugutil "github.com/1mb-dev/markgo/internal/slug"
)

func TestInferPostType(t *testing.T) {
	tests := []struct {
		name     string
		article  *models.Article
		expected string
	}{
		{
			name: "explicit type respected",
			article: &models.Article{
				Type:    "link",
				Title:   "Some Title",
				Content: "Some content here",
			},
			expected: "link",
		},
		{
			name: "has link_url with no explicit type",
			article: &models.Article{
				LinkURL:   "https://example.com",
				Title:     "Interesting Link",
				Content:   "Check this out",
				WordCount: 3,
			},
			expected: "link",
		},
		{
			name: "no title and short content is thought",
			article: &models.Article{
				Content:   "Just a quick thought about something.",
				WordCount: 6,
			},
			expected: "thought",
		},
		{
			name: "no title but long content is article",
			article: &models.Article{
				Content:   "Long content here...",
				WordCount: 150,
			},
			expected: "article",
		},
		{
			name: "has title and content is article",
			article: &models.Article{
				Title:     "Getting Started with Go",
				Content:   "Go is a great language...",
				WordCount: 500,
			},
			expected: "article",
		},
		{
			name: "existing articles default to article",
			article: &models.Article{
				Title:     "Full Blog Post",
				Content:   "Lots of content here about programming.",
				WordCount: 200,
				Tags:      []string{"golang", "tutorial"},
			},
			expected: "article",
		},
		{
			name: "explicit type ama respected",
			article: &models.Article{
				Type:      "ama",
				Content:   "What is your favorite language?",
				WordCount: 6,
			},
			expected: "ama",
		},
		{
			name: "thought at boundary (99 words)",
			article: &models.Article{
				Content:   "Short",
				WordCount: 99,
			},
			expected: "thought",
		},
		{
			name: "not thought at boundary (100 words, no title)",
			article: &models.Article{
				Content:   "Longer",
				WordCount: 100,
			},
			expected: "article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferPostType(tt.article)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestThoughtSlugGeneration(t *testing.T) {
	fixedDate := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	// Titled article gets title-based slug
	gotSlug := slugutil.Generate("Getting Started with Go")
	assert.Equal(t, "getting-started-with-go", gotSlug)

	// Empty title generates empty slug
	emptySlug := slugutil.Generate("")
	assert.Equal(t, "", emptySlug)

	// Titleless posts use timestamp-based slug (as done in repository.go)
	thoughtSlug := fmt.Sprintf("thought-%d", fixedDate.UnixMilli())
	assert.Contains(t, thoughtSlug, "thought-")
	assert.Regexp(t, `^thought-\d+$`, thoughtSlug)
}
