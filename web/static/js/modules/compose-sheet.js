/**
 * Compose Sheet — quick capture overlay.
 *
 * Shell module: runs once, persists across SPA navigations.
 * Opens on fab:compose event, closes on backdrop/Escape/navigation.
 * Submits to POST /compose/quick with JSON + X-CSRF-Token header.
 * Auto-saves drafts to localStorage, recovers on re-open.
 */

import { showToast } from './toast.js';
import { authenticatedJSON } from './auth-fetch.js';
import { queuePost, drainQueue, getQueueCount } from './offline-queue.js';
import { trapFocus, releaseFocus } from './focus-trap.js';
import { key } from './blog-storage.js';

const DRAFT_KEY = key('compose-draft');
const SAVE_DELAY = 2000;
let initialized = false;

let overlay = null;
let textarea = null;
let titleInput = null;
let saveDraftBtn = null;
let publishLink = null;
let draftNotice = null;
let isOpen = false;
let isSubmitting = false;
let saveTimer = null;
let saveWarning = null;
let viewportHandler = null;

function autoGrow(el) {
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 300) + 'px';
}

function saveDraft() {
    const content = textarea?.value || '';
    const title = titleInput?.value || '';
    if (!content && !title) {
        clearDraft();
        return;
    }
    try {
        localStorage.setItem(DRAFT_KEY, JSON.stringify({ content, title, ts: Date.now() }));
        if (saveWarning) saveWarning.hidden = true;
    } catch (err) {
        console.warn('Draft auto-save failed:', err.message);
        if (saveWarning) saveWarning.hidden = false;
    }
}

function scheduleSave() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(saveDraft, SAVE_DELAY);
}

function loadDraft() {
    try {
        const raw = localStorage.getItem(DRAFT_KEY);
        if (!raw) return null;
        return JSON.parse(raw);
    } catch (err) {
        console.warn('Failed to load draft:', err.message);
        return null;
    }
}

function clearDraft() {
    clearTimeout(saveTimer);
    try { localStorage.removeItem(DRAFT_KEY); } catch (err) { console.warn('Failed to clear draft:', err.message); }
}

