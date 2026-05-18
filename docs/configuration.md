# Configuration

> All settings are environment variables, loaded from `.env` at startup.

---

## Quick Start

```bash
cp .env.example .env
```

Edit the three settings that matter most:

```bash
BLOG_TITLE=My Blog
BLOG_AUTHOR=Your Name
BASE_URL=http://localhost:3000
```

Everything else has sensible defaults for development.

---

## Core

| Variable | Default | Description |
|----------|---------|-------------|
| `ENVIRONMENT` | `development` | `development`, `production`, or `test`. Controls debug routes, rate limit defaults, and Gin mode. |
| `PORT` | `3000` | Server port (1-65535). |
| `BASE_URL` | `http://localhost:3000` | Full URL with protocol. Used for feeds, sitemaps, and social metadata. |

## Paths

| Variable | Default | Description |
|----------|---------|-------------|
| `ARTICLES_PATH` | `./articles` | Directory containing markdown files. |
| `STATIC_PATH` | *(empty)* | Overlay directory for static assets. When set and the directory exists, each request checks this path first and falls back to embedded assets per file. When unset or missing, all assets serve from the embedded FS. Set `LOG_LEVEL=debug` to log each overlay hit. Use atomic writes (write to a temp file, then `mv`) for in-place updates to avoid serving partial content. |
| `TEMPLATES_PATH` | *(empty)* | HTML templates directory. Optional — falls back to embedded templates if unset or missing. |

## Upload

| Variable | Default | Description |
|----------|---------|-------------|
| `UPLOAD_PATH` | `./uploads` | Directory for slug-scoped file uploads. Created at startup if missing. |
| `UPLOAD_MAX_SIZE` | `10485760` | Maximum upload file size in bytes (default 10MB, max 100MB). |

## Blog

| Variable | Default | Description |
|----------|---------|-------------|
| `BLOG_TITLE` | `Your Blog Title` | Displayed in header, feeds, and metadata. |
| `BLOG_TAGLINE` | *(empty)* | Short tagline under the title. Falls back to `BLOG_DESCRIPTION`. |
| `BLOG_DESCRIPTION` | `Your blog description goes here` | Used in feeds, footer, and SEO. |
| `BLOG_AUTHOR` | `Your Name` | Author name for articles and feeds. |
| `BLOG_AUTHOR_EMAIL` | `your.email@example.com` | Author email for feeds. |
| `BLOG_LANGUAGE` | `en` | ISO 639-1 code (e.g., `en`, `en-US`, `fr`). |
| `BLOG_THEME` | `default` | Color theme name. |
| `BLOG_STYLE` | `minimal` | CSS style theme: `minimal`, `editorial`, or `bold`. |
| `BLOG_POSTS_PER_PAGE` | `10` | Items per page in listings (1-100). |

## About Page

All fields are optional. The about page adapts to what's configured.

| Variable | Default | Description |
|----------|---------|-------------|
| `ABOUT_AVATAR` | *(empty)* | Image path relative to static dir (e.g., `img/avatar.jpg`). Falls back to CSS initials circle. |
| `ABOUT_TAGLINE` | *(empty)* | One-liner displayed under your name. |
| `ABOUT_BIO` | *(empty)* | Short bio in markdown. Alternative: create `articles/about.md` (preferred). |
| `ABOUT_LOCATION` | *(empty)* | Location text (e.g., "San Francisco, CA"). |
| `ABOUT_GITHUB` | *(empty)* | GitHub username or full URL. |
| `ABOUT_TWITTER` | *(empty)* | Twitter handle or full URL. |
| `ABOUT_LINKEDIN` | *(empty)* | Full LinkedIn profile URL. |
| `ABOUT_MASTODON` | *(empty)* | Full Mastodon profile URL. |
| `ABOUT_WEBSITE` | *(empty)* | Personal website URL. |

## Branding

