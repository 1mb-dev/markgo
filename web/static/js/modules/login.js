/**
 * Login — handles login popover (public), auth gate (protected), and admin popover (authenticated).
 * Login forms share .login-form class and POST to /login via fetch.
 * Admin popover provides quick access to dashboard, drafts, sign out.
 *
 * Reactive auth: on successful login from the popover, swaps login trigger/popover
 * to admin trigger/popover in-place (no full-page reload). Dispatches auth:statechange
 * so other modules can react (e.g. draft recovery toast).
 */

import { initPopover } from './popover.js';
import { showToast } from './toast.js';
import { key } from './blog-storage.js';

const DRAFT_KEY = key('compose-draft');

let loginPopoverCtrl = null;
let adminPopoverCtrl = null;

export function init() {
    // Event delegation: handles login forms in both popover and SPA-swapped auth gates
    document.addEventListener('submit', (e) => {
        const form = e.target.closest('.login-form');
        if (!form) return;

        e.preventDefault();
        const errorEl = form.querySelector('.login-error');
        if (errorEl) {
            errorEl.hidden = true;
            errorEl.textContent = '';
        }

        const submitBtn = form.querySelector('button[type="submit"]');
        if (submitBtn) submitBtn.disabled = true;

        const isPopover = form.closest('.login-popover') !== null;

        fetch('/login', {
            method: 'POST',
            headers: { Accept: 'application/json' },
            body: new FormData(form),
            credentials: 'same-origin',
        })
            .then((res) => {
                // 403/429 are handled by status BEFORE the body is read, so these
                // responses need not match the {success,error} JSON shape.
                if (res.status === 403) throw new Error('Session expired. Please refresh the page and try again.');
                if (res.status === 429) throw new Error('Too many attempts. Please wait and try again.');
                return res.json().then(
                    (data) => ({ ok: res.ok, data }),
                    () => { throw new Error('Server error. Please refresh the page.'); }
                );
            })
            .then((result) => {
                if (result.data.success) {
                    if (isPopover) {
                        // Reactive auth — swap UI in place
                        const swapped = swapToAuthenticatedUI();
                        if (swapped !== false) {
                            document.body.dataset.authenticated = 'true';
                            swapBottomNavToCompose();
                            addHeaderComposeLink();
                            // Await CSRF sync before enabling compose functionality
                            syncCSRFAfterLogin().then(() => {
                                document.dispatchEvent(new CustomEvent('auth:statechange', { detail: { authenticated: true } }));
                                document.dispatchEvent(new CustomEvent('auth:authenticated'));
                                checkDraftRecovery();
                            });
                        }
                    } else {
                        // Auth gate — full reload to render protected page
                        window.location.href = result.data.redirect || window.location.pathname;
                    }
                } else {
                    if (errorEl) {
                        errorEl.textContent = result.data.error || 'Login failed.';
                        errorEl.hidden = false;
                    }
                    if (submitBtn) submitBtn.disabled = false;
                }
            })
            .catch((err) => {
                if (errorEl) {
                    errorEl.textContent = err.message || 'Network error. Please try again.';
                    errorEl.hidden = false;
                }
                if (submitBtn) submitBtn.disabled = false;
            });
    });

    // Auto-focus the inline auth gate form (protected pages)
    const authGateInput = document.querySelector('.auth-gate-form input[name="username"]');
    if (authGateInput) authGateInput.focus();

    // Login popover toggle (unauthenticated, public pages)
    loginPopoverCtrl = initPopover('login-popover', '.login-trigger', (popover) => {
        const firstInput = popover.querySelector('input[name="username"]');
        if (firstInput) firstInput.focus();
    }, (popover) => {
        const errorEl = popover.querySelector('.login-error');
        if (errorEl) { errorEl.hidden = true; errorEl.textContent = ''; }
    });

    // Admin popover toggle (authenticated)
    adminPopoverCtrl = initPopover('admin-popover', '.admin-trigger');

    // Listen for programmatic open-login requests (e.g. from compose 401)
    document.addEventListener('auth:open-login', () => {
        const loginTrigger = document.querySelector('.login-trigger');
        if (loginTrigger) {
            loginTrigger.click();
        } else {
            // Login trigger not in DOM (e.g. session expired after reactive swap to admin)
            window.location.reload();
        }
    });
}