function buildOverlay() {
    const el = document.createElement('div');
    el.className = 'compose-sheet-overlay';
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-modal', 'true');
    el.setAttribute('aria-label', 'Quick compose');
    el.hidden = true;

    // Backdrop
    const backdrop = document.createElement('div');
    backdrop.className = 'compose-sheet-backdrop';
    backdrop.addEventListener('click', close);

    // Sheet
    const sheet = document.createElement('div');
    sheet.className = 'compose-sheet';

    // Header
    const header = document.createElement('div');
    header.className = 'compose-sheet-header';

    const heading = document.createElement('span');
    heading.className = 'compose-sheet-heading';
    heading.textContent = 'Quick compose';

    const closeBtn = document.createElement('button');
    closeBtn.className = 'compose-sheet-close';
    closeBtn.setAttribute('aria-label', 'Close');
    closeBtn.textContent = '\u00D7';
    closeBtn.addEventListener('click', close);

    header.appendChild(heading);
    header.appendChild(closeBtn);

    // Title toggle + input
    const titleGroup = document.createElement('div');
    titleGroup.className = 'compose-sheet-title-group';

    const titleToggle = document.createElement('button');
    titleToggle.className = 'compose-sheet-title-toggle';
    titleToggle.type = 'button';
    titleToggle.textContent = '+ Add title';
    titleToggle.setAttribute('aria-expanded', 'false');
    titleToggle.setAttribute('aria-controls', 'compose-sheet-title-input');
    titleToggle.addEventListener('click', () => {
        titleGroup.classList.toggle('expanded');
        if (titleGroup.classList.contains('expanded')) {
            titleToggle.textContent = '\u2212 Remove title';
            titleToggle.setAttribute('aria-expanded', 'true');
            titleInput.focus();
        } else {
            titleToggle.textContent = '+ Add title';
            titleToggle.setAttribute('aria-expanded', 'false');
            titleInput.value = '';
        }
    });

    titleInput = document.createElement('input');
    titleInput.type = 'text';
    titleInput.id = 'compose-sheet-title-input';
    titleInput.className = 'compose-sheet-title-input';
    titleInput.placeholder = 'Title (optional)';
    titleInput.maxLength = 200;

    titleGroup.appendChild(titleToggle);
    titleGroup.appendChild(titleInput);

    // Draft recovery notice
    draftNotice = document.createElement('div');
    draftNotice.className = 'compose-sheet-draft-notice';
    draftNotice.hidden = true;

    const draftText = document.createElement('span');
    draftText.textContent = 'Draft recovered';

    const draftDiscard = document.createElement('button');
    draftDiscard.type = 'button';
    draftDiscard.className = 'compose-sheet-draft-discard';
    draftDiscard.textContent = 'Discard';
    draftDiscard.addEventListener('click', () => {
        clearDraft();
        textarea.value = '';
        titleInput.value = '';
        const tg = overlay.querySelector('.compose-sheet-title-group');
        if (tg) {
            tg.classList.remove('expanded');
            const t = tg.querySelector('.compose-sheet-title-toggle');
            if (t) {
                t.textContent = '+ Add title';
                t.setAttribute('aria-expanded', 'false');
            }
        }
        draftNotice.hidden = true;
        textarea.focus();
    });

    draftNotice.appendChild(draftText);
    draftNotice.appendChild(draftDiscard);

    // Save warning (shown when localStorage is unavailable)
    saveWarning = document.createElement('div');
    saveWarning.className = 'compose-sheet-save-warning';
    saveWarning.setAttribute('role', 'status');
    saveWarning.textContent = 'Draft saving unavailable \u2014 copy your work before closing';
    saveWarning.hidden = true;

    // Textarea
    textarea = document.createElement('textarea');
    textarea.className = 'compose-sheet-textarea';
    textarea.placeholder = 'What\u2019s on your mind?';
    textarea.rows = 4;

    // Footer
    const footer = document.createElement('div');
    footer.className = 'compose-sheet-footer';

    const wordCount = document.createElement('span');
    wordCount.className = 'compose-sheet-wordcount';
    wordCount.textContent = 'thought';

    textarea.addEventListener('input', () => {
        const words = textarea.value.trim().split(/\s+/).filter(Boolean).length;
        const hasTitle = titleGroup.classList.contains('expanded') && titleInput.value.trim();
        if (hasTitle) {
            wordCount.textContent = 'article';
        } else if (words < 100) {
            wordCount.textContent = 'thought';
        } else {
            wordCount.textContent = 'article';
        }
        autoGrow(textarea);
        scheduleSave();
    });

    titleInput.addEventListener('input', scheduleSave);

    const actions = document.createElement('div');
    actions.className = 'compose-sheet-actions';

    saveDraftBtn = document.createElement('button');
    saveDraftBtn.className = 'compose-sheet-save-draft';
    saveDraftBtn.textContent = 'Save Draft';
    saveDraftBtn.addEventListener('click', () => handleSubmit(true));

    publishLink = document.createElement('button');
    publishLink.className = 'compose-sheet-publish-link';
    publishLink.textContent = 'Publish';
    publishLink.addEventListener('click', () => handleSubmit(false));

    actions.appendChild(saveDraftBtn);
    actions.appendChild(publishLink);

    footer.appendChild(wordCount);
    footer.appendChild(actions);

    // Assemble
    sheet.appendChild(header);
    sheet.appendChild(titleGroup);
    sheet.appendChild(draftNotice);
    sheet.appendChild(saveWarning);
    sheet.appendChild(textarea);
    sheet.appendChild(footer);

    el.appendChild(backdrop);
    el.appendChild(sheet);

    return el;
}

function open() {
    if (isOpen) return;
    if (!overlay) {
        overlay = buildOverlay();
        document.body.appendChild(overlay);
    }

    overlay.hidden = false;
    isOpen = true;

    // Prevent body scroll
    document.body.style.overflow = 'hidden';

    // Recover draft
    const draft = loadDraft();
    if (draft && (draft.content || draft.title)) {
        textarea.value = draft.content || '';
        if (draft.title) {
            titleInput.value = draft.title;
            const tg = overlay.querySelector('.compose-sheet-title-group');
            const toggle = tg?.querySelector('.compose-sheet-title-toggle');
            if (tg && toggle) {
                tg.classList.add('expanded');
                toggle.textContent = '\u2212 Remove title';
                toggle.setAttribute('aria-expanded', 'true');
            }
        }
        draftNotice.hidden = false;
    } else {
        draftNotice.hidden = true;
    }

    // Focus textarea after animation, reset auto-grow, trap focus inside dialog
    requestAnimationFrame(() => {
        autoGrow(textarea);
        textarea.focus();
    });

    trapFocus(overlay);
    document.addEventListener('keydown', handleKeydown);

    // Visual viewport handling — reposition above virtual keyboard on iOS
    if (window.visualViewport) {
        viewportHandler = () => {
            if (!overlay || !overlay.isConnected) return;
            const vv = window.visualViewport;
            overlay.style.height = vv.height + 'px';
            overlay.style.top = vv.offsetTop + 'px';
        };
        viewportHandler();
        window.visualViewport.addEventListener('resize', viewportHandler);
        window.visualViewport.addEventListener('scroll', viewportHandler);
    }
}

