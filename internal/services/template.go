package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/1mb-dev/goflow/pkg/scheduling/scheduler"
	"github.com/1mb-dev/goflow/pkg/scheduling/workerpool"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/dustin/go-humanize"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	"net/url"

	"github.com/1mb-dev/markgo/internal/config"
	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/web"
)

var (
	timeZoneCache *obcache.Cache
	titleCaser    = cases.Title(language.English)
)

func init() {
	// Initialize global timezone cache
	tzCacheConfig := obcache.NewDefaultConfig()
	tzCacheConfig.MaxEntries = 200 // Support many timezones
	tzCacheConfig.DefaultTTL = 0   // Timezones don't expire

	cache, err := obcache.New(tzCacheConfig)
	if err != nil {
		slog.Warn("Timezone cache init failed, trying fallback", "error", err)
		basicConfig := obcache.NewDefaultConfig()
		basicConfig.DefaultTTL = 0
		if fallbackCache, fbErr := obcache.New(basicConfig); fbErr == nil {
			timeZoneCache = fallbackCache
		} else {
			slog.Error("Timezone cache fallback also failed, timezone formatting will be slow", "error", fbErr)
			timeZoneCache = nil
		}
		return
	}
	timeZoneCache = cache
}

// CachedTemplateFunctions holds obcache-wrapped template operations
type CachedTemplateFunctions struct {
	RenderToString func(string, any) (string, error)
	ParseTemplate  func(string, string) (*template.Template, error)
}

