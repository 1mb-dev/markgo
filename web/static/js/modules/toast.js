/**
 * Toast — lightweight notification system.
 *
 * Shell module: runs once, persists across SPA navigations.
 * Container lives outside <main>, so content swaps don't affect it.
 *
 * Usage:
 *   import { showToast } from './modules/toast.js';
 *   showToast('Saved!', 'success');
 *   showToast('Something broke', 'error', { duration: 0 });
 *   const t = showToast('Working...', 'info', { duration: 0 });
 *   t.dismiss();
 */

const MAX_TOASTS = 3;
const DEFAULT_DURATION = 5000;

let container = null;

// ---------------------------------------------------------------------------
// Icons — pre-parsed SVG fragments (hardcoded, safe)
// ---------------------------------------------------------------------------

const ICON_SVG_MARKUP = {
    success: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zm-3.97-3.03a.75.75 0 0 0-1.08.022L7.477 9.417 5.384 7.323a.75.75 0 0 0-1.06 1.061L6.97 11.03a.75.75 0 0 0 1.079-.02l3.992-4.99a.75.75 0 0 0-.01-1.05z"/></svg>',
    error: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zM5.354 4.646a.5.5 0 1 0-.708.708L7.293 8l-2.647 2.646a.5.5 0 0 0 .708.708L8 8.707l2.646 2.647a.5.5 0 0 0 .708-.708L8.707 8l2.647-2.646a.5.5 0 0 0-.708-.708L8 7.293 5.354 4.646z"/></svg>',
    warning: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M8.982 1.566a1.13 1.13 0 0 0-1.96 0L.165 13.233c-.457.778.091 1.767.98 1.767h13.713c.889 0 1.438-.99.98-1.767L8.982 1.566zM8 5c.535 0 .954.462.9.995l-.35 3.507a.552.552 0 0 1-1.1 0L7.1 5.995A.905.905 0 0 1 8 5zm.002 6a1 1 0 1 1 0 2 1 1 0 0 1 0-2z"/></svg>',
    info: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M8 16A8 8 0 1 0 8 0a8 8 0 0 0 0 16zm.93-9.412-1 4.705c-.07.34.029.533.304.533.194 0 .487-.07.686-.246l-.088.416c-.287.346-.92.598-1.465.598-.703 0-1.002-.422-.808-1.319l.738-3.468c.064-.293.006-.399-.287-.399l-.442-.02.024-.112L8.71 6.088h.832l-.024.112-.738 3.468-.06.282z"/><circle cx="8" cy="4.5" r="1"/></svg>',
};

// Parse once at module load — these are hardcoded strings, not user input
const ICON_CACHE = {};
const parser = new DOMParser();
for (const [type, markup] of Object.entries(ICON_SVG_MARKUP)) {
    const doc = parser.parseFromString(markup, 'image/svg+xml');
    ICON_CACHE[type] = doc.documentElement;
}

// ---------------------------------------------------------------------------
// Create / dismiss
// ---------------------------------------------------------------------------

function createToast(message, type, options) {
    const el = document.createElement('div');
    el.className = `toast toast-${type}`;
    el.setAttribute('role', 'alert');

    // Icon
    const icon = document.createElement('div');
    icon.className = 'toast-icon';
    const svgTemplate = ICON_CACHE[type] || ICON_CACHE.info;
    icon.appendChild(svgTemplate.cloneNode(true));
    el.appendChild(icon);

    // Message
    const msg = document.createElement('div');
    msg.className = 'toast-message';
    msg.textContent = message;
    el.appendChild(msg);

    // Action link (optional) — a focusable affordance, e.g. "View post". The
    // href drives navigation (SPA router intercepts same-origin links); clicking
    // also dismisses the toast. When an action is present the toast defaults to
    // no auto-dismiss (see duration below) so keyboard/AT users can reach it.
    if (options.action && options.action.href && options.action.label) {
        const action = document.createElement('a');
        action.className = 'toast-action';
        action.href = options.action.href;
        action.textContent = options.action.label;
        action.addEventListener('click', () => dismissToast(el));
        el.appendChild(action);
    }

    // Close button
    if (options.dismissible !== false) {
        const btn = document.createElement('button');
        btn.className = 'toast-close';
        btn.setAttribute('aria-label', 'Dismiss');
        btn.setAttribute('type', 'button');
        btn.textContent = '\u00d7';
        btn.addEventListener('click', () => dismissToast(el));
        el.appendChild(btn);
    }

    // Auto-dismiss timer. A toast with an action persists by default so the
    // action stays reachable (WCAG 2.2.1) unless the caller sets a duration.
    const duration = options.duration ?? (options.action ? 0 : DEFAULT_DURATION);
    if (duration > 0) {
        el._timer = setTimeout(() => dismissToast(el), duration);
    }

    // Pause on hover
    el.addEventListener('mouseenter', () => {
        if (el._timer) {
            clearTimeout(el._timer);
            el._timer = null;
            el._paused = true;
        }
    });
    el.addEventListener('mouseleave', () => {
        if (el._paused) {
            el._paused = false;
            if (duration > 0) {
                el._timer = setTimeout(() => dismissToast(el), duration);
            }
        }
    });

    // ESC on focused toast
    el.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') dismissToast(el);
    });

    return el;
}

function dismissToast(el) {
    if (el._dismissed) return;
    el._dismissed = true;
    if (el._timer) {
        clearTimeout(el._timer);
        el._timer = null;
    }
    el.classList.add('dismissing');
    el.addEventListener('animationend', () => el.remove(), { once: true });
    // Fallback: remove after 300ms even if animationend doesn't fire
    setTimeout(() => { if (el.parentNode) el.remove(); }, 350);
}

function enforceMax() {
    if (!container) return;
    const toasts = container.querySelectorAll('.toast:not(.dismissing)');
    for (let i = 0; i < toasts.length - MAX_TOASTS; i++) {
        dismissToast(toasts[i]);
    }
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export function init() {
    if (container) return;
    container = document.createElement('div');
    container.className = 'toast-container';
    container.setAttribute('aria-live', 'polite');
    container.setAttribute('aria-atomic', 'true');
    document.body.appendChild(container);
}

/**
 * Show a toast notification.
 *
 * @param {string} message - Text to display
 * @param {'success'|'error'|'warning'|'info'} [type='info']
 * @param {Object} [options]
 * @param {number} [options.duration=5000] - Auto-dismiss ms (0 = manual only;
 *   defaults to 0 when options.action is set so the action stays reachable)
 * @param {boolean} [options.dismissible=true] - Show close button
 * @param {{label: string, href: string}} [options.action] - Optional focusable link
 * @returns {{ dismiss: () => void }}
 */
export function showToast(message, type = 'info', options = {}) {
    if (!container) init();

    const validTypes = ['success', 'error', 'warning', 'info'];
    if (!validTypes.includes(type)) type = 'info';

    const el = createToast(message, type, options);
    container.appendChild(el);
    enforceMax();

    return { dismiss: () => dismissToast(el) };
}
