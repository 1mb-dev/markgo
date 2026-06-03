package services

import (
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/web"
)

func TestNewTemplateService(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{}

	// Create test template files
	testTemplates := map[string]string{
		"base.html": `<!DOCTYPE html>
<html>
<head>
    <title>{{.title}}</title>
</head>
<body>
    <h1>{{.title}}</h1>
    <div>{{.content}}</div>
</body>
</html>`,

		"article.html": `<article>
    <h1>{{.title}}</h1>
    <p>{{.excerpt}}</p>
    <div>{{.content | safeHTML}}</div>
</article>`,

		"list.html": `<ul>
{{range .items}}
    <li>{{.}}</li>
{{end}}
</ul>`,
	}

	for filename, content := range testTemplates {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte(content), 0o600)
		require.NoError(t, err)
	}

	// Test successful creation
	service, err := NewTemplateService(tempDir, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.NotNil(t, service.templates)
	assert.Equal(t, cfg, service.config)

	// Test with non-existent directory — falls back to embedded templates
	embeddedService, err := NewTemplateService("/nonexistent/path", cfg)
	assert.NoError(t, err)
	assert.NotNil(t, embeddedService)
}

func TestTemplateService_Render(t *testing.T) {
	service := createTestTemplateService(t)

	tests := []struct {
		name         string
		templateName string
		data         any
		expectError  bool
		contains     []string
	}{
		{
			name:         "Valid template with data",
			templateName: "base.html",
			data: map[string]any{
				"title":   "Test Page",
				"content": "This is test content",
			},
			expectError: false,
			contains:    []string{"Test Page", "This is test content"},
		},
		{
			name:         "Template with safe HTML",
			templateName: "article.html",
			data: map[string]any{
				"title":   "Article Title",
				"excerpt": "Article excerpt",
				"content": "<p>HTML content</p>",
			},
			expectError: false,
			contains:    []string{"Article Title", "Article excerpt", "<p>HTML content</p>"},
		},
		{
			name:         "Template with loop",
			templateName: "list.html",
			data: map[string]any{
				"items": []string{"Item 1", "Item 2", "Item 3"},
			},
			expectError: false,
			contains:    []string{"Item 1", "Item 2", "Item 3"},
		},
		{
			name:         "Non-existent template",
			templateName: "nonexistent.html",
			data:         map[string]any{},
			expectError:  true,
		},
		{
			name:         "Nil data",
			templateName: "base.html",
			data:         nil,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			err := service.Render(&buf, tt.templateName, tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				output := buf.String()
				for _, expectedContent := range tt.contains {
					assert.Contains(t, output, expectedContent)
				}
			}
		})
	}
}

func TestTemplateService_RenderToString(t *testing.T) {
	service := createTestTemplateService(t)

	data := map[string]any{
		"title":   "Test Title",
		"content": "Test content",
	}

	output, err := service.RenderToString("base.html", data)
	assert.NoError(t, err)
	assert.Contains(t, output, "Test Title")
	assert.Contains(t, output, "Test content")

	// Test with non-existent template
	_, err = service.RenderToString("nonexistent.html", data)
	assert.Error(t, err)
}

func TestTemplateService_HasTemplate(t *testing.T) {
	service := createTestTemplateService(t)

	// Test existing templates
	assert.True(t, service.HasTemplate("base.html"))
	assert.True(t, service.HasTemplate("article.html"))
	assert.True(t, service.HasTemplate("list.html"))

	// Test non-existent template
	assert.False(t, service.HasTemplate("nonexistent.html"))
}

func TestTemplateService_ListTemplates(t *testing.T) {
	service := createTestTemplateService(t)

	templates := service.ListTemplates()
	assert.Greater(t, len(templates), 0)

	// Check that expected templates are in the list
	expectedTemplates := []string{"base.html", "article.html", "list.html"}
	for _, expected := range expectedTemplates {
		assert.Contains(t, templates, expected)
	}
}

func TestTemplateService_Reload(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{}

	// Create initial template
	initialTemplate := `<h1>Initial Template</h1>`
	filePath := filepath.Join(tempDir, "test.html")
	err := os.WriteFile(filePath, []byte(initialTemplate), 0o600)
	require.NoError(t, err)

	service, err := NewTemplateService(tempDir, cfg)
	require.NoError(t, err)

	// Test initial template
	output, err := service.RenderToString("test.html", nil)
	assert.NoError(t, err)
	assert.Contains(t, output, "Initial Template")

	// Update template file
	updatedTemplate := `<h1>Updated Template</h1>`
	err = os.WriteFile(filePath, []byte(updatedTemplate), 0o600)
	require.NoError(t, err)

	// Reload templates
	err = service.Reload(tempDir)
	assert.NoError(t, err)

	// Test updated template
	output, err = service.RenderToString("test.html", nil)
	assert.NoError(t, err)
	assert.Contains(t, output, "Updated Template")
	assert.NotContains(t, output, "Initial Template")
}

