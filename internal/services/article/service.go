// Package article provides article management services for the MarkGo blog engine.
// It handles reading, parsing, and managing markdown articles and metadata.
package article

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
)

// Service defines the main article service interface
type Service interface {
	// Repository operations
	GetAllArticles() []*models.Article
	GetPages() []*models.Article
	GetArticleBySlug(slug string) (*models.Article, error)
	GetArticlesByTag(tag string) []*models.Article
	GetArticlesByCategory(category string) []*models.Article
	GetArticlesForFeed(limit int) []*models.Article
	GetFeaturedArticles(limit int) []*models.Article
	GetRecentArticles(limit int) []*models.Article
	GetDraftArticles() []*models.Article
	GetDraftBySlug(slug string) (*models.Article, error)

	// Search operations
	SearchArticles(query string, limit int) []*models.SearchResult
	SearchInTitle(query string, limit int) []*models.SearchResult
	GetSearchSuggestions(query string, limit int) []string

	// Content processing
	ProcessArticleContent(article *models.Article) error

	// Statistics and metadata
	GetStats() *models.Stats
	GetAllTags() []string
	GetAllCategories() []string
	GetTagCounts() []models.TagCount
	GetCategoryCounts() []models.CategoryCount

	// Management operations
	ReloadArticles() error
	GetLastReloadTime() time.Time

	// Lifecycle
	Start(ctx context.Context) error
	Stop() error
}

// CompositeService implements Service using composed domain services
type CompositeService struct {
	// Core dependencies
	repository       Repository
	contentProcessor ContentProcessor
	searchService    SearchService
	logger           *slog.Logger

	// State management
	mutex   sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Caching
	searchIndex SearchIndex
	indexBuilt  bool
	indexMutex  sync.RWMutex
}

// NewCompositeService creates a new composite article service
func NewCompositeService(
	repository Repository,
	contentProcessor ContentProcessor,
	searchService SearchService,
	logger *slog.Logger,
) *CompositeService {
	return &CompositeService{
		repository:       repository,
		contentProcessor: contentProcessor,
		searchService:    searchService,
		logger:           logger,
	}
}

// Start initializes the service and loads articles
func (s *CompositeService) Start(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.started {
		return fmt.Errorf("service already started")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Load articles
	articles, err := s.repository.LoadAll(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to load articles: %w", err)
	}

	// Process article content
	s.logger.Info("Processing article content", "count", len(articles))
	for _, article := range articles {
		if err := s.ProcessArticleContent(article); err != nil {
			s.logger.Warn("Failed to process article content", "slug", article.Slug, "error", err)
		}
	}

	// Build search index
	s.buildSearchIndex(articles)

	s.started = true
	s.logger.Info("Article service started successfully", "articles_loaded", len(articles))

	return nil
}

// Stop shuts down the service
func (s *CompositeService) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.started {
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}

	s.started = false
	s.logger.Info("Article service stopped")

	return nil
}

// GetAllArticles returns all articles
func (s *CompositeService) GetAllArticles() []*models.Article {
	return s.repository.GetPublished()
}

// GetPages returns published type:page articles. See Repository.GetPages for
// the contract — natural insertion order, type-filtered, draft-excluded.
func (s *CompositeService) GetPages() []*models.Article {
	return s.repository.GetPages()
}

// GetArticleBySlug retrieves an article by slug
func (s *CompositeService) GetArticleBySlug(slug string) (*models.Article, error) {
	article, err := s.repository.GetBySlug(slug)
	if err != nil {
		return nil, err
	}

	// Skip draft articles for public access
	if article.Draft {
		return nil, fmt.Errorf("article not found: %s: %w", slug, apperrors.ErrArticleNotFound)
	}

	return article, nil
}

// GetArticlesByTag returns articles with the specified tag
func (s *CompositeService) GetArticlesByTag(tag string) []*models.Article {
	return s.repository.GetByTag(tag)
}