function close() {
    if (!isOpen) return;
    isOpen = false;

    // Capture values before resetting the form
    const draftContent = textarea.value;
    const draftTitle = titleInput.value;

    overlay.hidden = true;
    overlay.style.height = '';
    overlay.style.top = '';
    document.body.style.overflow = '';
    releaseFocus();
    document.removeEventListener('keydown', handleKeydown);

    // Remove visual viewport handler
    if (viewportHandler && window.visualViewport) {
        window.visualViewport.removeEventListener('resize', viewportHandler);
        window.visualViewport.removeEventListener('scroll', viewportHandler);
        viewportHandler = null;
    }

    // Reset form
    draftNotice.hidden = true;
    textarea.value = '';
    titleInput.value = '';
    const titleGroup = overlay.querySelector('.compose-sheet-title-group');
    if (titleGroup) {
        titleGroup.classList.remove('expanded');
        const toggle = titleGroup.querySelector('.compose-sheet-title-toggle');
        if (toggle) {
            toggle.textContent = '+ Add title';
            toggle.setAttribute('aria-expanded', 'false');
        }
    }

    // Reset buttons
    resetButtons();
    isSubmitting = false;

    // Save captured draft after form reset
    if (draftContent || draftTitle) {
        try {
            localStorage.setItem(DRAFT_KEY, JSON.stringify({ content: draftContent, title: draftTitle, ts: Date.now() }));
        } catch (err) {
            console.warn('Draft save on close failed:', err.message);
            showToast('Draft could not be saved \u2014 your work may be lost', 'error');
        }
    } else {
        clearDraft();
    }
}

function handleKeydown(e) {
    if (e.key === 'Escape') {
        e.preventDefault();
        close();
    }
}

function prependCard(data, content, title) {
    // Only update if we're on the feed page
    if (document.body.dataset.template !== 'feed') return;

    const feedStream = document.querySelector('.content-stream');
    if (!feedStream) return;

    const now = new Date().toISOString();
    const card = document.createElement('article');

    if (data.type === 'thought') {
        card.className = 'feed-card feed-card-thought';

        const accent = document.createElement('div');
        accent.className = 'feed-card-accent';

        const body = document.createElement('div');
        body.className = 'feed-card-body';

        const contentDiv = document.createElement('div');
        contentDiv.className = 'feed-card-content thought-content';
        const p = document.createElement('p');
        p.textContent = content;
        contentDiv.appendChild(p);

        const meta = document.createElement('div');
        meta.className = 'feed-card-meta';
        const time = document.createElement('time');
        time.className = 'feed-card-time';
        time.dateTime = now;
        time.textContent = 'just now';
        meta.appendChild(time);

        body.append(contentDiv, meta);
        card.append(accent, body);
    } else {
        // Article or link — card with title
        card.className = 'feed-card';

        const body = document.createElement('div');
        body.className = 'feed-card-body';

        const h3 = document.createElement('h3');
        h3.className = 'feed-card-title';
        const a = document.createElement('a');
        a.href = data.url;
        a.textContent = title || 'Untitled';
        h3.appendChild(a);

        const excerpt = document.createElement('p');
        excerpt.className = 'feed-card-excerpt';
        excerpt.textContent = content.substring(0, 160);

        const meta = document.createElement('div');
        meta.className = 'feed-card-meta';
        const time = document.createElement('time');
        time.className = 'feed-card-time';
        time.dateTime = now;
        time.textContent = 'just now';
        meta.appendChild(time);

        body.append(h3, excerpt, meta);
        card.appendChild(body);
    }

    // Animate entrance
    card.style.opacity = '0';
    card.style.transform = 'translateY(-10px)';
    feedStream.insertBefore(card, feedStream.firstChild);

    requestAnimationFrame(() => {
        card.style.transition = 'opacity var(--transition-base), transform var(--transition-base)';
        card.style.opacity = '1';
        card.style.transform = 'translateY(0)';
    });

    // Scroll to top to see the new card
    window.scrollTo({ top: 0, behavior: 'smooth' });
}