Visual identity surfaces are layered on top of the `STATIC_PATH` overlay (see [Paths](#paths)). Each is optional — drop the named file into `<STATIC_PATH>/img/` to override; absent files keep the embedded default.

### Custom brand logo (v3.12.0+)

Drop your SVG at `<STATIC_PATH>/img/brand-logo.svg`. markgo reads it at startup and inlines it in the header.

**Starter SVG** — uses CSS variables so the logo follows the active theme (light/dark/preset):

```svg
<svg viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
  <ellipse cx="32" cy="32" rx="22" ry="28" fill="var(--color-primary)"/>
  <path d="M 32 4 C 22 14 22 24 32 32 C 42 40 42 50 32 60"
        stroke="var(--color-bg-primary)" stroke-width="3.5"
        stroke-linecap="round" fill="none"/>
</svg>
```

**Validation contract.** Your SVG must be well-formed XML with an `<svg>` root element and ≤ 32 KiB. If your `<svg>` lacks a `class` attribute, markgo injects `class="brand-logo"` so the existing CSS sizing rules apply. If `class` is present (even with a custom value), markgo leaves it alone.

**On failure.** Malformed XML, oversize file, or wrong root → markgo logs a single warning and falls back to the embedded default. Missing file → silent fallback.

**Restart required.** STATIC_PATH overlay reads happen at startup; restart markgo after swapping the logo.

**Theme reactivity.** Use `var(--color-primary)` and `var(--color-bg-primary)` for fills and strokes if you want your logo to track the active theme. Hardcoded colors render unchanged regardless of theme.

**Trust model.** The operator-supplied SVG is inlined as-is into the rendered page. markgo treats your filesystem as a trusted boundary — same as STATIC_PATH-overlaid CSS and JS. Do not place untrusted content under STATIC_PATH.

### Other overlay-eligible brand assets

| File | Purpose |
|------|---------|
| `<STATIC_PATH>/img/favicon.svg` | Primary favicon (vector) |
| `<STATIC_PATH>/img/favicon-32x32.png` | PNG fallback favicon |
| `<STATIC_PATH>/img/apple-touch-icon.png` | iOS home-screen icon (180×180) |
| `<STATIC_PATH>/img/icon-192x192.png` | PWA icon |
| `<STATIC_PATH>/img/icon-512x512.png` | PWA icon (high-res) |
| `<STATIC_PATH>/img/og-default.png` | Default Open Graph card |
| `<STATIC_PATH>/img/og-article-default.png` | Per-article OG fallback |
| `<STATIC_PATH>/fonts/...` | Custom web fonts (referenced via `@font-face` in CSS) |
| `<STATIC_PATH>/css/<BLOG_STYLE>.css` | Custom theme stylesheet (any `BLOG_STYLE` name accepted) |

## Pages (v3.13.0+)

Pages are for evergreen content that shouldn't live in the writing feed — a "Run your own?", a `/now`, a `/credits`. They live at `/p/<slug>` rather than `/writing/<slug>`, and they're excluded from the writing index, RSS, JSON Feed, tag and category indexes. Search still finds them by content.

### Authoring a page

Drop a markdown file in `articles/` with `type: page` in the frontmatter:

```markdown
---
title: Run your own
type: page
slug: run-your-own
---

Quick guide to running your own log...
```

`type: page` must be explicit. Unlike article/thought/link, the page type is never inferred.

The slug determines the URL: `/p/run-your-own` in this example. Pages support the same frontmatter fields as articles (`banner`, `banner_alt`, `description`, etc.).

### Behavior

- **URL:** `/p/<slug>` is canonical. Legacy `/writing/<slug>` requests for `type: page` articles 301-redirect to `/p/<slug>`.
- **Feed exclusion:** pages do not appear in `/writing`, RSS (`/feed.xml`), JSON Feed (`/feed.json`), `/tags/<tag>`, or `/categories/<cat>`.
- **Sitemap:** pages are emitted in `sitemap.xml` with their canonical `/p/<slug>` URLs, alongside a static `/p` index entry. `/about` is also included via the same predicate-aware logic (v3.14.0+).
- **Search:** pages remain indexed; readers can find them via `/search?q=...` and follow the result to `/p/<slug>`.
- **Date and "Updated" line:** hidden in the rendered page. Pages are evergreen, not dated.
- **Tags:** rendered on the page itself but do not surface the page in tag indexes.
- **Admin drafts:** drafted pages (`type: page` + `draft: true`) appear in the admin drafts list. Operator publishes the page by removing the draft flag.

### Migrating an article to a page

Change `type: article` → `type: page` in the frontmatter and save. The article disappears from `/writing` and feeds; the URL shifts from `/writing/<slug>` to `/p/<slug>`. Existing inbound links to `/writing/<slug>` automatically 301-redirect, so no link equity is lost.

### What about /about?

`/about` is handled separately — it has its own dedicated handler with config-driven identity chrome (avatar, tagline, bio, social URLs from `.env`). The `articles/about.md` file is the body content. Don't make `/about` a page; keep using the dedicated handler.

### Future enhancements

- `nav_priority` ordering frontmatter (v3.14.0+ deferred; for now pages are listed alphabetically on `/p`)
- Compose form "new page" affordance (v3.14.0+ deferred; for now, author pages by dropping markdown into `articles/` or editing an existing page via `/compose/edit/<slug>`)

## Admin

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_USERNAME` | *(empty)* | Leave empty to disable admin, compose, and login routes entirely. |
| `ADMIN_PASSWORD` | *(empty)* | Required if username is set. Cannot be "changeme". |

When configured, enables: login/logout, compose form, quick capture, admin dashboard, drafts page, cache management, article reload.

Session: cookie-based, 7-day expiry, HttpOnly, SameSite=Strict. CSRF: double-submit cookie, 1-hour token.

## Cache

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_TTL` | `1h` | Time-to-live for cached articles. Go duration format (e.g., `1h`, `30m`, `24h`). |
| `CACHE_MAX_SIZE` | `1000` | Maximum cached items. |
| `CACHE_CLEANUP_INTERVAL` | `10m` | How often expired items are evicted. |

## Email (Contact Form)

Leave `EMAIL_HOST` empty to disable the contact form. When email is not configured, the about page shows a mailto link instead.

| Variable | Default | Description |
|----------|---------|-------------|
| `EMAIL_HOST` | *(empty)* | SMTP server (e.g., `smtp.gmail.com`). |
| `EMAIL_PORT` | `587` | SMTP port. 587 (STARTTLS) recommended. |
| `EMAIL_USERNAME` | *(empty)* | SMTP auth username. |
| `EMAIL_PASSWORD` | *(empty)* | SMTP auth password. |
| `EMAIL_FROM` | `noreply@yourdomain.com` | Sender address. |
| `EMAIL_TO` | `your.email@example.com` | Recipient for contact submissions. |
| `EMAIL_USE_SSL` | `true` | Enable SSL/TLS encryption. |

## Rate Limiting

Defaults are environment-aware: development allows 3000 general requests, production allows 100.

| Variable | Default (prod) | Description |
|----------|----------------|-------------|
| `RATE_LIMIT_GENERAL_REQUESTS` | `100` | Requests per window for public routes. |
| `RATE_LIMIT_GENERAL_WINDOW` | `900s` | Time window (15 minutes). |
| `RATE_LIMIT_CONTACT_REQUESTS` | `5` | Contact form submissions per window. |
| `RATE_LIMIT_CONTACT_WINDOW` | `3600s` | Time window (1 hour). |

Sliding window per IP. Static assets are excluded from rate limiting.

## CORS

| Variable | Default | Description |
|----------|---------|-------------|
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | Comma-separated origins. Be specific in production. |
| `CORS_ALLOWED_METHODS` | `GET,POST,PUT,DELETE,OPTIONS` | Allowed HTTP methods. |
| `CORS_ALLOWED_HEADERS` | `Origin,Content-Type,Accept,Authorization` | Allowed request headers. |

## Server Timeouts

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_READ_TIMEOUT` | `15s` | Max time to read the full request. |
| `SERVER_WRITE_TIMEOUT` | `15s` | Max time to write the response. |
| `SERVER_IDLE_TIMEOUT` | `60s` | Max time waiting for next request (keep-alive). |
| `SHUTDOWN_TIMEOUT` | `30s` | Deadline for graceful shutdown of in-flight HTTP requests on `SIGTERM`/`SIGINT`. Lower values speed up rolling restarts but may abort longer requests; higher values give in-flight work more time. Internal cleanup (sessions, rate-limiters, templates) always runs, regardless of whether the deadline is hit. |

## AMA Copy

Operator-controllable copy on the AMA (Ask Me Anything) submission overlay. All values are plaintext only — `html/template` auto-escapes the values into `<meta>` tags, which `ama-sheet.js` reads via `document.querySelector`. Multi-line values render as a single visual block (HTML whitespace rules); for paragraph breaks override the AMA sheet template via `TEMPLATES_PATH`.

| Variable | Default | Description |
|----------|---------|-------------|
| `AMA_PAGE_HEADING` | `Ask me anything` | Heading shown at the top of the AMA sheet AND in the `/about` reach section (v3.14.0+). Setting this empty in `.env` falls back to the default — hiding the AMA half requires overriding `about.html` via `TEMPLATES_PATH`. |
| `AMA_PAGE_INTRO` | `Curious about something? Submit a question and I'll answer it publicly.` | Intro paragraph rendered between the heading and the form on both the AMA sheet and the `/about` reach section (v3.14.0+). |
| `AMA_FORM_PLACEHOLDER` | `What would you like to know?` | Placeholder for the question textarea. |
| `AMA_SUBMIT_LABEL` | `Submit Question` | Label on the submit button on both the AMA sheet and the `/about` reach section (v3.14.0+). |
| `AMA_THANKYOU_COPY` | `Question submitted! It will appear once answered.` | Toast shown after a successful submission. |

## Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `LOG_FORMAT` | `json` | `json` (production) or `text` (development). |
| `LOG_OUTPUT` | `stdout` | `stdout`, `stderr`, or `file`. |
| `LOG_FILE` | *(empty)* | Required when `LOG_OUTPUT=file`. |
| `LOG_MAX_SIZE` | `100` | Max log file size in MB before rotation. |
| `LOG_MAX_BACKUPS` | `3` | Rotated log files to keep. |
| `LOG_MAX_AGE` | `28` | Max days to keep old log files. |
| `LOG_COMPRESS` | `true` | Compress rotated files. |
| `LOG_ADD_SOURCE` | `false` | Include source file:line in log entries. |
| `LOG_TIME_FORMAT` | `2006-01-02T15:04:05Z07:00` | Time format for text logs (Go format). |

## SEO

| Variable | Default | Description |
|----------|---------|-------------|
| `SEO_ENABLED` | `true` | Master toggle for all SEO features. |
| `SEO_SITEMAP_ENABLED` | `true` | Generate `/sitemap.xml`. |
| `SEO_SCHEMA_ENABLED` | `true` | JSON-LD structured data. |
| `SEO_OPEN_GRAPH_ENABLED` | `true` | Open Graph meta tags. |
| `SEO_TWITTER_CARD_ENABLED` | `true` | Twitter Card meta tags. |
| `SEO_ROBOTS_ALLOWED` | `/` | Comma-separated allowed paths for robots.txt. |
| `SEO_ROBOTS_DISALLOWED` | `/admin,/api` | Comma-separated disallowed paths. |
| `SEO_ROBOTS_CRAWL_DELAY` | `1` | Crawl delay in seconds. |
| `SEO_DEFAULT_IMAGE` | *(empty)* | Default image for social sharing. |
| `SEO_TWITTER_SITE` | *(empty)* | Twitter @handle for site. |
| `SEO_TWITTER_CREATOR` | *(empty)* | Twitter @handle for author. |
| `SEO_FACEBOOK_APP_ID` | *(empty)* | Facebook App ID for insights. |
| `SEO_GOOGLE_SITE_VERIFY` | *(empty)* | Google Search Console verification. |
| `SEO_BING_SITE_VERIFY` | *(empty)* | Bing Webmaster verification. |

## Production Checklist

```bash
ENVIRONMENT=production
BASE_URL=https://yourdomain.com

# Strong credentials or disable admin entirely
ADMIN_USERNAME=your-admin-user
ADMIN_PASSWORD=a-strong-unique-password

# Specific CORS origins
CORS_ALLOWED_ORIGINS=https://yourdomain.com

# File logging with rotation
LOG_LEVEL=warn
LOG_OUTPUT=file
LOG_FILE=/var/log/markgo/app.log

# Longer cache TTL
CACHE_TTL=24h
```
