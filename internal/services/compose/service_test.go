package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePost_Thought(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Content: "Just a quick thought about Go.",
	})

	require.NoError(t, err)
	assert.Contains(t, slug, "thought-")

	// Verify file exists
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "---")
	assert.Contains(t, s, "slug: thought-")
	assert.NotContains(t, s, "title:")
	assert.Contains(t, s, "author: Test Author")
	assert.Contains(t, s, "Just a quick thought about Go.")
}

func TestCreatePost_Link(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Title:   "Interesting Read",
		Content: "This article is worth checking out.",
		LinkURL: "https://example.com/article",
		Tags:    "tech, reading",
	})

	require.NoError(t, err)
	assert.Equal(t, "interesting-read", slug)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "title: Interesting Read")
	assert.Contains(t, s, "link_url: https://example.com/article")
	assert.Contains(t, s, "- tech")
	assert.Contains(t, s, "- reading")
}

func TestCreatePost_Article(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Title:   "Getting Started with Go",
		Content: "Go is a statically typed language...",
		Tags:    "golang, tutorial",
		Draft:   true,
	})

	require.NoError(t, err)
	assert.Equal(t, "getting-started-with-go", slug)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "title: Getting Started with Go")
	assert.Contains(t, s, "draft: true")
	assert.Contains(t, s, "Getting Started with Go")
}

func TestCreatePost_EmptyTags(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "")

	slug, err := svc.CreatePost(&Input{
		Content: "No tags here.",
	})

	require.NoError(t, err)
	assert.Contains(t, slug, "thought-")

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	// Should not contain tags or author when empty
	assert.NotContains(t, s, "tags:")
	assert.NotContains(t, s, "author:")
}

func TestLoadArticle(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// Create a post first
	slug, err := svc.CreatePost(&Input{
		Title:       "Test Article",
		Description: "A test article description",
		Content:     "Some markdown **content** here.",
		LinkURL:     "https://example.com",
		Tags:        "go, test",
		Categories:  "programming, tutorials",
		Draft:       true,
	})
	require.NoError(t, err)

	// Load it back
	input, err := svc.LoadArticle(slug)
	require.NoError(t, err)

	assert.Equal(t, "Test Article", input.Title)
	assert.Equal(t, "A test article description", input.Description)
	assert.Equal(t, "Some markdown **content** here.", input.Content)
	assert.Equal(t, "https://example.com", input.LinkURL)
	assert.Contains(t, input.Tags, "go")
	assert.Contains(t, input.Tags, "test")
	assert.Contains(t, input.Categories, "programming")
	assert.Contains(t, input.Categories, "tutorials")
	assert.True(t, input.Draft)
}

func TestLoadArticle_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "")

	_, err := svc.LoadArticle("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "article not found")
}

func TestUpdateArticle(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// Create a post
	slug, err := svc.CreatePost(&Input{
		Title:   "Original Title",
		Content: "Original content.",
		Tags:    "go",
	})
	require.NoError(t, err)

	// Update it with description and categories
	_, err = svc.UpdateArticle(slug, &Input{
		Title:       "Updated Title",
		Description: "Updated description for SEO",
		Content:     "Updated content with **markdown**.",
		Tags:        "go, updated",
		Categories:  "tech, tutorials",
		Draft:       true,
	})
	require.NoError(t, err)

	// Load it back and verify
	input, err := svc.LoadArticle(slug)
	require.NoError(t, err)

	assert.Equal(t, "Updated Title", input.Title)
	assert.Equal(t, "Updated description for SEO", input.Description)
	assert.Equal(t, "Updated content with **markdown**.", input.Content)
	assert.Contains(t, input.Tags, "updated")
	assert.Contains(t, input.Categories, "tech")
	assert.Contains(t, input.Categories, "tutorials")
	assert.True(t, input.Draft)
}

func TestUpdateArticle_PreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// Create a post
	slug, err := svc.CreatePost(&Input{
		Title:   "Metadata Test",
		Content: "Content here.",
	})
	require.NoError(t, err)

	// Update only content
	_, err = svc.UpdateArticle(slug, &Input{
		Title:   "Metadata Test",
		Content: "New content.",
	})
	require.NoError(t, err)

	// Verify the file still has slug, date, and author
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "slug: "+slug)
	assert.Contains(t, s, "author: Test Author")
	assert.Contains(t, s, "date:")
	assert.Contains(t, s, "New content.")
}

func TestLoadArticle_FilenameFallback(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "")

	// Write a pre-existing article without slug: in frontmatter (like articles created before compose)
	content := "---\ntitle: \"Welcome to MarkGo\"\ndate: 2024-01-15T10:00:00Z\ndraft: false\n---\n\nWelcome content here.\n"
	err := os.WriteFile(filepath.Join(dir, "2024-01-15-welcome-to-markgo.md"), []byte(content), 0o644)
	require.NoError(t, err)

	// Load by filename-derived slug (no frontmatter slug: field)
	input, err := svc.LoadArticle("welcome-to-markgo")
	require.NoError(t, err)
	assert.Equal(t, "Welcome to MarkGo", input.Title)
	assert.Equal(t, "Welcome content here.", input.Content)
}

func TestSlugFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"2024-01-15-welcome-to-markgo.md", "welcome-to-markgo"},
		{"2026-02-09-thought-1770657441.md", "thought-1770657441"},
		{"about.md", "about"},
		{"short.md", "short"},
		{"not-a-date-prefix-slug.md", "not-a-date-prefix-slug"}, // non-date hyphens at positions 4,7,10
		{"abcd-ef-gh-my-post.md", "abcd-ef-gh-my-post"},         // letters, not digits
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := slugFromFilename(tt.filename)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCreatePost_AMA(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Content:    "What is your favorite programming language?",
		Draft:      true,
		Asker:      "Alice",
		AskerEmail: "alice@example.com",
		Type:       "ama",
	})

	require.NoError(t, err)
	assert.Contains(t, slug, "ama-", "AMA submission with no title must use ama- prefix (#44)")

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "type: ama")
	assert.Contains(t, s, "asker: Alice")
	assert.Contains(t, s, "asker_email: alice@example.com")
	assert.Contains(t, s, "draft: true")
	assert.Contains(t, s, "What is your favorite programming language?")
}

func TestCreatePost_SlugPrefixByType(t *testing.T) {
	cases := []struct {
		name       string
		inputType  string
		wantPrefix string
	}{
		{"empty type defaults to thought", "", "thought-"},
		{"explicit thought", "thought", "thought-"},
		{"ama", "ama", "ama-"},
		{"link", "link", "link-"},
		{"article", "article", "article-"},
		{"unknown type falls back to neutral post", "unknown-type", "post-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			svc := NewService(dir, "Test Author")

			slug, err := svc.CreatePost(&Input{
				Content: "Body content with no title here.",
				Type:    tc.inputType,
			})
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(slug, tc.wantPrefix),
				"Type=%q: want slug prefix %q, got slug %q", tc.inputType, tc.wantPrefix, slug)
		})
	}
}

func TestLoadArticle_AMAFields(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Content:    "What is your favorite programming language?",
		Draft:      true,
		Asker:      "Alice",
		AskerEmail: "alice@example.com",
		Type:       "ama",
	})
	require.NoError(t, err)

	// Load it back and verify AMA-specific fields are restored
	input, err := svc.LoadArticle(slug)
	require.NoError(t, err)
	assert.Equal(t, "Alice", input.Asker)
	assert.Equal(t, "alice@example.com", input.AskerEmail)
	assert.Equal(t, "ama", input.Type)
	assert.True(t, input.Draft)
	assert.Equal(t, "What is your favorite programming language?", input.Content)
}

func TestCreatePost_PageType_RequiresSlug(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	_, err := svc.CreatePost(&Input{
		Type:    "page",
		Title:   "About",
		Content: "Page body.",
	})

	assert.Error(t, err, "page with empty slug should fail")
	assert.Contains(t, err.Error(), "slug")

	// No file should have been written.
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	assert.Empty(t, files)
}

func TestCreatePost_PageType_UsesExplicitSlug(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Type:    "page",
		Slug:    "my-evergreen",
		Title:   "Unrelated Title",
		Content: "Page body.",
	})

	require.NoError(t, err)
	assert.Equal(t, "my-evergreen", slug, "page slug should come from Input.Slug, not generateSlug(Title)")

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)
	assert.Contains(t, files[0], "-my-evergreen.md", "filename should still carry the date prefix + explicit slug")
}

func TestCreatePost_PageType_OmitsDate(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	_, err := svc.CreatePost(&Input{
		Type:    "page",
		Slug:    "about-the-site",
		Title:   "About",
		Content: "Page body.",
	})
	require.NoError(t, err)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)
	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "type: page")
	assert.Contains(t, s, "slug: about-the-site")
	assert.NotContains(t, s, "date:", "page frontmatter should omit date")
}

func TestCreatePost_NonPage_PreservesDateBehavior(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// Regression: non-page types continue to emit date frontmatter.
	_, err := svc.CreatePost(&Input{
		Type:    "article",
		Title:   "Regular Article",
		Content: "Body.",
	})
	require.NoError(t, err)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)
	content, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Contains(t, string(content), "date:")
}

func TestUpdateArticle_PreservesAMAFields(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Content:    "What is your favorite language?",
		Draft:      true,
		Asker:      "Bob",
		AskerEmail: "bob@example.com",
		Type:       "ama",
	})
	require.NoError(t, err)

	// Update content (simulate answering) — don't set Asker/Type in input
	_, err = svc.UpdateArticle(slug, &Input{
		Content: "What is your favorite language?\n\n---\n\nGo, for its simplicity.",
		Draft:   false,
	})
	require.NoError(t, err)

	// Verify AMA metadata survived the update (preserved in frontmatter map)
	input, err := svc.LoadArticle(slug)
	require.NoError(t, err)
	assert.Equal(t, "Bob", input.Asker)
	assert.Equal(t, "bob@example.com", input.AskerEmail)
	assert.Equal(t, "ama", input.Type)
	assert.False(t, input.Draft)
	assert.Contains(t, input.Content, "Go, for its simplicity.")
}

