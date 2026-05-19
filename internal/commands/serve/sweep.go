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
//
// GetPages is included alongside GetAllArticles because GetAllArticles
// applies the DedicatedRouteArticle predicate (v3.13.0) and excludes
// published type:page content from its result. Without GetPages in the
// reference set, Page banner uploads would appear unreferenced and be
// swept on every restart.
type articleLister interface {
	GetAllArticles() []*models.Article
	GetDraftArticles() []*models.Article
	GetPages() []*models.Article
}

// runOrphanSweep launches a single post-boot pass that removes upload
// files no article references. Honors ORPHAN_SWEEP_DISABLED. Logs a
// single slog.Info on completion with cleanup count and duration; errors
// during the walk are warn-logged inside the sweep itself.
//
// Recovers from any panic in the sweep path: best-effort cleanup must
// never bring down the server. A panic is logged at Error level so it
// surfaces in operator triage without losing the running process.
func runOrphanSweep(cfg *config.Config, articleSvc articleLister, logger *slog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("orphan sweep panicked", "panic", r)
		}
	}()

	if cfg.OrphanSweepDisabled {
		logger.Info("orphan sweep disabled via ORPHAN_SWEEP_DISABLED")
		return
	}
	if cfg.Upload.Path == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sweepTimeout)
	defer cancel()

	// GetAllArticles excludes dedicated-route content (type:page + about
	// slug) via the v3.13.0 predicate. Bring Pages back in explicitly
	// so Page banner uploads don't appear unreferenced. The /about
	// article remains a known gap — operators using a banner there
	// should rely on per-action unlink (Phase 3) and ORPHAN_SWEEP_DISABLED
	// for the startup pass.
	articles := articleSvc.GetAllArticles()
	articles = append(articles, articleSvc.GetDraftArticles()...)
	articles = append(articles, articleSvc.GetPages()...)

	start := time.Now()
	cleaned, errs := article.OrphanSweep(ctx, articles, cfg.Upload.Path, logger)
	logger.Info("orphan sweep complete",
		"cleaned", cleaned,
		"errors", len(errs),
		"duration", time.Since(start),
	)
}
