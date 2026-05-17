package services

import (
	"encoding/json"
	"html/template"
	"strings"
	"testing"
)

const (
	testArticleURL = "https://example.com/writing/test"
)

func TestGenerateJsonLD(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	generateJSONLD, exists := funcMap["generateJsonLD"]
	if !exists {
		t.Fatal("generateJSONLD function not found in template function map")
	}

	fn := generateJSONLD.(func(map[string]interface{}) template.HTML)

	testData := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Article",
		"headline": "Test Article",
		"author": map[string]interface{}{
			"@type": "Person",
			"name":  "Test Author",
		},
	}

	result := fn(testData)
	htmlResult := string(result)

	if !strings.Contains(htmlResult, `<script type="application/ld+json">`) {
		t.Error("Result should contain script tag")
	}

	if !strings.Contains(htmlResult, `"@context":"https://schema.org"`) {
		t.Error("Result should contain JSON-LD data")
	}

	if !strings.Contains(htmlResult, `"headline":"Test Article"`) {
		t.Error("Result should contain article headline")
	}

	if !strings.Contains(htmlResult, `</script>`) {
		t.Error("Result should close script tag")
	}
}

func TestRenderMetaTags(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	renderMetaTags, exists := funcMap["renderMetaTags"]
	if !exists {
		t.Fatal("renderMetaTags function not found in template function map")
	}

	fn := renderMetaTags.(func(map[string]string) template.HTML)

	testTags := map[string]string{
		"title":        "Test Article",
		"description":  "This is a test article description",
		"og:title":     "Test Article",
		"og:type":      "article",
		"twitter:card": "summary_large_image",
		"canonical":    "https://example.com/test-article",
	}

	result := fn(testTags)
	htmlResult := string(result)

	// Check meta tags
	if !strings.Contains(htmlResult, `<meta name="title" content="Test Article">`) {
		t.Error("Should contain title meta tag")
	}

	if !strings.Contains(htmlResult, `<meta name="description" content="This is a test article description">`) {
		t.Error("Should contain description meta tag")
	}

	// Check Open Graph tags (use property)
	if !strings.Contains(htmlResult, `<meta property="og:title" content="Test Article">`) {
		t.Error("Should contain og:title property tag")
	}

	if !strings.Contains(htmlResult, `<meta property="og:type" content="article">`) {
		t.Error("Should contain og:type property tag")
	}

	// Check Twitter tags (use property)
	if !strings.Contains(htmlResult, `<meta property="twitter:card" content="summary_large_image">`) {
		t.Error("Should contain twitter:card property tag")
	}

	// Check canonical link
	if !strings.Contains(htmlResult, `<link rel="canonical" href="https://example.com/test-article">`) {
		t.Error("Should contain canonical link")
	}
}

// TestRenderMetaTags_MultiTagEmitsMultipleProperties verifies that the
// "article:tag:<index>" encoding produces one <meta property="article:tag">
// line per tag. Regression guard for the pre-v3.10.3 bug where the loop
// overwrote a single map key, leaving only the last tag in output.
func TestRenderMetaTags_MultiTagEmitsMultipleProperties(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	fn := funcMap["renderMetaTags"].(func(map[string]string) template.HTML)

	cases := []struct {
		name    string
		tags    map[string]string
		wantSub []string
	}{
		{
			name: "three tags emit three article:tag lines",
			tags: map[string]string{
				"article:tag:0": "go",
				"article:tag:1": "blog",
				"article:tag:2": "web",
			},
			wantSub: []string{
				`<meta property="article:tag" content="go">`,
				`<meta property="article:tag" content="blog">`,
				`<meta property="article:tag" content="web">`,
			},
		},
		{
			name: "single tag emits one line with canonical property name",
			tags: map[string]string{
				"article:tag:0": "solo",
			},
			wantSub: []string{
				`<meta property="article:tag" content="solo">`,
			},
		},
		{
			name:    "zero tags emit nothing",
			tags:    map[string]string{},
			wantSub: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(fn(tc.tags))
			for _, want := range tc.wantSub {
				if !strings.Contains(out, want) {
					t.Errorf("want output to contain %q\ngot: %s", want, out)
				}
			}
			// Negative: no encoded key (article:tag:0) should leak into output.
			if strings.Contains(out, "article:tag:0") {
				t.Errorf("encoded suffix leaked into output: %s", out)
			}
		})
	}
}