async function handleSubmit(isDraft) {
    if (isSubmitting) return;

    const content = textarea.value.trim();
    if (!content) {
        showToast('Write something first', 'warning');
        textarea.focus();
        return;
    }

    isSubmitting = true;
    saveDraftBtn.disabled = true;
    publishLink.disabled = true;
    if (isDraft) {
        saveDraftBtn.textContent = 'Saving\u2026';
    } else {
        publishLink.textContent = 'Publishing\u2026';
    }

    const titleGroup = overlay.querySelector('.compose-sheet-title-group');
    const hasTitle = titleGroup?.classList.contains('expanded');
    const title = hasTitle ? titleInput.value.trim() : '';

    const body = { content, draft: isDraft };
    if (title) body.title = title;

    let result;
    try {
        result = await authenticatedJSON('/compose/quick', {
            method: 'POST',
            body,
        });
    } catch (err) {
        // TypeError = network failure; anything else = logic error (e.g. missing CSRF)
        if (!(err instanceof TypeError)) {
            showToast(err.message || 'Something went wrong', 'error');
            resetButtons();
            return;
        }
        // Network failure — queue for offline sync
        try {
            await queuePost(body);
            textarea.value = '';
            titleInput.value = '';
            clearDraft();
            close();
            showToast('Saved offline \u2014 will publish when back online', 'info');
        } catch (queueErr) {
            console.error('Offline queue failed:', queueErr);
            showToast('Network error \u2014 try again', 'error');
        }
        resetButtons();
        return;
    }

    if (result.status === 401 || result.status === 403) {
        close();
        showToast('Please sign in to publish', 'warning');
        return;
    }

    if (result.ok) {
        // Clear fields BEFORE close() so close() won't re-save them
        textarea.value = '';
        titleInput.value = '';
        clearDraft();
        close();
        if (!isDraft) prependCard(result.data, content, title);
        showToast(result.data.message || (isDraft ? 'Saved as draft' : 'Published!'), 'success');
    } else {
        showToast(result.error || 'Failed to save', 'error');
        resetButtons();
    }
}

function resetButtons() {
    isSubmitting = false;
    saveDraftBtn.disabled = false;
    publishLink.disabled = false;
    saveDraftBtn.textContent = 'Save Draft';
    publishLink.textContent = 'Publish';
}

async function syncQueue() {
    let count;
    try {
        count = await getQueueCount();
    } catch (err) {
        console.warn('Queue count check failed:', err?.message || err);
        return;
    }
    if (count === 0) return;

    showToast(`Syncing ${count} queued post${count > 1 ? 's' : ''}\u2026`, 'info');

    const result = await drainQueue();
    if (result.failed === -1) {
        showToast('Cannot sync \u2014 please refresh the page first', 'warning');
        return;
    }
    if (result.published > 0) {
        showToast(`Published ${result.published} queued post${result.published > 1 ? 's' : ''}`, 'success');
    }
    if (result.failed > 0) {
        showToast(`${result.failed} post${result.failed > 1 ? 's' : ''} still queued \u2014 will retry`, 'warning');
    }
}

export function init() {
    // Compose is owner-only — skip initialization for visitors
    if (document.body.dataset.authenticated !== 'true') {
        // Listen for reactive auth — initialize after login
        document.addEventListener('auth:authenticated', () => init(), { once: true });
        return;
    }

    if (initialized) return; // Prevent double listener registration on reactive auth
    initialized = true;

    // Listen for FAB trigger
    document.addEventListener('fab:compose', open);

    // Close on SPA navigation
    document.addEventListener('router:navigate-start', close);

    // Drain offline queue when connectivity returns
    window.addEventListener('online', () => {
        syncQueue().catch((err) => {
            console.error('Queue sync failed:', err);
            showToast('Failed to sync queued posts \u2014 try publishing again', 'warning');
        });
    });

    // Try to drain on init (may have queued items from previous session)
    if (navigator.onLine) {
        syncQueue().catch((err) => {
            console.error('Queue sync failed:', err);
            showToast('Failed to sync queued posts \u2014 try publishing again', 'warning');
        });
    }
}
