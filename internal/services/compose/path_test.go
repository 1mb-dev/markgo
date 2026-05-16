package compose

import (
	"errors"
	"path/filepath"
	"testing"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

func TestContainSlugPath(t *testing.T) {
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
			got, err := ContainSlugPath(base, tc.slug)

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

			// empty slug: resolves to base itself; we expect the containment check to fail
			// (joined path equals base, which is not strictly inside base+separator)
			if err == nil {
				t.Errorf("want error for empty slug, got success: %q", got)
			}
		})
	}
}

func TestContainSlugPath_BaseWithTrailingSeparator(t *testing.T) {
	base := t.TempDir() + string(filepath.Separator)
	got, err := ContainSlugPath(base, "valid-slug")
	if err != nil {
		t.Fatalf("trailing separator in base should not break containment: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty path")
	}
}
