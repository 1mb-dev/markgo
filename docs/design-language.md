# MarkGo Design Language

**Version:** 1.0.0 · **Last updated:** 2026-06-10

The living source of truth for MarkGo's UI decisions. Design tokens live in
`web/static/css/main.css :root`; this doc captures the *why* and the rules that
keep the interface coherent. Update it when a token changes or a rule is added.

## Posture

Mobile-first, calm, content-first. MarkGo is read and written mostly on phones;
the desktop layout is the enhancement, not the baseline. The UI should respect
attention (no engagement bait, no motion for its own sake) and get out of the
way of reading and quick capture.

**Breakpoints:** 320px base → 481px (phone+) → 769px (tablet+). Author the 320px
layout first; widen with `min-width` queries.

## Principles

1. **Mobile-first, always** — 320px is the design target, not an afterthought.
2. **Clarity over cleverness** — obvious beats slick.
3. **Calm** — attention-respecting; the content is the interface.
4. **Tokens are the source of truth** — no hardcoded colors, sizes, or spacing in
   components; reach for a `:root` token.
5. **Accessible by default** — WCAG AA is the floor, AAA where cheap.

## Tokens

Defined in `main.css :root`. Highlights (full set in the file):

| Group | Tokens |
|-------|--------|
| Type scale | `--font-size-xs … --font-size-5xl` (rem-based; reader scale via `--reading-scale`, v3.26.0) |
| Spacing | `--spacing-1 … --spacing-12` (4px base) |
| Layout | `--max-content-width: 42rem` (the reading measure — fixed; do not scale) |
| Touch | `--tap-target: 2.75rem` (44px), `--tap-target-min: 1.5rem` (24px) |
| Also | color family (`data-color-theme`), dark mode (`data-theme`), radius, shadow, z-index |

## Rules

### Touch targets (v3.27.0)

- **Primary nav/action controls meet `--tap-target` (44px)** — WCAG 2.5.5 (AAA)
  and mobile comfort. Applied via `min-block-size` + flex centering. Examples:
  nav-action buttons, feed filters, tag-cloud items, page back-link, search-clear,
  feed-card permalink, the theme-popover segmented buttons, the article pager.
- **Dense secondary controls may use `--tap-target-min` (24px)** — WCAG 2.5.8 (AA)
  — *only* with adequate separation. Examples: color swatches, metadata tag
  chips. Where a single-axis expansion is free (e.g. a swatch row), expand the
  hit-area on that axis toward 44px without growing the visual (see the
  `.color-swatch::after` vertical hit-area).
- **Inline text links are exempt** (WCAG 2.5.8 inline exception): breadcrumbs,
  links inside prose, footer link lists. Forcing 44px there bloats text layout.

### Horizontal overflow

**Nothing may scroll the page sideways.** A row that can exceed the viewport
**wraps** (`flex-wrap: wrap`) — preferred, since it keeps options visible and
avoids a scroll affordance (MarkGo ships no custom scrollbars) — or contains its
own scroll. Verified at 320px: `document.scrollWidth ≤ clientWidth` on every
page (see `docs/manual-acceptance.md`).

### Constraints (what we don't do)

No custom scrollbars · no auto-playing media · no motion > 500ms · no hover-only
interactions (mobile) · no hardcoded design values (use tokens) · reading text
stays `rem`-based so browser zoom/font settings compose.

## Decision log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-06-10 | `--tap-target` 44 / `--tap-target-min` 24 tokens; tiered standard | A 320px audit found ad-hoc tap sizing (24–40px) because no token existed; codify so it stops drifting. |
| 2026-06-10 | Wrap `.feed-filters`, don't scroll-chip | Only page-overflow at 320px; wrap keeps all filters visible + honors no-custom-scrollbars. |
| 2026-06-10 | Authed UI (compose/admin) tap-target adoption deferred | v3.27.0 hardened the audited public surfaces; compose/admin buttons (40px) adopt `--tap-target` in a later pass. |
| 2026-06-09 | Reader font-size scales prose only (not root) | Root scaling would widen the rem-based 42rem measure; container token-redefine keeps the measure fixed. |
| 2026-06-09 | `theme`/`colorTheme`/`fontSize` are bare localStorage keys | Read by the `<head>` FOUC script before modules load. |
