/**
 * Lazy-loads highlight.min.js — a classic (non-module) global script that sets
 * window.hljs. It can't be import()ed (not an ES module), so we inject a <script>
 * and await its load. The promise is cached, so repeated calls — across SPA
 * navigations, or from both highlight.js and compose.js — fetch at most once.
 */

let loadPromise = null;

/** True when the current document has at least one code block to highlight. */
export function hasCodeBlocks() {
    return document.querySelector('pre code') !== null;
}

/**
 * Resolves to window.hljs, injecting highlight.min.js on first call. Resolves to
 * null if the script fails to load — callers no-op rather than throw. On failure
 * the cache is cleared so a later navigation can retry: a transient network blip
 * shouldn't permanently disable highlighting, and a genuinely missing asset just
 * re-attempts cheaply on each code page (acceptable for a blog).
 */
export function loadHighlighter() {
    if (loadPromise) return loadPromise;

    loadPromise = new Promise((resolve) => {
        if (typeof window.hljs !== 'undefined') {
            resolve(window.hljs);
            return;
        }
        const script = document.createElement('script');
        script.src = '/static/js/highlight.min.js';
        script.onload = () => resolve(window.hljs ?? null);
        script.onerror = () => {
            loadPromise = null; // allow retry on a later navigation
            resolve(null);
        };
        document.head.appendChild(script);
    });

    return loadPromise;
}
