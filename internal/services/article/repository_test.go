package article

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validArticle = `---
title: "Test Article"
description: "A test article"
date: 2025-06-15T10:00:00Z
tags: [go, testing]
categories: [tech]
slug: "test-article"
draft: false
featured: false
author: "Jane Doe"
---

# Test Article

This is the body of a test article with enough words to test reading time.
`

const draftArticle = `---
title: "Draft Article"
description: "A draft"
date: 2025-06-14T10:00:00Z
tags: [draft]
categories: [tech]
slug: "draft-article"
draft: true
featured: false
author: "Jane Doe"
---

# Draft

Work in progress.
`

const featuredArticle = `---
title: "Featured Article"
description: "Featured"
date: 2025-06-16T10:00:00Z
tags: [featured, go]
categories: [news]
slug: "featured-article"
draft: false
featured: true
author: "Jane Doe"
---

# Featured

This is a featured article.
`

func setupTestRepo(t *testing.T, files map[string]string) *FileSystemRepository {
	t.Helper()
	dir := t.TempDir()
	uploadDir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return NewFileSystemRepository(dir, uploadDir, slog.Default())
}

// setupRepoWithUploads creates a repo with article files AND banner asset files
// under uploads/<slug>/. Returns the repo and a captured-log buffer for assertions.
func setupRepoWithUploads(t *testing.T, articles map[string]string, uploads map[string][]byte) (*FileSystemRepository, *bytes.Buffer) {
	t.Helper()
	articlesDir := t.TempDir()
	uploadDir := t.TempDir()
	for name, content := range articles {
		path := filepath.Join(articlesDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	for relPath, content := range uploads {
		full := filepath.Join(uploadDir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
		require.NoError(t, os.WriteFile(full, content, 0o600))
	}
	logBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewFileSystemRepository(articlesDir, uploadDir, logger), logBuf
}

// articleWithBanner builds a minimal valid article markdown with banner fields.
func articleWithBanner(slug, banner, bannerAlt string) string {
	return fmt.Sprintf(`---
title: "Post %s"
description: "post"
date: 2025-06-15T10:00:00Z
slug: "%s"
banner: "%s"
banner_alt: "%s"
---

# %s

Body content here for word count.
`, slug, slug, banner, bannerAlt, slug)
}

func TestLoadAll_ValidFiles(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":     validArticle,
		"draft-article.md":    draftArticle,
		"featured-article.md": featuredArticle,
	})

	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, articles, 3)

	// Should be sorted by date descending (newest first)
	assert.Equal(t, "featured-article", articles[0].Slug)
	assert.Equal(t, "test-article", articles[1].Slug)
	assert.Equal(t, "draft-article", articles[2].Slug)
}

func TestLoadAll_CorruptedFileSkipped(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"good.md": validArticle,
		"bad.md":  "this is not valid frontmatter at all",
	})

	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	// Corrupted file is skipped with a warning (intentional behavior)
	assert.Len(t, articles, 1)
	assert.Equal(t, "test-article", articles[0].Slug)
}

func TestLoadAll_EmptyDirectory(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{})

	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, articles)
}

func TestLoadAll_ContextCancellation(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md": validArticle,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := repo.LoadAll(ctx)
	assert.Error(t, err)
}

func TestGetBySlug(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md": validArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		article, err := repo.GetBySlug("test-article")
		require.NoError(t, err)
		assert.Equal(t, "Test Article", article.Title)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetBySlug("nonexistent")
		assert.Error(t, err)
	})
}

func TestGetByTag(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":     validArticle,
		"featured-article.md": featuredArticle,
		"draft-article.md":    draftArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	// "go" tag matches test-article and featured-article
	articles := repo.GetByTag("go")
	assert.Len(t, articles, 2)

	// Case-insensitive
	articles = repo.GetByTag("GO")
	assert.Len(t, articles, 2)

	// No match
	articles = repo.GetByTag("nonexistent")
	assert.Empty(t, articles)
}

func TestGetByCategory(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":     validArticle,
		"featured-article.md": featuredArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	articles := repo.GetByCategory("tech")
	assert.Len(t, articles, 1)
	assert.Equal(t, "test-article", articles[0].Slug)

	articles = repo.GetByCategory("news")
	assert.Len(t, articles, 1)
	assert.Equal(t, "featured-article", articles[0].Slug)

	// Case-insensitive
	articles = repo.GetByCategory("TECH")
	assert.Len(t, articles, 1)
}

func TestGetPublished(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":  validArticle,
		"draft-article.md": draftArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	published := repo.GetPublished()
	assert.Len(t, published, 1)
	assert.Equal(t, "test-article", published[0].Slug)
}

func TestGetDrafts(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":  validArticle,
		"draft-article.md": draftArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	drafts := repo.GetDrafts()
	assert.Len(t, drafts, 1)
	assert.Equal(t, "draft-article", drafts[0].Slug)
}

func TestGetFeatured(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":     validArticle,
		"featured-article.md": featuredArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	featured := repo.GetFeatured(10)
	assert.Len(t, featured, 1)
	assert.Equal(t, "featured-article", featured[0].Slug)

	// Limit of 1 returns exactly 1
	featured = repo.GetFeatured(1)
	assert.Len(t, featured, 1)
}