func TestTemplateFunctions_SafeHTML(t *testing.T) {
	service := createTestTemplateService(t)

	// Create template that uses safeHTML function
	tempDir := t.TempDir()
	templateContent := `{{.content | safeHTML}}`
	filePath := filepath.Join(tempDir, "safehtml.html")
	err := os.WriteFile(filePath, []byte(templateContent), 0o600)
	require.NoError(t, err)

	// Reload with new template
	err = service.Reload(tempDir)
	require.NoError(t, err)

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "HTML content",
			content:  "<p>Hello <strong>World</strong></p>",
			expected: "<p>Hello <strong>World</strong></p>",
		},
		{
			name:     "Escaped HTML",
			content:  "&lt;p&gt;Escaped&lt;/p&gt;",
			expected: "<p>Escaped</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{"content": tt.content}
			output, err := service.RenderToString("safehtml.html", data)
			assert.NoError(t, err)
			assert.Contains(t, output, tt.expected)
		})
	}
}

func TestTemplateFunctions_StringOperations(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	// Test join function
	joinFunc := funcMap["join"].(func(string, []string) string)
	result := joinFunc(", ", []string{"a", "b", "c"})
	assert.Equal(t, "a, b, c", result)

	// Test lower function
	lowerFunc := funcMap["lower"].(func(string) string)
	result = lowerFunc("HELLO WORLD")
	assert.Equal(t, "hello world", result)

	// Test upper function
	upperFunc := funcMap["upper"].(func(string) string)
	result = upperFunc("hello world")
	assert.Equal(t, "HELLO WORLD", result)

	// Test title function
	titleFunc := funcMap["title"].(func(string) string)
	result = titleFunc("hello world")
	assert.Equal(t, "Hello World", result)

	// Test trim function
	trimFunc := funcMap["trim"].(func(string) string)
	result = trimFunc("  hello world  ")
	assert.Equal(t, "hello world", result)

	// Test truncate function
	truncateFunc := funcMap["truncate"].(func(string, int) string)
	result = truncateFunc("hello world", 5)
	assert.Equal(t, "hello...", result)

	// Test contains function
	containsFunc := funcMap["contains"].(func(string, string) bool)
	assert.True(t, containsFunc("hello world", "world"))
	assert.False(t, containsFunc("hello world", "foo"))

	// Test hasPrefix function
	hasPrefixFunc := funcMap["hasPrefix"].(func(string, string) bool)
	assert.True(t, hasPrefixFunc("hello world", "hello"))
	assert.False(t, hasPrefixFunc("hello world", "world"))

	// Test hasSuffix function
	hasSuffixFunc := funcMap["hasSuffix"].(func(string, string) bool)
	assert.True(t, hasSuffixFunc("hello world", "world"))
	assert.False(t, hasSuffixFunc("hello world", "hello"))
}

func TestTemplateFunctions_MathOperations(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	// Test add function
	addFunc := funcMap["add"].(func(int, int) int)
	assert.Equal(t, 7, addFunc(3, 4))

	// Test sub function
	subFunc := funcMap["sub"].(func(int, int) int)
	assert.Equal(t, 1, subFunc(5, 4))

	// Test mul function
	mulFunc := funcMap["mul"].(func(int, int) int)
	assert.Equal(t, 12, mulFunc(3, 4))

	// Test div function
	divFunc := funcMap["div"].(func(int, int) int)
	assert.Equal(t, 3, divFunc(12, 4))
	assert.Equal(t, 0, divFunc(12, 0)) // Division by zero protection

	// Test mod function
	modFunc := funcMap["mod"].(func(int, int) int)
	assert.Equal(t, 1, modFunc(7, 3))
	assert.Equal(t, 0, modFunc(7, 0)) // Modulo by zero protection

	// Test max function
	maxFunc := funcMap["max"].(func(int, int) int)
	assert.Equal(t, 5, maxFunc(3, 5))
	assert.Equal(t, 5, maxFunc(5, 3))

	// Test min function
	minFunc := funcMap["min"].(func(int, int) int)
	assert.Equal(t, 3, minFunc(3, 5))
	assert.Equal(t, 3, minFunc(5, 3))
}

