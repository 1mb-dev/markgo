# Manual Acceptance — Client-Side UX

These features are pure browser behavior (JS/CSS/DOM) with no server-side test
seam, so they're verified by browser observation rather than Go tests. A CI
Playwright suite is intentionally **not** committed: browser E2E is the flakiest
tier and would rot on a single-author, no-JS-build project — the cost/benefit
doesn't justify the toolchain. Instead, run the checks below before a release
that touches the theme popover, toast, offline handling, or article CSS.

## How to run (session acceptance)

```bash
# Build and serve from embedded assets (overlay off so you test the shipped CSS/JS)
make build
PORT=8906 ARTICLES_PATH=./articles STATIC_PATH=/tmp/none ./build/markgo serve
```

Drive it with any browser, or a throwaway Playwright script reading computed
styles / `localStorage` / `dataset` (the assertions below are what to check).
Nothing Playwright is committed; install it ad hoc if you want to automate.

## Reader font-size (theme popover → Text size)

- [ ] Default (no `localStorage.fontSize`): `<html>` has **no** `data-font-size`;
      article prose renders at its normal size (no regression vs. previous release).
- [ ] Click **A+ (Large)**: `<html data-font-size="l"]`, `localStorage.fontSize==="l"`,
      the button's `aria-checked==="true"`, and `.article-content` prose grows ~15%.
- [ ] Click **A− (Small)**: `data-font-size="s"`, prose shrinks ~10%.
- [ ] Click **A (Medium)**: `data-font-size` attribute removed, `fontSize==="m"`,
      prose back to default.
- [ ] **Chrome is unaffected**: nav / header / feed-card text does not change at any size.
- [ ] **Persistence**: set Large, reload — prose is large immediately (no flash of
      default size; the `<head>` FOUC script applies it before paint).
- [ ] Browser zoom and OS font size still compound on top (reading text stays `rem`-based).

## Offline indicator

- [ ] Go offline mid-session → "You are offline" toast appears; back online →
      it dismisses and "Back online" shows.
- [ ] **Load while already offline** (offline, then reload a cached page) → the
      "You are offline" toast appears on load (not only on transition).

## Publish orientation

- [ ] Quick-capture → Publish (signed in): success toast shows **"View post →"**;
      the toast does not auto-dismiss; the link opens the published post.
- [ ] Save Draft: success toast has **no** "View post" link (drafts have no public URL).

## Guardrails (don't regress)

- Editing the `<head>` FOUC inline script changes its CSP hash —
  `TestSecurity_FOUCScriptHashMatches` fails until `foucScriptHash` is updated.
- `theme`, `colorTheme`, `fontSize` are the only **bare** localStorage keys (read
  by the pre-render FOUC script); everything else is namespaced via `key()`.