func TestGetRecent(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":     validArticle,
		"featured-article.md": featuredArticle,
		"draft-article.md":    draftArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	// Should only include published articles, sorted by date
	recent := repo.GetRecent(1)
	assert.Len(t, recent, 1)
	assert.Equal(t, "featured-article", recent[0].Slug) // newest published

	recent = repo.GetRecent(10)
	assert.Len(t, recent, 2) // excludes the draft
}

func TestUpdateDraftStatus(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md": validArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	// Publish -> draft
	err = repo.UpdateDraftStatus("test-article", true)
	require.NoError(t, err)

	article, err := repo.GetBySlug("test-article")
	require.NoError(t, err)
	assert.True(t, article.Draft)

	// Verify file was updated
	content, err := os.ReadFile(filepath.Join(repo.articlesPath, "test-article.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "draft: true")

	t.Run("invalid slug", func(t *testing.T) {
		err := repo.UpdateDraftStatus("../escape", false)
		assert.Error(t, err)
	})

	t.Run("empty slug", func(t *testing.T) {
		err := repo.UpdateDraftStatus("", false)
		assert.Error(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdateDraftStatus("nonexistent", false)
		assert.Error(t, err)
	})
}

func TestGetStats(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md":     validArticle,
		"draft-article.md":    draftArticle,
		"featured-article.md": featuredArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	stats := repo.GetStats()
	assert.Equal(t, 3, stats.TotalArticles)
	assert.Equal(t, 2, stats.PublishedCount)
	assert.Equal(t, 1, stats.DraftCount)
	assert.Greater(t, stats.TotalTags, 0)
	assert.Greater(t, stats.TotalCategories, 0)
}

func TestGetLastModified(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md": validArticle,
	})

	before := time.Now()
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	lastMod := repo.GetLastModified()
	assert.False(t, lastMod.Before(before))
}

func TestReload(t *testing.T) {
	repo := setupTestRepo(t, map[string]string{
		"test-article.md": validArticle,
	})
	_, err := repo.LoadAll(context.Background())
	require.NoError(t, err)

	err = repo.Reload(context.Background())
	require.NoError(t, err)
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"simple", "Hello World", "hello-world"},
		{"special chars", "Go 1.21: What's New?", "go-121-whats-new"},
		{"consecutive hyphens collapsed", "Hello   World", "hello-world"},
		{"leading trailing trimmed", " -Hello- ", "hello"},
		{"non-latin chars dropped", "日本語タイトル", ""},
		{"mixed", "My Post #42 — Tips & Tricks", "my-post-42-tips-tricks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateSlug(tt.title)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCalculateReadingTime(t *testing.T) {
	tests := []struct {
		name      string
		wordCount int
		want      int
	}{
		{"zero words", 0, 1},    // minimum 1 minute
		{"one word", 1, 1},      // minimum 1 minute
		{"199 words", 199, 1},   // < 200 wpm rounds down, but min is 1
		{"200 words", 200, 1},   // exactly 1 minute
		{"400 words", 400, 2},   // 2 minutes
		{"1000 words", 1000, 5}, // 5 minutes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateReadingTime(tt.wordCount)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsMarkdownFile(t *testing.T) {
	assert.True(t, isMarkdownFile("article.md"))
	assert.True(t, isMarkdownFile("article.markdown"))
	assert.True(t, isMarkdownFile("article.mdown"))
	assert.True(t, isMarkdownFile("article.mkd"))
	assert.False(t, isMarkdownFile("article.txt"))
	assert.False(t, isMarkdownFile("article.html"))
}

func TestBannerValidation_TraversalRejected(t *testing.T) {
	repo, _ := setupRepoWithUploads(t,
		map[string]string{"post.md": articleWithBanner("malicious", "../../../etc/passwd", "")},
		nil,
	)
	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, articles, "article with traversal banner must be rejected")
}

func TestBannerValidation_DangerousSchemeRejected(t *testing.T) {
	repo, _ := setupRepoWithUploads(t,
		map[string]string{"post.md": articleWithBanner("xss-attempt", "javascript:alert(1)", "")},
		nil,
	)
	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, articles, "article with javascript: scheme must be rejected")
}

func TestBannerValidation_AbsoluteURLAccepted(t *testing.T) {
	repo, _ := setupRepoWithUploads(t,
		map[string]string{"post.md": articleWithBanner("cdn-banner", "https://cdn.example.com/img/hero.jpg", "Cover")},
		nil,
	)
	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "https://cdn.example.com/img/hero.jpg", articles[0].Banner)
	assert.Equal(t, "https://cdn.example.com/img/hero.jpg", articles[0].BannerSrc())
}

func TestBannerValidation_RelativePathFileExists(t *testing.T) {
	repo, _ := setupRepoWithUploads(t,
		map[string]string{"post.md": articleWithBanner("local-banner", "hero.jpg", "Cover")},
		map[string][]byte{"local-banner/hero.jpg": []byte("fakeimg")},
	)
	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "hero.jpg", articles[0].Banner)
	assert.Equal(t, "/uploads/local-banner/hero.jpg", articles[0].BannerSrc())
}

