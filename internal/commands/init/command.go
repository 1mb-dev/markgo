// Package init provides the blog initialization command for MarkGo.
package init

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/1mb-dev/markgo/internal/constants"
	apperrors "github.com/1mb-dev/markgo/internal/errors"
)

const (
	envDevelopment = "development"
)

type BlogConfig struct {
	Title       string
	Description string
	Author      string
	Email       string
	Domain      string
	Port        string
	Environment string
}

// Run executes the init command
func Run(args []string) {
	// Create a new flag set for this command
	fs := flag.NewFlagSet("init", flag.ExitOnError)

	help := fs.Bool("help", false, "Show help message")
	interactive := fs.Bool("interactive", true, "Run in interactive mode")
	quick := fs.Bool("quick", false, "Quick setup with defaults")
	dir := fs.String("dir", ".", "Directory to initialize blog in")
	force := fs.Bool("force", false, "Force overwrite existing files")

	cleanup := func() {
		// Cleanup function for error handling
	}

	if err := fs.Parse(args[1:]); err != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("flag parsing", "Failed to parse command flags", err, 1),
			cleanup,
		)
	}

	if *help {
		showHelp()
		return
	}

	fmt.Printf("🚀 MarkGo Blog Engine %s - Quick Setup\n", constants.AppVersion)
	fmt.Println("=====================================")
	fmt.Println()

	// Validate target directory
	targetDir, err := filepath.Abs(*dir)
	if err != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("directory validation", fmt.Sprintf("Invalid directory path: %s", *dir), err, 1),
			cleanup,
		)
		return
	}

	// Check if directory exists and is writable
	if err := validateDirectory(targetDir); err != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("directory access", fmt.Sprintf("Cannot access directory: %s", targetDir), err, 1),
			cleanup,
		)
		return
	}

	// Check if already initialized
	if !*force && isAlreadyInitialized(targetDir) {
		fmt.Printf("❌ MarkGo blog already exists in %s\n", targetDir)
		fmt.Println("   Use --force flag to overwrite existing files")
		return
	}

	var config BlogConfig

	switch {
	case *quick:
		config = getQuickDefaults()
		fmt.Println("⚡ Quick setup mode - using sensible defaults")
	case *interactive:
		config = runInteractiveSetup()
	default:
		config = getQuickDefaults()
	}

	// Create blog structure
	if err := createBlogStructure(targetDir, &config); err != nil {
		apperrors.HandleCLIError(
			apperrors.NewCLIError("blog creation", "Failed to create blog structure", err, 1),
			cleanup,
		)
		return
	}

	// Show success message with next steps
	showSuccessMessage(targetDir, &config)
}

func validateDirectory(dir string) error {
	// Check if directory exists or can be created
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o750)
	}

	// Check if writable
	testFile := filepath.Join(dir, ".markgo-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	_ = os.Remove(testFile)

	return nil
}

func isAlreadyInitialized(dir string) bool {
	markers := []string{".env", "config.yaml", "articles"}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func getQuickDefaults() BlogConfig {
	currentUser, err := user.Current()
	username := "Blog Author"
	email := ""

	if err == nil && currentUser != nil {
		username = currentUser.Username
		if currentUser.Name != "" {
			username = currentUser.Name
		}
	}

	return BlogConfig{
		Title:       "My MarkGo Blog",
		Description: "A fast, developer-friendly blog powered by MarkGo",
		Author:      username,
		Email:       email,
		Domain:      "localhost:3000",
		Port:        "3000",
		Environment: envDevelopment,
	}
}

func runInteractiveSetup() BlogConfig {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("📝 Let's set up your new blog! (Press Enter for defaults)")
	fmt.Println()

	defaults := getQuickDefaults()

	config := BlogConfig{
		Title:       getInput(reader, "Blog title", defaults.Title),
		Description: getInput(reader, "Blog description", defaults.Description),
		Author:      getInput(reader, "Author name", defaults.Author),
		Email:       getInput(reader, "Author email", defaults.Email),
		Domain:      getInput(reader, "Domain (for production)", "yourdomain.com"),
		Port:        getInput(reader, "Development port", defaults.Port),
		Environment: getEnvironmentInput(reader),
	}

	fmt.Println()
	fmt.Println("📋 Configuration Summary:")
	fmt.Printf("   Title: %s\n", config.Title)
	fmt.Printf("   Author: %s <%s>\n", config.Author, config.Email)
	fmt.Printf("   Domain: %s\n", config.Domain)
	fmt.Printf("   Environment: %s\n", config.Environment)
	fmt.Println()

	if !getConfirmation(reader, "Proceed with this configuration?") {
		fmt.Println("Setup canceled.")
		os.Exit(0)
	}

	return config
}

func getInput(reader *bufio.Reader, prompt, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}

	return input
}