func TestTemplateFunctions_ComparisonOperations(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	// Test gt function
	gtFunc := funcMap["gt"].(func(int, int) bool)
	assert.True(t, gtFunc(5, 3))
	assert.False(t, gtFunc(3, 5))

	// Test lt function
	ltFunc := funcMap["lt"].(func(int, int) bool)
	assert.True(t, ltFunc(3, 5))
	assert.False(t, ltFunc(5, 3))

	// Test eq function (variadic — matches first arg against any remaining)
	eqFunc := funcMap["eq"].(func(...any) bool)
	assert.True(t, eqFunc(5, 5))
	assert.False(t, eqFunc(5, 3))
	assert.True(t, eqFunc("hello", "hello"))
	assert.True(t, eqFunc("a", "b", "a"))  // multi-arg: "a" matches third
	assert.False(t, eqFunc("a", "b", "c")) // multi-arg: no match
	assert.False(t, eqFunc(42))            // single arg: insufficient

	// Test ne function
	neFunc := funcMap["ne"].(func(any, any) bool)
	assert.True(t, neFunc(5, 3))
	assert.False(t, neFunc(5, 5))

	// Test le function
	leFunc := funcMap["le"].(func(any, any) bool)
	assert.True(t, leFunc(3, 5))
	assert.True(t, leFunc(5, 5))
	assert.False(t, leFunc(5, 3))
}

func TestTemplateFunctions_LogicalOperations(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	// Test and function
	andFunc := funcMap["and"].(func(bool, bool) bool)
	assert.True(t, andFunc(true, true))
	assert.False(t, andFunc(true, false))
	assert.False(t, andFunc(false, true))
	assert.False(t, andFunc(false, false))

	// Test or function
	orFunc := funcMap["or"].(func(bool, bool) bool)
	assert.True(t, orFunc(true, true))
	assert.True(t, orFunc(true, false))
	assert.True(t, orFunc(false, true))
	assert.False(t, orFunc(false, false))

	// Test not function
	notFunc := funcMap["not"].(func(bool) bool)
	assert.False(t, notFunc(true))
	assert.True(t, notFunc(false))
}

func TestTemplateFunctions_CollectionOperations(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	// Test len function
	lenFunc := funcMap["len"].(func(any) int)
	assert.Equal(t, 3, lenFunc([]string{"a", "b", "c"}))
	assert.Equal(t, 5, lenFunc("hello"))
	assert.Equal(t, 2, lenFunc(map[string]int{"a": 1, "b": 2}))

	// Test slice function
	sliceFunc := funcMap["slice"].(func(any, int, int) any)
	arr := []string{"a", "b", "c", "d", "e"}
	result := sliceFunc(arr, 1, 3)
	expected := []string{"b", "c"}
	assert.Equal(t, expected, result)

	// Test slice with bounds checking
	result = sliceFunc(arr, 0, 10) // End beyond array
	assert.Equal(t, arr, result)

	result = sliceFunc(arr, -1, 3) // Start before array
	expected = []string{"a", "b", "c"}
	assert.Equal(t, expected, result)
}

func TestTemplateFunctions_DateOperations(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	// Test formatDate function
	formatDateFunc := funcMap["formatDate"].(func(time.Time, string) string)
	testTime := time.Date(2023, 1, 15, 14, 30, 0, 0, time.UTC)
	result := formatDateFunc(testTime, "2006-01-02")
	assert.Equal(t, "2023-01-15", result)

	// Test formatDateInZone function
	formatDateInZoneFunc := funcMap["formatDateInZone"].(func(time.Time, string, string) string)
	result = formatDateInZoneFunc(testTime, "UTC", "2006-01-02 15:04")
	assert.Equal(t, "2023-01-15 14:30", result)

	// Test now function
	nowFunc := funcMap["now"].(func() time.Time)
	now := nowFunc()
	assert.True(t, time.Since(now) < time.Second)

	// Test timeAgo function
	timeAgoFunc := funcMap["timeAgo"].(func(time.Time) string)

	// Test recent time
	recent := time.Now().Add(-30 * time.Second)
	result = timeAgoFunc(recent)
	assert.Equal(t, "just now", result)

	// Test minutes ago
	minutesAgo := time.Now().Add(-5 * time.Minute)
	result = timeAgoFunc(minutesAgo)
	assert.Contains(t, result, "minute")

	// Test hours ago
	hoursAgo := time.Now().Add(-2 * time.Hour)
	result = timeAgoFunc(hoursAgo)
	assert.Contains(t, result, "hour")

	// Test days ago
	daysAgo := time.Now().Add(-3 * 24 * time.Hour)
	result = timeAgoFunc(daysAgo)
	assert.Contains(t, result, "day")

	// Test zero time returns empty string
	result = timeAgoFunc(time.Time{})
	assert.Equal(t, "", result)

	// Test relativeTime function
	relativeTimeFunc := funcMap["relativeTime"].(func(time.Time) string)

	// Test zero time returns empty string
	result = relativeTimeFunc(time.Time{})
	assert.Equal(t, "", result)

	// Test recent time
	result = relativeTimeFunc(time.Now().Add(-30 * time.Second))
	assert.Equal(t, "just now", result)

	// Test minutes ago
	result = relativeTimeFunc(time.Now().Add(-5 * time.Minute))
	assert.Equal(t, "5m ago", result)

	// Test hours ago
	result = relativeTimeFunc(time.Now().Add(-2 * time.Hour))
	assert.Equal(t, "2h ago", result)

	// Test days ago
	result = relativeTimeFunc(time.Now().Add(-3 * 24 * time.Hour))
	assert.Equal(t, "3d ago", result)

	// Test older dates fall back to date format
	result = relativeTimeFunc(time.Now().Add(-30 * 24 * time.Hour))
	assert.NotEmpty(t, result)
	assert.NotContains(t, result, "ago")
}

