package serve

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
)

type stubArticleLister struct {
	published []*models.Article
	drafts    []*models.Article
}

func (s *stubArticleLister) GetAllArticles() []*models.Article   { return s.published }
func (s *stubArticleLister) GetDraftArticles() []*models.Article { return s.drafts }

func stageSweepFile(t *testing.T, root, slug, filename string) string {
	t.Helper()
	dir := filepath.Join(root, slug)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	return p
}

func TestRunOrphanSweep_RemovesOrphansWhenEnabled(t *testing.T) {
	uploadPath := t.TempDir()
	orphan := stageSweepFile(t, uploadPath, "stale", "orphan.png")

	cfg := &config.Config{
		Upload:              config.UploadConfig{Path: uploadPath},
		OrphanSweepDisabled: false,
	}
	lister := &stubArticleLister{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runOrphanSweep(cfg, lister, logger)

	_, err := os.Stat(orphan)
	assert.True(t, os.IsNotExist(err), "orphan must be removed when sweep enabled")
}

func TestRunOrphanSweep_SkippedWhenDisabled(t *testing.T) {
	uploadPath := t.TempDir()
	orphan := stageSweepFile(t, uploadPath, "stale", "orphan.png")

	cfg := &config.Config{
		Upload:              config.UploadConfig{Path: uploadPath},
		OrphanSweepDisabled: true,
	}
	lister := &stubArticleLister{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runOrphanSweep(cfg, lister, logger)

	_, err := os.Stat(orphan)
	assert.NoError(t, err, "orphan must survive when sweep disabled")
}

func TestRunOrphanSweep_SkippedWhenUploadPathEmpty(t *testing.T) {
	cfg := &config.Config{
		Upload:              config.UploadConfig{Path: ""},
		OrphanSweepDisabled: false,
	}
	lister := &stubArticleLister{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Must not panic on empty upload path.
	runOrphanSweep(cfg, lister, logger)
}

// panickyLister forces a panic when the sweep tries to enumerate articles,
// proving that runOrphanSweep's defer/recover keeps the goroutine alive.
type panickyLister struct{}

func (panickyLister) GetAllArticles() []*models.Article {
	panic("simulated lister failure")
}
func (panickyLister) GetDraftArticles() []*models.Article { return nil }

func TestRunOrphanSweep_RecoversFromPanic(t *testing.T) {
	cfg := &config.Config{
		Upload:              config.UploadConfig{Path: t.TempDir()},
		OrphanSweepDisabled: false,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// If recover() weren't in place, this call would propagate the panic
	// up to the runtime — fatal in a goroutine. The fact that the test
	// reaches the next line is the assertion.
	assert.NotPanics(t, func() {
		runOrphanSweep(cfg, panickyLister{}, logger)
	})
}
