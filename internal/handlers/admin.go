// Package handlers provides HTTP request handlers for the MarkGo blog engine.
// It includes handlers for admin operations, article management, API endpoints, and more.
package handlers

import (
	"fmt"
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// AdminHandler handles administrative HTTP requests
type AdminHandler struct {
	*BaseHandler
	articleService services.ArticleServiceInterface
	startTime      time.Time
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(
	base *BaseHandler,
	articleService services.ArticleServiceInterface,
	startTime time.Time,
) *AdminHandler {
	return &AdminHandler{
		BaseHandler:    base,
		articleService: articleService,
		startTime:      startTime,
	}
}

// formatDuration formats a time duration into human-readable string
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := math.Mod(d.Seconds(), 60)

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %.0fs", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %.0fs", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %.2fs", minutes, seconds)
	}
	return fmt.Sprintf("%.2fs", seconds)
}

// AdminHome handles the admin dashboard/home page
func (h *AdminHandler) AdminHome(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(h.startTime)

	// Get both published and draft articles for accurate counting
	publishedArticles := h.articleService.GetAllArticles()
	draftArticles := h.articleService.GetDraftArticles()

	publishedCount := len(publishedArticles)
	draftCount := len(draftArticles)

	// Get tag and category counts
	tagCounts := h.articleService.GetTagCounts()
	categoryCounts := h.articleService.GetCategoryCounts()

	// Count pending AMA questions
	pendingAMA := 0
	for _, d := range draftArticles {
		if d.Type == templateAMA {
			pendingAMA++
		}
	}

	stats := map[string]any{
		"published":   publishedCount,
		"drafts":      draftCount,
		"tags":        len(tagCounts),
		"categories":  len(categoryCounts),
		"pending_ama": pendingAMA,
	}

	system := map[string]any{
		"environment": h.config.Environment,
		"uptime":      formatDuration(uptime),
		"memory":      formatBytes(m.Alloc),
		"go_version":  runtime.Version(),
		"goroutines":  runtime.NumGoroutine(),
	}

	isDev := h.config.Environment == config.DevelopmentEnvironment

	if h.shouldReturnJSON(c) {
		c.JSON(http.StatusOK, gin.H{
			"stats":     stats,
			"system":    system,
			"is_dev":    isDev,
			"timestamp": time.Now().Unix(),
		})
		return
	}

	data := h.buildBaseTemplateData("Dashboard")
	data["description"] = "Admin dashboard"
	data["stats"] = stats
	data["system"] = system
	data["is_dev"] = isDev
	data["template"] = "admin_home"

	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// Metrics handles the metrics endpoint for performance monitoring
func (h *AdminHandler) Metrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(h.startTime)

	h.respondWithJSON(c, http.StatusOK, func() map[string]any {
		return map[string]any{
			"uptime":         uptime.String(),
			"uptime_seconds": int64(uptime.Seconds()),
			"memory": map[string]any{
				"alloc":       m.Alloc,
				"total_alloc": m.TotalAlloc,
				"sys":         m.Sys,
				"heap_alloc":  m.HeapAlloc,
				"heap_sys":    m.HeapSys,
				"num_gc":      m.NumGC,
			},
			"goroutines":  runtime.NumGoroutine(),
			"go_version":  runtime.Version(),
			"environment": h.config.Environment,
			"timestamp":   time.Now().Unix(),
		}
	})
}

// Stats handles the stats endpoint
func (h *AdminHandler) Stats(c *gin.Context) {
	data, err := h.getStatsDataUncached()
	if err != nil {
		h.handleError(c, err, "Failed to get stats")
		return
	}

	c.JSON(http.StatusOK, data)
}

