package article

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/models"
)

func stageOrphanFile(t *testing.T, uploadDir, slug, filename string) string {
	t.Helper()
	slugDir := filepath.Join(uploadDir, slug)
	require.NoError(t, os.MkdirAll(slugDir, 0o755))
	path := filepath.Join(slugDir, filename)
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0o600))
	return path
}

func quietSweepLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestOrphanSweep_DoesNotDeleteInlineReferencedFiles is the load-bearing
// safety test for Phase 4 of v3.18.1. An article with no banner field but
// an inline markdown image reference must protect that file from the
// sweep. Failing this test means the sweep is unsafe to ship.
func TestOrphanSweep_DoesNotDeleteInlineReferencedFiles(t *testing.T) {
	uploadDir := t.TempDir()
	slug := "post-with-inline"
	inlinePath := stageOrphanFile(t, uploadDir, slug, "inline.png")

	article := &models.Article{
		Slug:    slug,
		Banner:  "",
		Content: "Some text.\n\n![alt](/uploads/" + slug + "/inline.png)\n\nMore text.",
	}

	cleaned, errs := OrphanSweep(
		context.Background(),
		[]*models.Article{article},
		uploadDir,
		quietSweepLogger(),
	)

	assert.Empty(t, errs)
	assert.Equal(t, 0, cleaned, "inline-referenced file must protect against sweep")
	_, err := os.Stat(inlinePath)
	assert.NoError(t, err, "inline.png must survive sweep")
}

func TestOrphanSweep_DeletesUnreferencedOrphan(t *testing.T) {
	uploadDir := t.TempDir()
	orphanPath := stageOrphanFile(t, uploadDir, "stale-slug", "orphan.png")

	cleaned, errs := OrphanSweep(
		context.Background(),
		nil, // no articles reference anything
		uploadDir,
		quietSweepLogger(),
	)

	assert.Empty(t, errs)
	assert.Equal(t, 1, cleaned)
	_, err := os.Stat(orphanPath)
	assert.True(t, os.IsNotExist(err), "orphan.png must be removed")
}

func TestOrphanSweep_BannerFieldServerAbsoluteProtects(t *testing.T) {
	uploadDir := t.TempDir()
	slug := "post"
	bannerPath := stageOrphanFile(t, uploadDir, slug, "banner.png")

	article := &models.Article{
		Slug:   slug,
		Banner: "/uploads/" + slug + "/banner.png",
	}

	cleaned, _ := OrphanSweep(context.Background(), []*models.Article{article}, uploadDir, quietSweepLogger())
	assert.Equal(t, 0, cleaned)
	_, err := os.Stat(bannerPath)
	assert.NoError(t, err)
}

func TestOrphanSweep_BannerFieldBareFilenameProtects(t *testing.T) {
	uploadDir := t.TempDir()
	slug := "post"
	bannerPath := stageOrphanFile(t, uploadDir, slug, "bare.png")

	article := &models.Article{
		Slug:   slug,
		Banner: "bare.png", // bare filename — resolves to <slug>/<filename>
	}

	cleaned, _ := OrphanSweep(context.Background(), []*models.Article{article}, uploadDir, quietSweepLogger())
	assert.Equal(t, 0, cleaned)
	_, err := os.Stat(bannerPath)
	assert.NoError(t, err)
}

func TestOrphanSweep_ExternalURLDoesNotProtect(t *testing.T) {
	uploadDir := t.TempDir()
	slug := "post"
	// File on disk whose name collides with an external URL's basename. The
	// external URL must NOT preserve the local file.
	collidingPath := stageOrphanFile(t, uploadDir, slug, "remote.png")

	article := &models.Article{
		Slug:   slug,
		Banner: "https://cdn.example.com/img/remote.png",
	}

	cleaned, _ := OrphanSweep(context.Background(), []*models.Article{article}, uploadDir, quietSweepLogger())
	assert.Equal(t, 1, cleaned, "external banner URL must not preserve a locally orphaned file")
	_, err := os.Stat(collidingPath)
	assert.True(t, os.IsNotExist(err))
}