func getEnvironmentInput(reader *bufio.Reader) string {
	fmt.Println("Environment:")
	fmt.Println("  1. development (recommended for local development)")
	fmt.Println("  2. production (for live deployment)")
	fmt.Printf("Choose environment [1]: ")

	input, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(input) == "" || strings.TrimSpace(input) == "1" {
		return envDevelopment
	}

	if strings.TrimSpace(input) == "2" {
		return "production"
	}

	return envDevelopment
}

func getConfirmation(reader *bufio.Reader, prompt string) bool {
	fmt.Printf("%s (y/N): ", prompt)

	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes"
}

func createBlogStructure(dir string, config *BlogConfig) error {
	// Only create articles/ — web assets are embedded in the binary
	if err := os.MkdirAll(filepath.Join(dir, "articles"), 0o750); err != nil {
		return fmt.Errorf("failed to create articles directory: %w", err)
	}

	if err := createEnvFile(dir, config); err != nil {
		return fmt.Errorf("failed to create .env file: %w", err)
	}

	if err := createSampleArticles(dir, config); err != nil {
		return fmt.Errorf("failed to create sample articles: %w", err)
	}

	if err := createReadme(dir, config); err != nil {
		return fmt.Errorf("failed to create README: %w", err)
	}

	if err := createGitignore(dir); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	return nil
}

func createEnvFile(dir string, config *BlogConfig) error {
	baseURL := "http://localhost:" + config.Port
	if config.Environment == "production" {
		baseURL = "https://" + config.Domain
	}

	content := fmt.Sprintf(`# MarkGo Blog Configuration
# Generated by markgo init on %s

# Environment Configuration
ENVIRONMENT=%s
PORT=%s
BASE_URL=%s

# Blog Information
BLOG_TITLE=%s
BLOG_TAGLINE=
BLOG_DESCRIPTION=%s
BLOG_AUTHOR=%s
BLOG_AUTHOR_EMAIL=%s
BLOG_LANGUAGE=en
BLOG_THEME=default
BLOG_STYLE=minimal
BLOG_POSTS_PER_PAGE=10

# Paths (optional — binary has embedded web assets)
ARTICLES_PATH=./articles

# Upload Configuration
UPLOAD_PATH=./uploads
UPLOAD_MAX_SIZE=10485760

# About Page (optional)
ABOUT_TAGLINE=
ABOUT_LOCATION=

# Cache Configuration
CACHE_TTL=1h
CACHE_MAX_SIZE=1000
CACHE_CLEANUP_INTERVAL=10m

# Rate Limiting
RATE_LIMIT_GENERAL_REQUESTS=100
RATE_LIMIT_GENERAL_WINDOW=900s
RATE_LIMIT_CONTACT_REQUESTS=5
RATE_LIMIT_CONTACT_WINDOW=3600s

# CORS Configuration
CORS_ALLOWED_ORIGINS=%s
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOWED_HEADERS=Origin,Content-Type,Accept,Authorization

# Server Timeouts
SERVER_READ_TIMEOUT=15s
SERVER_WRITE_TIMEOUT=15s
SERVER_IDLE_TIMEOUT=60s

# Logging Configuration
LOG_LEVEL=info
LOG_FORMAT=json
LOG_OUTPUT=stdout

# Email Configuration (Contact Form)
# Configure these to enable contact form functionality
EMAIL_HOST=
EMAIL_PORT=587
EMAIL_USERNAME=
EMAIL_PASSWORD=
EMAIL_FROM=noreply@%s
EMAIL_TO=%s
EMAIL_USE_SSL=true

# Admin Interface (Optional)
# Set credentials to enable compose form and admin panel
ADMIN_USERNAME=
ADMIN_PASSWORD=

`,
		time.Now().Format("2006-01-02 15:04:05"),
		config.Environment,
		config.Port,
		baseURL,
		config.Title,
		config.Description,
		config.Author,
		config.Email,
		baseURL,
		config.Domain,
		config.Email,
	)

	return os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600)
}

