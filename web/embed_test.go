package web

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// embedProbe is a tracked dotfile that must stay excluded from the embed. It
// makes this test bite in a clean CI checkout, where .DS_Store is absent: without
// a committed dotfile, a restored `all:` prefix would embed nothing to catch and
// the test would pass vacuously.
const embedProbe = "static/.embed-probe"

// TestAssetsExcludeDotfiles guards the embed directive against the `all:` form,
// which would bake dot/underscore-prefixed cruft (a local .DS_Store, editor
// swap files) into the binary. It asserts the probe is present on disk (so the
// guard can't be silently defanged by deleting it), absent from the embed, and
// that a real asset is embedded (catching a pattern typo that embeds nothing).
func TestAssetsExcludeDotfiles(t *testing.T) {
	if _, err := os.Stat(embedProbe); err != nil {
		t.Fatalf("embed probe %s missing on disk — re-create it (see web/embed.go): %v", embedProbe, err)
	}
	if _, err := fs.Stat(Assets, embedProbe); err == nil {
		t.Errorf("%s is embedded — embed.go must exclude dotfiles (drop the all: prefix)", embedProbe)
	}

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
