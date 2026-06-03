package article

import (
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/1mb-dev/markgo/internal/models"
)

// SearchService provides article search functionality
type SearchService interface {
	// Core search operations
	Search(articles []*models.Article, query string, limit int) []*models.SearchResult
	SearchInTitle(articles []*models.Article, query string, limit int) []*models.SearchResult
	SearchByTag(articles []*models.Article, tag string) []*models.Article
	SearchByCategory(articles []*models.Article, category string) []*models.Article

	// Advanced search
	SearchWithFilters(articles []*models.Article, query string, filters *SearchFilters) []*models.SearchResult
	GetSuggestions(articles []*models.Article, query string, limit int) []string

	// Indexing and optimization
	BuildSearchIndex(articles []*models.Article) SearchIndex
	SearchWithIndex(index SearchIndex, query string, limit int) []*models.SearchResult
}

// SearchFilters defines search filtering options
type SearchFilters struct {
	Tags          []string
	Categories    []string
	DateFrom      string
	DateTo        string
	OnlyFeatured  bool
	OnlyPublished bool
}

// SearchIndex provides fast text search capabilities
type SearchIndex struct {
	Articles     []*models.Article
	TitleIndex   map[string][]*models.Article
	ContentIndex map[string][]*models.Article
	TagIndex     map[string][]*models.Article
}

// TextSearchService implements SearchService using text-based search algorithms
type TextSearchService struct {
	logger *slog.Logger

	// Stop words to ignore in search
	stopWords map[string]bool
}

// NewTextSearchService creates a new text-based search service
func NewTextSearchService(logger *slog.Logger) *TextSearchService {
	return &TextSearchService{
		logger: logger,
		stopWords: map[string]bool{
			"the": true, "a": true, "an": true, "and": true, "or": true,
			"but": true, "in": true, "on": true, "at": true, "to": true,
			"for": true, "of": true, "with": true, "by": true, "is": true,
			"are": true, "was": true, "were": true, "be": true, "been": true,
			"have": true, "has": true, "had": true, "do": true, "does": true,
			"did": true, "will": true, "would": true, "could": true, "should": true,
			"this": true, "that": true, "these": true, "those": true,
		},
	}
}

