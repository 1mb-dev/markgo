# Architecture

> Last revised: February 2026 (v3.3)

---

## Overview

MarkGo is a single-binary blog engine. Markdown files in, web pages out. No database, no build step, no external dependencies at runtime.

The server reads markdown files from a directory, infers content types (article, thought, link), and serves them through a progressively-enhanced SPA with offline support. The same binary handles the CLI (init, new) and the web server.

```
┌────────────────────────────────────────────────────┐
│  Browser (SPA shell)                               │
│  router.js → fetch HTML → swap <main> → history    │
│  Service Worker → offline cache → compose queue    │
├────────────────────────────────────────────────────┤
│  HTTP Layer (Gin)                                  │
│  Middleware → Handlers → Template rendering        │
├────────────────────────────────────────────────────┤
│  Services                                          │
│  Articles, Feed, Compose, SEO, Email, Templates    │
├────────────────────────────────────────────────────┤
│  Filesystem                                        │
│  articles/*.md → read, parse, cache, serve         │
└────────────────────────────────────────────────────┘
```

---

## Server

### Handlers

Eleven handler types, each focused on one concern. All share a `BaseHandler` that provides config, logger, template service, build info, and SEO service.

| Handler | Routes | Purpose |
|---------|--------|---------|
| FeedHandler | `GET /` | Homepage feed with article/thought/link cards |
| PostHandler | `GET /writing`, `GET /writing/:slug` | Article listing and single article |
| TaxonomyHandler | `GET /tags`, `GET /tags/:tag`, `GET /categories`, `GET /categories/:category` | Tag cloud, category cards, filtered listings |
| SearchHandler | `GET /search` | Full-text search |
| AboutHandler | `GET /about` | Config-driven about page with contact section |
| ContactHandler | `POST /contact` | Contact form submission (SMTP) |
| SyndicationHandler | `GET /feed.xml`, `GET /feed.json`, `GET /sitemap.xml`, `GET /robots.txt` | RSS, JSON Feed, sitemap, robots |
| HealthHandler | `GET /health`, `GET /manifest.json`, `GET /offline` | Health check, PWA manifest, offline page |
| AuthHandler | `GET/POST /login`, `GET /logout` | Session-based login/logout |
| ComposeHandler | `GET/POST /compose`, `GET/POST /compose/edit/:slug`, `POST /compose/preview`, `POST /compose/upload/:slug`, `POST /compose/publish/:slug`, `POST /compose/quick` | Content creation, editing, preview, file upload, publish, quick capture |
| AdminHandler | `GET /admin`, `GET /admin/drafts`, `GET /admin/writing`, `GET /admin/stats`, `GET /admin/metrics`, `POST /admin/cache/clear`, `POST /admin/articles/reload` | Dashboard, drafts, published content, stats, metrics, cache management |

Auth and Compose handlers are only registered when admin credentials are configured. Debug and pprof routes are only registered in development.

### Middleware

Applied in this order on every request:

1. **Recovery** — Panic recovery with type-aware logging
2. **Logger** — Structured request logging (static assets demoted to debug)
3. **Performance** — X-Response-Time header, slow request warnings (>1s)
4. **SmartCacheHeaders** — Default `Cache-Control: public, max-age=3600`
5. **CORS** — Exact origin matching with Vary header
6. **Security** — X-Content-Type-Options, X-Frame-Options, Referrer-Policy
7. **RateLimit** — Sliding window per IP, excludes static assets
8. **ErrorHandler** — Centralized error logging

Route-specific middleware:
- **Login routes**: CSRF (double-submit cookie)
- **Contact**: Stricter rate limit
- **Compose routes**: SoftSessionAuth + NoCache + CSRF
- **Admin routes**: SoftSessionAuth + NoCache
- **Debug routes**: Hard session auth (development only)

### Services

| Service | Responsibility |
|---------|---------------|
| ArticleService | Load, parse, cache markdown files. Search index. Tag/category aggregation. Content type inference. |
| FeedService | Generate RSS (XML), JSON Feed, and sitemap from article data |
| ComposeService | Write markdown files to disk. Atomic writes (temp file + rename). Image upload with content type detection. |
| TemplateService | Load and render Go HTML templates. 30+ custom template functions. Graceful shutdown. |
| SEOService | Generate Open Graph, Twitter Card, Schema.org, and canonical URL metadata |
| EmailService | SMTP delivery for contact form submissions |
| LoggingService | Structured logging via slog |

### Content Type Inference

Three content types, inferred from what you write:

```
Explicit `type` in frontmatter → wins always
Has `link_url` field          → link
No title, under 100 words     → thought
Everything else               → article
```

Rules live in `internal/services/article/inference.go`. You never pick a type — you just write.

---

## Frontend

### SPA Router

Turbo Drive pattern. No client-side rendering — the server returns full HTML pages, and the router swaps the `<main>` element.

```
Click link → fetch full HTML → DOMParser → swap <main>
  → update <title> and meta tags
  → push history state
  → load/unload page modules
  → announce route change (aria-live)
  → focus <main> element
```

Prefetch on hover (65ms delay, max 5 cached, 30s expiry). CSS-only progress bar with `prefers-reduced-motion` support. Redirects detected via `response.redirected`.