func TestTemplateFunctions_UtilityOperations(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	// Test printf function
	printfFunc := funcMap["printf"].(func(string, ...any) string)
	result := printfFunc("Hello %s, you have %d messages", "John", 5)
	assert.Equal(t, "Hello John, you have 5 messages", result)

	// Test seq function
	seqFunc := funcMap["seq"].(func(int, int) []int)
	seqResult := seqFunc(1, 5)
	expectedSeq := []int{1, 2, 3, 4, 5}
	assert.Equal(t, expectedSeq, seqResult)

	// Test seq with invalid range
	seqResult = seqFunc(5, 1)
	assert.Equal(t, []int{}, seqResult)

	// Test slugify function
	slugifyFunc := funcMap["slugify"].(func(string) string)
	result = slugifyFunc("Hello World! This is a Test")
	assert.Equal(t, "hello-world-this-is-a-test", result)

	// Test storageNamespace function
	nsFunc := funcMap["storageNamespace"].(func(string) string)
	cases := map[string]string{
		"":                              "markgo:default",
		"https://blog.example-a.com":    "markgo:blog-example-a-com",
		"https://blog.example-b.com":    "markgo:blog-example-b-com",
		"https://blog.example.com/":     "markgo:blog-example-com",
		"https://site.com/blog-a":       "markgo:site-com-blog-a",
		"http://localhost:3000":         "markgo:localhost-3000",
		"https://blog.example.com:8443": "markgo:blog-example-com-8443",
	}
	for input, want := range cases {
		assert.Equal(t, want, nsFunc(input), "input=%q", input)
	}

	// Test isNil function
	isNilFunc := funcMap["isNil"].(func(any) bool)
	assert.True(t, isNilFunc(nil))
	assert.False(t, isNilFunc("hello"))

	// Test isNotNil function
	isNotNilFunc := funcMap["isNotNil"].(func(any) bool)
	assert.False(t, isNotNilFunc(nil))
	assert.True(t, isNotNilFunc("hello"))

	// Test default function
	defaultFunc := funcMap["default"].(func(any, any) any)
	assert.Equal(t, "default", defaultFunc("default", nil))
	assert.Equal(t, "default", defaultFunc("default", ""))
	assert.Equal(t, "value", defaultFunc("default", "value"))

	// Test ternary function
	ternaryFunc := funcMap["ternary"].(func(bool, any, any) any)
	assert.Equal(t, "yes", ternaryFunc(true, "yes", "no"))
	assert.Equal(t, "no", ternaryFunc(false, "yes", "no"))
}

