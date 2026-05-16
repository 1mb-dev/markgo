# API Reference

> All HTTP routes served by MarkGo. All responses are HTML unless noted otherwise.

---

## Public Routes

### Feed & Content

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/` | FeedHandler | Homepage feed. Supports `?type=thought\|link\|article` and `?page=N`. |
| GET | `/writing` | PostHandler | Articles listing with pagination (`?page=N`). |
| GET | `/writing/:slug` | PostHandler | Single article by slug. |
| GET | `/search` | SearchHandler | Full-text search. Query param: `?q=term`. |
| GET | `/about` | AboutHandler | Config-driven about page with contact section. |
| GET | `/contact` | — | 301 redirect to `/about#contact`. |
| POST | `/contact` | ContactHandler | Contact form submission. Rate limited. |

### Taxonomy

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/tags` | TaxonomyHandler | All tags with article counts. |
| GET | `/tags/:tag` | TaxonomyHandler | Articles filtered by tag. |
| GET | `/categories` | TaxonomyHandler | All categories with article counts. |
| GET | `/categories/:category` | TaxonomyHandler | Articles filtered by category. |

### Feeds & SEO

| Method | Path | Response Type | Description |
|--------|------|---------------|-------------|
| GET | `/feed.xml` | `application/rss+xml` | RSS 2.0 feed. |
| GET | `/feed.json` | `application/json` | JSON Feed v1. |
| GET | `/sitemap.xml` | `application/xml` | XML sitemap for search engines. |
| GET | `/robots.txt` | `text/plain` | Robots.txt (configurable via `SEO_ROBOTS_*`). |

### Health & PWA

| Method | Path | Response Type | Description |
|--------|------|---------------|-------------|
| GET | `/health` | `application/json` | Health check with uptime and version. |
| GET | `/manifest.json` | `application/json` | PWA manifest (config-driven). |
| GET | `/offline` | HTML | Offline fallback page (served by Service Worker). |

### Static Assets

| Path | Description |
|------|-------------|
| `/static/*` | CSS, JS, images, fonts. Cached with `Cache-Control: public, max-age=3600`. |
| `/favicon.ico` | Favicon. |
| `/sw.js` | Service Worker (served from root for scope). |

---

## Auth Routes

Only registered when `ADMIN_USERNAME` and `ADMIN_PASSWORD` are configured.

| Method | Path | Middleware | Description |
|--------|------|-----------|-------------|
| GET | `/login` | CSRF | Login form. |
| POST | `/login` | CSRF | Authenticate. Sets session cookie (7-day, HttpOnly, SameSite=Strict). |
| GET | `/logout` | — | Clear session and redirect. |

---

## Compose Routes

Require authentication (soft auth: unauthenticated GET shows login popover, unauthenticated POST returns 401). All routes have CSRF and NoCache middleware.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/compose` | Compose form (title, content, tags, preview, upload). |
| POST | `/compose` | Create new article. Writes markdown file to disk. Redirects to article. |
| GET | `/compose/edit/:slug` | Edit form pre-filled with existing article. |
| POST | `/compose/edit/:slug` | Update existing article. Atomic write (temp + rename). |
| POST | `/compose/preview` | Render markdown to HTML. Returns HTML fragment. |
| POST | `/compose/upload/:slug` | Upload file to slug-scoped directory. Extension blocklist validates type. Returns JSON: url, markdown, filename |
| POST | `/compose/publish/:slug` | Publish a draft article. Returns JSON with url and message |
| POST | `/compose/quick` | Quick capture API. Returns JSON: `{ slug, url, type }`. |

---

## Admin Routes

Require authentication (soft auth). NoCache middleware.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin` | Admin dashboard. |
| GET | `/admin/drafts` | List all draft articles with edit links. |
| GET | `/admin/writing` | Published content list with edit links. |
| GET | `/admin/stats` | Site statistics (JSON). |
| GET | `/admin/metrics` | Performance metrics (JSON). |
| POST | `/admin/cache/clear` | Clear article cache. |
| POST | `/admin/articles/reload` | Reload articles from disk. |

---

## Debug Routes

Only available when `ENVIRONMENT=development`. Require hard session auth.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/debug/memory` | Memory statistics. |
| GET | `/debug/runtime` | Runtime information. |
| GET | `/debug/config` | Current configuration. |
| GET | `/debug/requests` | Recent request log. |
| GET | `/debug/pprof/*` | Go pprof profiling (Index, Cmdline, Profile, Symbol, Trace, heap, goroutine, allocs, block, mutex). |

---

## Contact Form

`POST /contact` accepts `application/x-www-form-urlencoded`:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Sender name. |
| `email` | Yes | Sender email (validated). |
| `message` | Yes | Message content. |

Responses: 200 (success), 400 (validation), 429 (rate limited).

---

## Security

**Headers** on all responses:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: SAMEORIGIN`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `X-Response-Time` (performance timing)

**Rate limiting**: Sliding window per IP. Static assets excluded. General: 100 req/15min (production). Contact: 5 req/hour.

**CSRF**: Double-submit cookie on login and compose routes. Token in cookie (`_csrf`, HttpOnly, SameSite=Strict) and form field/header (`_csrf` / `X-CSRF-Token`). 1-hour expiry.

**Authentication**: Session cookie (`_session`), 7-day expiry, HttpOnly, SameSite=Strict, Secure in production.
