/**
 * Router — SPA-like navigation via content swapping.
 *
 * Intercepts internal link clicks, fetches the target page as full HTML,
 * parses with DOMParser, and swaps <main> content. Turbo Drive pattern:
 * zero server changes, full HTML always available for SEO and no-JS.
 *
 * Features:
 * - Progress bar during fetch (thin, top-fixed, primary-colored)
 * - Content fade transition (120ms, respects prefers-reduced-motion)
 * - Prefetch on hover/touchstart for near-instant navigations
 * - AbortController cancels in-flight requests on rapid navigation
 * - Scroll position saved/restored per history entry
 * - Meta tag updates (description, OG) for share-while-browsing
 * - Focus management after swap for accessibility
 * - Custom events for extensibility (router:navigate-start, router:navigate-end)
 */

// Routes that always need a full page load
const BYPASS = new Set(['/logout', '/feed.xml', '/feed.json', '/sitemap.xml', '/robots.txt', '/manifest.json', '/health', '/sw.js']);
const BYPASS_PREFIXES = ['/static/', '/uploads/', '/api/', '/debug/', '/compose/preview', '/compose/upload'];

let controller = null;
let onNavigate = null;
let prefetchCache = new Map();

// ---------------------------------------------------------------------------
// Progress bar
// ---------------------------------------------------------------------------

const progress = document.createElement('div');
progress.className = 'router-progress';

function showProgress() {
    progress.classList.remove('done', 'hiding');
    progress.classList.add('active');
}

function completeProgress() {
    progress.classList.remove('active');
    progress.classList.add('done');
    setTimeout(() => {
        progress.classList.add('hiding');
        setTimeout(() => progress.classList.remove('done', 'hiding'), 300);
    }, 150);
}

// ---------------------------------------------------------------------------
// Link filtering
// ---------------------------------------------------------------------------

function shouldIntercept(link, event) {
    // Modified clicks → new tab intent
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return false;
    if (event.button !== 0) return false;

    // Link opt-outs
    if (link.hasAttribute('download')) return false;
    if (link.target === '_blank') return false;

    // Different origin
    const url = new URL(link.href, location.origin);
    if (url.origin !== location.origin) return false;

    // Bypass paths
    const path = url.pathname;
    if (BYPASS.has(path)) return false;
    if (BYPASS_PREFIXES.some((p) => path.startsWith(p))) return false;

    // Hash-only on same page
    if (url.pathname === location.pathname && url.hash) return false;

    // Same URL — no navigation needed
    if (link.href === location.href) return false;

    return true;
}

// ---------------------------------------------------------------------------
// Content extraction
// ---------------------------------------------------------------------------

function extractPage(doc) {
    return {
        main: doc.querySelector('main.main-content'),
        title: doc.querySelector('title')?.textContent || '',
        template: doc.body.dataset.template || 'feed',
        bodyClass: doc.body.className,
        authenticated: doc.body.dataset.authenticated || '',
        meta: extractMeta(doc),
        csrfToken: doc.querySelector('meta[name="csrf-token"]')?.content || '',
    };
}

function extractMeta(doc) {
    const meta = {};
    const selectors = [
        'meta[name="description"]',
        'meta[property="og:title"]',
        'meta[property="og:description"]',
        'meta[property="og:url"]',
        'meta[property="og:type"]',
        'meta[name="twitter:title"]',
        'meta[name="twitter:description"]',
    ];
    for (const sel of selectors) {
        const el = doc.querySelector(sel);
        if (el) meta[sel] = el.getAttribute('content');
    }
    // Canonical link
    const canonical = doc.querySelector('link[rel="canonical"]');
    if (canonical) meta['link[rel="canonical"]'] = canonical.getAttribute('href');
    return meta;
}

function syncCSRFToken(token) {
    if (!token) return;
    let meta = document.querySelector('meta[name="csrf-token"]');
    if (meta) {
        meta.setAttribute('content', token);
    } else {
        meta = document.createElement('meta');
        meta.name = 'csrf-token';
        meta.content = token;
        document.head.appendChild(meta);
    }
    // Sync hidden inputs in login popover (outside <main>, not swapped by router)
    document.querySelectorAll('input[name="_csrf"]').forEach((input) => {
        input.value = token;
    });
}

