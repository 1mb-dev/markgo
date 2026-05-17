package compose

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

// Service handles creating new posts from compose form input.
type Service struct {
	articlesPath  string
	defaultAuthor string
}

// NewService creates a new compose service.
func NewService(articlesPath, defaultAuthor string) *Service {
	return &Service{
		articlesPath:  articlesPath,
		defaultAuthor: defaultAuthor,
	}
}

// Input represents the compose form or API submission.
type Input struct {
	Content     string `json:"content"`
	Title       string `json:"title"`
	Description string `json:"description"`
	LinkURL     string `json:"link_url"`
	Tags        string `json:"tags"`
	Categories  string `json:"categories"`
	Banner      string `json:"banner"`
	BannerAlt   string `json:"banner_alt"`
	Draft       bool   `json:"draft"`
	Asker       string `json:"asker"`
	AskerEmail  string `json:"asker_email"`
	Type        string `json:"type"`
}

// CreatePost creates a new markdown post file from compose input.
// Returns the generated slug.
func (s *Service) CreatePost(input *Input) (string, error) {
	now := time.Now()

	// Generate slug (fall back to timestamp-prefixed slug if title produces
	// empty slug, e.g. non-ASCII titles or empty Title field). Prefix
	// reflects content type so `ls articles/` reveals intent at a glance.
	var slug string
	if input.Title != "" {
		slug = generateSlug(input.Title)
	}
	if slug == "" {
		slug = fmt.Sprintf("%s-%d", slugPrefixFor(input.Type), now.UnixMilli())
	}

	// Parse comma-separated tags
	var tags []string
	for _, t := range strings.Split(input.Tags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	// Build frontmatter map (only non-empty fields)
	fm := map[string]any{
		"slug": slug,
		"date": now.Format(time.RFC3339),
	}
	setIfNonEmpty(fm, "title", input.Title)
	setIfNonEmpty(fm, "description", input.Description)
	setIfNonEmpty(fm, "link_url", input.LinkURL)
	// Parse comma-separated categories
	var categories []string
	for _, cat := range strings.Split(input.Categories, ",") {
		cat = strings.TrimSpace(cat)
		if cat != "" {
			categories = append(categories, cat)
		}
	}
	if len(tags) > 0 {
		fm["tags"] = tags
	}
	if len(categories) > 0 {
		fm["categories"] = categories
	}
	setIfNonEmpty(fm, "banner", input.Banner)
	if input.Banner != "" {
		setIfNonEmpty(fm, "banner_alt", input.BannerAlt)
	}
	setIfNonEmpty(fm, "author", s.defaultAuthor)
	setIfNonEmpty(fm, "type", input.Type)
	setIfNonEmpty(fm, "asker", input.Asker)
	setIfNonEmpty(fm, "asker_email", input.AskerEmail)
	fm["draft"] = input.Draft

	// Marshal frontmatter to YAML
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Build file content
	content := fmt.Sprintf("---\n%s---\n\n%s\n", string(yamlBytes), strings.TrimSpace(input.Content))

	// Build filename with date prefix
	dateStr := now.Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.md", dateStr, slug)
	filePath := filepath.Join(s.articlesPath, filename)

	// Write file (O_EXCL prevents overwriting an existing post)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // article files should be world-readable
	if err != nil {
		return "", fmt.Errorf("failed to create article file (may already exist): %w", err)
	}
	_, writeErr := f.WriteString(content)
	closeErr := f.Close()
	if writeErr != nil {
		return "", fmt.Errorf("failed to write article file: %w", writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("failed to close article file: %w", closeErr)
	}

	return slug, nil
}

// LoadArticle reads an existing article file by slug and returns its content for editing.
func (s *Service) LoadArticle(slug string) (*Input, error) {
	_, content, err := s.findFileBySlug(slug)
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid article format: missing frontmatter")
	}

	var fm map[string]any
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	input := &Input{
		Content: strings.TrimSpace(parts[2]),
	}
	if title, ok := fm["title"].(string); ok {
		input.Title = title
	}
	if desc, ok := fm["description"].(string); ok {
		input.Description = desc
	}
	if linkURL, ok := fm["link_url"].(string); ok {
		input.LinkURL = linkURL
	}
	if tags, ok := fm["tags"].([]any); ok {
		var tagStrs []string
		for _, t := range tags {
			if str, ok := t.(string); ok {
				tagStrs = append(tagStrs, str)
			}
		}
		input.Tags = strings.Join(tagStrs, ", ")
	}
	if cats, ok := fm["categories"].([]any); ok {
		var catStrs []string
		for _, c := range cats {
			if str, ok := c.(string); ok {
				catStrs = append(catStrs, str)
			}
		}
		input.Categories = strings.Join(catStrs, ", ")
	}
	if banner, ok := fm["banner"].(string); ok {
		input.Banner = banner
	}
	if bannerAlt, ok := fm["banner_alt"].(string); ok {
		input.BannerAlt = bannerAlt
	}
	if draft, ok := fm["draft"].(bool); ok {
		input.Draft = draft
	}
	if asker, ok := fm["asker"].(string); ok {
		input.Asker = asker
	}
	if askerEmail, ok := fm["asker_email"].(string); ok {
		input.AskerEmail = askerEmail
	}
	if typ, ok := fm["type"].(string); ok {
		input.Type = typ
	}

	return input, nil
}

// UpdateArticle overwrites an existing article file with updated content.
// Preserves fields not exposed in the compose form (slug, date, author).
func (s *Service) UpdateArticle(slug string, input *Input) error {
	filePath, existingContent, err := s.findFileBySlug(slug)
	if err != nil {
		return err
	}

	parts := strings.SplitN(string(existingContent), "---", 3)
	if len(parts) < 3 {
		return fmt.Errorf("invalid article format: missing frontmatter")
	}

	var fm map[string]any
	if unmarshalErr := yaml.Unmarshal([]byte(parts[1]), &fm); unmarshalErr != nil {
		return fmt.Errorf("failed to parse frontmatter: %w", unmarshalErr)
	}

	// Update editable fields, preserve everything else (slug, date, author)
	if input.Title != "" {
		fm["title"] = input.Title
	} else {
		delete(fm, "title")
	}
	if input.Description != "" {
		fm["description"] = input.Description
	} else {
		delete(fm, "description")
	}
	if input.LinkURL != "" {
		fm["link_url"] = input.LinkURL
	} else {
		delete(fm, "link_url")
	}

	var tags []string
	for _, t := range strings.Split(input.Tags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	if len(tags) > 0 {
		fm["tags"] = tags
	} else {
		delete(fm, "tags")
	}

	var categories []string
	for _, cat := range strings.Split(input.Categories, ",") {
		cat = strings.TrimSpace(cat)
		if cat != "" {
			categories = append(categories, cat)
		}
	}
	if len(categories) > 0 {
		fm["categories"] = categories
	} else {
		delete(fm, "categories")
	}

	if input.Banner != "" {
		fm["banner"] = input.Banner
		if input.BannerAlt != "" {
			fm["banner_alt"] = input.BannerAlt
		} else {
			delete(fm, "banner_alt")
		}
	} else {
		// banner_alt is meaningless without banner — drop both together
		// so removing a banner doesn't leave orphaned alt text in frontmatter.
		delete(fm, "banner")
		delete(fm, "banner_alt")
	}

	fm["draft"] = input.Draft

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	fileContent := fmt.Sprintf("---\n%s---\n\n%s\n", string(yamlBytes), strings.TrimSpace(input.Content))

	// Atomic write: temp file + rename prevents data loss on crash
	tmpFile, err := os.CreateTemp(filepath.Dir(filePath), ".markgo-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(fileContent); err != nil {
		tmpFile.Close()    //nolint:gosec // best-effort cleanup
		os.Remove(tmpPath) //nolint:gosec // best-effort cleanup
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath) //nolint:gosec // best-effort cleanup
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath) //nolint:gosec // best-effort cleanup
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// DeletePost removes an article file by slug.
func (s *Service) DeletePost(slug string) error {
	filePath, _, err := s.findFileBySlug(slug)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete article file: %w", err)
	}
	return nil
}

// findFileBySlug scans the articles directory for a file with matching slug.
// First checks the frontmatter `slug:` field (compose-created articles), then falls back
// to matching the filename-derived slug (pre-existing articles without explicit slug field).
// Returns the file path and raw content to avoid a redundant re-read by callers.
func (s *Service) findFileBySlug(slug string) (string, []byte, error) {
	entries, err := os.ReadDir(s.articlesPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read articles directory: %w", err)
	}

	// Track filename-derived match as fallback
	var fallbackPath string
	var fallbackContent []byte

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(s.articlesPath, entry.Name())
		content, err := os.ReadFile(filePath) // #nosec G304 -- filePath is built from articlesPath + directory entry
		if err != nil {
			slog.Warn("Skipping unreadable article file", "path", filePath, "error", err)
			continue
		}

		parts := strings.SplitN(string(content), "---", 3)
		if len(parts) < 3 {
			continue
		}

		// Check filename-derived slug as fallback (before yaml parse, so
		// files with corrupted frontmatter can still match by filename)
		if fallbackPath == "" && slugFromFilename(entry.Name()) == slug {
			fallbackPath = filePath
			fallbackContent = content
		}

		var fm struct {
			Slug string `yaml:"slug"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
			continue
		}

		// Prefer explicit frontmatter slug (exact match)
		if fm.Slug == slug {
			return filePath, content, nil
		}
	}

	if fallbackPath != "" {
		return fallbackPath, fallbackContent, nil
	}

	return "", nil, fmt.Errorf("article not found: %s: %w", slug, apperrors.ErrArticleNotFound)
}

// slugFromFilename derives a slug from an article filename.
// "2024-01-15-welcome-to-markgo.md" → "welcome-to-markgo"
// "about.md" → "about"
func slugFromFilename(filename string) string {
	name := strings.TrimSuffix(filename, ".md")
	// Strip YYYY-MM-DD- date prefix if present (validate digits, not just hyphens)
	if len(name) > 11 && name[4] == '-' && name[7] == '-' && name[10] == '-' && isDatePrefix(name[:10]) {
		name = name[11:]
	}
	return name
}

// isDatePrefix checks if a 10-char string looks like YYYY-MM-DD (all digits except positions 4,7).
func isDatePrefix(s string) bool {
	for i, c := range s {
		if i == 4 || i == 7 {
			continue // already verified as '-'
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// generateSlug creates a URL-friendly slug from a title.
// slugPrefixFor returns the fallback slug prefix for a content type — used
// when a post has no title and we fall back to "<prefix>-<timestamp>".
// Picking by type keeps `ls articles/` legible and grep-friendly.
//
// Empty type → "thought": preserves historical behavior (the compose
// handler doesn't set Type before CreatePost; inference happens at read
// time). Truly unknown types → "post": flags miscategorization with a
// neutral prefix rather than masquerading as a known type.
func slugPrefixFor(contentType string) string {
	switch contentType {
	case "", "thought":
		return "thought"
	case "ama":
		return "ama"
	case "link":
		return "link"
	case "article":
		return "article"
	default:
		return "post"
	}
}

// setIfNonEmpty assigns val to m[key] iff val is non-empty. Used by
// CreatePost to keep the frontmatter-build block uniform; UpdateArticle
// can't use it because it needs to delete keys on empty input (different
// semantics).
func setIfNonEmpty(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func generateSlug(title string) string {
	slug := strings.ToLower(title)
	slug = strings.ReplaceAll(slug, " ", "-")

	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	slug = result.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	return slug
}
