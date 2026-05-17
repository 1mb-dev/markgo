# Getting Started

> Zero to running blog in under 5 minutes.

---

## Install

**Download a release** from [GitHub Releases](https://github.com/1mb-dev/markgo/releases), or build from source:

```bash
git clone https://github.com/1mb-dev/markgo.git && cd markgo
make build
```

The binary lands at `./build/markgo`.

## Initialize

```bash
markgo init --quick
```

This creates a `.env` config and an `articles/` directory with sample content (an article, a thought, and a link post). Templates and static assets are embedded in the binary — no `web/` directory needed.

## Run

```bash
markgo serve
```

Visit http://localhost:3000. Your blog is live.

---

## Write Something

MarkGo has three content types. You never pick one — the system infers it from what you write.

### From the command line

```bash
# Article — has a title, intended for long-form
markgo new --title "How I Built This" --tags "golang,blogging"

# Thought — no title needed, short-form
markgo new --type thought

# Link — sharing a URL with commentary
markgo new --type link
```

Articles are markdown files in `articles/` with YAML frontmatter:

```markdown
---
title: "How I Built This"
description: "A walkthrough of the architecture"
tags: ["golang", "blogging"]
category: "engineering"
date: 2026-02-12T00:00:00Z
---

Your content here, in standard markdown.
```

### From the browser

If you configure admin credentials in `.env`:

```bash
ADMIN_USERNAME=you
ADMIN_PASSWORD=something-strong
```

Then restart the server. You'll see a Compose link in the nav and a floating action button (FAB) on mobile. The compose form puts the cursor in the content field — not the title, not a category picker. Write first, categorize never.

**Quick capture**: Tap the FAB, type a thought, hit Publish. Under 5 seconds.

**Full compose**: Navigate to `/compose` for the full form with title, tags, markdown preview, and image upload.

**Edit**: Any published post can be edited at `/compose/edit/:slug`.

### Content type inference

You don't choose a type. MarkGo figures it out:

| What you write | What it becomes |
|---|---|
| No title, under 100 words | Thought |
| Has a `link_url` in frontmatter | Link |
| Everything else | Article |

You can override this by setting `type: thought`, `type: link`, or `type: article` in the frontmatter.

### Banner image (essays)

Long-form posts can declare a banner image that renders above the title, drives social-share previews (`og:image`, Twitter cards), and appears in JSON Feed entries:

```yaml
---
title: "How I Built This"
banner: "hero.jpg"
banner_alt: "Workbench with scattered notes"
---
```

`banner` accepts three forms:

- **Relative path** — resolved against `uploads/<slug>/`. Place the image alongside any other slug-scoped assets you uploaded via compose.
- **Server-absolute path** (`/static/...` or any value starting with `/`) — passes through unchanged; the static handler serves the asset. Use this for source-controlled editorial banners shipped under `web/static/` (e.g. `banner: "/static/img/banners/launch.png"`), so frontmatter stays portable across deployments and local preview without coupling to `BASE_URL`.
- **Absolute URL** (`https://...` or `http://...`) — passes through unchanged. Useful when you host images on a CDN.

`banner_alt` is optional; alt text falls back to the article title.

**Validation rules:**

- Bad URL schemes (`javascript:`, `data:`, `file://`) and path-traversal attempts reject the article at load time.
- Server-absolute paths must be canonical (no `..`, `.`, or `//` segments) — non-canonical forms reject the article.
- If `banner` points at a relative path but the file is missing, the article still loads — your browser will show a broken image where the banner should be (the visible failure signal). Fix the typo or upload the file and reload. Server-absolute paths are not checked at load; missing assets also surface as broken images at render time.

**Scope:** Banner renders only on essays (`type: article`). Setting `banner` on thoughts, links, or AMA posts logs a warning at load and is otherwise ignored — those content types have their own visual shape (thoughts are chrome-light, link cards center on the link, AMA centers on the asker).

When no banner is set, the OG image falls back through tiers: first inline image in the post → static default. Setting `banner:` is the only way to control the social card explicitly.

**Setting a banner from the compose form** (v3.11.0+): the compose UI exposes a "Banner image" field above the publish controls. Upload an image and it lands in `uploads/<slug>/` and writes the relative `banner:` line for you. Banners using `/static/...` or absolute URLs are preserved on edit but displayed read-only — to swap them, edit the markdown frontmatter directly. There is no "URL or upload" toggle in the form; the form's upload control owns the `uploads/`-based path only.

---

## Configure

Edit `.env`. The essential settings:

```bash
# Your blog
BLOG_TITLE=My Blog
BLOG_AUTHOR=Your Name
BASE_URL=http://localhost:3000

# For production
ENVIRONMENT=production
BASE_URL=https://yourdomain.com
```

See [configuration.md](configuration.md) for every option.

---

## How It Works

The MarkGo binary embeds all web assets (templates, CSS, JS, images). When you run `markgo init`, it only creates `articles/` and `.env` — no `web/` directory needed.

**Filesystem overrides**: Two env vars unlock filesystem customization without forking. Set `TEMPLATES_PATH` in `.env` to override embedded templates wholesale (replace mode — your directory provides every template). Set `STATIC_PATH` to overlay individual static assets per file (overlay mode, v3.10.2+) — local files win, missing paths fall back to embedded. Drop `<STATIC_PATH>/img/brand-logo.svg` to swap the header logo (v3.12.0+). See `docs/configuration.md#branding` for the full list of overlay-eligible brand assets.

**Content-only structure**: Your blog is just markdown files and a config file. To move to a new server, copy `articles/` and `.env`, then run the binary.

---

## Updating MarkGo

Download the new binary, replace the old one, restart:

```bash
# Stop the server
# Replace the binary
cp markgo-new /usr/local/bin/markgo
# Start the server
markgo serve
```

Your content (`articles/`) and configuration (`.env`) are untouched. Embedded assets update automatically with the binary.

---

## Development

```bash
make dev      # Live reload server at :3000 (requires air)
make build    # Build binary
make test     # Run tests
make lint     # Run linter
```

---

## Deploy

**Docker:**
```bash
docker compose up -d
```

**Manual:**
```bash
make build
scp build/markgo server:/usr/local/bin/
```

See [deployment.md](deployment.md) for reverse proxy setup, systemd, and production configuration.

---

## Features You Get Out of the Box

- **SPA navigation** — Instant page transitions, no full reloads
- **PWA** — Installable, works offline, caches pages for offline reading
- **Quick capture** — FAB on mobile, Cmd/Ctrl+N on desktop
- **Offline compose** — Write when disconnected, auto-syncs when back online
- **Search** — Full-text search across all content
- **Feeds** — RSS (`/feed.xml`), JSON Feed (`/feed.json`), sitemap (`/sitemap.xml`)
- **SEO** — Open Graph, Twitter Cards, Schema.org, canonical URLs
- **Themes** — Light/Dark/Auto mode, 5 color presets, 3 style themes
- **Contact form** — SMTP email delivery with rate limiting
- **Admin panel** — Stats, draft management, cache controls

All of this works without JavaScript for core reading. JS enhances, it doesn't gate.

---

## Next Steps

- [Configuration](configuration.md) — All environment variables
- [Architecture](architecture.md) — How the system works
- [API](api.md) — Every HTTP endpoint
- [Deployment](deployment.md) — Production setup
- [Design Language](design.md) — The principles behind the decisions