// TemplateService provides template rendering functionality.
type TemplateService struct {
	templates *template.Template
	config    *config.Config

	// brandLogoSVG is the rendered header logo (embedded default or operator overlay).
	brandLogoSVG template.HTML

	// obcache integration
	obcache         *obcache.Cache
	cachedFunctions CachedTemplateFunctions

	// goflow integration
	scheduler scheduler.Scheduler
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewTemplateService creates a new TemplateService instance.
func NewTemplateService(templatesPath string, cfg *config.Config) (*TemplateService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize obcache for template operations
	cacheConfig := obcache.NewDefaultConfig()
	cacheConfig.MaxEntries = 500              // Templates are smaller, fewer entries needed
	cacheConfig.DefaultTTL = 30 * time.Minute // Templates change less frequently

	obcacheInstance, err := obcache.New(cacheConfig)
	if err != nil {
		slog.Warn("Template cache init failed, rendering will be slower", "error", err)
		cancel()
	}

	// Initialize goflow scheduler for template maintenance
	goflowScheduler := scheduler.New()
	if err := goflowScheduler.Start(); err != nil {
		slog.Warn("Template scheduler failed to start, cache cleanup won't run", "error", err)
	}

	service := &TemplateService{
		config:    cfg,
		obcache:   obcacheInstance,
		scheduler: goflowScheduler,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Initialize cached functions
	service.initializeCachedFunctions()

	// Setup background maintenance
	service.setupTemplateMaintenance()

	if err := service.loadBrandLogo(cfg.StaticPath); err != nil {
		cancel()
		return nil, err
	}

	if err := service.loadTemplates(templatesPath); err != nil {
		cancel()
		return nil, err
	}

	return service, nil
}

func (t *TemplateService) loadTemplates(templatesPath string) error {
	funcMap := t.funcMap()
	pattern := filepath.Join(templatesPath, "*.html")

	// Check if filesystem templates exist before parsing
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return apperrors.NewHTTPError(500, fmt.Sprintf("Invalid template glob pattern: %v", err), apperrors.ErrTemplateParseError)
	}

	if len(matches) > 0 {
		// Filesystem templates exist — parse errors are real errors, not fallback triggers
		tmpl, parseErr := template.New("").Funcs(funcMap).ParseGlob(pattern)
		if parseErr != nil {
			return apperrors.NewHTTPError(500, fmt.Sprintf("Failed to parse filesystem templates: %v", parseErr), apperrors.ErrTemplateParseError)
		}
		t.templates = tmpl
		return nil
	}

	// No filesystem templates — fall back to embedded
	slog.Info("Filesystem templates not found, using embedded templates", "path", templatesPath)
	embeddedFS, subErr := fs.Sub(web.Assets, "templates")
	if subErr != nil {
		return apperrors.NewHTTPError(500, fmt.Sprintf("Failed to access embedded templates: %v", subErr), apperrors.ErrTemplateParseError)
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(embeddedFS, "*.html")
	if err != nil {
		return apperrors.NewHTTPError(500, fmt.Sprintf("Failed to parse embedded templates: %v", err), apperrors.ErrTemplateParseError)
	}

	t.templates = tmpl
	return nil
}

// brandLogoFS is the source for the embedded brand-logo SVG.
// Tests may temporarily swap this to exercise the embedded-read-failure path.
var brandLogoFS fs.FS = web.Assets

// loadEmbeddedBrandLogo reads the embedded brand-logo SVG. Returning an error
// indicates a build invariant violation — markgo must refuse to boot.
func loadEmbeddedBrandLogo() ([]byte, error) {
	return fs.ReadFile(brandLogoFS, "static/img/brand-logo.svg")
}

// brandLogoMaxBytes caps operator-supplied overlay SVGs. Real brand logos sit
// well under 10 KiB; 32 KiB leaves headroom for inline paths and metadata.
const brandLogoMaxBytes = 32 * 1024

// loadBrandLogo populates t.brandLogoSVG. Reads the operator overlay at
// <staticPath>/img/brand-logo.svg when staticPath is set; falls back to the
// embedded default on absence or validation failure. Returns error only when
// the embedded default itself cannot be read (build invariant).
func (t *TemplateService) loadBrandLogo(staticPath string) error {
	embedded, err := loadEmbeddedBrandLogo()
	if err != nil {
		return fmt.Errorf("embedded brand-logo missing or unreadable: %w", err)
	}

	if staticPath != "" {
		overlayPath := filepath.Join(staticPath, "img", "brand-logo.svg")
		overlay, reason := readAndValidateOverlay(overlayPath)
		switch {
		case overlay != nil:
			// #nosec G203 - Operator-supplied SVG from STATIC_PATH overlay. Trust boundary documented in docs/configuration.md (same model as overlaid CSS/JS).
			t.brandLogoSVG = template.HTML(injectBrandLogoClass(overlay))
			return nil
		case reason != "":
			slog.Warn("brand-logo overlay rejected, using embedded default",
				"path", overlayPath, "reason", reason)
		}
	}

	// #nosec G203 - brand-logo SVG is markgo's own embedded asset. Trust boundary documented in docs/configuration.md.
	t.brandLogoSVG = template.HTML(injectBrandLogoClass(embedded))
	return nil
}

// readAndValidateOverlay reads the operator's brand-logo SVG and runs the
// validation contract. Returns:
//   - (bytes, "") when the file exists and validates
//   - (nil, "")   when the file does not exist (silent fallback)
//   - (nil, reason) on any validation failure (caller logs warning)
func readAndValidateOverlay(path string) ([]byte, string) {
	f, err := os.Open(path) // #nosec G304 - operator-controlled path, trust boundary documented
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, fmt.Sprintf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, brandLogoMaxBytes+1))
	if err != nil {
		return nil, fmt.Sprintf("read: %v", err)
	}
	if len(data) > brandLogoMaxBytes {
		return nil, "exceeds 32 KiB cap"
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, terr := decoder.Token()
		if terr != nil {
			return nil, fmt.Sprintf("malformed XML: %v", terr)
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local != "svg" {
				return nil, "root element is not <svg>"
			}
			return data, ""
		}
	}
}

