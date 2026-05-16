/**
 * Blog Storage — namespacing for engine-owned client storage.
 *
 * The server computes a per-install namespace from Config.BaseURL and exposes
 * it via <meta name="markgo-storage-namespace" content="markgo:slug">. All
 * `markgo:`-prefixed localStorage keys and the IndexedDB DB name derive from
 * `NS` here, so same-origin path-mounted MarkGo installs don't collide on
 * compose drafts, install-banner state, or offline post queues.
 *
 * Out of scope: `theme` and `colorTheme` localStorage keys are intentionally
 * bare so the FOUC inline script in base.html can read them before any
 * module loads. They share across path-mounted instances by design.
 *
 * Update this module and `storageNamespace` in internal/services/template.go
 * in sync — they're two ends of the same contract.
 */

const NS_META = 'markgo-storage-namespace';
const NAME_META = 'application-name';
const FALLBACK_NS = 'markgo:default';

export const NS = document.querySelector(`meta[name="${NS_META}"]`)?.content || FALLBACK_NS;
export const BLOG_TITLE = document.querySelector(`meta[name="${NAME_META}"]`)?.content || '';
export const key = (suffix) => `${NS}:${suffix}`;
export const dbName = NS;
