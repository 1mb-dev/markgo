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
		{"normal article", &models.Article{Slug: "intro", Type: "article"}, "/writing/intro"},
		{"thought", &models.Article{Slug: "quick-take", Type: "thought"}, "/writing/quick-take"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CanonicalURLFor(tt.a))
		})
	}
}
