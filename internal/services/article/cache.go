package article

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"

	"github.com/1mb-dev/markgo/internal/models"
)

// CacheCoordinator manages caching across all article services
type CacheCoordinator struct {
	obcache *obcache.Cache
	logger  *slog.Logger
	mu      sync.RWMutex

	// Cache settings
	articleTTL time.Duration
	searchTTL  time.Duration
	contentTTL time.Duration

	// Cache keys
	articlePrefix string
	searchPrefix  string
	contentPrefix string
	statsKey      string

	// Cache statistics — accessed atomically to avoid data races under RLock
	hits   int64
	misses int64

	// Lifecycle
	stopCh chan struct{}
}

// CacheConfig holds configuration for the cache coordinator
type CacheConfig struct {
	MaxEntries    int           `json:"max_entries"`
	ArticleTTL    time.Duration `json:"article_ttl"`
	SearchTTL     time.Duration `json:"search_ttl"`
	ContentTTL    time.Duration `json:"content_ttl"`
	CleanupPeriod time.Duration `json:"cleanup_period"`
}

// DefaultCacheConfig returns a default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxEntries:    5000,
		ArticleTTL:    30 * time.Minute,
		SearchTTL:     10 * time.Minute,
		ContentTTL:    60 * time.Minute,
		CleanupPeriod: 15 * time.Minute,
	}
}

// NewCacheCoordinator creates a new cache coordinator
func NewCacheCoordinator(config *CacheConfig, logger *slog.Logger) (*CacheCoordinator, error) {
	if config == nil {
		config = DefaultCacheConfig()
	}

	// Initialize obcache with configuration
	cacheConfig := obcache.NewDefaultConfig().
		WithMaxEntries(config.MaxEntries).
		WithDefaultTTL(config.ArticleTTL)

	cache, err := obcache.New(cacheConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create obcache: %w", err)
	}

	coordinator := &CacheCoordinator{
		obcache:       cache,
		logger:        logger,
		articleTTL:    config.ArticleTTL,
		searchTTL:     config.SearchTTL,
		contentTTL:    config.ContentTTL,
		articlePrefix: "article:",
		searchPrefix:  "search:",
		contentPrefix: "content:",
		statsKey:      "stats",
		stopCh:        make(chan struct{}),
	}

	// Start background cleanup if configured
	if config.CleanupPeriod > 0 {
		go coordinator.startCleanup(config.CleanupPeriod)
	}

	return coordinator, nil
}

// Article caching methods

