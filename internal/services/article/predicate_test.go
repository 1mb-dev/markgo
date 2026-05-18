package article

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/1mb-dev/markgo/internal/models"
)

func TestDedicatedRouteArticle(t *testing.T) {
	tests := []struct {
		name string
		a    *models.Article
		want bool
	}{
		{"about slug", &models.Article{Slug: "about", Type: "article"}, true},
		{"page type", &models.Article{Slug: "run-your-own", Type: TypePage}, true},
		{"about slug and page type", &models.Article{Slug: "about", Type: TypePage}, true},
		{"normal article", &models.Article{Slug: "intro", Type: "article"}, false},
		{"thought", &models.Article{Slug: "quick-take", Type: "thought"}, false},
		{"link", &models.Article{Slug: "hn-link", Type: "link"}, false},
		{"ama", &models.Article{Slug: "ama-stack", Type: "ama"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DedicatedRouteArticle(tt.a))
		})
	}
}

func TestCanonicalURLFor(t *testing.T) {
	tests := []struct {
		name string
		a    *models.Article
		want string
	}{
		{"about slug", &models.Article{Slug: "about", Type: "article"}, "/about"},
		{"page type", &models.Article{Slug: "run-your-own", Type: TypePage}, "/p/run-your-own"},
		{"about slug + page type", &models.Article{Slug: "about", Type: TypePage}, "/about"},
		{"normal article", &models.Article{Slug: "intro", Type: "article"}, "/writing/intro"},
		{"thought", &models.Article{Slug: "quick-take", Type: "thought"}, "/writing/quick-take"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CanonicalURLFor(tt.a))
		})
	}
}

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{"valid lowercase letters", "intro", false},
		{"valid with hyphens", "my-evergreen-page", false},
		{"valid with digits", "release-2026", false},
		{"valid digits only", "123", false},
		{"valid single char", "a", false},

		{"empty", "", true},
		{"whitespace only", "   ", true},

		{"path traversal dots", "..", true},
		{"embedded traversal", "foo/../bar", true},
		{"forward slash", "foo/bar", true},
		{"backslash", "foo\\bar", true},

		{"uppercase letter", "Intro", true},
		{"underscore", "my_page", true},
		{"space", "my page", true},
		{"unicode", "café", true},
		{"dot", "my.page", true},

		{"too long", strings.Repeat("a", SlugMaxLength+1), true},
		{"at length limit", strings.Repeat("a", SlugMaxLength), false},

		{"reserved index", "index", true},
		{"reserved feed", "feed", true},
		{"reserved rss", "rss", true},
		{"reserved atom", "atom", true},
		{"reserved-adjacent ok", "feed-2026", false},

		// Leading/trailing hyphens — must reject; the codebase-wide
		// validSlug gate at compose.go:22 also rejects these, and a
		// mismatch would let pages be created but not edited.
		{"leading hyphen", "-mypage", true},
		{"trailing hyphen", "mypage-", true},
		{"only hyphen", "-", true},
		{"only hyphens", "---", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlug(tt.slug)
			if tt.wantErr {
				assert.Error(t, err, "expected error for %q", tt.slug)
			} else {
				assert.NoError(t, err, "expected no error for %q", tt.slug)
			}
		})
	}
}
