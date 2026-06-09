# Performance Baseline — v3.23.0 perf+UX arc

Establishes before/after numbers for the performance arc (R1–R3). **Non-gating**: this
documents success criteria, it does not block any release. Captured first (on `main`) so
the "after" comparison is real.

## Method

- **Deterministic deltas** (below, "Static asset load"): static analysis of what each page
  ships — asset classes, byte sizes, render-blocking count, eager vs on-demand. Reproducible
  with `grep`/`wc`; no browser needed. This is the load-bearing evidence for R1's claims.
- **Field/lab CWV**: Lighthouse mobile preset against a running instance. Prefer log.1mb.dev
  (real edge: Caddy + HTTP/2 + compression). Local fallback: `make build && ./build/markgo serve`
  then Lighthouse CLI, or a Playwright `performance.getEntriesByType('resource')` trace for
  transfer/byte/request deltas (markgo has no JS test framework, so Playwright is also the
  acceptance path — see R1 plan). Clear SW registrations + caches before each run.
- **URLs**: home feed `/`, a text thought (no code), an article with code blocks, search `/search`.
- **Metrics**: FCP, LCP, TBT, CLS, total transfer, request count, JS bytes, font bytes.

## Static asset load (deterministic)

Measured on `main` @ v3.22.5 (2026-06-09).

| Asset class | Before (main) | After (v3.23.0 target) |
|---|---|---|
| Render-blocking CSS | 23 `<link>` (22 files + conditional theme), 136 KB | unchanged in R1 (concat/minify deferred to R2, measurement-gated) |
| `highlight.min.js` | **every page**, 122 KB | **code pages only** (lazy, on `pre code` presence) |
| `app.js` (ESM, implicitly deferred) | every page, 8 KB | unchanged |
| Inter woff2 (body, always used) | loaded, **not preloaded**, 230 KB | **preloaded** (`<link rel=preload crossorigin>`) |
| Fira Code woff2 (code only) | lazy via `font-display:swap`, 103 KB | unchanged (stays lazy — not preloaded) |
| Font preload hints | none | Inter only |
| Prev/next article nav | absent | present (non-page article footers) |

**R1 headline:** a text page (thought/link/feed — the common case) stops shipping the 122 KB
highlight.min.js entirely (transfer + parse), and Inter starts fetching at preload priority
instead of after CSS parse. Code pages are unchanged in bytes but unchanged in correctness.

## Cache headers (R2, v3.24.0)

| Response class | Before (≤v3.23.0) | After (v3.24.0) |
|---|---|---|
| HTML pages | `public, max-age=3600` (stale ≤1h) | `no-cache` + weak ETag → revalidate; `304` when unchanged (session-stable; body includes the `_csrf` token) |
| Static CSS/JS | `public, max-age=3600`, no validator (stale ≤1h after deploy) | `no-cache` + strong build-version ETag (`"<version>"`) → `304` within a version, fresh after a release |
| Static fonts/images | `public, max-age=3600` | unchanged (not deploy-churned) |
| Feeds, sitemap | `public, max-age=3600` | unchanged |
| Admin, compose | `no-cache, no-store` | unchanged |

**R2 headline:** new/edited content is never stale (HTML revalidates), and a deploy's CSS/JS is
picked up on the next request instead of after an arbitrary hour — without URL versioning
(deferred: `?v=`/path-prefix doesn't propagate through ES-module imports, and `immutable`'s
zero-request win is marginal behind Caddy HTTP/2 + the service worker).

## CWV (lab) — to measure

Clear SW caches first. Fill both passes; non-gating.

| URL | Pass | FCP | LCP | TBT | CLS | Transfer | Requests | JS bytes |
|---|---|---|---|---|---|---|---|---|
| `/` (feed) | before | | | | | | | |
| `/` (feed) | after | | | | | | | |
| text thought | before | | | | | | | |
| text thought | after | | | | | | | |
| article w/ code | before | | | | | | | |
| article w/ code | after | | | | | | | |
| `/search` | before | | | | | | | |
| `/search` | after | | | | | | | |

> Note: the flagship runs behind Caddy (HTTP/2 + compression), so transport-tier wins (R3
> compression) won't move flagship CWV much — they serve the bare-binary forker. R1's wins
> (preload, lazy highlight) are application-tier and move CWV for *everyone*.