// Debug handles debug information endpoint
func (h *AdminHandler) Debug(c *gin.Context) {
	if !h.requireDevelopmentEnv(c) {
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	buildInfo := map[string]any{
		"version":     unknownValue,
		"git_commit":  unknownValue,
		"build_time":  unknownValue,
		"environment": h.config.Environment,
	}
	if h.buildInfo != nil {
		if h.buildInfo.Version != "" {
			buildInfo["version"] = h.buildInfo.Version
		}
		if h.buildInfo.GitCommit != "" {
			buildInfo["git_commit"] = h.buildInfo.GitCommit
		}
		if h.buildInfo.BuildTime != "" {
			buildInfo["build_time"] = h.buildInfo.BuildTime
		}
	}

	debugInfo := map[string]any{
		"runtime": map[string]any{
			"go_version":    runtime.Version(),
			"num_cpu":       runtime.NumCPU(),
			"num_goroutine": runtime.NumGoroutine(),
			"compiler":      runtime.Compiler,
			"arch":          runtime.GOARCH,
			"os":            runtime.GOOS,
		},
		"memory": map[string]any{
			"alloc":           formatBytes(m.Alloc),
			"total_alloc":     formatBytes(m.TotalAlloc),
			"sys":             formatBytes(m.Sys),
			"heap_alloc":      formatBytes(m.HeapAlloc),
			"heap_sys":        formatBytes(m.HeapSys),
			"heap_idle":       formatBytes(m.HeapIdle),
			"heap_inuse":      formatBytes(m.HeapInuse),
			"heap_released":   formatBytes(m.HeapReleased),
			"heap_objects":    m.HeapObjects,
			"stack_inuse":     formatBytes(m.StackInuse),
			"stack_sys":       formatBytes(m.StackSys),
			"num_gc":          m.NumGC,
			"gc_cpu_fraction": m.GCCPUFraction,
		},
		"config": map[string]any{
			"environment":    h.config.Environment,
			"port":           h.config.Port,
			"base_url":       h.config.BaseURL,
			"cache_ttl":      h.config.Cache.TTL.String(),
			"cache_max_size": h.config.Cache.MaxSize,
		},
		"build":  buildInfo,
		"uptime": time.Since(h.startTime).String(),
	}

	// Debug endpoints always return JSON (no template exists for debug data)
	c.JSON(http.StatusOK, debugInfo)
}

// ReloadArticles handles reloading articles (development only)
func (h *AdminHandler) ReloadArticles(c *gin.Context) {
	if !h.requireDevelopmentEnv(c) {
		return
	}

	h.logger.Info("Reloading articles requested")

	if err := h.articleService.ReloadArticles(); err != nil {
		h.handleError(c, err, "Failed to reload articles")
		return
	}

	h.logger.Info("Articles reloaded successfully")

	c.JSON(http.StatusOK, gin.H{
		"message":   "Articles reloaded successfully",
		"timestamp": time.Now().Unix(),
	})
}

// Writing handles the admin writing page — all published content with direct edit links
func (h *AdminHandler) Writing(c *gin.Context) {
	articles := h.articleService.GetAllArticles()

	if h.shouldReturnJSON(c) {
		listings := make([]*models.ArticleList, len(articles))
		for i, a := range articles {
			listings[i] = a.ToListView()
		}
		c.JSON(http.StatusOK, gin.H{
			"articles":      listings,
			"article_count": len(articles),
			"timestamp":     time.Now().Unix(),
		})
		return
	}

	data := h.buildBaseTemplateData("Writing - " + h.config.Blog.Title)
	data["description"] = "Manage published content"
	data["template"] = "admin_writing"
	data["articles"] = articles
	data["article_count"] = len(articles)

	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// Drafts handles the draft management page
func (h *AdminHandler) Drafts(c *gin.Context) {
	drafts := h.articleService.GetDraftArticles()

	if h.shouldReturnJSON(c) {
		listings := make([]*models.ArticleList, len(drafts))
		for i, a := range drafts {
			listings[i] = a.ToListView()
		}
		c.JSON(http.StatusOK, gin.H{
			"drafts":      listings,
			"draft_count": len(drafts),
			"timestamp":   time.Now().Unix(),
		})
		return
	}

	data := h.buildBaseTemplateData("Drafts - " + h.config.Blog.Title)
	data["description"] = "Manage draft articles"
	data["template"] = "drafts"
	data["drafts"] = drafts
	data["draft_count"] = len(drafts)
	data["csrf_token"] = csrfToken(c)

	h.renderHTML(c, http.StatusOK, "base.html", data)
}

// Uncached data generation methods

func (h *AdminHandler) getStatsDataUncached() (map[string]any, error) {
	stats := h.articleService.GetStats()
	if stats == nil {
		return nil, fmt.Errorf("failed to get article stats")
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]any{
		"articles": map[string]any{
			"total":      stats.TotalArticles,
			"published":  stats.PublishedCount,
			"drafts":     stats.DraftCount,
			"tags":       stats.TotalTags,
			"categories": stats.TotalCategories,
		},
		"system": map[string]any{
			"uptime":       time.Since(h.startTime).String(),
			"goroutines":   runtime.NumGoroutine(),
			"memory_alloc": formatBytes(m.Alloc),
			"memory_sys":   formatBytes(m.Sys),
		},
		"config": map[string]any{
			"environment":   h.config.Environment,
			"cache_enabled": h.config.Cache.TTL > 0,
		},
		"timestamp": time.Now().Unix(),
	}, nil
}