// GetArticlesByCategory returns articles in the specified category
func (s *CompositeService) GetArticlesByCategory(category string) []*models.Article {
	return s.repository.GetByCategory(category)
}

// GetArticlesForFeed returns recent articles for RSS/JSON feeds
func (s *CompositeService) GetArticlesForFeed(limit int) []*models.Article {
	return s.repository.GetRecent(limit)
}

// GetFeaturedArticles returns featured articles
func (s *CompositeService) GetFeaturedArticles(limit int) []*models.Article {
	return s.repository.GetFeatured(limit)
}

// GetRecentArticles returns recent articles
func (s *CompositeService) GetRecentArticles(limit int) []*models.Article {
	return s.repository.GetRecent(limit)
}

// GetDraftArticles returns all draft articles
func (s *CompositeService) GetDraftArticles() []*models.Article {
	return s.repository.GetDrafts()
}

// GetDraftBySlug retrieves a draft article by slug
func (s *CompositeService) GetDraftBySlug(slug string) (*models.Article, error) {
	article, err := s.repository.GetBySlug(slug)
	if err != nil {
		return nil, err
	}

	// Only return if it's actually a draft
	if !article.Draft {
		return nil, fmt.Errorf("article is not a draft: %s", slug)
	}

	return article, nil
}

// SearchArticles performs full-text search
func (s *CompositeService) SearchArticles(query string, limit int) []*models.SearchResult {
	// Use index if available, otherwise fallback to direct search
	s.indexMutex.RLock()
	if s.indexBuilt {
		results := s.searchService.SearchWithIndex(s.searchIndex, query, limit)
		s.indexMutex.RUnlock()
		return results
	}
	s.indexMutex.RUnlock()

	// Fallback to direct search
	articles := s.repository.GetPublished()
	return s.searchService.Search(articles, query, limit)
}

// SearchInTitle searches only in article titles
func (s *CompositeService) SearchInTitle(query string, limit int) []*models.SearchResult {
	articles := s.repository.GetPublished()
	return s.searchService.SearchInTitle(articles, query, limit)
}

// GetSearchSuggestions returns search suggestions
func (s *CompositeService) GetSearchSuggestions(query string, limit int) []string {
	articles := s.repository.GetPublished()
	return s.searchService.GetSuggestions(articles, query, limit)
}

// amaSeparator is the deterministic boundary the AMA answer flow writes
// between a legacy question and its answer ("question\n\n---\n\nanswer").
const amaSeparator = "\n\n---\n\n"

// splitLegacyAMA splits a legacy AMA body into (question, answer) at the FIRST
// separator. Unanswered legacy drafts (no separator) return (whole, ""). The
// split is on the FIRST separator only, so a question is always taken whole;
// any "---" inside an answer stays with the answer.
func splitLegacyAMA(body string) (question, answer string) {
	q, a, found := strings.Cut(body, amaSeparator)
	if !found {
		return strings.TrimSpace(body), ""
	}
	return strings.TrimSpace(q), strings.TrimSpace(a)
}

// ProcessArticleContent processes article content through the content processor
func (s *CompositeService) ProcessArticleContent(article *models.Article) error {
	// AMA normalization (the single split site): legacy AMAs stored the
	// question and answer concatenated in the body with no frontmatter
	// question. Derive Question once here so every downstream surface (card,
	// detail, search) reads a uniform shape — Question = question, Content =
	// answer. New AMAs carry Question in frontmatter (parsed by the
	// repository), so the Question=="" guard skips them, leaving any author
	// "---" in their answer untouched.
	if article.Type == TypeAMA && article.Question == "" {
		article.Question, article.Content = splitLegacyAMA(article.Content)
	}

	// Process markdown content to HTML
	processedHTML, err := s.contentProcessor.ProcessMarkdown(article.Content)
	if err != nil {
		return err
	}

	// Process duplicate titles
	article.ProcessedContent = s.contentProcessor.ProcessDuplicateTitles(article.Title, processedHTML)

	// Generate excerpt
	article.Excerpt = s.contentProcessor.GenerateExcerpt(article.Content, 200)

	// Calculate reading time and word count
	article.ReadingTime = s.contentProcessor.CalculateReadingTime(article.Content)
	article.WordCount = len(strings.Fields(article.Content))

	return nil
}

