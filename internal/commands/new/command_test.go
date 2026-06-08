package new

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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
		{name: "over-long derived slug errors, not silently truncated", title: strings.Repeat("verylongword ", 10), wantErr: true},
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

// TestGeneratedArticleSlugHonorsResolvedSlug pins the end-to-end contract: the
// repository derives an article's served slug from frontmatter `slug:` (mirrored
// here by the same split + yaml parse), falling back to slug.Generate(title)
// only when absent. So a CLI-created article must carry the resolved slug in
// frontmatter — otherwise --slug would set only the filename and the served URL
// would silently become the title-derived slug.
func TestGeneratedArticleSlugHonorsResolvedSlug(t *testing.T) {
	type frontmatter struct {
		Slug  string `yaml:"slug"`
		Title string `yaml:"title"`
	}
	tmpl := GetAvailableTemplates()["default"]
	cases := []struct {
		name, title, resolved, want string
	}{
		{"explicit --slug overrides title derivation", "Go 1.26 Notes", "go126", "go126"},
		{"derived slug is persisted, not re-derived", "Getting Started with Go", "getting-started-with-go", "getting-started-with-go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := tmpl.Generator(tc.title, "", "general", "uncategorized", "vmx", false, false)
			content = injectSlugFrontmatter(content, tc.resolved)

			parts := strings.SplitN(content, "---", 3) // mirrors repository.go load
			if len(parts) < 3 {
				t.Fatalf("generated content missing frontmatter:\n%s", content)
			}
			var fm frontmatter
			if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
				t.Fatalf("frontmatter is not valid YAML: %v\n%s", err, parts[1])
			}
			if fm.Slug != tc.want {
				t.Errorf("served slug = %q, want %q (--slug ignored at serve time?)", fm.Slug, tc.want)
			}
			if tc.resolved != slugutil.Generate(tc.title) && fm.Slug == slugutil.Generate(tc.title) {
				t.Errorf("explicit slug %q collapsed to title-derived %q", tc.resolved, fm.Slug)
			}
		})
	}
}

// TestInjectSlugFrontmatter covers the insertion helper directly.
func TestInjectSlugFrontmatter(t *testing.T) {
	got := injectSlugFrontmatter("---\ntitle: \"X\"\n---\n\nbody", "my-slug")
	if !strings.HasPrefix(got, "---\nslug: \"my-slug\"\ntitle: \"X\"\n") {
		t.Errorf("slug not inserted at frontmatter head:\n%s", got)
	}
	if got := injectSlugFrontmatter("no frontmatter", "x"); got != "no frontmatter" {
		t.Errorf("expected no-op on content without frontmatter, got %q", got)
	}
}
