package article

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ServiceContainer provides dependency injection and service orchestration
type ServiceContainer struct {
	// Core services
	repository       Repository
	contentProcessor ContentProcessor
	searchService    SearchService
	compositeService *CompositeService

	// Caching
	cacheCoordinator *CacheCoordinator

	logger *slog.Logger

	// Configuration
	config *Config
}

// Config holds configuration for the article services
type Config struct {
	ArticlesPath string
	UploadPath   string
	CacheEnabled bool
	CacheConfig  *CacheConfig
	SearchIndex  bool
}

// NewServiceContainer creates and initializes all article-related services
func NewServiceContainer(config *Config, logger *slog.Logger) (*ServiceContainer, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	container := &ServiceContainer{
		config: config,
		logger: logger,
	}

	// Initialize services in dependency order
	container.initializeServices()

	return container, nil
}

// initializeServices initializes all services in the correct order
func (c *ServiceContainer) initializeServices() {
	// 1. Initialize cache coordinator if caching is enabled
	if c.config.CacheEnabled {
		cacheConfig := c.config.CacheConfig
		if cacheConfig == nil {
			cacheConfig = DefaultCacheConfig()
		}

		var err error
		c.cacheCoordinator, err = NewCacheCoordinator(cacheConfig, c.logger)
		if err != nil {
			c.logger.Warn("Failed to initialize cache coordinator, proceeding without caching", "error", err)
		}
	}

	// 2. Initialize base repository (data access layer)
	baseRepository := NewFileSystemRepository(c.config.ArticlesPath, c.config.UploadPath, c.logger)

	// 3. Wrap repository with caching if available
	if c.cacheCoordinator != nil {
		c.repository = NewCachedRepository(baseRepository, c.cacheCoordinator, c.logger)
		c.logger.Info("Initialized cached repository")
	} else {
		c.repository = baseRepository
		c.logger.Info("Initialized non-cached repository")
	}

	// 4. Initialize content processor
	c.contentProcessor = NewMarkdownContentProcessor(c.logger)

	// 5. Initialize search service
	c.searchService = NewTextSearchService(c.logger)

	// 6. Initialize composite service that orchestrates all services
	c.compositeService = NewCompositeService(
		c.repository,
		c.contentProcessor,
		c.searchService,
		c.logger,
	)
}

// Start initializes and starts all services
func (c *ServiceContainer) Start(ctx context.Context) error {
	c.logger.Info("Starting article service container")

	// Start the composite service which will handle initialization
	if err := c.compositeService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start composite service: %w", err)
	}

	c.logger.Info("Article service container started successfully")
	return nil
}

// Stop gracefully shuts down all services
func (c *ServiceContainer) Stop() error {
	c.logger.Info("Stopping article service container")

	// Stop composite service first
	if c.compositeService != nil {
		if err := c.compositeService.Stop(); err != nil {
			c.logger.Error("Error stopping composite service", "error", err)
		}
	}

	// Stop cache coordinator
	if c.cacheCoordinator != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := c.cacheCoordinator.Shutdown(ctx); err != nil {
			c.logger.Error("Error stopping cache coordinator", "error", err)
		}
	}

	c.logger.Info("Article service container stopped")
	return nil
}

// GetService returns the main article service interface
func (c *ServiceContainer) GetService() Service {
	return c.compositeService
}

// Health check methods

// IsHealthy checks if all services are functioning properly
func (c *ServiceContainer) IsHealthy() bool {
	// Check if composite service is started
	if c.compositeService == nil {
		return false
	}

	// Basic health check - try to get stats
	stats := c.compositeService.GetStats()
	return stats != nil
}

// GetHealthStatus returns detailed health information
func (c *ServiceContainer) GetHealthStatus() map[string]interface{} {
	status := map[string]interface{}{
		"healthy": c.IsHealthy(),
		"services": map[string]interface{}{
			"repository":        c.repository != nil,
			"content_processor": c.contentProcessor != nil,
			"search_service":    c.searchService != nil,
			"composite_service": c.compositeService != nil,
		},
	}

	if c.compositeService != nil {
		stats := c.compositeService.GetStats()
		if stats != nil {
			status["stats"] = map[string]interface{}{
				"total_articles":   stats.TotalArticles,
				"published_count":  stats.PublishedCount,
				"draft_count":      stats.DraftCount,
				"last_reload_time": c.compositeService.GetLastReloadTime(),
			}
		}
	}

	// Add cache statistics if available
	if c.cacheCoordinator != nil {
		status["cache"] = c.cacheCoordinator.GetCacheStats()
		status["cache_healthy"] = c.cacheCoordinator.IsHealthy()
	}

	return status
}