/**
 * Create the person SVG icon used by both login and admin triggers.
 * Built with DOM API to avoid innerHTML.
 */
function createPersonIcon() {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('width', '16');
    svg.setAttribute('height', '16');
    svg.setAttribute('fill', 'currentColor');
    svg.setAttribute('viewBox', '0 0 16 16');
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('d', 'M8 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zm2-3a2 2 0 1 1-4 0 2 2 0 0 1 4 0zm4 8c0 1-1 1-1 1H3s-1 0-1-1 1-4 6-4 6 3 6 4zm-1-.004c-.001-.246-.154-.986-.832-1.664C11.516 10.68 10.289 10 8 10c-2.29 0-3.516.68-4.168 1.332-.678.678-.83 1.418-.832 1.664h10z');
    svg.appendChild(path);
    return svg;
}

/**
 * Swap login trigger + popover to admin trigger + popover.
 * Old DOM nodes and their event listeners are garbage-collected.
 * Mirrors server-rendered admin elements in base.html (.admin-trigger, #admin-popover).
 */
function swapToAuthenticatedUI() {
    const loginTrigger = document.querySelector('.login-trigger');
    const loginPopover = document.getElementById('login-popover');
    if (!loginTrigger) {
        // DOM is in unexpected state — fall back to full reload
        console.warn('login-trigger not found for reactive auth swap — reloading');
        window.location.reload();
        return false;
    }

    // Create admin trigger button
    const adminTrigger = document.createElement('button');
    adminTrigger.className = 'nav-action-btn admin-trigger';
    adminTrigger.setAttribute('aria-label', 'Account');
    adminTrigger.setAttribute('aria-expanded', 'false');
    adminTrigger.setAttribute('aria-haspopup', 'true');
    adminTrigger.title = 'Account';
    adminTrigger.appendChild(createPersonIcon());

    // Replace login trigger with admin trigger
    loginTrigger.replaceWith(adminTrigger);

    // Build admin popover with DOM API
    const adminPopover = document.createElement('div');
    adminPopover.className = 'admin-popover';
    adminPopover.id = 'admin-popover';
    adminPopover.hidden = true;

    const nav = document.createElement('nav');
    nav.className = 'admin-popover-nav';
    nav.setAttribute('aria-label', 'Admin');

    const dashLink = document.createElement('a');
    dashLink.href = '/admin';
    dashLink.className = 'admin-popover-link';
    dashLink.textContent = 'Dashboard';

    const writingLink = document.createElement('a');
    writingLink.href = '/admin/writing';
    writingLink.className = 'admin-popover-link';
    writingLink.textContent = 'Writing';

    const draftsLink = document.createElement('a');
    draftsLink.href = '/admin/drafts';
    draftsLink.className = 'admin-popover-link';
    draftsLink.textContent = 'Drafts';

    nav.appendChild(dashLink);
    nav.appendChild(writingLink);
    nav.appendChild(draftsLink);

    const divider = document.createElement('div');
    divider.className = 'admin-popover-divider';

    const logoutLink = document.createElement('a');
    logoutLink.href = '/logout';
    logoutLink.className = 'admin-popover-link admin-popover-logout';
    logoutLink.textContent = 'Sign out';

    adminPopover.appendChild(nav);
    adminPopover.appendChild(divider);
    adminPopover.appendChild(logoutLink);

    // Replace login popover with admin popover
    if (loginPopover) {
        loginPopover.replaceWith(adminPopover);
    } else {
        console.warn('login-popover not found — using fallback container append');
        const container = adminTrigger.closest('.container');
        if (container) {
            container.appendChild(adminPopover);
        } else {
            console.warn('No .container found for admin popover — admin menu non-functional');
            showToast('Please refresh the page to access your account menu', 'warning');
            return false;
        }
    }

    // Destroy old login popover listeners before initializing admin popover
    if (loginPopoverCtrl) {
        loginPopoverCtrl.destroy();
        loginPopoverCtrl = null;
    }

    // Initialize popover behavior on new elements
    adminPopoverCtrl = initPopover('admin-popover', '.admin-trigger');
    return true;
}