### ES Modules

No build step. Vanilla ES modules loaded via `<script type="module">`.

**Entry point:** `app.js` orchestrates three module types:

| Type | Lifecycle | Examples |
|------|-----------|---------|
| Shell modules | Load once at startup | router, navigation, theme, scroll, login, toast, fab, compose-sheet, popover, search-popover, subscribe-popover |
| Content modules | Re-run after each page swap | highlight, lazy, clipboard |
| Page modules | Load/unload per template | search-page, contact, compose, admin, drafts |

Page modules are dynamically imported based on `data-template` attribute. Each exports `init()` and optionally `destroy()`.

### Service Worker

Three-tier caching strategy:

| Tier | Strategy | What |
|------|----------|------|
| Precache | Cache on install | `offline.html` |
| Static | Stale-while-revalidate | CSS, JS, images, fonts |
| Content | Network-first | HTML pages |

Network-only routes (never cached): admin, compose, login, logout, feeds, API.

Offline compose queue: IndexedDB (`markgo` database, `compose-queue` store). Queued posts auto-sync when the browser comes back online.

### CSS

Mobile-first with design tokens. All colors, spacing, typography, and shadows defined as CSS custom properties in `main.css :root`.

- **Base**: 320px
- **Phone+**: 481px
- **Tablet+**: 769px

Dark mode via dual-selector pattern: system preference + manual toggle stored in `localStorage`. Five color presets via `data-color-theme` attribute. Three style themes (minimal, editorial, bold) via additional CSS files.

All CSS loaded unconditionally for SPA (scoped by body class). Total: ~3KB gzipped.

### Templates

Go `html/template` with a base layout. Template name drives body class, conditional CSS, and head/content blocks via `$tpl := .template`.

17 templates total. Required templates validated at startup in `setupTemplates()`.

---

## CLI

```
markgo serve     # Start the web server (default if no command given)
markgo init      # Initialize a new blog (creates .env, articles/, etc.)
markgo new       # Create a new article (supports --title, --tags, --type)
markgo version   # Show version information
```

---

## Project Structure

```
markgo/
├── cmd/markgo/main.go           # CLI entry point, subcommand routing
├── internal/
│   ├── commands/
│   │   ├── serve/command.go     # Server setup, route registration
│   │   ├── init/                # Blog initialization
│   │   └── new/                 # Article creation
│   ├── handlers/
│   │   ├── router.go            # Router struct, holds all 11 handler types
│   │   ├── base.go              # BaseHandler (shared config, logger, templates)
│   │   └── *.go                 # One file per handler type
│   ├── middleware/               # Rate limiting, CORS, security, auth, CSRF
│   ├── services/
│   │   ├── article/             # Article loading, caching, search, inference
│   │   ├── feed/                # RSS, JSON Feed, sitemap generation
│   │   ├── compose/             # File-writing compose service
│   │   ├── seo/                 # SEO metadata generation
│   │   ├── template.go          # Template service with custom FuncMap
│   │   ├── email.go             # SMTP email service
│   │   └── logging.go           # Structured logging
│   ├── models/                  # Article, Pagination, ContactMessage
│   ├── config/                  # .env loading and validation
│   ├── errors/                  # Typed error system
│   └── constants/               # Build-time ldflags (version, commit)
├── web/
│   ├── static/
│   │   ├── css/                 # 20 CSS files + 1 theme, mobile-first tokens
│   │   ├── js/                  # ES modules: app.js + modules/ + page modules
│   │   ├── sw.js                # Service Worker
│   │   └── img/                 # Favicons, PWA icons
│   └── templates/               # 17 Go HTML templates
├── articles/                    # Markdown files (the content)
├── deployments/                 # Dockerfile, docker-compose, systemd unit
└── docs/                        # This documentation
```

---

## Performance

| Metric | Value |
|--------|-------|
| Startup | < 1 second |
| Memory | ~30MB typical |
| Binary | ~29MB |
| Cached response | < 5ms |
| Uncached response | < 50ms |
| Throughput | 1000+ req/s (single core) |

Stateless design. Horizontal scaling via load balancer if needed.

---

## Security

- **Authentication**: Session-based (cookie, 7-day expiry, HttpOnly, SameSite=Strict)
- **CSRF**: Double-submit cookie on login, compose, and edit routes (1-hour token, constant-time compare)
- **Input validation**: Slug regex with length limits, sanitized user input
- **XSS protection**: Go html/template auto-escaping, no innerHTML in JS (DOM API only)
- **Headers**: X-Content-Type-Options, X-Frame-Options, Referrer-Policy
- **Rate limiting**: Sliding window per IP (general + stricter contact limit)

---

## Testing

Tests alongside source (`*_test.go`). Coverage ~52% (CI threshold: 45%).

- `testify` for assertions
- `httptest` for handler tests
- Mock interfaces for service isolation (canned data, not reimplemented business logic)
- Race detector: `make test-race`

---

*See [configuration.md](configuration.md) for all environment variables, [api.md](api.md) for the full route reference, and [deployment.md](deployment.md) for production setup.*