// injectBrandLogoClass returns svg unchanged if a class attribute is already
// present on the root <svg>; otherwise it splices class="brand-logo" into
// the root start-tag. Assumes svg has already been validated by
// readAndValidateOverlay (root is <svg>, well-formed XML).
func injectBrandLogoClass(svg []byte) []byte {
	svgStart, tagEnd, ok := findSVGTagBounds(svg)
	if !ok {
		return svg
	}
	if hasClassAttribute(svg[svgStart+4 : tagEnd]) {
		return svg
	}
	out := make([]byte, 0, len(svg)+len(` class="brand-logo"`))
	out = append(out, svg[:svgStart+4]...)
	out = append(out, ` class="brand-logo"`...)
	out = append(out, svg[svgStart+4:]...)
	return out
}

// findSVGTagBounds returns the byte offsets of "<svg" and the matching ">"
// that closes the start-tag, respecting quoted attribute values. ok is false
// only if the input does not contain a parseable <svg ...> opener.
func findSVGTagBounds(svg []byte) (svgStart, tagEnd int, ok bool) {
	svgStart = bytes.Index(svg, []byte("<svg"))
	if svgStart == -1 {
		return 0, 0, false
	}
	var quote byte
	for i := svgStart + 4; i < len(svg); i++ {
		c := svg[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '>' {
			return svgStart, i, true
		}
	}
	return 0, 0, false
}

// hasClassAttribute scans the byte range between "<svg" and ">" for a
// top-level class attribute, ignoring occurrences inside quoted values.
func hasClassAttribute(tagWindow []byte) bool {
	const needle = "class"
	var quote byte
	for i := range len(tagWindow) {
		c := tagWindow[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == 'c' && i+len(needle) <= len(tagWindow) && string(tagWindow[i:i+len(needle)]) == needle {
			if i == 0 || isASCIIWhitespace(tagWindow[i-1]) {
				j := i + len(needle)
				for j < len(tagWindow) && isASCIIWhitespace(tagWindow[j]) {
					j++
				}
				if j < len(tagWindow) && tagWindow[j] == '=' {
					return true
				}
			}
		}
	}
	return false
}

func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// funcMap returns the template FuncMap with the per-instance brandLogoSVG
// closure appended to the stateless base map.
func (t *TemplateService) funcMap() template.FuncMap {
	m := make(template.FuncMap, len(templateFuncs)+1)
	maps.Copy(m, templateFuncs)
	m["brandLogoSVG"] = func() template.HTML { return t.brandLogoSVG }
	return m
}

// Render renders a template to the provided writer.
func (t *TemplateService) Render(w io.Writer, templateName string, data any) error {
	if t.templates == nil {
		return apperrors.ErrTemplateNotFound
	}

	tmpl := t.templates.Lookup(templateName)
	if tmpl == nil {
		return apperrors.NewHTTPError(404,
			fmt.Sprintf("Template '%s' not found", templateName),
			apperrors.ErrTemplateNotFound)
	}

	return tmpl.Execute(w, data)
}

// RenderToString renders a template and returns the result as a string.
func (t *TemplateService) RenderToString(templateName string, data any) (string, error) {
	// Use cached function if available
	if t.cachedFunctions.RenderToString != nil {
		return t.cachedFunctions.RenderToString(templateName, data)
	}

	// Fallback to uncached version
	return t.renderToStringUncached(templateName, data)
}

// HasTemplate checks if a template with the given name exists.
func (t *TemplateService) HasTemplate(templateName string) bool {
	if t.templates == nil {
		return false
	}
	return t.templates.Lookup(templateName) != nil
}

// ListTemplates returns a list of all available template names.
func (t *TemplateService) ListTemplates() []string {
	if t.templates == nil {
		return []string{}
	}

	var names []string
	for _, tmpl := range t.templates.Templates() {
		if tmpl.Name() != "" {
			names = append(names, tmpl.Name())
		}
	}
	return names
}

// Reload reloads all templates from the specified path.
func (t *TemplateService) Reload(templatesPath string) error {
	// Clear cache before reloading
	if t.obcache != nil {
		if err := t.obcache.Clear(); err != nil {
			slog.Warn("Failed to clear template cache during reload", "error", err)
		}
	}
	return t.loadTemplates(templatesPath)
}

// GetTemplate returns the internal template for Gin integration
func (t *TemplateService) GetTemplate() *template.Template {
	return t.templates
}

// initializeCachedFunctions initializes obcache-wrapped template functions
func (t *TemplateService) initializeCachedFunctions() {
	if t.obcache == nil {
		return
	}

	// Wrap template rendering with obcache
	t.cachedFunctions.RenderToString = obcache.Wrap(
		t.obcache,
		t.renderToStringUncached,
		obcache.WithKeyFunc(func(args []any) string {
			if len(args) >= 2 {
				if templateName, ok := args[0].(string); ok {
					// Create hash of data for cache key
					hash := sha256.Sum256([]byte(fmt.Sprintf("%v", args[1])))
					dataHash := fmt.Sprintf("%x", hash[:8])
					return fmt.Sprintf("render:%s:%s", templateName, dataHash)
				}
			}
			return "render:default"
		}),
		obcache.WithTTL(15*time.Minute),
	)
}

// setupTemplateMaintenance sets up background maintenance using goflow
func (t *TemplateService) setupTemplateMaintenance() {
	if t.scheduler == nil {
		return
	}

	// Template cache cleanup every hour
	cleanupTask := workerpool.TaskFunc(func(_ context.Context) error {
		if t.obcache != nil {
			t.obcache.Cleanup()
		}
		return nil
	})

	// Schedule cleanup using proper cron format (6 fields: second, minute, hour, day, month, weekday)
	if err := t.scheduler.ScheduleCron("template-cache-cleanup", "0 0 * * * *", cleanupTask); err != nil {
		slog.Warn("Failed to schedule template cache cleanup", "error", err)
	}
}

// renderToStringUncached renders template without caching
func (t *TemplateService) renderToStringUncached(templateName string, data any) (string, error) {
	var buf bytes.Buffer
	if err := t.Render(&buf, templateName, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Shutdown gracefully shuts down the template service
func (t *TemplateService) Shutdown() {
	if t.cancel != nil {
		t.cancel()
	}

	if t.scheduler != nil {
		t.scheduler.Stop()
	}

	if t.obcache != nil {
		if err := t.obcache.Close(); err != nil {
			slog.Warn("Failed to close template cache during shutdown", "error", err)
		}
	}
}

// GetTemplateFuncMap returns the stateless template function map for reuse in
// other services. Per-instance helpers such as brandLogoSVG (registered on the
// TemplateService via funcMap()) are NOT included; callers parsing templates
// that reference those helpers must obtain the funcMap from a TemplateService.
func GetTemplateFuncMap() template.FuncMap {
	return templateFuncs
}

var templateFuncs = template.FuncMap{
	"safeHTML": func(s string) template.HTML {
		s = html.UnescapeString(s)
		// Basic sanitization using bluemonday UGC policy
		policy := bluemonday.UGCPolicy()
		// #nosec G203 - This is intentional HTML output for templates, sanitized by bluemonday
		return template.HTML(policy.Sanitize(s))
	},
	"join": func(sep string, items []string) string {
		return strings.Join(items, sep)
	},
	"slice": func(items any, start, end int) any {
		v := reflect.ValueOf(items)
		if v.Kind() != reflect.Slice {
			return items
		}
		length := v.Len()
		if start < 0 {
			start = 0
		}
		if end > length {
			end = length
		}
		if start >= end {
			return reflect.MakeSlice(v.Type(), 0, 0).Interface()
		}
		return v.Slice(start, end).Interface()
	},
	"add": func(a, b int) int {
		return a + b
	},
	"sub": func(a, b int) int {
		return a - b
	},
	"mul": func(a, b int) int {
		return a * b
	},
	"printf": fmt.Sprintf,
	"le": func(a, b any) bool {
		switch va := a.(type) {
		case int:
			if vb, ok := b.(int); ok {
				return va <= vb
			}
		case float64:
			if vb, ok := b.(float64); ok {
				return va <= vb
			}
		}
		return false
	},
	"div": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a / b
	},
	"seq": func(start, end int) []int {
		if start > end {
			return []int{}
		}
		seq := make([]int, end-start+1)
		for i := range seq {
			seq[i] = start + i
		}
		return seq
	},
	"gt": func(a, b int) bool {
		return a > b
	},
	"lt": func(a, b int) bool {
		return a < b
	},
	"eq": func(args ...any) bool {
		if len(args) < 2 {
			return false
		}
		for _, arg := range args[1:] {
			if reflect.DeepEqual(args[0], arg) {
				return true
			}
		}
		return false
	},
	"len": func(items any) int {
		v := reflect.ValueOf(items)
		switch v.Kind() {
		case reflect.Slice, reflect.Array, reflect.String, reflect.Map:
			return v.Len()
		default:
			return 0
		}
	},
	"compare": func(a, b any) int {
		af, aok := toFloat(a)
		bf, bok := toFloat(b)
		if !aok || !bok {
			return 0
		}
		switch {
		case af > bf:
			return 1
		case af < bf:
			return -1
		default:
			return 0
		}
	},
	"formatNumber": func(num any) string {
		switch n := num.(type) {
		case int:
			return humanize.Comma(int64(n))
		case float64:
			return humanize.Ftoa(n)
		default:
			return fmt.Sprint(n)
		}
	},
	"formatDate": func(date time.Time, format string) string {
		return date.Format(format)
	},
	"formatDateInZone": func(date time.Time, zone, format string) string {
		if timeZoneCache == nil {
			loc, err := time.LoadLocation(zone)
			if err != nil {
				return date.Format(format)
			}
			return date.In(loc).Format(format)
		}
		if cachedLoc, found := timeZoneCache.Get(zone); found {
			if loc, ok := cachedLoc.(*time.Location); ok {
				return date.In(loc).Format(format)
			}
			// Fallback if type assertion fails
			if loc, err := time.LoadLocation(zone); err == nil {
				return date.In(loc).Format(format)
			}
			return date.Format(format)
		}

		loc, err := time.LoadLocation(zone)
		if err != nil {
			return date.Format(format) // Fallback to original timezone
		}

		// Store timezone location in cache with no expiration
		if err := timeZoneCache.Set(zone, loc, 0); err != nil {
			slog.Debug("Failed to cache timezone location", "zone", zone, "error", err)
		}
		return date.In(loc).Format(format)
	},
	"now": time.Now,
	"max": func(a, b int) int {
		if a > b {
			return a
		}
		return b
	},
	"min": func(a, b int) int {
		if a < b {
			return a
		}
		return b
	},
	"mod": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a % b
	},
	"ne": func(a, b any) bool {
		return !reflect.DeepEqual(a, b)
	},
	"and": func(a, b bool) bool {
		return a && b
	},
	"or": func(a, b bool) bool {
		return a || b
	},
	"not": func(a bool) bool {
		return !a
	},
	"contains":  strings.Contains,
	"hasPrefix": strings.HasPrefix,
	"hasSuffix": strings.HasSuffix,
	"lower":     strings.ToLower,
	"upper":     strings.ToUpper,
	"title":     titleCaser.String,
	"trim":      strings.TrimSpace,
	"truncate": func(s string, length int) string {
		runes := []rune(s)
		if len(runes) <= length {
			return s
		}
		return string(runes[:length]) + "..."
	},
	"truncateHTML": func(s string, length int) template.HTML {
		runes := []rune(s)
		if len(runes) <= length {
			// #nosec G203 - This is intentional HTML output for templates
			return template.HTML(s)
		}
		safe := template.HTMLEscapeString(string(runes[:length])) + "..."
		// #nosec G203 - This is intentional HTML output for templates, content is escaped
		return template.HTML(safe)
	},
	"slugify": func(s string) string {
		s = strings.ToLower(s)
		var b strings.Builder
		for _, r := range norm.NFD.String(s) {
			switch {
			case r >= 'a' && r <= 'z':
				b.WriteRune(r)
			case r >= '0' && r <= '9':
				b.WriteRune(r)
			case r == ' ' || r == '-':
				b.WriteRune('-')
			case unicode.Is(unicode.Mn, r): // Remove diacritics
				continue
			}
		}
		return b.String()
	},
	// storageNamespace derives a client-storage namespace from the install's BaseURL.
	// URL-aware: '.', '/', ':' all become '-' (slugify would drop them).
	// Used to namespace localStorage keys and IndexedDB names so same-origin
	// path-mounted MarkGo instances don't collide. Empty BaseURL → "markgo:default".
	"storageNamespace": func(baseURL string) string {
		src := strings.TrimPrefix(baseURL, "https://")
		src = strings.TrimPrefix(src, "http://")
		src = strings.TrimSuffix(src, "/")
		if src == "" {
			return "markgo:default"
		}
		var b strings.Builder
		b.WriteString("markgo:")
		dashed := true // suppress leading dash
		for _, r := range strings.ToLower(src) {
			switch {
			case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
				b.WriteRune(r)
				dashed = false
			default:
				if !dashed {
					b.WriteRune('-')
					dashed = true
				}
			}
		}
		return strings.TrimSuffix(b.String(), "-")
	},
	"timeAgo": func(date time.Time) string {
		if date.IsZero() {
			return ""
		}
		duration := time.Since(date)
		switch {
		case duration < time.Minute:
			return "just now"
		case duration < time.Hour:
			return pluralize(int(duration.Minutes()), "minute", "minutes")
		case duration < 24*time.Hour:
			return pluralize(int(duration.Hours()), "hour", "hours")
		case duration < 30*24*time.Hour:
			return pluralize(int(duration.Hours()/24), "day", "days")
		case duration < 365*24*time.Hour:
			return pluralize(int(duration.Hours()/(24*30)), "month", "months")
		default:
			return pluralize(int(duration.Hours()/(24*365)), "year", "years")
		}
	},
	"isNil": func(value any) bool {
		return value == nil
	},
	"isNotNil": func(value any) bool {
		return value != nil
	},
	"default": func(defaultValue, value any) any {
		if value == nil {
			return defaultValue
		}

		val := reflect.ValueOf(value)
		switch val.Kind() {
		case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
			if val.Len() == 0 {
				return defaultValue
			}
		case reflect.Ptr, reflect.Interface:
			if val.IsNil() {
				return defaultValue
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if val.Int() == 0 {
				return defaultValue
			}
		case reflect.Float32, reflect.Float64:
			if val.Float() == 0 {
				return defaultValue
			}
		case reflect.Bool:
			if !val.Bool() {
				return defaultValue
			}
		}

		return value
	},
	"get": func(m map[string]any, key string) any {
		return m[key]
	},
	"ternary": func(cond bool, trueVal, falseVal any) any {
		if cond {
			return trueVal
		}
		return falseVal
	},
	"initials": func(s string) string {
		words := strings.Fields(strings.TrimSpace(s))
		if len(words) == 0 {
			return ""
		}

		initials := ""
		for i, word := range words {
			if i >= 2 { // Only take first 2 initials
				break
			}
			if word != "" {
				if r, _ := utf8.DecodeRuneInString(word); r != utf8.RuneError {
					initials += strings.ToUpper(string(r))
				}
			}
		}
		return initials
	},

	// SEO Template Functions
	"generateJsonLD": func(data map[string]interface{}) template.HTML {
		// Convert map to JSON for structured data
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return template.HTML("")
		}
		// #nosec G203 - This is intentional JSON-LD structured data for SEO
		return template.HTML(fmt.Sprintf(`<script type="application/ld+json">%s</script>`, string(jsonBytes)))
	},
	"renderMetaTags": func(tags map[string]string) template.HTML {
		var buf strings.Builder
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, name := range keys {
			content := tags[name]
			if content == "" {
				continue
			}
			// Escape HTML entities in meta content
			escapedContent := html.EscapeString(content)

			// article:tag is a multi-value tag encoded as "article:tag:<index>"
			// in the map (since map keys must be unique). Decode back to
			// canonical property name before emitting.
			propertyName := name
			if strings.HasPrefix(name, "article:tag:") {
				propertyName = "article:tag"
			}

			// Determine meta tag type
			if strings.HasPrefix(propertyName, "og:") || strings.HasPrefix(propertyName, "twitter:") || strings.HasPrefix(propertyName, "article:") {
				fmt.Fprintf(&buf, `<meta property="%s" content="%s">`, html.EscapeString(propertyName), escapedContent) //nolint:gocritic // HTML attribute quoting, not Go string quoting
			} else if propertyName == "canonical" {
				fmt.Fprintf(&buf, `<link rel="canonical" href="%s">`, escapedContent) //nolint:gocritic // HTML attribute quoting, not Go string quoting
			} else {
				fmt.Fprintf(&buf, `<meta name="%s" content="%s">`, html.EscapeString(propertyName), escapedContent) //nolint:gocritic // HTML attribute quoting, not Go string quoting
			}
			buf.WriteString("\n")
		}
		// #nosec G203 - This is intentional HTML output for meta tags, content is escaped
		return template.HTML(buf.String())
	},
	"seoExcerpt": func(content string, maxLength int) string {
		// Remove markdown formatting for SEO description
		text := content
		text = strings.ReplaceAll(text, "#", "")
		text = strings.ReplaceAll(text, "*", "")
		text = strings.ReplaceAll(text, "_", "")
		text = strings.ReplaceAll(text, "`", "")
		text = strings.ReplaceAll(text, "[", "")
		text = strings.ReplaceAll(text, "]", "")
		text = strings.ReplaceAll(text, "(", "")
		text = strings.ReplaceAll(text, ")", "")

		// Clean up whitespace
		words := strings.Fields(text)
		if len(words) == 0 {
			return ""
		}

		// Build excerpt
		var excerpt strings.Builder
		for _, word := range words {
			if excerpt.Len()+len(word)+1 > maxLength {
				break
			}
			if excerpt.Len() > 0 {
				excerpt.WriteString(" ")
			}
			excerpt.WriteString(word)
		}

		result := excerpt.String()
		if len(result) < len(strings.Join(words, " ")) {
			result += "..."
		}

		return result
	},
	"readingTime": func(content string, wordsPerMinute int) int {
		if wordsPerMinute <= 0 {
			wordsPerMinute = 200 // Default reading speed
		}
		words := strings.Fields(content)
		minutes := len(words) / wordsPerMinute
		if minutes == 0 && len(words) > 0 {
			return 1 // Minimum 1 minute
		}
		return minutes
	},
	"buildURL": func(baseURL, path string) string {
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return path // Already absolute URL
		}
		baseURL = strings.TrimRight(baseURL, "/")
		path = strings.TrimLeft(path, "/")
		return fmt.Sprintf("%s/%s", baseURL, path)
	},
	"relativeTime": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		duration := time.Since(t)
		switch {
		case duration < time.Hour:
			m := int(duration.Minutes())
			if m <= 1 {
				return "just now"
			}
			return fmt.Sprintf("%dm ago", m)
		case duration < 24*time.Hour:
			return fmt.Sprintf("%dh ago", int(duration.Hours()))
		case duration < 7*24*time.Hour:
			return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
		default:
			return t.Format("Jan 2")
		}
	},
	"extractDomain": func(urlStr string) string {
		u, err := url.Parse(urlStr)
		if err != nil {
			return urlStr
		}
		host := u.Hostname()
		if host == "" {
			return urlStr
		}
		return strings.TrimPrefix(host, "www.")
	},
	"displayTitle": func(a *models.Article) string {
		return a.DisplayTitle()
	},
}

// Helper function
func toFloat(v any) (float64, bool) {
	switch v := v.(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s ago", singular)
	}
	return fmt.Sprintf("%d %s ago", n, plural)
}