// GetArticle retrieves an article from cache
func (c *CacheCoordinator) GetArticle(slug string) (*models.Article, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.articlePrefix + slug
	value, found := c.obcache.Get(key)
	if !found {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	if article, ok := value.(*models.Article); ok {
		return article, true
	}

	// Invalid type in cache — log and return miss (cannot Delete under RLock)
	c.logger.Warn("Invalid type in article cache entry", "key", key)
	return nil, false
}

// SetArticle stores an article in cache
func (c *CacheCoordinator) SetArticle(slug string, article *models.Article) {
	if article == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.articlePrefix + slug
	if err := c.obcache.Set(key, article, c.articleTTL); err != nil {
		c.logger.Warn("Failed to cache article", "key", key, "error", err)
	}
}

// InvalidateArticle removes an article from cache
func (c *CacheCoordinator) InvalidateArticle(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.articlePrefix + slug
	if err := c.obcache.Delete(key); err != nil {
		c.logger.Warn("Failed to delete article from cache", "key", key, "error", err)
	}
}

// Search result caching methods

// GetSearchResults retrieves search results from cache
func (c *CacheCoordinator) GetSearchResults(query string, limit int) ([]*models.SearchResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s%s:%d", c.searchPrefix, query, limit)
	value, found := c.obcache.Get(key)
	if !found {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	if results, ok := value.([]*models.SearchResult); ok {
		return results, true
	}

	// Invalid type in cache — log and return miss (cannot Delete under RLock)
	c.logger.Warn("Invalid type in search cache entry", "key", key)
	return nil, false
}

// SetSearchResults stores search results in cache
func (c *CacheCoordinator) SetSearchResults(query string, limit int, results []*models.SearchResult) {
	if results == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s%s:%d", c.searchPrefix, query, limit)
	if err := c.obcache.Set(key, results, c.searchTTL); err != nil {
		c.logger.Warn("Failed to cache search results", "key", key, "error", err)
	}
}

// InvalidateSearchCache clears all search-related cache entries
func (c *CacheCoordinator) InvalidateSearchCache() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Since obcache doesn't have prefix-based deletion, we need to clear the entire cache
	// In a production environment, you might want to track search keys separately
	c.logger.Debug("Invalidating search cache (clearing all)")
	if err := c.obcache.Clear(); err != nil {
		c.logger.Warn("Failed to clear search cache", "error", err)
	}
}

// Content caching methods

// GetProcessedContent retrieves processed content from cache
func (c *CacheCoordinator) GetProcessedContent(contentHash string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.contentPrefix + contentHash
	value, found := c.obcache.Get(key)
	if !found {
		atomic.AddInt64(&c.misses, 1)
		return "", false
	}

	atomic.AddInt64(&c.hits, 1)
	if content, ok := value.(string); ok {
		return content, true
	}

	// Invalid type in cache — log and return miss (cannot Delete under RLock)
	c.logger.Warn("Invalid type in content cache entry", "key", key)
	return "", false
}

// SetProcessedContent stores processed content in cache
func (c *CacheCoordinator) SetProcessedContent(contentHash, processedContent string) {
	if processedContent == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.contentPrefix + contentHash
	if err := c.obcache.Set(key, processedContent, c.contentTTL); err != nil {
		c.logger.Warn("Failed to cache processed content", "key", key, "error", err)
	}
}

// Stats caching methods

// GetStats retrieves stats from cache
func (c *CacheCoordinator) GetStats() (*models.Stats, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, found := c.obcache.Get(c.statsKey)
	if !found {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	if stats, ok := value.(*models.Stats); ok {
		return stats, true
	}

	// Invalid type in cache — log and return miss (cannot Delete under RLock)
	c.logger.Warn("Invalid type in stats cache entry")
	return nil, false
}

// SetStats stores stats in cache
func (c *CacheCoordinator) SetStats(stats *models.Stats) {
	if stats == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.obcache.Set(c.statsKey, stats, c.articleTTL); err != nil {
		c.logger.Warn("Failed to cache stats", "error", err)
	}
}

// Cache management methods

// InvalidateAll clears all cache entries
func (c *CacheCoordinator) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("Invalidating all cached data")
	if err := c.obcache.Clear(); err != nil {
		c.logger.Warn("Failed to clear all cache", "error", err)
	}
	atomic.StoreInt64(&c.hits, 0)
	atomic.StoreInt64(&c.misses, 0)
}

// InvalidateByTag invalidates cache entries related to a specific tag
func (c *CacheCoordinator) InvalidateByTag(tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clears the entire cache since obcache doesn't support prefix-based deletion.
	// In a production system, you might want to maintain tag-to-key mappings.
	c.logger.Debug("Invalidating cache for tag", "tag", tag)
	if err := c.obcache.Clear(); err != nil { // This is aggressive but safe
		c.logger.Warn("Failed to clear cache for tag", "tag", tag, "error", err)
	}
}

// GetCacheStats returns cache performance statistics
func (c *CacheCoordinator) GetCacheStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	obcacheStats := c.obcache.Stats()

	hitRate := float64(0)
	hits := atomic.LoadInt64(&c.hits)
	misses := atomic.LoadInt64(&c.misses)
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return map[string]interface{}{
		"hits":     hits,
		"misses":   misses,
		"hit_rate": hitRate,
		"obcache_stats": map[string]interface{}{
			"key_count":        obcacheStats.KeyCount(),
			"obcache_hits":     obcacheStats.Hits(),
			"obcache_misses":   obcacheStats.Misses(),
			"evictions":        obcacheStats.Evictions(),
			"obcache_hit_rate": obcacheStats.HitRate(),
		},
	}
}

// IsHealthy checks if the cache coordinator is in a healthy state
func (c *CacheCoordinator) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.obcache != nil
}

// Cleanup and lifecycle management

// startCleanup starts a background goroutine that periodically cleans up expired entries.
// It stops when Shutdown closes stopCh.
func (c *CacheCoordinator) startCleanup(period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup performs cache maintenance
func (c *CacheCoordinator) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("Performing cache cleanup")
	c.obcache.Cleanup()
}