func TestTemplateFunctions_FormatNumber(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	formatNumberFunc := funcMap["formatNumber"].(func(any) string)

	tests := []struct {
		input    any
		expected string
	}{
		{1234, "1,234"},
		{1234567, "1,234,567"},
		{123.456, "123.456"},
		{"not a number", "not a number"},
	}

	for _, tt := range tests {
		result := formatNumberFunc(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestTemplateFunctions_TruncateHTML(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	truncateHTMLFunc := funcMap["truncateHTML"].(func(string, int) template.HTML)

	tests := []struct {
		input    string
		length   int
		expected string
	}{
		{"Hello World", 20, "Hello World"},
		{"Hello World", 5, "Hello..."},
		{"<p>Hello</p>", 10, "&lt;p&gt;Hello&lt;/p&gt;"},
	}

	for _, tt := range tests {
		result := truncateHTMLFunc(tt.input, tt.length)
		resultStr := string(result)
		if tt.length >= len([]rune(tt.input)) {
			assert.Contains(t, resultStr, tt.input)
		} else {
			assert.Contains(t, resultStr, "...")
		}
	}
}

func TestTemplateFunctions_Compare(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	compareFunc := funcMap["compare"].(func(any, any) int)

	tests := []struct {
		a, b     any
		expected int
	}{
		{5, 3, 1},
		{3, 5, -1},
		{5, 5, 0},
		{5.5, 3.2, 1},
		{"invalid", 5, 0}, // Non-numeric values
	}

	for _, tt := range tests {
		result := compareFunc(tt.a, tt.b)
		assert.Equal(t, tt.expected, result)
	}
}

func TestTemplateFunctions_Get(t *testing.T) {
	funcMap := GetTemplateFuncMap()

	getFunc := funcMap["get"].(func(map[string]any, string) any)

	testMap := map[string]any{
		"name": "John",
		"age":  30,
	}

	assert.Equal(t, "John", getFunc(testMap, "name"))
	assert.Equal(t, 30, getFunc(testMap, "age"))
	assert.Nil(t, getFunc(testMap, "nonexistent"))
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
		ok       bool
	}{
		{5, 5.0, true},
		{3.14, 3.14, true},
		{"string", 0, false},
		{nil, 0, false},
	}

	for _, tt := range tests {
		result, ok := toFloat(tt.input)
		assert.Equal(t, tt.expected, result)
		assert.Equal(t, tt.ok, ok)
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		n        int
		singular string
		plural   string
		expected string
	}{
		{1, "minute", "minutes", "1 minute ago"},
		{5, "minute", "minutes", "5 minutes ago"},
		{0, "hour", "hours", "0 hours ago"},
	}

	for _, tt := range tests {
		result := pluralize(tt.n, tt.singular, tt.plural)
		assert.Equal(t, tt.expected, result)
	}
}

func TestTemplateService_ErrorHandling(t *testing.T) {
	// Test with templates that don't exist
	cfg := &config.Config{}
	service := &TemplateService{
		config: cfg,
	}

	var buf strings.Builder
	err := service.Render(&buf, "nonexistent.html", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template not found")

	// Test RenderToString with nil templates
	_, err = service.RenderToString("test.html", nil)
	assert.Error(t, err)

	// Test HasTemplate with nil templates
	assert.False(t, service.HasTemplate("test.html"))

	// Test ListTemplates with nil templates
	templates := service.ListTemplates()
	assert.Equal(t, []string{}, templates)
}

// TestRender_PagesTemplate guards against the strict-typed-template
// silent-truncation gotcha (see project CLAUDE.md): markgo's or/not/eq
// are narrower than Go template builtins, and a misuse fails at render
// time by truncating mid-response instead of returning an error. This
// renders the real embedded pages.html block (via NewTemplateService
// fallback to web.Assets when filesystem path is empty) with empty,
// single, and many-page inputs to verify each shape completes cleanly.
func TestRender_PagesTemplate(t *testing.T) {
	tempDir := t.TempDir() // empty dir → service falls back to embedded templates
	cfg := &config.Config{
		Blog: config.BlogConfig{Title: "Test Blog"},
	}
	service, err := NewTemplateService(tempDir, cfg)
	require.NoError(t, err)
	require.True(t, service.HasTemplate("pages.html"), "embedded pages.html must be loaded")

	mkPage := func(slug, title, desc string) *models.Article {
		return &models.Article{Slug: slug, Title: title, Description: desc}
	}

	tests := []struct {
		name      string
		pageCount int
		pages     []*models.Article
		wants     []string
		notWants  []string
	}{
		{
			name:      "empty state",
			pageCount: 0,
			pages:     nil,
			wants:     []string{"No pages yet", "type: page"},
			notWants:  []string{"pages-list-item"},
		},
		{
			name:      "single page",
			pageCount: 1,
			pages:     []*models.Article{mkPage("about-us", "About Us", "Who we are")},
			wants:     []string{"About Us", "Who we are", `href="/p/about-us"`, "pages-list-item"},
			notWants:  []string{"No pages yet"},
		},
		{
			name:      "many pages without descriptions",
			pageCount: 3,
			pages: []*models.Article{
				mkPage("a", "Alpha", ""),
				mkPage("b", "Beta", ""),
				mkPage("c", "Gamma", ""),
			},
			wants:    []string{"Alpha", "Beta", "Gamma", `href="/p/a"`, `href="/p/c"`},
			notWants: []string{"pages-list-excerpt", "No pages yet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			data := map[string]any{
				"config":    cfg,
				"pageCount": tt.pageCount,
				"pages":     tt.pages,
			}
			err := service.GetTemplate().ExecuteTemplate(&buf, "pages-content", data)
			require.NoError(t, err, "pages-content template must render without error — silent truncation would NOT raise here, so check output integrity too")

			out := buf.String()
			assert.True(t, strings.HasSuffix(strings.TrimSpace(out), "</div>"),
				"render output must end with closing div, not truncate mid-template (got tail: %q)", tail(out, 80))

			for _, want := range tt.wants {
				assert.Contains(t, out, want)
			}
			for _, notWant := range tt.notWants {
				assert.NotContains(t, out, notWant)
			}
		})
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func createTestTemplateService(t *testing.T) *TemplateService {
	tempDir := t.TempDir()
	cfg := &config.Config{}

	// Create test templates
	testTemplates := map[string]string{
		"base.html": `<!DOCTYPE html>
<html>
<head>
    <title>{{.title}}</title>
</head>
<body>
    <h1>{{.title}}</h1>
    <div>{{.content}}</div>
</body>
</html>`,

		"article.html": `<article>
    <h1>{{.title}}</h1>
    <p>{{.excerpt}}</p>
    <div>{{.content | safeHTML}}</div>
</article>`,

		"list.html": `<ul>
{{range .items}}
    <li>{{.}}</li>
{{end}}
</ul>`,

		"functions.html": `
{{printf "Hello %s" .name}}
{{add 2 3}}
{{.items | len}}
{{slice .items 0 2}}
{{join ", " .tags}}
{{.text | lower}}
{{.text | upper}}
{{.date | formatDate "2006-01-02"}}
`,
	}

	for filename, content := range testTemplates {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte(content), 0o600)
		require.NoError(t, err)
	}

	service, err := NewTemplateService(tempDir, cfg)
	require.NoError(t, err)

	return service
}

func TestLoadBrandLogo_EmbeddedDefault(t *testing.T) {
	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(""))

	got := string(svc.brandLogoSVG)
	assert.Contains(t, got, `class="brand-logo"`, "embedded default must carry brand-logo class")
	assert.Contains(t, got, `viewBox="0 0 64 64"`, "embedded default must carry the canonical viewBox")
	assert.Contains(t, got, `var(--color-primary)`, "embedded default must use theme CSS variables")
}

func TestLoadBrandLogo_EmbeddedReadFails_ReturnsStartupError(t *testing.T) {
	original := brandLogoFS
	brandLogoFS = fstest.MapFS{}
	t.Cleanup(func() { brandLogoFS = original })

	svc := &TemplateService{}
	err := svc.loadBrandLogo("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedded brand-logo missing or unreadable")
}

func TestLoadBrandLogo_EmbeddedDefaultInjectsClassIfRemoved(t *testing.T) {
	// Defensive consistency: if a future refactor drops class="brand-logo" from
	// the embedded SVG, the fallback path must still inject it so CSS sizing
	// keeps working on installs without an overlay.
	classless := []byte(`<svg viewBox="0 0 64 64" xmlns="http://www.w3.org/2000/svg"/>`)
	original := brandLogoFS
	brandLogoFS = fstest.MapFS{
		"static/img/brand-logo.svg": &fstest.MapFile{Data: classless},
	}
	t.Cleanup(func() { brandLogoFS = original })

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(""))
	assert.Contains(t, string(svc.brandLogoSVG), `class="brand-logo"`,
		"embedded fallback must run through injectBrandLogoClass")
}

func TestTemplateService_FuncMapExposesBrandLogo(t *testing.T) {
	svc := &TemplateService{brandLogoSVG: template.HTML(`<svg data-test="x"/>`)}

	fm := svc.funcMap()
	fn, ok := fm["brandLogoSVG"].(func() template.HTML)
	require.True(t, ok, "brandLogoSVG must be registered as func() template.HTML")
	assert.Equal(t, template.HTML(`<svg data-test="x"/>`), fn())
}

// writeOverlayFixture creates <staticPath>/img/brand-logo.svg with content.
// Returns the staticPath dir (the parent containing img/).
func writeOverlayFixture(t *testing.T, content []byte) string {
	t.Helper()
	staticPath := t.TempDir()
	imgDir := filepath.Join(staticPath, "img")
	require.NoError(t, os.MkdirAll(imgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(imgDir, "brand-logo.svg"), content, 0o600))
	return staticPath
}

func TestLoadBrandLogo_OverlayReplaces(t *testing.T) {
	staticPath := writeOverlayFixture(t, []byte(
		`<svg id="operator-marker" class="brand-logo" xmlns="http://www.w3.org/2000/svg"/>`))

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(staticPath))

	got := string(svc.brandLogoSVG)
	assert.Contains(t, got, `id="operator-marker"`)
	assert.NotContains(t, got, `viewBox="0 0 64 64"`, "operator overlay must replace embedded default")
}