func TestSeoExcerpt(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	seoExcerpt, exists := funcMap["seoExcerpt"]
	if !exists {
		t.Fatal("seoExcerpt function not found in template function map")
	}

	fn := seoExcerpt.(func(string, int) string)

	content := "# Test Article\n\nThis is a **test** article with *markdown* formatting and `code` snippets. It has [links](http://example.com) and ![images](image.jpg)."
	maxLength := 100

	result := fn(content, maxLength)

	// Should remove markdown formatting
	if strings.Contains(result, "#") || strings.Contains(result, "*") || strings.Contains(result, "`") {
		t.Error("Result should not contain markdown formatting")
	}

	if strings.Contains(result, "[") || strings.Contains(result, "]") || strings.Contains(result, "(") || strings.Contains(result, ")") {
		t.Error("Result should not contain link/image markdown")
	}

	if len(result) > maxLength+3 { // +3 for "..."
		t.Errorf("Result should not exceed max length, got %d characters", len(result))
	}

	if !strings.Contains(result, "This is a test article") {
		t.Error("Result should contain main content")
	}
}

func TestReadingTime(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	readingTime, exists := funcMap["readingTime"]
	if !exists {
		t.Fatal("readingTime function not found in template function map")
	}

	fn := readingTime.(func(string, int) int)

	// Test with 400 words at 200 WPM
	words := make([]string, 400)
	for i := range words {
		words[i] = "word"
	}
	content := strings.Join(words, " ")

	result := fn(content, 200)
	expected := 2 // 400 words / 200 WPM = 2 minutes

	if result != expected {
		t.Errorf("Expected %d minutes, got %d", expected, result)
	}

	// Test minimum 1 minute
	shortContent := "Just a few words here"
	result = fn(shortContent, 200)
	if result != 1 {
		t.Errorf("Expected minimum 1 minute for short content, got %d", result)
	}

	// Test default WPM
	result = fn(content, 0) // 0 should use default 200 WPM
	if result != 2 {
		t.Errorf("Expected 2 minutes with default WPM, got %d", result)
	}
}

func TestBuildURL(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	buildURL, exists := funcMap["buildURL"]
	if !exists {
		t.Fatal("buildURL function not found in template function map")
	}

	fn := buildURL.(func(string, string) string)

	// Test relative path
	result := fn("https://example.com", "/writing/test")
	expected := testArticleURL
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	// Test absolute URL (should return as-is)
	result = fn("https://example.com", "https://other.com/page")
	expected = "https://other.com/page"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	// Test path without leading slash
	result = fn("https://example.com", "writing/test")
	expected = testArticleURL
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	// Test base URL with trailing slash
	result = fn("https://example.com/", "/writing/test")
	expected = testArticleURL
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestJSONLDValidation(t *testing.T) {
	funcMap := GetTemplateFuncMap()
	generateJSONLD := funcMap["generateJsonLD"].(func(map[string]interface{}) template.HTML)

	// Test complex nested structure
	complexData := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Article",
		"headline": "Complex Test Article",
		"author": map[string]interface{}{
			"@type": "Person",
			"name":  "Test Author",
		},
		"publisher": map[string]interface{}{
			"@type": "Organization",
			"name":  "Test Blog",
			"logo": map[string]interface{}{
				"@type": "ImageObject",
				"url":   "https://example.com/logo.png",
			},
		},
		"mainEntityOfPage": map[string]interface{}{
			"@type": "WebPage",
			"@id":   testArticleURL,
		},
		"image": []string{
			"https://example.com/image1.jpg",
			"https://example.com/image2.jpg",
		},
		"datePublished": "2023-01-01T12:00:00Z",
		"dateModified":  "2023-01-02T12:00:00Z",
	}

	result := generateJSONLD(complexData)
	htmlResult := string(result)

	// Extract JSON from script tag
	start := strings.Index(htmlResult, ">") + 1
	end := strings.LastIndex(htmlResult, "<")
	jsonStr := htmlResult[start:end]

	// Validate JSON structure
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Generated JSON-LD is invalid: %v", err)
	}

	// Verify key fields
	if parsed["@context"] != "https://schema.org" {
		t.Error("@context should be preserved")
	}

	if parsed["@type"] != "Article" {
		t.Error("@type should be preserved")
	}

	if parsed["headline"] != "Complex Test Article" {
		t.Error("headline should be preserved")
	}

	// Verify nested objects
	author, ok := parsed["author"].(map[string]interface{})
	if !ok {
		t.Fatal("author should be an object")
	}

	if author["@type"] != "Person" {
		t.Error("author @type should be preserved")
	}

	if author["name"] != "Test Author" {
		t.Error("author name should be preserved")
	}
}