func TestOrphanSweep_HTMLImgSrcProtects(t *testing.T) {
	uploadDir := t.TempDir()
	slug := "post"
	imgPath := stageOrphanFile(t, uploadDir, slug, "html-img.png")

	article := &models.Article{
		Slug:    slug,
		Content: `Body with raw HTML: <img src="/uploads/` + slug + `/html-img.png" alt="x">`,
	}

	cleaned, _ := OrphanSweep(context.Background(), []*models.Article{article}, uploadDir, quietSweepLogger())
	assert.Equal(t, 0, cleaned, "raw HTML img src must protect referenced file")
	_, err := os.Stat(imgPath)
	assert.NoError(t, err)
}

func TestOrphanSweep_QueryStringInBodyStillProtects(t *testing.T) {
	uploadDir := t.TempDir()
	slug := "post"
	imgPath := stageOrphanFile(t, uploadDir, slug, "queried.png")

	article := &models.Article{
		Slug:    slug,
		Content: `Look: ![x](/uploads/` + slug + `/queried.png?v=2)`,
	}

	cleaned, _ := OrphanSweep(context.Background(), []*models.Article{article}, uploadDir, quietSweepLogger())
	assert.Equal(t, 0, cleaned, "query-string suffix must not break the protection")
	_, err := os.Stat(imgPath)
	assert.NoError(t, err)
}

func TestOrphanSweep_CrossSlugReferenceProtects(t *testing.T) {
	uploadDir := t.TempDir()
	// File lives under slug-a, but is referenced from an article whose own
	// slug is slug-b. Body-content scan must still recognize this.
	imgPath := stageOrphanFile(t, uploadDir, "slug-a", "shared.png")

	article := &models.Article{
		Slug:    "slug-b",
		Content: "![x](/uploads/slug-a/shared.png)",
	}

	cleaned, _ := OrphanSweep(context.Background(), []*models.Article{article}, uploadDir, quietSweepLogger())
	assert.Equal(t, 0, cleaned, "cross-slug body reference must protect the file")
	_, err := os.Stat(imgPath)
	assert.NoError(t, err)
}

func TestOrphanSweep_RespectsContextCancellation(t *testing.T) {
	uploadDir := t.TempDir()
	stageOrphanFile(t, uploadDir, "slug-1", "a.png")
	stageOrphanFile(t, uploadDir, "slug-2", "b.png")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cleaned, _ := OrphanSweep(ctx, nil, uploadDir, quietSweepLogger())
	// Cancelled before any work — must not panic; may have cleaned 0.
	assert.LessOrEqual(t, cleaned, 2)
}

func TestOrphanSweep_NonexistentUploadDir(t *testing.T) {
	cleaned, errs := OrphanSweep(
		context.Background(),
		nil,
		filepath.Join(t.TempDir(), "does-not-exist"),
		quietSweepLogger(),
	)
	assert.Empty(t, errs, "missing upload dir is not an error — operator may not have uploaded yet")
	assert.Equal(t, 0, cleaned)
}

func TestOrphanSweep_MultipleOrphansAcrossSlugs(t *testing.T) {
	uploadDir := t.TempDir()
	orphan1 := stageOrphanFile(t, uploadDir, "slug-a", "orphan-1.png")
	orphan2 := stageOrphanFile(t, uploadDir, "slug-a", "orphan-2.png")
	kept := stageOrphanFile(t, uploadDir, "slug-a", "kept.png")
	orphan3 := stageOrphanFile(t, uploadDir, "slug-b", "orphan-3.png")

	article := &models.Article{
		Slug:   "slug-a",
		Banner: "/uploads/slug-a/kept.png",
	}

	cleaned, _ := OrphanSweep(context.Background(), []*models.Article{article}, uploadDir, quietSweepLogger())
	assert.Equal(t, 3, cleaned)
	for _, p := range []string{orphan1, orphan2, orphan3} {
		_, err := os.Stat(p)
		assert.True(t, os.IsNotExist(err), "expected %s to be removed", p)
	}
	_, err := os.Stat(kept)
	assert.NoError(t, err, "banner-referenced file must survive")
}