func TestLoadBrandLogo_InjectsClassIfMissing(t *testing.T) {
	staticPath := writeOverlayFixture(t, []byte(
		`<svg id="no-class" xmlns="http://www.w3.org/2000/svg"/>`))

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(staticPath))

	got := string(svc.brandLogoSVG)
	assert.Contains(t, got, `id="no-class"`)
	assert.Contains(t, got, `class="brand-logo"`, "class attribute must be injected when absent")
}

func TestLoadBrandLogo_PreservesExistingClass(t *testing.T) {
	staticPath := writeOverlayFixture(t, []byte(
		`<svg class="my-logo" xmlns="http://www.w3.org/2000/svg"/>`))

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(staticPath))

	got := string(svc.brandLogoSVG)
	assert.Contains(t, got, `class="my-logo"`)
	assert.NotContains(t, got, `class="brand-logo"`, "must not inject when operator chose another class")
}

func TestLoadBrandLogo_FallsBackOnMalformedXML(t *testing.T) {
	staticPath := writeOverlayFixture(t, []byte(`<not-xml<<`))

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(staticPath))

	// Falls back to embedded default — canonical viewBox is the marker.
	assert.Contains(t, string(svc.brandLogoSVG), `viewBox="0 0 64 64"`)
}

func TestLoadBrandLogo_FallsBackOnOversize(t *testing.T) {
	// Build a 33 KiB SVG that's structurally valid: <svg> with padding inside.
	padding := strings.Repeat("x", brandLogoMaxBytes)
	oversize := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><desc>` + padding + `</desc></svg>`)
	staticPath := writeOverlayFixture(t, oversize)

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(staticPath))

	assert.Contains(t, string(svc.brandLogoSVG), `viewBox="0 0 64 64"`)
}

func TestLoadBrandLogo_FallsBackOnWrongRoot(t *testing.T) {
	staticPath := writeOverlayFixture(t, []byte(
		`<?xml version="1.0"?><html><body/></html>`))

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(staticPath))

	assert.Contains(t, string(svc.brandLogoSVG), `viewBox="0 0 64 64"`)
}

func TestLoadBrandLogo_OverlayAbsentIsSilent(t *testing.T) {
	// staticPath set but no brand-logo.svg under img/.
	staticPath := t.TempDir()

	svc := &TemplateService{}
	require.NoError(t, svc.loadBrandLogo(staticPath))

	// Falls back to embedded silently.
	assert.Contains(t, string(svc.brandLogoSVG), `viewBox="0 0 64 64"`)
}

func TestInjectBrandLogoClass_HandlesQuotedAttrs(t *testing.T) {
	// id value contains the literal text "class=" which must not fool the scanner.
	in := []byte(`<svg id="contains class=foo"/>`)
	out := injectBrandLogoClass(in)
	assert.Equal(t, `<svg class="brand-logo" id="contains class=foo"/>`, string(out))
}

func TestInjectBrandLogoClass_HandlesSingleQuotes(t *testing.T) {
	in := []byte(`<svg class='existing-logo'/>`)
	out := injectBrandLogoClass(in)
	assert.Equal(t, string(in), string(out), "must preserve operator's single-quoted class")
}

func TestInjectBrandLogoClass_HandlesNewlineInTag(t *testing.T) {
	in := []byte("<svg\n  width=\"28\"\n  height=\"28\"\n/>")
	out := injectBrandLogoClass(in)
	assert.Contains(t, string(out), `class="brand-logo"`)
	assert.Contains(t, string(out), `width="28"`)
	assert.Contains(t, string(out), `height="28"`)
}

func TestInjectBrandLogoClass_NoSvgIsPassThrough(t *testing.T) {
	in := []byte(`<not-svg/>`)
	assert.Equal(t, string(in), string(injectBrandLogoClass(in)))
}

// TestBrandLogo_RendersThroughTemplateService is the Phase 2 integration test:
// drives the full overlay → validation → injection → template render path
// without needing the full base.html data shape.
func TestBrandLogo_RendersThroughTemplateService(t *testing.T) {
	operator := []byte(
		`<svg id="operator-marker" xmlns="http://www.w3.org/2000/svg"><circle cx="16" cy="16" r="8"/></svg>`)
	staticPath := writeOverlayFixture(t, operator)

	templatesDir := t.TempDir()
	stub := `<!DOCTYPE html><html><body>{{ brandLogoSVG }}</body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "stub.html"), []byte(stub), 0o600))

	cfg := &config.Config{StaticPath: staticPath}
	svc, err := NewTemplateService(templatesDir, cfg)
	require.NoError(t, err)

	var buf strings.Builder
	require.NoError(t, svc.Render(&buf, "stub.html", nil))
	out := buf.String()
	assert.Contains(t, out, `id="operator-marker"`, "rendered HTML must contain operator SVG")
	assert.Contains(t, out, `class="brand-logo"`, "class must be injected end-to-end")
}

