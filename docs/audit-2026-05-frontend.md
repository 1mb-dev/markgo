# Frontend Audit — 2026-05 (v3.16.0 scoping pass)

Index of frontend-surface findings deferred from the v3.16.0 audit cycle. Each item is filed as a separate GitHub issue with the `audit-finding` label; this doc is the consolidated entry point. Items get closed in subsequent cycles when their issues land.

The execution scope for v3.16.0 itself was: security headers (CSP/HSTS/Permissions-Policy + X-XSS-Protection drop), Pages-demo retire (README → log.1mb.dev + operator actions A1/A2), and this audit doc.

## Findings

| # | Finding | Severity | Location | Suggested direction | Issue |
|---|---------|----------|----------|---------------------|-------|
| 1 | `innerHTML =` on draft card rendering | Medium | `web/static/js/modules/compose-sheet.js:376,389` | Audit each call site for source trust; if any user-rendered content reaches the assignment, switch to `textContent` + DOM builder helper. Even when source is trusted, prefer explicit construction for grep-ability. | TBD |
| 2 | Vestigial CSS file | Low | `web/static/css/articles.css` (5 lines) | Verify zero callers (`grep articles.css web/`), delete if confirmed dead. | TBD |
| 3 | Compose flow accretion | Medium | `compose.js` (485) + `compose-sheet.js` (556) = 1,041 lines | State-machine cleanup: modes (page/article/AMA/edit/quick) branch on multiple `data-*` attrs (mode, type, link-url, banner, banner-path-form). Consider a single resolved-state read at sheet open, then mode-specific branches operate on that snapshot. Likely v3.17+ dedicated focus area. | TBD |
| 4 | `<link rel="modulepreload">` opportunities | Low | `web/templates/base.html` head | Per-template dynamic-import modules (compose, search-page, contact, admin, drafts) could preload conditionally based on current template. Shaves INP on first interaction. Measure before optimizing. | TBD |
| 5 | Reading-progress affordance for long-form articles | Low | `web/static/css/article.css`, `web/static/js/app.js` | Calm-design fork: scroll-progress bar at top of article view. Common blog UX, defensible per "minimal chrome" guideline. Considered noise by some readers; gate behind a (potentially) operator-toggleable behavior. | TBD |
| 6 | Dark-mode AA contrast spot-check across color presets | Low | `web/static/css/main.css :root` | v3.10.1 lifted contrast for berry/ocean/forest/sunset but no comprehensive audit since. Run an automated contrast-check (axe-core, pa11y) across all 5 presets in both light + dark modes. Document deltas, fix outliers. | TBD |
| 7 | Service Worker `CACHE_VERSION` is manually bumped | Low | `web/static/sw.js:11` (`const CACHE_VERSION = 7`) | Inject from build version via ldflags-style substitution at startup (sw.js is served by `serveSwJs` handler — opportunity to template-substitute). Risk of forgotten bumps is real (any client-side cache change must invalidate, and humans forget). | TBD |
| 8 | Cmd-K command palette | Low | new | Modern blog UX expectation (Astro, Hugo themes). Search-popover already exists at `/static/js/modules/search-popover.js` — a Cmd-K palette would extend with navigation actions (go to /writing, /tags, etc.) + recent articles. v3.17+ candidate. | TBD |
| 9 | `<meta>` proliferation audit | Low | `web/templates/base.html` (62 meta tags) | Walk every meta tag, verify each has a present-day consumer (browser, crawler, JS). msapplication-* tags target legacy IE/Edge; some may be droppable. apple-touch-icon variations — keep all (no harm) or trim to modern sizes (60/76/120/152). Apple favicons reference `vnykmshr.github.io/markgo/static/img/...` — likely stale post-1mb-dev-migration; verify or replace with `{{ .config.BaseURL }}` resolution. | TBD |
| 10 | PWA install prompt UX | Low | `web/static/js/app.js` (no install handler) | No visible "install" affordance. Could add a one-time toast on `beforeinstallprompt`, or leave it browser-native. Defensible either way; surface to operators in docs at minimum. | TBD |
| 11 | `highlight.min.js` bundle trim | Low | `web/static/js/highlight.min.js` (1,212 lines, vendored full distribution) | Bundle ships every language. Most blogs use ~5. Trim to subset via highlight.js's custom-build tool, OR migrate to CSS-only `<pre><code>` styling (lose syntax color, gain ~50KB). v3.17+ candidate. | TBD |

## Filing protocol

Each item above gets a GitHub issue with:

- Title: `audit-finding: <short subject>`
- Label: `audit-finding`
- Body: the row content from this doc + any additional context discovered while filing.
- After filing, the `Issue` column above is updated with `#N` linking back.

Items are not blockers for v3.16.0 merge — they are scoping inputs for v3.17.0+ planning cycles.
