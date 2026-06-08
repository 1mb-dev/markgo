package article

import (
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

// Strict slug validation moved to internal/slug (slug.TestValidate), which
// pins the union of this package's former contract and the CLI's.
