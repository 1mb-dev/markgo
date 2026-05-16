/**
 * MarkGo Service Worker — offline reading + app shell caching.
 *
 * Strategies:
 * - Precache: offline fallback page (installed with SW)
 * - Stale-while-revalidate: static assets (CSS, JS, images, fonts)
 * - Network-first: HTML pages (cached for offline reading, LRU 50)
 * - Network-only: auth routes, compose, admin, feeds, API
 */

const CACHE_VERSION = 7;
const PRECACHE = `markgo-precache-v${CACHE_VERSION}`;
const STATIC_CACHE = `markgo-static-v${CACHE_VERSION}`;
const CONTENT_CACHE = `markgo-content-v${CACHE_VERSION}`;
const CONTENT_MAX_ENTRIES = 50;

const PRECACHE_URLS = ['/offline'];

// Routes that must never be cached (auth-gated, dynamic, or side-effect-bearing)
const NETWORK_ONLY_PREFIXES = [
    '/admin', '/compose', '/login', '/logout',
    '/api/', '/debug/', '/health',
];
const NETWORK_ONLY_EXACT = new Set([
    '/feed.xml', '/feed.json', '/sitemap.xml', '/robots.txt', '/manifest.json',
]);

// ── Install ──────────────────────────────────────────────────────────────────

self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(PRECACHE)
            .then((cache) => cache.addAll(PRECACHE_URLS))
            .then(() => self.skipWaiting())
    );
});

// ── Activate ─────────────────────────────────────────────────────────────────

self.addEventListener('activate', (event) => {
    const currentCaches = new Set([PRECACHE, STATIC_CACHE, CONTENT_CACHE]);
    event.waitUntil(
        caches.keys()
            .then((names) => Promise.all(
                names
                    .filter((name) => name.startsWith('markgo-') && !currentCaches.has(name))
                    .map((name) => caches.delete(name))
            ))
            .then(() => self.clients.claim())
    );
});

// ── Fetch ────────────────────────────────────────────────────────────────────

self.addEventListener('fetch', (event) => {
    const { request } = event;

    // Only handle GET requests
    if (request.method !== 'GET') return;

    const url = new URL(request.url);

    // Only handle same-origin requests
    if (url.origin !== self.location.origin) return;

    const path = url.pathname;

    // Network-only routes (auth, API, feeds)
    if (isNetworkOnly(path)) return;

    // Static assets and uploads → stale-while-revalidate
    if (path.startsWith('/static/') || path.startsWith('/uploads/')) {
        event.respondWith(staleWhileRevalidate(request, STATIC_CACHE));
        return;
    }

    // Everything else (HTML pages) → network-first with offline fallback
    event.respondWith(networkFirstWithOffline(request));
});

// ── Strategies ───────────────────────────────────────────────────────────────

/**
 * Stale-while-revalidate: return cached response immediately,
 * fetch update in background for next time.
 */
async function staleWhileRevalidate(request, cacheName) {
    const cache = await caches.open(cacheName);
    const cached = await cache.match(request);

    const fetchPromise = fetch(request).then((response) => {
        if (response.ok) {
            cache.put(request, response.clone());
        }
        return response;
    }).catch(() => null);

    // Return cached immediately if available, otherwise wait for network
    if (cached) {
        // Fire-and-forget background update
        fetchPromise; // eslint-disable-line no-unused-expressions
        return cached;
    }

    const response = await fetchPromise;
    if (response) return response;

    // Both cache and network failed
    return new Response('Resource unavailable offline', {
        status: 503,
        headers: { 'Content-Type': 'text/plain' },
    });
}

/**
 * Network-first with cache fallback and offline page as last resort.
 * Caches successful HTML responses for offline reading (LRU).
 */
async function networkFirstWithOffline(request) {
    try {
        const response = await fetch(request);
        if (response.ok && response.headers.get('content-type')?.includes('text/html')) {
            const cache = await caches.open(CONTENT_CACHE);
            cache.put(request, response.clone());
            trimCache(CONTENT_CACHE, CONTENT_MAX_ENTRIES).catch(() => {});
        }
        return response;
    } catch {
        // Network failed — try cache
        const cached = await caches.match(request);
        if (cached) return cached;

        // No cache — serve offline page
        const offline = await caches.match('/offline');
        if (offline) return offline;

        // Last resort (should never happen if precache succeeded)
        return new Response('You are offline', {
            status: 503,
            headers: { 'Content-Type': 'text/plain' },
        });
    }
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function isNetworkOnly(path) {
    if (NETWORK_ONLY_EXACT.has(path)) return true;
    return NETWORK_ONLY_PREFIXES.some((prefix) => path.startsWith(prefix));
}

/**
 * Evict oldest entries when cache exceeds maxEntries.
 * Cache API guarantees insertion order (FIFO), so keys[0] is oldest.
 */
async function trimCache(cacheName, maxEntries) {
    try {
        const cache = await caches.open(cacheName);
        const keys = await cache.keys();
        if (keys.length <= maxEntries) return;
        const toDelete = keys.slice(0, keys.length - maxEntries);
        await Promise.allSettled(toDelete.map((key) => cache.delete(key)));
    } catch {
        // Cache eviction is best-effort — don't crash the SW
    }
}