// TestComposeTemplate_SaveWarningHiddenByDefault pins the #102 server-side
// contract: the autosave warning element must always be rendered with the
// `hidden` attribute. The only path to visible-on-screen is compose.js
// flipping it after a localStorage.setItem failure. Without `hidden`, CSS
// does not suppress display and the operator sees the warning at initial
// load. If this test breaks, a future template change has removed the
// guard.
func TestComposeTemplate_SaveWarningHiddenByDefault(t *testing.T) {
	body, err := web.Assets.ReadFile("templates/compose.html")
	require.NoError(t, err, "embedded compose.html must be readable")

	pattern := regexp.MustCompile(`<div\b[^>]*\bid="compose-save-warning"[^>]*\bhidden\b[^>]*>`)
	assert.Regexp(t, pattern, string(body),
		"compose-save-warning must carry the hidden attribute at initial render")
}

// TestTemplateFunctions_Permalink locks the card permalink wiring: the
// `permalink` func must delegate to article.CanonicalURLFor so card templates
// never hardcode the URL shape (CLAUDE.md canonical-URL rule).
func TestTemplateFunctions_Permalink(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	permalink, ok := funcMap["permalink"].(func(*models.Article) string)
	require.True(t, ok, "permalink func must be registered with the expected signature")

	assert.Equal(t, "/writing/hello-world", permalink(&models.Article{Slug: "hello-world", Type: "article"}))
	assert.Equal(t, "/writing/a-thought", permalink(&models.Article{Slug: "a-thought", Type: "thought"}))
	assert.Equal(t, "/p/colophon", permalink(&models.Article{Slug: "colophon", Type: "page"}))
	assert.Equal(t, "/about", permalink(&models.Article{Slug: "about"}))
}

