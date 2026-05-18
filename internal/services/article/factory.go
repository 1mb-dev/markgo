package article

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/1mb-dev/markgo/internal/models"
)

// ServiceFactory creates and configures article services for different use cases
type ServiceFactory struct {
	logger *slog.Logger
}

// NewServiceFactory creates a new service factory
func NewServiceFactory(logger *slog.Logger) *ServiceFactory {
	return &ServiceFactory{
		logger: logger,
	}
}

// CreateService creates a new article service using the modular architecture.
// Returns a ServiceAdapter that implements services.ArticleServiceInterface (defined in parent package).
// Note: We avoid importing parent package to prevent import cycles; structural typing handles interface compatibility.
func (f *ServiceFactory) CreateService(articlesPath, uploadPath string) (*ServiceAdapter, error) {
	// Create configuration
	config := &Config{
		ArticlesPath: articlesPath,
		UploadPath:   uploadPath,
		CacheEnabled: true,
		CacheConfig:  DefaultCacheConfig(),
		SearchIndex:  true,
	}

	// Create service container
	container, err := NewServiceContainer(config, f.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create service container: %w", err)
	}

	// Initialize the container
	ctx := context.Background()
	if err := container.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start service container: %w", err)
	}

	// Create service adapter that implements the ArticleServiceInterface
	adapter := &ServiceAdapter{
		service:   container.compositeService,
		container: container,
		logger:    f.logger,
	}

	return adapter, nil
}

// ServiceAdapter adapts the service container to implement services.ArticleServiceInterface.
// This adapter provides the bridge between the internal modular architecture
// and the public interface defined in the parent services package.
type ServiceAdapter struct {
	service   Service
	container *ServiceContainer
	logger    *slog.Logger
}

// GetAllArticles returns all articles from the underlying service
func (a *ServiceAdapter) GetAllArticles() []*models.Article {
	return a.service.GetAllArticles()
}

// GetPages retrieves all published type:page articles.
func (a *ServiceAdapter) GetPages() []*models.Article {
	return a.service.GetPages()
}

// GetArticleBySlug retrieves an article by slug.
func (a *ServiceAdapter) GetArticleBySlug(slug string) (*models.Article, error) {
	return a.service.GetArticleBySlug(slug)
}

// GetArticlesByTag retrieves articles by tag.
func (a *ServiceAdapter) GetArticlesByTag(tag string) []*models.Article {
	return a.service.GetArticlesByTag(tag)
}

// GetArticlesByCategory retrieves articles by category.
func (a *ServiceAdapter) GetArticlesByCategory(category string) []*models.Article {
	return a.service.GetArticlesByCategory(category)
}

// GetArticlesForFeed retrieves articles for feed generation.
func (a *ServiceAdapter) GetArticlesForFeed(limit int) []*models.Article {
	return a.service.GetArticlesForFeed(limit)
}

// GetFeaturedArticles retrieves featured articles.
func (a *ServiceAdapter) GetFeaturedArticles(limit int) []*models.Article {
	return a.service.GetFeaturedArticles(limit)
}

// GetRecentArticles retrieves recent articles.
func (a *ServiceAdapter) GetRecentArticles(limit int) []*models.Article {
	return a.service.GetRecentArticles(limit)
}

// GetAllTags retrieves all available tags.
func (a *ServiceAdapter) GetAllTags() []string {
	return a.service.GetAllTags()
}

// GetAllCategories retrieves all available categories.
func (a *ServiceAdapter) GetAllCategories() []string {
	return a.service.GetAllCategories()
}

// GetTagCounts retrieves tag usage counts.
func (a *ServiceAdapter) GetTagCounts() []models.TagCount {
	return a.service.GetTagCounts()
}

// GetCategoryCounts retrieves category usage counts.
func (a *ServiceAdapter) GetCategoryCounts() []models.CategoryCount {
	return a.service.GetCategoryCounts()
}

// GetStats retrieves article statistics.
func (a *ServiceAdapter) GetStats() *models.Stats {
	return a.service.GetStats()
}

// ReloadArticles reloads all articles.
func (a *ServiceAdapter) ReloadArticles() error {
	return a.service.ReloadArticles()
}

// GetDraftArticles retrieves all draft articles.
func (a *ServiceAdapter) GetDraftArticles() []*models.Article {
	return a.service.GetDraftArticles()
}

// GetDraftBySlug retrieves a draft article by slug.
func (a *ServiceAdapter) GetDraftBySlug(slug string) (*models.Article, error) {
	return a.service.GetDraftBySlug(slug)
}

// Shutdown gracefully shuts down the service
func (a *ServiceAdapter) Shutdown() error {
	if a.container != nil {
		return a.container.Stop()
	}
	return nil
}

// IsHealthy returns the health status of the service adapter
func (a *ServiceAdapter) IsHealthy() bool {
	return a.container.IsHealthy()
}

// GetHealthStatus returns the health status.
func (a *ServiceAdapter) GetHealthStatus() map[string]interface{} {
	return a.container.GetHealthStatus()
}

// SearchArticles performs article search using the underlying service
func (a *ServiceAdapter) SearchArticles(query string, limit int) []*models.SearchResult {
	return a.service.SearchArticles(query, limit)
}
