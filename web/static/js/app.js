/**
 * MarkGo Engine — ES Module Entry Point
 *
 * Shell modules (navigation, theme, scroll, login) run once and persist.
 * Content modules (highlight, lazy) re-run after each SPA navigation.
 * Page-specific modules load/unload based on data-template attribute.
 * Router intercepts links and swaps <main> content without full reloads.
 */

import { NS, key, BLOG_TITLE } from './modules/blog-storage.js';
import { init as initNavigation } from './modules/navigation.js';
import { init as initTheme } from './modules/theme.js';
import { init as initHighlight } from './modules/highlight.js';
import { init as initScroll } from './modules/scroll.js';
import { init as initLazy } from './modules/lazy.js';
import { init as initLogin } from './modules/login.js';
import { init as initToast, showToast } from './modules/toast.js';
import { init as initFab } from './modules/fab.js';
import { init as initComposeSheet } from './modules/compose-sheet.js';
import { init as initAMASheet } from './modules/ama-sheet.js';
import { init as initSearchPopover } from './modules/search-popover.js';
import { init as initSubscribePopover } from './modules/subscribe-popover.js';
import { init as initRouter } from './modules/router.js';

// One-shot migration: any markgo:* localStorage key not under current NS gets
// rewritten into it. Catches v3.8 flat keys and prior-deploy slugged keys
// (BaseURL change). Idempotent via NS:migrated-v1 flag. Fail-closed: storage
// exception leaves keys; readers see no-data fallback.
try {
    const FLAG = key('migrated-v1');
    if (!localStorage.getItem(FLAG)) {
        const nsPrefix = `${NS}:`;
        for (let i = localStorage.length - 1; i >= 0; i--) {
            const k = localStorage.key(i);
            if (!k?.startsWith('markgo:') || k.startsWith(nsPrefix)) continue;
            const rest = k.slice('markgo:'.length);
            const sep = rest.indexOf(':');
            const suffix = sep === -1 ? rest : rest.slice(sep + 1);
            if (suffix === 'migrated-v1' || suffix === 'idb-migrated-v1') {
                localStorage.removeItem(k);
                continue;
            }
            const newK = key(suffix);
            const v = localStorage.getItem(k);
            const existing = localStorage.getItem(newK);
            if (v === null || existing === v) {
                // Empty source or identical target — safe to drop source.
                localStorage.removeItem(k);
            } else if (existing === null) {
                // Migrate: copy then drop.
                localStorage.setItem(newK, v);
                localStorage.removeItem(k);
            }
            // else: conflict (target already has a different value) — keep
            // both. FLAG below skips this loop on next session, so the
            // orphaned source key persists for manual inspection rather than
            // silently losing user data.
        }
        localStorage.setItem(FLAG, '1');
    }
} catch { /* storage disabled — accept loss */ }

// Page-specific module loaders
const PAGE_MODULES = {
    search: () => import('./search-page.js'),
    about: () => import('./contact.js'),
    compose: () => import('./compose.js'),
    admin_home: () => import('./admin.js'),
    admin_ama: () => import('./admin-ama.js'),
    drafts: () => import('./drafts.js'),
};

let currentPageModule = null;

async function loadPageModule(template) {
    // Cleanup previous module if it supports it
    if (currentPageModule?.destroy) currentPageModule.destroy();
    currentPageModule = null;

    const loader = PAGE_MODULES[template];
    if (loader) {
        try {
            const mod = await loader();
            mod.init();
            currentPageModule = mod;
        } catch (err) {
            console.error(`Failed to load page module for "${template}":`, err);
        }
    }
}

/**
 * Called by the router after content swap.
 * Re-runs content-dependent modules and loads page-specific JS.
 */
function reinitPage(template) {
    initHighlight();
    initLazy();
    loadPageModule(template);
}

// ── Service Worker registration ──────────────────────────────────────────────

if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch((err) => {
        console.error('Service worker registration failed:', err);
    });
}

// ── Install prompt ───────────────────────────────────────────────────────────

const INSTALL_VISIT_KEY = key('visit-count');
const INSTALL_DISMISSED_KEY = key('install-dismissed');
const INSTALL_THRESHOLD = 3;
const INSTALL_DISMISS_DAYS = 30;

let deferredPrompt = null;

window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault();
    deferredPrompt = e;

    // Don't show if user dismissed recently (re-prompt after 30 days)
    try {
        const dismissedAt = parseInt(localStorage.getItem(INSTALL_DISMISSED_KEY) || '0', 10);
        if (dismissedAt && Date.now() - dismissedAt < INSTALL_DISMISS_DAYS * 86400000) return;
    } catch { /* ignore */ }

    // Track visit count
    let visits = 0;
    try {
        visits = parseInt(localStorage.getItem(INSTALL_VISIT_KEY) || '0', 10) + 1;
        localStorage.setItem(INSTALL_VISIT_KEY, String(visits));
    } catch { /* ignore */ }

    if (visits >= INSTALL_THRESHOLD) {
        showInstallBanner();
    }
});

function showInstallBanner() {
    if (!deferredPrompt) return;

    const banner = document.createElement('div');
    banner.className = 'install-banner';
    banner.setAttribute('role', 'complementary');
    banner.setAttribute('aria-label', 'Install app');

    const text = document.createElement('span');
    text.className = 'install-banner-text';
    text.textContent = `Install ${BLOG_TITLE || 'this site'} for quick access`;

    const installBtn = document.createElement('button');
    installBtn.className = 'install-banner-btn';
    installBtn.textContent = 'Install';
    installBtn.addEventListener('click', async () => {
        banner.remove();
        if (!deferredPrompt) return;
        deferredPrompt.prompt();
        const { outcome } = await deferredPrompt.userChoice;
        if (outcome === 'accepted') {
            showToast('App installed!', 'success');
        }
        deferredPrompt = null;
    });

    const dismissBtn = document.createElement('button');
    dismissBtn.className = 'install-banner-dismiss';
    dismissBtn.setAttribute('aria-label', 'Dismiss');
    dismissBtn.textContent = '\u00d7';
    dismissBtn.addEventListener('click', () => {
        banner.remove();
        try { localStorage.setItem(INSTALL_DISMISSED_KEY, String(Date.now())); } catch { /* ignore */ }
    });

    banner.appendChild(text);
    banner.appendChild(installBtn);
    banner.appendChild(dismissBtn);
    document.body.appendChild(banner);
}

// ── Offline indicator ────────────────────────────────────────────────────────

let offlineToast = null;
window.addEventListener('offline', () => {
    offlineToast = showToast('You are offline', 'warning', { duration: 0 });
});
window.addEventListener('online', () => {
    if (offlineToast) {
        offlineToast.dismiss();
        offlineToast = null;
    }
    showToast('Back online', 'success');
});

document.addEventListener('DOMContentLoaded', () => {
    // Shell modules — run once, persist across navigations
    initNavigation();
    initTheme();
    initScroll();
    initLogin();
    initToast();
    initFab();
    initComposeSheet();
    initAMASheet();
    initSearchPopover();
    initSubscribePopover();

    // Content modules — initial page
    initHighlight();
    initLazy();

    // Page-specific module — initial page
    loadPageModule(document.body.dataset.template);

    // Router — last, passes reinitPage as callback
    initRouter(reinitPage);
});
