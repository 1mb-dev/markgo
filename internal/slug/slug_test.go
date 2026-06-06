package slug

import (
	"errors"
	"path/filepath"
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
