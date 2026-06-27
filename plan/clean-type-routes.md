# Plan: Clean Type Routes

**Created:** 2026-06-27
**Scope Lock:** locked 2026-06-27
**Issue:** [#135](https://github.com/1mb-dev/markgo/issues/135)

## Goal

Replace `/?type=thought` → `/thought`, `/?type=link` → `/link`, `/?type=article` → `/article`, `/?type=ama` → `/ama` as canonical paths — route registration, template links, canonical `<link>`, sitemap entries, and 301 backward compat from query form.

## Scope

**In:** route registration (4 new routes), `FeedHandler.Type` method, centralized `isValidFeedType`, 301 `/?type=X` → `/X` preserving non-type params, template filter pill + pagination hrefs, `canonicalPath`/`path`/per-type title in template data, 4 sitemap static entries, tests (route existence, 301 param preservation, sitemap entries)

**Out:** new templates, new `/:param` wildcard route, config changes, operator migration steps, authed-UI changes

## Approach

Register 4 explicit `registerGET` calls → shared `FeedHandler.Type` method extracting type from `c.Request.URL.Path`. Centralize type validation in a `feedVisibleTypes` map used by both the new `Type` method and the existing `Home` method (replacing the inline switch). `Home` gains an early-return 301 when `?type=` is non-empty, preserving other query params. Template feed.html updates filter pills to clean paths and pagination to use a `feedPath` variable instead of deriving the path from `activeFilter`. Sitemap gets 4 static `<url>` entries.

## Steps

### 1. Centralize feed-visible type validation

`internal/handlers/feed_handler.go` — add package-level `feedVisibleTypes` map and `isValidFeedType` helper. Replace the inline switch in `Home` (line 35–40) with a call to `isValidFeedType`. The map includes the four feed-visible types; the helper treats `""` as valid (All filter).

```go
var feedVisibleTypes = map[string]bool{
    "thought": true, "link": true, "article": true, "ama": true,
}

func isValidFeedType(t string) bool {
    return t == "" || feedVisibleTypes[t]
}
```

- **Files:** `internal/handlers/feed_handler.go`
- **Action:** edit — add map + helper above `Home`, replace lines 35–40 with `if !isValidFeedType(typeFilter) { typeFilter = "" }`

### 2. Add `FeedHandler.Type` handler method

`internal/handlers/feed_handler.go` — new method extracting the type from `c.Request.URL.Path` (last segment, after `/`). Validates via `isValidFeedType`; 404 if invalid. Sets template data keys: `activeFilter`, `canonicalPath` (same as path), `path` (for SEO breadcrumb), `feedPath` (for template pagination), per-type `title` ("Thoughts — Blog Title" etc.).

```go
func (h *FeedHandler) Type(c *gin.Context) {
    pageStr := c.DefaultQuery("page", "1")
    page, _ := strconv.Atoi(pageStr)
    if page < 1 { page = 1 }

    typeFilter := strings.TrimPrefix(c.Request.URL.Path, "/")
    if !isValidFeedType(typeFilter) || typeFilter == "" {
        h.handleError(c, apperrors.ErrArticleNotFound, "Type not found")
        return
    }

    data, err := h.getHomeData(page, typeFilter)
    if err != nil {
        h.handleError(c, err, "Failed to get home data")
        return
    }

    data["canonicalPath"] = "/" + typeFilter
    data["path"] = "/" + typeFilter
    data["feedPath"] = "/" + typeFilter
    data["title"] = typeDisplayName(typeFilter) + " — " + h.config.Blog.Title
    data["activeFilter"] = typeFilter

    h.enhanceTemplateDataWithSEO(data, c.Request.URL.Path)
    h.renderHTML(c, http.StatusOK, "base.html", data)
}
```

Helper to map type key → display name:

```go
var feedTypeDisplayNames = map[string]string{
    "thought": "Thoughts",
    "link":    "Links",
    "article": "Articles",
    "ama":     "AMA",
}

func typeDisplayName(t string) string {
    if n, ok := feedTypeDisplayNames[t]; ok {
        return n
    }
    return t
}
```

- **Files:** `internal/handlers/feed_handler.go`
- **Action:** create new method + helpers; add `apperrors "github.com/1mb-dev/markgo/internal/errors"` to imports

### 3. Add 301 redirect from query-param form in `Home`

`internal/handlers/feed_handler.go` — `Home` method: after `typeFilter` validation, if `typeFilter != ""`, strip `type` from the query string, build redirect URL `/<typeFilter>` with any remaining params, and return 301.

```go
if typeFilter != "" {
    q := c.Request.URL.Query()
    q.Del("type")
    dst := "/" + typeFilter
    if encoded := q.Encode(); encoded != "" {
        dst += "?" + encoded
    }
    c.Redirect(http.StatusMovedPermanently, dst)
    return
}
```

Reorder `Home`: read `typeFilter` first → validate → short-circuit redirect if non-empty. Move the `page` read after the redirect guard so it only runs for the All view.

- **Files:** `internal/handlers/feed_handler.go`
- **Action:** edit — add redirect block between validation and `getHomeData` call

### 4. Register 4 clean type routes

`internal/commands/serve/command.go` — add four `registerGET` calls right after `/` (line 514):

```go
registerGET(router, "/",        h.Feed.Home)
registerGET(router, "/thought", h.Feed.Type)
registerGET(router, "/link",    h.Feed.Type)
registerGET(router, "/article", h.Feed.Type)
registerGET(router, "/ama",     h.Feed.Type)
```

- **Files:** `internal/commands/serve/command.go`
- **Action:** edit — add 4 lines after the `/` route

### 5. Update template filter pills + pagination

`web/templates/feed.html`:

**Filter pills (lines 16–21):** Replace `/?type=X` with clean paths. All stays `/`.

```
<a href="/" class="feed-filter{{ if eq .activeFilter "" }} active{{ end }}">All</a>
<a href="/thought" class="feed-filter{{ if eq .activeFilter "thought" }} active{{ end }}">Thoughts</a>
<a href="/link" class="feed-filter{{ if eq .activeFilter "link" }} active{{ end }}">Links</a>
<a href="/article" class="feed-filter{{ if eq .activeFilter "article" }} active{{ end }}">Articles</a>
<a href="/ama" class="feed-filter{{ if eq .activeFilter "ama" }} active{{ end }}">AMA</a>
```

**Pagination (lines 42–54):** Use `feedPath` template variable instead of conditional `&type=` append. If `.feedPath` is set, prepend it to the path; otherwise use `/`.

```
{{ $fp := .feedPath }}
...
<a href="{{ if $fp }}{{ $fp }}{{ else }}/{{ end }}?page={{ $p.PreviousPage }}">← Newer</a>
...
<a href="{{ if $fp }}{{ $fp }}{{ else }}/{{ end }}?page={{ $p.NextPage }}">Older →</a>
```

Remove the `$f` variable (`{{ $f := .activeFilter }}`) since `feedPath` replaces it.

**Feed header `<h1>` (line 6):** Change from hardcoded `{{ .config.Blog.Title }} — Feed` to `{{ if .title }}{{ .title }}{{ else }}{{ .config.Blog.Title }} — Feed{{ end }}` so type pages show their per-type title.

- **Files:** `web/templates/feed.html`
- **Action:** edit — 3 blocks: filter pills, pagination previous, pagination next

### 6. Add sitemap entries for type pages

`internal/services/feed/feed.go` — add 4 static entries in `GenerateSitemap` after the existing static block (line 161):

```go
{Loc: s.config.BaseURL + "/thought",  ChangeFreq: "weekly", Priority: 0.6},
{Loc: s.config.BaseURL + "/link",     ChangeFreq: "weekly", Priority: 0.6},
{Loc: s.config.BaseURL + "/article",  ChangeFreq: "weekly", Priority: 0.6},
{Loc: s.config.BaseURL + "/ama",      ChangeFreq: "weekly", Priority: 0.6},
```

Update the URL count assertion in `TestGenerateSitemap` from 8 → 12 (6 static + 4 type static + 2 articles).

- **Files:** `internal/services/feed/feed.go`, `internal/services/feed/feed_test.go`
- **Action:** edit — add entries + update test count

### 7. Write tests

Three new tests, plus one existing-test update (step 6).

**7a. Route existence test** — `internal/handlers/handlers_test.go`:

Table-driven: for each type in `["thought", "link", "article", "ama"]`, send `GET /<type>` and assert 200. Send `GET /invalid-type` and assert 404.

**7b. 301 param preservation test** — `internal/handlers/handlers_test.go`:

`GET /?type=thought` → 301 → Location: `/thought`
`GET /?type=thought&page=2` → 301 → Location: `/thought?page=2`
`GET /?type=link&foo=bar` → 301 → Location: `/link?foo=bar`

**7c. Sitemap type entries test** — `internal/services/feed/feed_test.go`:

Assert `GenerateSitemap` output contains `/thought`, `/link`, `/article`, `/ama` URLs.

- **Files:** `internal/handlers/handlers_test.go`, `internal/services/feed/feed_test.go`
- **Action:** create/add tests

## Verification

- [ ] `make test` — all tests pass, including 3 new + 1 updated
- [ ] `make test-race` — no races in the added test (feedVisibleTypes is read-only post-init; no sync needed)
- [ ] `make lint` — golangci-lint clean
- [ ] Manual smoke: `make dev`, open `http://localhost:3000/thought` → 200 with Thoughts filter pill active
- [ ] Manual smoke: `http://localhost:3000/?type=thought` → 301 → `/thought`
- [ ] Manual smoke: `curl -sI http://localhost:3000/sitemap.xml | head -1` → 200, check `/thought` entry present

## Rollback

`git revert <sha>` for each commit. No migrations, no config changes, no operator impact.

## Design Notes

- **Validation map vs enum:** The four feed-visible types are an effective closed set. A map is the simplest correct check. The map is read-only post-init so no mutex needed.
- **`feedPath` vs deriving from `activeFilter`:** Pagination needs the URL path prefix (`/thought`), not the filter value (`thought`). A separate template variable avoids string concatenation in the template and keeps `activeFilter` solely for CSS pill toggling.
- **`data["path"]` vs `canonicalPath`:** Both needed. `canonicalPath` feeds og/twitter/canonical link (consumed by `base.html`). `path` feeds SEO breadcrumb (consumed by `enhanceWithSEO` → `GeneratePageSEOData`). These are separate consumers with separate keys in the existing code; don't collapse them in this change.
- **Home's `page` param when redirecting:** The redirect fires before pagination — `page` is preserved in the remaining query string. `Home` no longer needs to read `page` itself when `type` is present (the redirect target handles it).
- **Sitemap URL count:** Current expected is 8 (6 static + 2 articles). After adding 4 type-page entries, expected becomes 12. The `TestGenerateSitemap` assertion at line 203 must update.
