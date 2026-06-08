package new

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeForYAML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "Hello World", "Hello World"},
		{"double quotes", `My Post: "A Guide"`, `My Post: \"A Guide\"`},
		{"backslash", `path\to\file`, `path\\to\\file`},
		{"backslash then quote", `say \"hello\"`, `say \\\"hello\\\"`},
		{"colon safe in double quotes", "My Post: A Guide", "My Post: A Guide"},
		{"hash safe in double quotes", "Issue #42", "Issue #42"},
		{"empty string", "", ""},
		{"only quotes", `"""`, `\"\"\"`},
		{"only backslashes", `\\\`, `\\\\\\`},
		{"newline", "line1\nline2", `line1\nline2`},
		{"carriage return", "line1\rline2", `line1\rline2`},
		{"crlf", "line1\r\nline2", `line1\r\nline2`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForYAML(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateArticleInput(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		description string
		tags        string
		category    string
		author      string
		template    string
		wantValid   bool
		wantFields  []string // fields that should have errors
	}{
		{
			name:      "valid input",
			title:     "My Great Article",
			tags:      "go, testing",
			category:  "tech",
			author:    "Jane Doe",
			wantValid: true,
		},
		{
			name:       "empty title",
			title:      "",
			tags:       "go",
			category:   "tech",
			author:     "Jane",
			wantValid:  false,
			wantFields: []string{"title"},
		},
		{
			name:       "title too short",
			title:      "Hi",
			tags:       "go",
			category:   "tech",
			author:     "Jane",
			wantValid:  false,
			wantFields: []string{"title"},
		},
		{
			name:       "title too long",
			title:      strings.Repeat("x", 201),
			tags:       "go",
			category:   "tech",
			author:     "Jane",
			wantValid:  false,
			wantFields: []string{"title"},
		},
		{
			name:       "empty tags",
			title:      "Good Title",
			tags:       "",
			category:   "tech",
			author:     "Jane",
			wantValid:  false,
			wantFields: []string{"tags"},
		},
		{
			name:       "too many tags",
			title:      "Good Title",
			tags:       "a,b,c,d,e,f,g,h,i,j,k",
			category:   "tech",
			author:     "Jane",
			wantValid:  false,
			wantFields: []string{"tags"},
		},
		{
			name:       "empty category",
			title:      "Good Title",
			tags:       "go",
			category:   "",
			author:     "Jane",
			wantValid:  false,
			wantFields: []string{"category"},
		},
		{
			name:       "empty author",
			title:      "Good Title",
			tags:       "go",
			category:   "tech",
			author:     "",
			wantValid:  false,
			wantFields: []string{"author"},
		},
		{
			name:       "author too short",
			title:      "Good Title",
			tags:       "go",
			category:   "tech",
			author:     "J",
			wantValid:  false,
			wantFields: []string{"author"},
		},
		{
			name:       "invalid template",
			title:      "Good Title",
			tags:       "go",
			category:   "tech",
			author:     "Jane",
			template:   "nonexistent",
			wantValid:  false,
			wantFields: []string{"template"},
		},
		{
			name:      "valid template",
			title:     "Good Title",
			tags:      "go",
			category:  "tech",
			author:    "Jane",
			template:  "tutorial",
			wantValid: true,
		},
		{
			name:      "empty template uses default",
			title:     "Good Title",
			tags:      "go",
			category:  "tech",
			author:    "Jane",
			template:  "",
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArticleInput(tt.title, tt.description, tt.tags, tt.category, tt.author, tt.template)
			assert.Equal(t, tt.wantValid, result.Valid, "Valid mismatch")

			if !tt.wantValid {
				require.NotEmpty(t, result.Errors, "expected validation errors")
				errorFields := make(map[string]bool)
				for _, e := range result.Errors {
					errorFields[e.Field] = true
				}
				for _, field := range tt.wantFields {
					assert.True(t, errorFields[field], "expected error for field %q", field)
				}
			}
		})
	}
}

// Slug validation moved to internal/slug (see slug.TestValidate, the union of
// this command's and the compose form's former strict validators).

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"trim spaces", "  hello  ", "hello"},
		{"collapse multiple spaces", "hello    world", "hello world"},
		{"tabs and newlines", "hello\t\n  world", "hello world"},
		{"already clean", "hello world", "hello world"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeInput(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateOutputPath(t *testing.T) {
	t.Run("new file in temp dir", func(t *testing.T) {
		dir := t.TempDir()
		err := ValidateOutputPath(dir + "/new-article.md")
		assert.NoError(t, err)
	})

	t.Run("file already exists", func(t *testing.T) {
		dir := t.TempDir()
		path := dir + "/existing.md"
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o600))

		err := ValidateOutputPath(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}