function updateMeta(meta) {
    for (const [sel, content] of Object.entries(meta)) {
        if (sel.startsWith('link[')) {
            const el = document.querySelector(sel);
            if (el) el.setAttribute('href', content);
        } else {
            const el = document.querySelector(sel);
            if (el) el.setAttribute('content', content);
        }
    }
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

async function navigate(url, { push = true } = {}) {
    // Cancel any in-flight request
    if (controller) controller.abort();
    controller = new AbortController();

    // Save current scroll before navigating away
    if (push) {
        history.replaceState({ url: location.href, scrollY: window.scrollY }, '');
    }

    showProgress();
    document.dispatchEvent(new CustomEvent('router:navigate-start', { detail: { url } }));

    try {
        // Check prefetch cache first
        let html = prefetchCache.get(url);
        let finalUrl = url;
        if (!html) {
            const response = await fetch(url, { signal: controller.signal });
            if (!response.ok) {
                window.location.href = url;
                return;
            }
            // Detect server-side redirects (fetch follows them transparently)
            if (response.redirected) {
                finalUrl = response.url;
            }
            html = await response.text();
        }
        prefetchCache.delete(url);

        const doc = new DOMParser().parseFromString(html, 'text/html');
        const page = extractPage(doc);

        if (!page.main) {
            // Non-HTML response or malformed — full reload
            window.location.href = url;
            return;
        }

        // Transition: fade out
        const main = document.querySelector('main.main-content');
        main.classList.add('swapping');

        // Wait for fade-out (CSS transition handles timing, we use a short delay)
        await new Promise((r) => setTimeout(r, 120));

        // Swap content
        main.replaceChildren(...page.main.childNodes);

        // Update document state
        document.title = page.title;
        document.body.dataset.template = page.template;
        document.body.className = page.bodyClass;
        updateMeta(page.meta);
        syncCSRFToken(page.csrfToken);

        // Sync auth state — detect session expiry across SPA navigations
        const wasAuth = document.body.dataset.authenticated === 'true';
        if (page.authenticated) {
            document.body.dataset.authenticated = page.authenticated;
        } else {
            delete document.body.dataset.authenticated;
        }
        const isAuth = document.body.dataset.authenticated === 'true';
        if (wasAuth !== isAuth) {
            document.dispatchEvent(new CustomEvent('auth:statechange', { detail: { authenticated: isAuth } }));
        }

        // Restore the swapping class removal (body class was just replaced)
        // main-content class is on the element, not body — we need to fade in
        main.classList.add('swapping');
        // Force reflow, then remove to trigger fade-in
        void main.offsetHeight;
        main.classList.remove('swapping');

        // History — use finalUrl if server redirected
        if (push) {
            history.pushState({ url: finalUrl, scrollY: 0 }, '', finalUrl);
        }

        // Scroll
        const hash = new URL(finalUrl, location.origin).hash;
        if (hash) {
            const target = document.querySelector(hash);
            if (target) target.scrollIntoView({ behavior: 'smooth' });
        } else if (push) {
            window.scrollTo(0, 0);
        }

        // Accessibility: move focus to main content, remove tabindex after blur
        main.setAttribute('tabindex', '-1');
        main.focus({ preventScroll: true });
        main.addEventListener('blur', () => main.removeAttribute('tabindex'), { once: true });

        // Announce new page to screen readers
        const announcer = document.getElementById('route-announcer');
        if (announcer) {
            const heading = main.querySelector('h1');
            announcer.textContent = heading ? heading.textContent : page.title;
        }

        // Re-initialize page modules
        if (onNavigate) onNavigate(page.template);

        completeProgress();
        document.dispatchEvent(new CustomEvent('router:navigate-end', { detail: { url: finalUrl, template: page.template } }));

    } catch (err) {
        if (err.name === 'AbortError') return;
        console.error('Router navigation failed:', err);
        window.location.href = url;
    } finally {
        controller = null;
    }
}

// ---------------------------------------------------------------------------
// Popstate (back/forward)
// ---------------------------------------------------------------------------

function handlePopstate(e) {
    const state = e.state;
    if (!state?.url) return;

    navigate(state.url, { push: false }).then(() => {
        if (typeof state.scrollY === 'number') {
            window.scrollTo(0, state.scrollY);
        }
    });
}

// ---------------------------------------------------------------------------
// Prefetch on hover
// ---------------------------------------------------------------------------

let prefetchTimeout = null;

function handlePrefetch(e) {
    const link = e.target.closest('a[href]');
    if (!link) return;

    const url = new URL(link.href, location.origin);
    if (url.origin !== location.origin) return;
    if (BYPASS.has(url.pathname)) return;
    if (BYPASS_PREFIXES.some((p) => url.pathname.startsWith(p))) return;
    if (prefetchCache.has(link.href)) return;
    if (link.href === location.href) return;

    clearTimeout(prefetchTimeout);
    prefetchTimeout = setTimeout(() => {
        // Don't prefetch if we have too many cached
        if (prefetchCache.size >= 5) {
            // Evict oldest entry
            const firstKey = prefetchCache.keys().next().value;
            prefetchCache.delete(firstKey);
        }

        fetch(link.href)
            .then((r) => r.ok ? r.text() : null)
            .then((html) => {
                if (html) {
                    prefetchCache.set(link.href, html);
                    // Expire after 10 seconds
                    setTimeout(() => prefetchCache.delete(link.href), 10000);
                }
            })
            .catch(() => {}); // silent — prefetch is best-effort
    }, 65); // small delay to avoid prefetching on mouse pass-through
}

function cancelPrefetch() {
    clearTimeout(prefetchTimeout);
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

export function init(callback) {
    if (!history.pushState) return; // progressive enhancement

    onNavigate = callback;

    // Insert progress bar
    document.body.prepend(progress);

    // Save initial state
    history.replaceState({ url: location.href, scrollY: window.scrollY }, '');

    // Link interception via event delegation
    document.addEventListener('click', (e) => {
        const link = e.target.closest('a[href]');
        if (link && shouldIntercept(link, e)) {
            e.preventDefault();
            navigate(link.href);
        }
    });

    // Back/forward
    window.addEventListener('popstate', handlePopstate);

    // Prefetch on hover
    document.addEventListener('mouseover', handlePrefetch);
    document.addEventListener('touchstart', handlePrefetch, { passive: true });
    document.addEventListener('mouseout', cancelPrefetch);
}
