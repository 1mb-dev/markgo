/**
 * Blog Storage — single source of truth for client-side storage namespacing.
 *
 * The server computes a per-install namespace from Config.BaseURL and exposes
 * it via <meta name="markgo-storage-namespace" content="markgo:slug">. This
 * module is the sole consumer; all localStorage keys and the IndexedDB name
 * derive from `NS` so same-origin path-mounted MarkGo installs don't collide.
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
