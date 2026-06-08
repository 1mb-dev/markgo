package slug

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/stretchr/testify/assert"
)

// TestGenerate is the union of every case previously pinned in
// compose/service_test.go, article/repository_test.go, and
// article/inference_test.go. A green run proves the two former
// generateSlug copies were behaviorally identical (test-first).
func TestGenerate(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"simple", "Hello World", "hello-world"},
		{"trailing punctuation dropped", "Getting Started with Go!", "getting-started-with-go"},
		{"no punctuation", "Getting Started with Go", "getting-started-with-go"},
		{"surrounding spaces", "  spaces  and  stuff  ", "spaces-and-stuff"},
		{"empty", "", ""},
		{"leading digits", "123 Numbers", "123-numbers"},
		{"special chars", "Go 1.21: What's New?", "go-121-whats-new"},
		{"consecutive hyphens collapsed", "Hello   World", "hello-world"},
		{"leading trailing trimmed", " -Hello- ", "hello"},
		{"non-latin chars dropped", "日本語タイトル", ""},
		{"mixed", "My Post #42 — Tips & Tricks", "my-post-42-tips-tricks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Generate(tt.title))
		})
	}
}

// TestContainPath migrates the path-containment + traversal-rejection cases
// from the former compose/path_test.go. These are security assertions —
// every one must survive the move.
func TestContainPath(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name       string
		slug       string
		wantErr    error
		wantInBase bool
	}{
		{name: "clean slug", slug: "my-post", wantInBase: true},
		{name: "clean slug with digits", slug: "2026-01-15-welcome", wantInBase: true},
		{name: "traversal", slug: "../escape", wantErr: apperrors.ErrPathEscape},
		{name: "deep traversal", slug: "../../etc/passwd", wantErr: apperrors.ErrPathEscape},
		{name: "empty slug", slug: "", wantInBase: false}, // resolves to base itself, not contained
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ContainPath(base, tc.slug)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if tc.wantInBase {
				if err != nil {
					t.Fatalf("want no error, got %v", err)
				}
				absBase, _ := filepath.Abs(base)
				if filepath.Dir(got) != absBase {
					t.Errorf("got %q, want parent to be %q", got, absBase)
				}
				return
			}

			// empty slug resolves to base itself, which is not strictly contained
			if err == nil {
				t.Errorf("want error for empty slug, got success: %q", got)
			}
		})
	}
}

func TestContainPath_BaseWithTrailingSeparator(t *testing.T) {
	base := t.TempDir() + string(filepath.Separator)
	got, err := ContainPath(base, "valid-slug")
	if err != nil {
		t.Fatalf("trailing separator in base should not break containment: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty path")
	}
}

// TestValidate is the union of every case previously pinned in
// commands/new.TestValidateSlug and article.TestValidateSlug — the two strict
// create-time validators this package now replaces. A green run proves the
// merged contract is no looser than either predecessor on any input.
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		// valid
		{"simple", "my-great-article", false},
		{"with digits", "article-123", false},
		{"digits only", "123", false},
		{"single char", "a", false},
		{"reserved-adjacent ok", "feed-2026", false},
		{"at length limit", strings.Repeat("a", MaxLength), false},

		// empty / whitespace
		{"empty", "", true},
		{"whitespace only", "   ", true},

		// path traversal
		{"dots", "..", true},
		{"embedded traversal", "foo/../bar", true},
		{"forward slash", "foo/bar", true},
		{"backslash", "foo\\bar", true},

		// charset
		{"uppercase", "My-Article", true},
		{"underscore", "my_page", true},
		{"space", "my article", true},
		{"unicode", "café", true},
		{"dot", "my.page", true},
		{"special chars", "my@article!", true},

		// length
		{"too long", strings.Repeat("a", MaxLength+1), true},

		// hyphen placement
		{"leading hyphen", "-my-article", true},
		{"trailing hyphen", "my-article-", true},
		{"only hyphen", "-", true},
		{"only hyphens", "---", true},
		{"consecutive hyphens", "my--article", true},

		// reserved
		{"reserved index", "index", true},
		{"reserved feed", "feed", true},
		{"reserved rss", "rss", true},
		{"reserved atom", "atom", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.slug)
			if tt.wantErr {
				assert.Error(t, err, "expected error for %q", tt.slug)
			} else {
				assert.NoError(t, err, "unexpected error for %q", tt.slug)
			}
		})
	}
}

// TestWellFormed pins the permissive route-param/stored-slug guard: charset and
// length only. Unlike Validate it accepts reserved names and consecutive
// hyphens (existing slugs must stay lookup-able), but still rejects anything
// that is not a clean URL component.
func TestWellFormed(t *testing.T) {
	tests := []struct {
		name string
		slug string
		want bool
	}{
		{"simple", "my-post", true},
		{"single char", "a", true},
		{"digits", "123", true},
		{"reserved name accepted", "feed", true},
		{"consecutive hyphens accepted", "my--post", true},
		{"at well-formed ceiling", strings.Repeat("a", wellFormedMaxLength), true},
		{"beyond create cap but within ceiling", strings.Repeat("a", MaxLength+1), true},

		{"empty", "", false},
		{"over ceiling", strings.Repeat("a", wellFormedMaxLength+1), false},
		{"uppercase", "Foo", false},
		{"space", "my post", false},
		{"slash", "foo/bar", false},
		{"traversal", "..", false},
		{"leading hyphen", "-x", false},
		{"trailing hyphen", "x-", false},
		{"unicode", "café", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WellFormed(tt.slug), "WellFormed(%q)", tt.slug)
		})
	}
}

// TestValidateImpliesWellFormed pins the contract relationship the two guards
// depend on: anything strict enough to be created is always accepted by the
// permissive lookup guard ("accepted at create ⟹ accepted at edit").
func TestValidateImpliesWellFormed(t *testing.T) {
	for _, s := range []string{"a", "my-post", "article-123", "feed-2026", strings.Repeat("a", MaxLength)} {
		if Validate(s) == nil && !WellFormed(s) {
			t.Errorf("Validate accepts %q but WellFormed rejects it", s)
		}
	}
}