// GetStats returns article statistics
func (s *CompositeService) GetStats() *models.Stats {
	return s.repository.GetStats()
}

// GetAllTags returns all unique tags
func (s *CompositeService) GetAllTags() []string {
	articles := s.repository.GetPublished()
	tagSet := make(map[string]bool)

	for _, article := range articles {
		for _, tag := range article.Tags {
			tagSet[tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}

	slices.Sort(tags)
	return tags
}

// GetAllCategories returns all unique categories
func (s *CompositeService) GetAllCategories() []string {
	articles := s.repository.GetPublished()
	categorySet := make(map[string]bool)

	for _, article := range articles {
		for _, category := range article.Categories {
			categorySet[category] = true
		}
	}

	categories := make([]string, 0, len(categorySet))
	for category := range categorySet {
		categories = append(categories, category)
	}

	slices.Sort(categories)
	return categories
}

// GetTagCounts returns tag usage statistics
func (s *CompositeService) GetTagCounts() []models.TagCount {
	articles := s.repository.GetPublished()
	tagCounts := make(map[string]int)

	for _, article := range articles {
		for _, tag := range article.Tags {
			tagCounts[tag]++
		}
	}

	result := make([]models.TagCount, 0, len(tagCounts))
	for tag, count := range tagCounts {
		result = append(result, models.TagCount{
			Tag:   tag,
			Count: count,
		})
	}

	slices.SortFunc(result, func(a, b models.TagCount) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Tag, b.Tag)
	})

	return result
}

// GetCategoryCounts returns category usage statistics
func (s *CompositeService) GetCategoryCounts() []models.CategoryCount {
	articles := s.repository.GetPublished()
	categoryCounts := make(map[string]int)

	for _, article := range articles {
		for _, category := range article.Categories {
			categoryCounts[category]++
		}
	}

	result := make([]models.CategoryCount, 0, len(categoryCounts))
	for category, count := range categoryCounts {
		result = append(result, models.CategoryCount{
			Category: category,
			Count:    count,
		})
	}

	slices.SortFunc(result, func(a, b models.CategoryCount) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Category, b.Category)
	})

	return result
}

// ReloadArticles reloads all articles from storage
func (s *CompositeService) ReloadArticles() error {
	s.logger.Info("Reloading articles")

	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	err := s.repository.Reload(ctx)
	if err != nil {
		s.logger.Error("Failed to reload articles", "error", err)
		return fmt.Errorf("failed to reload articles: %w", err)
	}

	// Get reloaded articles and process them
	articles := s.repository.GetPublished()
	for _, article := range articles {
		if err := s.ProcessArticleContent(article); err != nil {
			s.logger.Warn("Failed to process reloaded article content", "slug", article.Slug, "error", err)
		}
	}

	// Rebuild search index
	s.buildSearchIndex(articles)

	s.logger.Info("Articles reloaded successfully", "count", len(articles))
	return nil
}

// GetLastReloadTime returns the time of the last article reload
func (s *CompositeService) GetLastReloadTime() time.Time {
	return s.repository.GetLastModified()
}

// Private methods

func (s *CompositeService) buildSearchIndex(articles []*models.Article) {
	s.logger.Info("Building search index", "articles", len(articles))

	s.indexMutex.Lock()
	defer s.indexMutex.Unlock()

	s.searchIndex = s.searchService.BuildSearchIndex(articles)
	s.indexBuilt = true

	s.logger.Info("Search index built successfully")
}

// Ensure CompositeService implements Service interface
var _ Service = (*CompositeService)(nil)
