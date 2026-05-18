# MarkGo

[![CI](https://github.com/1mb-dev/markgo/actions/workflows/ci.yml/badge.svg)](https://github.com/1mb-dev/markgo/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26.0+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A blog engine where you write first and categorize never.

Type two sentences without a title and it becomes a thought. Paste a URL with commentary and it becomes a link. Write something long with a title and it becomes an article. Ask a question and it becomes an AMA. Plus a fifth type — page — for evergreen content (explicit `type: page` in frontmatter, served at `/p/<slug>`, listed at `/p`). Four inferred, one explicit. No database. No build step.

## Quick Start

```bash
git clone https://github.com/1mb-dev/markgo && cd markgo
make build

./build/markgo init --quick
./build/markgo serve
# Visit http://localhost:3000
```

Or download a release from [GitHub Releases](https://github.com/1mb-dev/markgo/releases).

## Live Install

[log.1mb.dev](https://log.1mb.dev) — running latest release (may briefly lag during binary-pull cycles).

## What You Get

**Write from anywhere** — CLI for drafting in your editor, web compose form for publishing from your phone. Quick capture: tap the FAB, type a thought, hit Publish. Under 5 seconds.

**SPA navigation** — Instant page transitions. The router fetches HTML and swaps content — no full page reloads, no client-side rendering framework.

**Works offline** — Installable PWA with Service Worker. Pages cached for offline reading. Compose queue syncs when you're back online.

**Mobile-native feel** — Bottom navigation, full-screen search overlay, visual viewport handling for iOS keyboards. CSS starts at 320px and works up.

**Your files, your control** — Articles are markdown files with YAML frontmatter. Edit in vim, version with git, back up however you like. The compose form writes files to disk — it's a convenience layer, not a lock-in.

**Zero dependencies** — Single Go binary with embedded web assets. No Node.js, no PHP, no database. `markgo init` creates only your content directory and config.

## Usage

```bash
markgo serve                             # Start the web server
markgo init [--quick]                    # Initialize a new blog
markgo new [--title "..." --tags "..."]  # Create an article
markgo new --type thought                # Quick thought (no title needed)
markgo new --type link                   # Share a link
markgo new --type ama --title "Why X?"   # AMA question (admin moderated)
```

## Configuration

Copy `.env.example` to `.env`:

```bash
BLOG_TITLE="Your Blog Title"
BLOG_AUTHOR="Your Name"
BASE_URL="https://yourdomain.com"
```

Admin credentials enable the compose form, admin panel, and login:

```bash
ADMIN_USERNAME=you
ADMIN_PASSWORD=something-strong
```

See [Configuration Guide](docs/configuration.md) for all options.

## Deploy

**Docker:**
```bash
docker compose up -d
```

See [Deployment Guide](docs/deployment.md) for systemd, reverse proxy, and production setup.

## Development

```bash
make dev             # Dev server with hot reload
make build           # Build binary
make lint            # golangci-lint
make test            # Run tests
make test-race       # Race detector
make coverage        # Coverage report
```

## Documentation

- [Getting Started](docs/GETTING-STARTED.md) — Install, first post, and features overview
- [Configuration](docs/configuration.md) — All environment variables
- [Architecture](docs/architecture.md) — How the system works
- [API](docs/API.md) — Every HTTP endpoint
- [Deployment](docs/deployment.md) — Docker, systemd, reverse proxy
- [Design Language](docs/design.md) — The principles behind every decision

## License

MIT License — see [LICENSE](LICENSE).