// TestTemplateService_CardMeta pins the v3.19.0 unified card-navigation model:
// every feed card type renders one VISIBLE permalink to its detail post, with no
// whole-card overlay (issue #105). It also guards the strict-typed or/not/eq
// usage in the card-meta define against the silent render-truncation failure mode.
func TestTemplateService_CardMeta(t *testing.T) {
	cfg := &config.Config{}
	svc, err := NewTemplateService("/nonexistent/path", cfg) // falls back to embedded templates
	require.NoError(t, err)

	cases := []struct {
		typ          string
		slug         string
		wantHref     string
		wantContains string // type-specific marker
		wantArrow    bool   // visible arrow cue only on title-less cards
	}{
		{"thought", "a-thought", "/writing/a-thought", `View thought`, true},
		{"ama", "an-ama", "/writing/an-ama", `View question`, true},
		{"link", "a-link", "/writing/a-link", `View post`, false},
		{"article", "an-article", "/writing/an-article", `min read`, false},
	}

	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			a := &models.Article{
				Slug:        c.slug,
				Type:        c.typ,
				Date:        time.Now(),
				ReadingTime: 5,
				Tags:        []string{"go"},
			}
			var buf strings.Builder
			require.NoError(t, svc.Render(&buf, "card-meta", a)) // strict-FuncMap render must not fail
			out := buf.String()

			assert.Contains(t, out, `class="feed-card-permalink"`)
			assert.Contains(t, out, `href="`+c.wantHref+`"`)
			assert.Contains(t, out, c.wantContains)
			assert.Contains(t, out, `class="feed-tag"`, "tags render as real links, lifted out from under any overlay")
			assert.NotContains(t, out, "feed-card--clickable", "no whole-card overlay in Model B")
			if c.wantArrow {
				assert.Contains(t, out, "feed-card-permalink-arrow", "title-less cards carry the arrow cue")
			} else {
				assert.NotContains(t, out, "feed-card-permalink-arrow", "titled cards omit the arrow cue (title is the door)")
			}
		})
	}
}