// Search performs full-text search across articles
func (s *TextSearchService) Search(articles []*models.Article, query string, limit int) []*models.SearchResult {
	if query == "" {
		return []*models.SearchResult{}
	}

	query = strings.ToLower(strings.TrimSpace(query))
	searchTerms := s.extractSearchTerms(query)

	if len(searchTerms) == 0 {
		return []*models.SearchResult{}
	}

	var results []*models.SearchResult

	for _, article := range articles {
		if article.Draft {
			continue // Skip draft articles in search
		}

		score, matchedFields := s.calculateRelevanceScore(article, searchTerms)
		if score > 0 {
			results = append(results, &models.SearchResult{
				Article:       article,
				Score:         score,
				MatchedFields: matchedFields,
			})
		}
	}

	// Sort by relevance score (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// SearchInTitle searches only in article titles
func (s *TextSearchService) SearchInTitle(articles []*models.Article, query string, limit int) []*models.SearchResult {
	if query == "" {
		return []*models.SearchResult{}
	}

	query = strings.ToLower(strings.TrimSpace(query))
	searchTerms := s.extractSearchTerms(query)

	var results []*models.SearchResult

	for _, article := range articles {
		if article.Draft {
			continue
		}

		titleLower := strings.ToLower(article.Title)
		score := 0.0
		var matchedFields []string

		for _, term := range searchTerms {
			if strings.Contains(titleLower, term) {
				score += 10.0 // High weight for title matches
				matchedFields = append(matchedFields, "title")
			}
		}

		if score > 0 {
			results = append(results, &models.SearchResult{
				Article:       article,
				Score:         score,
				MatchedFields: matchedFields,
			})
		}
	}

	// Sort by relevance score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// SearchByTag returns articles with the specified tag
func (s *TextSearchService) SearchByTag(articles []*models.Article, tag string) []*models.Article {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return []*models.Article{}
	}

	var results []*models.Article
	for _, article := range articles {
		if article.Draft {
			continue
		}

		for _, articleTag := range article.Tags {
			if strings.EqualFold(articleTag, tag) {
				results = append(results, article)
				break
			}
		}
	}

	return results
}

// SearchByCategory returns articles in the specified category
func (s *TextSearchService) SearchByCategory(articles []*models.Article, category string) []*models.Article {
	category = strings.ToLower(strings.TrimSpace(category))
	if category == "" {
		return []*models.Article{}
	}

	var results []*models.Article
	for _, article := range articles {
		if article.Draft {
			continue
		}

		for _, articleCategory := range article.Categories {
			if strings.EqualFold(articleCategory, category) {
				results = append(results, article)
				break
			}
		}
	}

	return results
}

// SearchWithFilters performs search with additional filtering
func (s *TextSearchService) SearchWithFilters(
	articles []*models.Article,
	query string,
	filters *SearchFilters,
) []*models.SearchResult {
	// First, apply filters to narrow down articles
	filtered := s.applyFilters(articles, filters)

	// Then perform search on filtered articles
	return s.Search(filtered, query, 0) // No limit here, let caller handle
}

// GetSuggestions returns search suggestions based on article content
func (s *TextSearchService) GetSuggestions(articles []*models.Article, query string, limit int) []string {
	if query == "" {
		return []string{}
	}

	query = strings.ToLower(strings.TrimSpace(query))
	suggestions := make(map[string]int)

	for _, article := range articles {
		if article.Draft {
			continue
		}

		// Extract suggestions from title words
		titleWords := s.extractWords(strings.ToLower(article.Title))
		for _, word := range titleWords {
			if strings.HasPrefix(word, query) && len(word) > len(query) {
				suggestions[word]++
			}
		}

		// Extract suggestions from tags
		for _, tag := range article.Tags {
			tagLower := strings.ToLower(tag)
			if strings.HasPrefix(tagLower, query) && len(tagLower) > len(query) {
				suggestions[tagLower]++
			}
		}
	}

	// Convert to sorted slice
	type suggestionCount struct {
		term  string
		count int
	}

	suggestionList := make([]suggestionCount, 0, len(suggestions))
	for term, count := range suggestions {
		suggestionList = append(suggestionList, suggestionCount{term, count})
	}

	// Sort by frequency
	sort.Slice(suggestionList, func(i, j int) bool {
		return suggestionList[i].count > suggestionList[j].count
	})

	// Extract terms and apply limit
	result := make([]string, 0, len(suggestionList))
	for _, item := range suggestionList {
		result = append(result, item.term)
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result
}

// BuildSearchIndex creates an optimized search index
func (s *TextSearchService) BuildSearchIndex(articles []*models.Article) SearchIndex {
	index := SearchIndex{
		Articles:     articles,
		TitleIndex:   make(map[string][]*models.Article),
		ContentIndex: make(map[string][]*models.Article),
		TagIndex:     make(map[string][]*models.Article),
	}

	for _, article := range articles {
		if article.Draft {
			continue // Don't index draft articles
		}

		// Index title words
		titleWords := s.extractWords(strings.ToLower(article.Title))
		for _, word := range titleWords {
			if !s.stopWords[word] {
				index.TitleIndex[word] = append(index.TitleIndex[word], article)
			}
		}

		// Index content words (sample for performance)
		contentWords := s.extractWords(strings.ToLower(article.Content))
		for i, word := range contentWords {
			if !s.stopWords[word] && i < 100 { // Limit to first 100 words for performance
				index.ContentIndex[word] = append(index.ContentIndex[word], article)
			}
		}

		// Index AMA question words. The question is the AMA's title-analog and
		// lives in frontmatter, not the body, so it must be indexed explicitly
		// or readers couldn't find an AMA by its question.
		for _, word := range s.extractWords(strings.ToLower(article.Question)) {
			if !s.stopWords[word] {
				index.ContentIndex[word] = append(index.ContentIndex[word], article)
			}
		}

		// Index tags
		for _, tag := range article.Tags {
			tagLower := strings.ToLower(tag)
			index.TagIndex[tagLower] = append(index.TagIndex[tagLower], article)
		}
	}

	return index
}

// SearchWithIndex performs fast search using pre-built index
func (s *TextSearchService) SearchWithIndex(index SearchIndex, query string, limit int) []*models.SearchResult {
	if query == "" {
		return []*models.SearchResult{}
	}

	query = strings.ToLower(strings.TrimSpace(query))
	searchTerms := s.extractSearchTerms(query)

	articleScores := make(map[*models.Article]*models.SearchResult)

	for _, term := range searchTerms {
		// Search in title index
		if titleArticles, exists := index.TitleIndex[term]; exists {
			for _, article := range titleArticles {
				if result, exists := articleScores[article]; exists {
					result.Score += 10.0 // High weight for title matches
					result.MatchedFields = append(result.MatchedFields, "title")
				} else {
					articleScores[article] = &models.SearchResult{
						Article:       article,
						Score:         10.0,
						MatchedFields: []string{"title"},
					}
				}
			}
		}

		// Search in content index
		if contentArticles, exists := index.ContentIndex[term]; exists {
			for _, article := range contentArticles {
				if result, exists := articleScores[article]; exists {
					result.Score += 2.0 // Lower weight for content matches
					result.MatchedFields = append(result.MatchedFields, "content")
				} else {
					articleScores[article] = &models.SearchResult{
						Article:       article,
						Score:         2.0,
						MatchedFields: []string{"content"},
					}
				}
			}
		}

		// Search in tag index
		if tagArticles, exists := index.TagIndex[term]; exists {
			for _, article := range tagArticles {
				if result, exists := articleScores[article]; exists {
					result.Score += 5.0 // Medium weight for tag matches
					result.MatchedFields = append(result.MatchedFields, "tags")
				} else {
					articleScores[article] = &models.SearchResult{
						Article:       article,
						Score:         5.0,
						MatchedFields: []string{"tags"},
					}
				}
			}
		}
	}

	// Convert to slice and sort
	results := make([]*models.SearchResult, 0, len(articleScores))
	for _, result := range articleScores {
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// Helper methods

func (s *TextSearchService) extractSearchTerms(query string) []string {
	words := s.extractWords(query)
	var terms []string

	for _, word := range words {
		if !s.stopWords[word] && len(word) > 1 {
			terms = append(terms, word)
		}
	}

	return terms
}

func (s *TextSearchService) extractWords(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

func (s *TextSearchService) calculateRelevanceScore(
	article *models.Article,
	searchTerms []string,
) (score float64, matchedFields []string) {
	titleLower := strings.ToLower(article.Title)
	contentLower := strings.ToLower(article.Content)
	descriptionLower := strings.ToLower(article.Description)
	questionLower := strings.ToLower(article.Question)

	for _, term := range searchTerms {
		// Title matches (highest weight)
		if strings.Contains(titleLower, term) {
			score += 10.0
			matchedFields = append(matchedFields, "title")
		}

		// Description matches (high weight)
		if strings.Contains(descriptionLower, term) {
			score += 5.0
			matchedFields = append(matchedFields, "description")
		}

		// AMA question matches (title-analog, high weight). Empty for
		// non-AMA articles, so the Contains is a no-op there.
		if strings.Contains(questionLower, term) {
			score += 5.0
			matchedFields = append(matchedFields, "question")
		}

		// Tag matches (medium weight)
		for _, tag := range article.Tags {
			if strings.EqualFold(tag, term) {
				score += 5.0
				matchedFields = append(matchedFields, "tags")
				break
			}
		}

		// Category matches (medium weight)
		for _, category := range article.Categories {
			if strings.EqualFold(category, term) {
				score += 5.0
				matchedFields = append(matchedFields, "categories")
				break
			}
		}

		// Content matches (lower weight, but count frequency)
		contentMatches := strings.Count(contentLower, term)
		score += float64(contentMatches) * 1.0
		if contentMatches > 0 {
			matchedFields = append(matchedFields, "content")
		}
	}

	// Bonus for featured articles
	if article.Featured {
		score *= 1.2
	}

	// Remove duplicate matched fields
	matchedFields = removeDuplicateStrings(matchedFields)

	return score, matchedFields
}

func (s *TextSearchService) applyFilters(articles []*models.Article, filters *SearchFilters) []*models.Article {
	filtered := make([]*models.Article, 0, len(articles))

	for _, article := range articles {
		if !s.matchesPublishedFilter(article, filters) {
			continue
		}

		if !s.matchesFeaturedFilter(article, filters) {
			continue
		}

		if !s.matchesTagFilter(article, filters) {
			continue
		}

		if !s.matchesCategoryFilter(article, filters) {
			continue
		}

		if !s.matchesDateFilter(article, filters) {
			continue
		}

		filtered = append(filtered, article)
	}

	return filtered
}

// matchesPublishedFilter checks if article matches the published status filter
func (s *TextSearchService) matchesPublishedFilter(article *models.Article, filters *SearchFilters) bool {
	if filters.OnlyPublished && article.Draft {
		return false
	}
	return true
}

// matchesFeaturedFilter checks if article matches the featured status filter
func (s *TextSearchService) matchesFeaturedFilter(article *models.Article, filters *SearchFilters) bool {
	if filters.OnlyFeatured && !article.Featured {
		return false
	}
	return true
}

// matchesTagFilter checks if article matches any of the specified tags
func (s *TextSearchService) matchesTagFilter(article *models.Article, filters *SearchFilters) bool {
	if len(filters.Tags) == 0 {
		return true // No tag filter specified
	}

	for _, filterTag := range filters.Tags {
		for _, articleTag := range article.Tags {
			if strings.EqualFold(articleTag, filterTag) {
				return true
			}
		}
	}
	return false
}

// matchesCategoryFilter checks if article matches any of the specified categories
func (s *TextSearchService) matchesCategoryFilter(article *models.Article, filters *SearchFilters) bool {
	if len(filters.Categories) == 0 {
		return true // No category filter specified
	}

	for _, filterCategory := range filters.Categories {
		for _, articleCategory := range article.Categories {
			if strings.EqualFold(articleCategory, filterCategory) {
				return true
			}
		}
	}
	return false
}

// matchesDateFilter checks if article falls within the specified date range
func (s *TextSearchService) matchesDateFilter(article *models.Article, filters *SearchFilters) bool {
	if filters.DateFrom == "" && filters.DateTo == "" {
		return true // No date filter specified
	}

	articleDate := article.Date

	// Check DateFrom constraint
	if filters.DateFrom != "" {
		dateFrom, err := time.Parse("2006-01-02", filters.DateFrom)
		if err != nil {
			s.logger.Warn("Invalid DateFrom format, skipping date filter", "date", filters.DateFrom, "error", err)
		} else if articleDate.Before(dateFrom) {
			return false
		}
	}

	// Check DateTo constraint
	if filters.DateTo != "" {
		dateTo, err := time.Parse("2006-01-02", filters.DateTo)
		if err != nil {
			s.logger.Warn("Invalid DateTo format, skipping date filter", "date", filters.DateTo, "error", err)
		} else {
			// Add 24 hours to make DateTo inclusive (end of day)
			dateToEndOfDay := dateTo.Add(24 * time.Hour).Add(-time.Nanosecond)
			if articleDate.After(dateToEndOfDay) {
				return false
			}
		}
	}

	return true
}

func removeDuplicateStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// Ensure TextSearchService implements SearchService
var _ SearchService = (*TextSearchService)(nil)