func createSampleArticles(dir string, config *BlogConfig) error {
	articlesDir := filepath.Join(dir, "articles")

	// Welcome article (type: article — has title, long-form)
	welcomeContent := "---\n" +
		"title: \"Welcome to " + config.Title + "\"\n" +
		"description: \"Getting started with your new MarkGo blog\"\n" +
		"author: \"" + config.Author + "\"\n" +
		"date: " + time.Now().Format("2006-01-02T15:04:05Z07:00") + "\n" +
		"tags: [\"welcome\", \"getting-started\"]\n" +
		"category: \"general\"\n" +
		"draft: false\n" +
		"featured: true\n" +
		"---\n\n" +
		"# Welcome to Your New MarkGo Blog!\n\n" +
		"Congratulations on setting up your new blog! MarkGo is a single-binary blogging companion — SPA navigation, offline support, quick capture from your phone.\n\n" +
		"## Three Content Types\n\n" +
		"You never pick a type. Just write, and MarkGo figures it out:\n\n" +
		"- **Article** — has a title, intended for long-form writing\n" +
		"- **Thought** — no title, under 100 words, a quick note\n" +
		"- **Link** — has a `link_url`, sharing something you found\n\n" +
		"## Quick Start\n\n" +
		"1. Edit this article: `articles/welcome.md`\n" +
		"2. Create new content: `markgo new --title \"Hello\"`\n" +
		"3. Or use the compose form in your browser\n\n" +
		"## Writing from the Browser\n\n" +
		"Set admin credentials in `.env` to enable the compose form:\n\n" +
		"```bash\n" +
		"ADMIN_USERNAME=you\n" +
		"ADMIN_PASSWORD=something-strong\n" +
		"```\n\n" +
		"Then restart the server. You'll see a floating action button — tap it, type a thought, hit Publish.\n\n" +
		"## Next Steps\n\n" +
		"- Customize settings in `.env`\n" +
		"- Set up email for the contact form\n" +
		"- Install as a PWA on your phone\n\n" +
		"Happy blogging!\n\n" +
		"---\n\n" +
		"*Generated by `markgo init`. Edit or delete freely.*"

	if err := os.WriteFile(filepath.Join(articlesDir, "welcome.md"), []byte(welcomeContent), 0o600); err != nil {
		return err
	}

	// Thought (type: thought — no title, short, <100 words, demonstrates auto-inference)
	thoughtContent := "---\n" +
		"author: \"" + config.Author + "\"\n" +
		"date: " + time.Now().Add(-1*time.Hour).Format("2006-01-02T15:04:05Z07:00") + "\n" +
		"tags: [\"meta\"]\n" +
		"draft: false\n" +
		"---\n\n" +
		"Just set up a new blog with MarkGo. No title on this one — it's a thought. Under 100 words, no title field, and MarkGo infers the type automatically. Nice."

	if err := os.WriteFile(filepath.Join(articlesDir, "first-thought.md"), []byte(thoughtContent), 0o600); err != nil {
		return err
	}

	// Link post (type: link — has link_url, demonstrates link type)
	linkContent := "---\n" +
		"author: \"" + config.Author + "\"\n" +
		"date: " + time.Now().Add(-2*time.Hour).Format("2006-01-02T15:04:05Z07:00") + "\n" +
		"link_url: \"https://github.com/1mb-dev/markgo\"\n" +
		"tags: [\"markgo\", \"open-source\"]\n" +
		"draft: false\n" +
		"---\n\n" +
		"The MarkGo source code. Single Go binary, markdown files, SPA navigation, PWA with offline support. No database, no JS build step."

	return os.WriteFile(filepath.Join(articlesDir, "markgo-source.md"), []byte(linkContent), 0o600)
}

