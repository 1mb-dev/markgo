package new

import (
	"strings"
	"testing"

	slugutil "github.com/1mb-dev/markgo/internal/slug"
)

func TestResolveSlug(t *testing.T) {
	tests := []struct {
		name     string
		slugFlag string
		title    string
		want     string
		wantErr  bool
	}{
		{name: "derives faithful slug from title", title: "Hello World", want: "hello-world"},
		{name: "stop words preserved (old slugify dropped them)", title: "Getting Started with Go", want: "getting-started-with-go"},
		{name: "no 5-word cap (old slugify truncated)", title: "One Two Three Four Five Six Seven", want: "one-two-three-four-five-six-seven"},
		{name: "explicit --slug overrides derivation", slugFlag: "custom-slug", title: "Totally Different Title", want: "custom-slug"},
		{name: "explicit --slug is validated and rejected", slugFlag: "Bad Slug!", title: "x", wantErr: true},
		{name: "empty-derived slug errors instead of silent untitled", title: "日本語タイトル", wantErr: true},
		{name: "punctuation-only title errors", title: "!!! ???", wantErr: true},
		{name: "default Untitled Article title stays valid", title: defaultTitle, want: "untitled-article"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSlug(tt.slugFlag, tt.title)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveSlug(%q, %q) = %q, want error", tt.slugFlag, tt.title, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSlug(%q, %q) unexpected error: %v", tt.slugFlag, tt.title, err)
			}
			if got != tt.want {
				t.Errorf("resolveSlug(%q, %q) = %q, want %q", tt.slugFlag, tt.title, got, tt.want)
			}
		})
	}
}

// TestResolveSlugMatchesRuntime is the core of v3.21.0: a title must yield the
// same slug whether an article is created via the CLI or the runtime compose
// path. Both now route through slug.Generate; this pins that they cannot drift.
func TestResolveSlugMatchesRuntime(t *testing.T) {
	titles := []string{
		"Hello World",
		"Getting Started with Go",
		"Go 1.21: What's New?",
		"My Post #42 — Tips & Tricks",
		"The Quick Brown Fox Jumps Over the Lazy Dog",
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			got, err := resolveSlug("", title)
			if err != nil {
				t.Fatalf("resolveSlug(%q, %q) error: %v", "", title, err)
			}
			if want := slugutil.Generate(title); got != want {
				t.Errorf("CLI slug %q diverges from runtime %q for title %q", got, want, title)
			}
		})
	}
}

// TestResolveSlugErrorGuidesToFlag pins the recovery-oriented error: when a
// title derives nothing usable, the operator is told to pass --slug.
func TestResolveSlugErrorGuidesToFlag(t *testing.T) {
	_, err := resolveSlug("", "日本語タイトル")
	if err == nil {
		t.Fatal("expected error for empty-derived slug")
	}
	if !strings.Contains(err.Error(), "--slug") {
		t.Errorf("error should point operator at --slug, got: %v", err)
	}
}
