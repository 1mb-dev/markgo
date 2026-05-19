package serve

import (
	"context"
	"log/slog"
	"time"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services/article"
)

// sweepTimeout bounds the post-boot orphan sweep so a misconfigured upload
// directory cannot wedge the goroutine indefinitely.
const sweepTimeout = 60 * time.Second

// articleLister is the slice of ArticleServiceInterface that the sweep
// needs. Declared locally to avoid importing the parent services package.
type articleLister interface {
	GetAllArticles() []*models.Article
	GetDraftArticles() []*models.Article
}

// runOrphanSweep launches a single post-boot pass that removes upload
// files no article references. Honors ORPHAN_SWEEP_DISABLED. Logs a
// single slog.Info on completion with cleanup count and duration; errors
// during the walk are warn-logged inside the sweep itself.
func runOrphanSweep(cfg *config.Config, articleSvc articleLister, logger *slog.Logger) {
	if cfg.OrphanSweepDisabled {
		logger.Info("orphan sweep disabled via ORPHAN_SWEEP_DISABLED")
		return
	}
	if cfg.Upload.Path == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sweepTimeout)
	defer cancel()

	articles := append(articleSvc.GetAllArticles(), articleSvc.GetDraftArticles()...)

	start := time.Now()
	cleaned, errs := article.OrphanSweep(ctx, articles, cfg.Upload.Path, logger)
	logger.Info("orphan sweep complete",
		"cleaned", cleaned,
		"errors", len(errs),
		"duration", time.Since(start),
	)
}