/**
 * Swap bottom nav AMA button to compose button after login.
 * Mirrors server-rendered .bottom-nav-compose in base.html.
 */
function swapBottomNavToCompose() {
    const amaBtn = document.querySelector('.bottom-nav-ama');
    if (!amaBtn) return;

    const composeBtn = document.createElement('button');
    composeBtn.className = 'bottom-nav-item bottom-nav-compose';
    composeBtn.dataset.nav = 'compose';
    composeBtn.setAttribute('aria-label', 'New post');

    const icon = document.createElement('span');
    icon.className = 'bottom-nav-icon';

    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('width', '20');
    svg.setAttribute('height', '20');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2.5');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('aria-hidden', 'true');

    const line1 = document.createElementNS('http://www.w3.org/2000/svg', 'line');
    line1.setAttribute('x1', '12'); line1.setAttribute('y1', '5');
    line1.setAttribute('x2', '12'); line1.setAttribute('y2', '19');
    const line2 = document.createElementNS('http://www.w3.org/2000/svg', 'line');
    line2.setAttribute('x1', '5'); line2.setAttribute('y1', '12');
    line2.setAttribute('x2', '19'); line2.setAttribute('y2', '12');
    svg.appendChild(line1);
    svg.appendChild(line2);
    icon.appendChild(svg);
    composeBtn.appendChild(icon);

    composeBtn.addEventListener('click', () => {
        document.dispatchEvent(new CustomEvent('fab:compose'));
    });

    amaBtn.replaceWith(composeBtn);
}

/**
 * Add Compose link to desktop header nav after login.
 * Mirrors server-rendered Compose nav-item in base.html (.navbar-nav).
 */
function addHeaderComposeLink() {
    const navList = document.querySelector('.navbar-nav');
    if (!navList) return;
    // Don't add if already present
    if (navList.querySelector('a[href="/compose"]')) return;

    const li = document.createElement('li');
    li.className = 'nav-item';
    const a = document.createElement('a');
    a.href = '/compose';
    a.className = 'nav-link';
    a.textContent = 'Compose';
    li.appendChild(a);
    navList.appendChild(li);
}

/**
 * Sync the CSRF meta tag after reactive auth login.
 * The session cookie changed but the <meta name="csrf-token"> in <head> still has the
 * old value. Since the CSRF cookie is HttpOnly, JS can't read it directly — fetch the
 * current page and extract the fresh token from the rendered meta tag.
 */
function syncCSRFAfterLogin() {
    return fetch(location.href, { credentials: 'same-origin' })
        .then((res) => {
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            return res.text();
        })
        .then((html) => {
            const doc = new DOMParser().parseFromString(html, 'text/html');
            const freshToken = doc.querySelector('meta[name="csrf-token"]')?.content;
            if (!freshToken) {
                console.warn('CSRF token not found in refreshed page');
                showToast('Session may be stale \u2014 refresh if publishing fails', 'warning');
                return;
            }

            // Update meta tag
            let meta = document.querySelector('meta[name="csrf-token"]');
            if (meta) {
                meta.setAttribute('content', freshToken);
            } else {
                meta = document.createElement('meta');
                meta.name = 'csrf-token';
                meta.content = freshToken;
                document.head.appendChild(meta);
            }

            // Update hidden CSRF inputs outside <main> (e.g. in popover forms)
            document.querySelectorAll('input[name="_csrf"]').forEach((input) => {
                input.value = freshToken;
            });
        })
        .catch((err) => {
            console.error('CSRF sync after login failed:', err);
            showToast('Session sync failed \u2014 refresh the page before publishing', 'warning');
        });
}

/**
 * After login, check if there's a preserved quick compose draft
 * and surface it via toast.
 */
function checkDraftRecovery() {
    try {
        const raw = localStorage.getItem(DRAFT_KEY);
        if (!raw) return;
        const draft = JSON.parse(raw);
        if (draft && (draft.content || draft.title)) {
            showToast('Draft preserved \u2014 tap + to continue', 'info');
        }
    } catch (err) {
        console.warn('Draft recovery check failed:', err.message);
    }
}