func TestBannerValidation_RelativePathMissingFile(t *testing.T) {
	repo, logBuf := setupRepoWithUploads(t,
		map[string]string{"post.md": articleWithBanner("missing-banner", "ghost.jpg", "")},
		nil, // no upload file
	)
	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	require.Len(t, articles, 1, "article should load despite missing banner file")
	assert.Equal(t, "ghost.jpg", articles[0].Banner)
	assert.Contains(t, logBuf.String(), "Banner file not found")
}

func TestBannerValidation_WarnOnNonEssayType(t *testing.T) {
	thoughtMD := `---
date: 2025-06-15T10:00:00Z
slug: "just-a-thought"
banner: "ignored.jpg"
---

Short thought.
`
	repo, logBuf := setupRepoWithUploads(t,
		map[string]string{"thought.md": thoughtMD},
		map[string][]byte{"just-a-thought/ignored.jpg": []byte("img")},
	)
	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "thought", articles[0].Type)
	assert.Contains(t, logBuf.String(), "Banner field is essay-only")
}

func TestBannerValidation_EmptyFieldsNoBanner(t *testing.T) {
	repo, logBuf := setupRepoWithUploads(t,
		map[string]string{"post.md": articleWithBanner("no-banner", "", "")},
		nil,
	)
	articles, err := repo.LoadAll(context.Background())
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Empty(t, articles[0].Banner)
	assert.Empty(t, articles[0].BannerSrc())
	assert.NotContains(t, logBuf.String(), `msg="Banner`)
}

func TestBannerValidation_ServerAbsoluteAccepted(t *testing.T) {
	cases := []struct {
		name   string
		slug   string
		banner string
	}{
		{"under static tree", "static-banner", "/static/img/foo.png"},
		{"cross-slug uploads path is coherent under broad reading", "cross-uploads", "/uploads/other/foo.png"},
		{"non-existent route is the writer's problem", "admin-typo", "/admin/anything"},
		{"literal dots in filename do not trip path.Clean", "dotty", "/static/foo..bar.png"},
		{"root path is canonical", "root", "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := setupRepoWithUploads(t,
				map[string]string{"post.md": articleWithBanner(tc.slug, tc.banner, "")},
				nil, // no uploads asset needed — server-absolute paths are served elsewhere
			)
			articles, err := repo.LoadAll(context.Background())
			require.NoError(t, err)
			require.Len(t, articles, 1)
			assert.Equal(t, tc.banner, articles[0].Banner)
			assert.Equal(t, tc.banner, articles[0].BannerSrc())
		})
	}
}

func TestBannerValidation_ServerAbsoluteNonCanonicalRejected(t *testing.T) {
	cases := []struct {
		name   string
		banner string
	}{
		{"parent-dir traversal", "/static/../etc/passwd"},
		{"single-dot segment", "/static/./foo.png"},
		{"double-slash segment", "/static//foo.png"},
		{"mid-path traversal", "/static/img/../../etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, logBuf := setupRepoWithUploads(t,
				map[string]string{"post.md": articleWithBanner("non-canonical", tc.banner, "")},
				nil,
			)
			articles, err := repo.LoadAll(context.Background())
			require.NoError(t, err)
			assert.Empty(t, articles, "non-canonical server-absolute banner must reject the article")
			assert.Contains(t, logBuf.String(), "failed canonical check")
		})
	}
}

// TestWriteFileAtomically_CleansUpBackupOnSuccess verifies the defer-based
// cleanup introduced for F4 in v3.10.3. Regression guard: pre-fix the success
// path explicitly removed the backup; we replaced that with a defer that
// covers BOTH success AND rename-failure paths in one statement.
func TestWriteFileAtomically_CleansUpBackupOnSuccess(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "article.md")
	require.NoError(t, os.WriteFile(filePath, []byte("original"), 0o600))

	repo := NewFileSystemRepository(dir, t.TempDir(), slog.Default())
	require.NoError(t, repo.writeFileAtomically(filePath, "new content"))

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(content))

	// The backup must not linger after success — defer should have removed it.
	_, err = os.Stat(filePath + ".backup")
	assert.True(t, os.IsNotExist(err), "backup file should not exist after successful write; got err=%v", err)
}

// TestWriteFileAtomically_NoBackupLeakWhenParentMissing verifies the defer
// is a no-op (suppressed fs.ErrNotExist) when the backup write itself failed
// because the parent directory doesn't exist. Function returns an error from
// the prior os.ReadFile (cannot read original); no stranded files.
func TestWriteFileAtomically_NoBackupLeakWhenParentMissing(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "subdir-does-not-exist", "article.md")

	repo := NewFileSystemRepository(dir, t.TempDir(), slog.Default())
	err := repo.writeFileAtomically(filePath, "new content")
	require.Error(t, err)

	// No backup file should be created in the non-existent parent path.
	_, err = os.Stat(filePath + ".backup")
	assert.True(t, os.IsNotExist(err), "no backup file should be stranded")
}