// Shutdown gracefully shuts down the cache coordinator.
// It signals the cleanup goroutine to stop and closes the underlying cache.
func (c *CacheCoordinator) Shutdown(_ context.Context) error {
	c.logger.Info("Shutting down cache coordinator")

	// Signal cleanup goroutine to stop (safe to call even if cleanup isn't running)
	select {
	case <-c.stopCh:
		// Already closed
	default:
		close(c.stopCh)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.obcache != nil {
		_ = c.obcache.Close()
		c.obcache = nil
	}

	return nil
}

// CachedRepository wraps a Repository with caching capabilities
type CachedRepository struct {
	repository Repository
	cache      *CacheCoordinator
	logger     *slog.Logger
}

// NewCachedRepository creates a new cached repository
func NewCachedRepository(repository Repository, cache *CacheCoordinator, logger *slog.Logger) *CachedRepository {
	return &CachedRepository{
		repository: repository,
		cache:      cache,
		logger:     logger,
	}
}

// Implement Repository interface with caching

// LoadAll loads all articles from the repository with caching.
func (cr *CachedRepository) LoadAll(ctx context.Context) ([]*models.Article, error) {
	// Don't cache LoadAll as it's typically called once at startup
	articles, err := cr.repository.LoadAll(ctx)
	if err == nil {
		// Cache individual articles
		for _, article := range articles {
			cr.cache.SetArticle(article.Slug, article)
		}
	}
	return articles, err
}

// GetBySlug retrieves an article by slug with caching.
func (cr *CachedRepository) GetBySlug(slug string) (*models.Article, error) {
	// Check cache first
	if article, found := cr.cache.GetArticle(slug); found {
		return article, nil
	}

	// Not in cache, get from repository
	article, err := cr.repository.GetBySlug(slug)
	if err == nil && article != nil {
		cr.cache.SetArticle(slug, article)
	}

	return article, err
}

// GetByTag retrieves articles by tag.
func (cr *CachedRepository) GetByTag(tag string) []*models.Article {
	return cr.repository.GetByTag(tag)
}

// GetByCategory retrieves articles by category.
func (cr *CachedRepository) GetByCategory(category string) []*models.Article {
	return cr.repository.GetByCategory(category)
}

// GetPublished retrieves all published articles.
func (cr *CachedRepository) GetPublished() []*models.Article {
	return cr.repository.GetPublished()
}

// GetPages is a passthrough — not cached, matching GetPublished / GetByTag /
// GetByCategory / GetFeatured / GetRecent / GetDrafts. Only GetBySlug and
// GetStats actually cache in this layer.
func (cr *CachedRepository) GetPages() []*models.Article {
	return cr.repository.GetPages()
}

// GetDrafts retrieves all draft articles.
func (cr *CachedRepository) GetDrafts() []*models.Article {
	return cr.repository.GetDrafts()
}

// GetFeatured retrieves featured articles.
func (cr *CachedRepository) GetFeatured(limit int) []*models.Article {
	return cr.repository.GetFeatured(limit)
}

// GetRecent retrieves recent articles.
func (cr *CachedRepository) GetRecent(limit int) []*models.Article {
	return cr.repository.GetRecent(limit)
}

// Reload reloads articles and invalidates cache.
func (cr *CachedRepository) Reload(ctx context.Context) error {
	err := cr.repository.Reload(ctx)
	if err == nil {
		// Invalidate all cached articles since data has been reloaded
		cr.cache.InvalidateAll()
	}
	return err
}

// GetLastModified returns the last modification time.
func (cr *CachedRepository) GetLastModified() time.Time {
	return cr.repository.GetLastModified()
}

// GetStats retrieves statistics with caching.
func (cr *CachedRepository) GetStats() *models.Stats {
	// Check cache first
	if stats, found := cr.cache.GetStats(); found {
		return stats
	}

	// Not in cache, get from repository
	stats := cr.repository.GetStats()
	if stats != nil {
		cr.cache.SetStats(stats)
	}

	return stats
}

// UpdateDraftStatus updates the draft status of an article and invalidates cache.
func (cr *CachedRepository) UpdateDraftStatus(slug string, isDraft bool) error {
	err := cr.repository.UpdateDraftStatus(slug, isDraft)
	if err == nil {
		// Invalidate all cache since article status affects lists, tags, categories, etc.
		cr.cache.InvalidateAll()
	}
	return err
}

// Ensure CachedRepository implements Repository interface
var _ Repository = (*CachedRepository)(nil)
