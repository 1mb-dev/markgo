package web

import (
	"io/fs"
	"strings"
	"testing"
)

// TestAssetsExcludeDotfiles guards the embed directive: it must not use the
// `all:` form, which would bake dot-prefixed cruft (notably a local .DS_Store
// under web/static) into dev builds. A real asset is also asserted present so a
// pattern typo that embeds nothing is caught too.
func TestAssetsExcludeDotfiles(t *testing.T) {
	var sawRealAsset bool
	err := fs.WalkDir(Assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if base := d.Name(); strings.HasPrefix(base, ".") && base != "." {
			t.Errorf("embedded FS contains dot-prefixed entry %q — drop the all: prefix in embed.go", path)
		}
		if path == "static/css/main.css" {
			sawRealAsset = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded assets: %v", err)
	}
	if !sawRealAsset {
		t.Error("expected static/css/main.css in embedded FS — embed pattern may be broken")
	}
}
