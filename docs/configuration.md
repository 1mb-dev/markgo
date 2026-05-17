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