func createReadme(dir string, config *BlogConfig) error {
	readme := "# " + config.Title + "\n\n" +
		config.Description + "\n\n" +
		"Powered by [MarkGo](https://github.com/1mb-dev/markgo) — a single-binary blogging companion.\n\n" +
		"## Quick Start\n\n" +
		"```bash\n" +
		"markgo serve\n" +
		"# Visit http://localhost:" + config.Port + "\n" +
		"```\n\n" +
		"## Writing\n\n" +
		"```bash\n" +
		"markgo new --title \"My Post\" --tags \"tech,go\"\n" +
		"markgo new --type thought\n" +
		"markgo new --type link\n" +
		"```\n\n" +
		"Or use the compose form in your browser (set ADMIN_USERNAME/PASSWORD in `.env`).\n\n" +
		"## Structure\n\n" +
		"- `articles/` — Your content (markdown with YAML frontmatter)\n" +
		"- `.env` — Configuration\n\n" +
		"Web assets (templates, CSS, JS) are embedded in the MarkGo binary.\n" +
		"To customize, create a `web/` directory — filesystem paths take precedence.\n\n" +
		"## Features\n\n" +
		"SPA navigation, PWA with offline support, quick capture, three content types (article/thought/link), " +
		"RSS and JSON feeds, full-text search, SEO, themes, admin panel.\n\n" +
		"## Author\n\n" +
		"**" + config.Author + "**  \n" +
		"Email: " + config.Email + "\n\n" +
		"---\n\n" +
		"*Generated by MarkGo " + constants.AppVersion + "*"

	return os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o600)
}

func createGitignore(dir string) error {
	gitignore := `# Binaries
markgo
markgo.exe
build/
dist/

# Environment files
.env.local
.env.production

# Logs
*.log
logs/

# Cache and temporary files
cache/
tmp/
.tmp/

# Editor and IDE files
.vscode/
.idea/
*.swp
*.swo
*~

# OS generated files
.DS_Store
.DS_Store?
._*
.Spotlight-V100
.Trashes
ehthumbs.db
Thumbs.db

# Coverage reports
coverage.out
coverage.html

# Test files
test-results/
stress-test-results.json

# Backup files
*.backup
backups/

# Development artifacts
profiles/
debug/
*.pprof
trace.out

# Dependencies (if vendoring)
vendor/

# Local configuration overrides
config.local.yaml
.env.development.local
`

	return os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0o600)
}

func showSuccessMessage(dir string, config *BlogConfig) {
	fmt.Println("🎉 MarkGo blog created successfully!")
	fmt.Println()
	fmt.Printf("📁 Location: %s\n", dir)
	fmt.Printf("🌟 Blog: %s\n", config.Title)
	fmt.Printf("👤 Author: %s\n", config.Author)
	fmt.Println()

	fmt.Println("📋 What was created:")
	fmt.Println("   ✅ Configuration file (.env)")
	fmt.Println("   ✅ Content directory (articles/)")
	fmt.Println("   ✅ Sample content — article, thought, and link")
	fmt.Println("   ✅ README.md and .gitignore")
	fmt.Println()

	fmt.Println("🚀 Next steps:")
	if dir != "." {
		fmt.Printf("   1. cd %s\n", dir)
	}
	fmt.Println("   2. markgo serve        # Start your blog server")
	fmt.Printf("   3. Open http://localhost:%s\n", config.Port)
	fmt.Println("   4. markgo new          # Create new content")
	fmt.Println()

	fmt.Println("💡 Tips:")
	fmt.Println("   • Set ADMIN_USERNAME/PASSWORD in .env to enable the compose form")
	fmt.Println("   • Templates and CSS are embedded — override with a web/ directory")
	fmt.Println("   • Install as a PWA from your phone's browser")
	fmt.Println()

	fmt.Println("Happy blogging! 🎊")
}

func showHelp() {
	fmt.Printf("markgo init - Initialize a new MarkGo blog\n")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  markgo init [OPTIONS]")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  --dir DIR         Directory to initialize blog in (default: current directory)")
	fmt.Println("  --quick          Quick setup with sensible defaults")
	fmt.Println("  --interactive    Run interactive setup (default)")
	fmt.Println("  --force          Force overwrite existing files")
	fmt.Println("  --help           Show this help message")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  markgo init                    # Interactive setup in current directory")
	fmt.Println("  markgo init --quick            # Quick setup with defaults")
	fmt.Println("  markgo init --dir my-blog      # Setup in new directory")
	fmt.Println("  markgo init --force            # Overwrite existing files")
	fmt.Println()
	fmt.Println("WHAT IT CREATES:")
	fmt.Println("  .env                  Configuration file")
	fmt.Println("  articles/             Content directory with sample posts")
	fmt.Println("  README.md             Project documentation")
	fmt.Println("  .gitignore            Git ignore file")
	fmt.Println()
	fmt.Println("Templates and static assets are embedded in the binary.")
	fmt.Println("To customize, create web/templates/ and web/static/ directories.")
	fmt.Println()
	fmt.Printf("MarkGo %s\n", constants.AppVersion)
}