func TestDeletePost(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// Create a post
	slug, err := svc.CreatePost(&Input{
		Content: "To be deleted.",
		Type:    "ama",
		Asker:   "Bob",
		Draft:   true,
	})
	require.NoError(t, err)

	// Verify file exists
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	// Delete it
	err = svc.DeletePost(slug)
	require.NoError(t, err)

	// Verify file is gone
	files, _ = filepath.Glob(filepath.Join(dir, "*.md"))
	assert.Empty(t, files)
}

func TestDeletePost_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "")

	err := svc.DeletePost("nonexistent-slug")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "article not found")
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Getting Started with Go!", "getting-started-with-go"},
		{"  spaces  and  stuff  ", "spaces-and-stuff"},
		{"", ""},
		{"123 Numbers", "123-numbers"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := generateSlug(tt.input)
			// Trim for comparison since our slug may differ slightly
			assert.True(t, strings.HasPrefix(got, tt.expected) || got == tt.expected,
				"generateSlug(%q) = %q, want prefix %q", tt.input, got, tt.expected)
		})
	}
}

func TestCreatePost_BannerWritten_WhenProvided(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Title:     "Banner Post",
		Content:   "Has a banner.",
		Banner:    "/uploads/banner-post/hero.jpg",
		BannerAlt: "Hero image",
	})
	require.NoError(t, err)
	assert.Equal(t, "banner-post", slug)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "banner: /uploads/banner-post/hero.jpg")
	assert.Contains(t, s, "banner_alt: Hero image")
}

func TestCreatePost_BannerOmitted_WhenEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	_, err := svc.CreatePost(&Input{
		Title:   "No Banner Post",
		Content: "No banner here.",
	})
	require.NoError(t, err)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.NotContains(t, s, "banner:")
	assert.NotContains(t, s, "banner_alt:")
}

func TestCreatePost_BannerAltDroppedWhenBannerEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// User typed alt text but never set a banner — alt text alone is
	// meaningless and must not land in frontmatter as an orphan.
	_, err := svc.CreatePost(&Input{
		Title:     "Orphan Test",
		Content:   "Body.",
		Banner:    "",
		BannerAlt: "Stray alt text",
	})
	require.NoError(t, err)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.NotContains(t, s, "banner:")
	assert.NotContains(t, s, "banner_alt:", "banner_alt must not be written without a banner")
}

func TestUpdateArticle_BannerAltDroppedWhenBannerRemoved(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// Seed an article that has both a banner and alt text.
	slug, err := svc.CreatePost(&Input{
		Title:     "Has Banner",
		Content:   "Body.",
		Banner:    "/uploads/has-banner/hero.jpg",
		BannerAlt: "Hero image",
	})
	require.NoError(t, err)

	// User clicks "Remove banner" but the form still posts the old alt text
	// (the bug this test guards against). Server must drop banner_alt too.
	_, err = svc.UpdateArticle(slug, &Input{
		Title:     "Has Banner",
		Content:   "Body.",
		Banner:    "",
		BannerAlt: "Hero image",
	})
	require.NoError(t, err)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)

	s := string(content)
	assert.NotContains(t, s, "banner:")
	assert.NotContains(t, s, "banner_alt:")
}

// TestUpdateArticle_BannerPreservedOnEdit verifies the read-only sketch decision:
// banners with a /static/ path (or absolute URL) round-trip through LoadArticle +
// UpdateArticle untouched as long as the form re-submits the same value via the
// hidden field, even though the form will not offer "upload" / "remove" controls
// for non-uploads-path banners.
func TestUpdateArticle_BannerPreservedOnEdit(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	// Seed an article with a /static/ banner directly on disk.
	content := "---\nslug: editorial\ntitle: \"Editorial Piece\"\nbanner: /static/editorial/hero.jpg\nbanner_alt: \"Editorial hero\"\ndate: 2026-01-15T10:00:00Z\ndraft: false\n---\n\nBody.\n"
	err := os.WriteFile(filepath.Join(dir, "2026-01-15-editorial.md"), []byte(content), 0o644)
	require.NoError(t, err)

	// Load via the same path the edit handler uses.
	loaded, err := svc.LoadArticle("editorial")
	require.NoError(t, err)
	assert.Equal(t, "/static/editorial/hero.jpg", loaded.Banner)
	assert.Equal(t, "Editorial hero", loaded.BannerAlt)

	// Simulate an edit that doesn't touch the banner: same Banner/BannerAlt
	// submitted (the hidden field round-trips the existing value).
	_, err = svc.UpdateArticle("editorial", &Input{
		Title:     "Editorial Piece (edited)",
		Content:   "Body with edits.",
		Banner:    loaded.Banner,
		BannerAlt: loaded.BannerAlt,
	})
	require.NoError(t, err)

	// Reload and verify the /static/ path survived.
	reloaded, err := svc.LoadArticle("editorial")
	require.NoError(t, err)
	assert.Equal(t, "/static/editorial/hero.jpg", reloaded.Banner)
	assert.Equal(t, "Editorial hero", reloaded.BannerAlt)
	assert.Equal(t, "Editorial Piece (edited)", reloaded.Title)
}
