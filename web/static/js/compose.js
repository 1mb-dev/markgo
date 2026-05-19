/**
 * Compose Page — markdown preview (debounced), image upload, fetch-based submit.
 *
 * GET /compose is public. POST /compose requires auth — on 401, shows a toast
 * and opens the login popover. After login, the page reloads.
 *
 * Draft-first: "Save Draft" is primary CTA, "Publish/Update" is secondary.
 * Uses e.submitter + FormData to distinguish which button was clicked.
 *
 * Auto-save: debounced localStorage save for all form fields.
 * Separate key (compose-full-draft) from quick compose (compose-draft);
 * both namespaced via blog-storage.js.
 */

import { showToast } from './modules/toast.js';
import { authenticatedFetch } from './modules/auth-fetch.js';
import { key } from './modules/blog-storage.js';

const FULL_DRAFT_KEY = key('compose-full-draft');
const SAVE_DELAY = 2000;

let ac = null;
let saveTimer = null;

export function init() {
    ac = new AbortController();
    const { signal } = ac;

    // --- Element references ---
    const textarea = document.getElementById('content');
    const previewBtn = document.querySelector('.compose-preview-btn');
    const previewPanel = document.querySelector('.compose-preview-panel');
    const previewContent = document.querySelector('.compose-preview-content');
    const form = document.querySelector('.compose-form');
    const saveDraftBtn = document.querySelector('.compose-submit');
    const publishBtn = document.querySelector('.compose-submit-secondary');
    const isEditing = form?.action?.includes('/compose/edit/');

    if (!textarea || !previewBtn || !previewPanel || !previewContent) return;

    const titleInput = document.getElementById('title');
    const descInput = document.getElementById('description');
    const linkInput = document.getElementById('link_url');
    const tagsInput = document.getElementById('tags');
    const catsInput = document.getElementById('categories');
    const draftNotice = document.getElementById('compose-draft-notice');
    const draftDiscard = document.getElementById('compose-draft-discard');
    const saveWarning = document.getElementById('compose-save-warning');

    const uploadBtn = document.querySelector('.compose-upload-btn');
    const fileInput = document.querySelector('.compose-file-input');
    const uploadStatus = document.querySelector('.upload-status');

    const bannerInput = document.querySelector('[data-banner-input]');
    const bannerPreview = document.querySelector('[data-banner-preview]');
    const bannerUploadBtn = document.querySelector('[data-banner-upload-btn]');
    const bannerRemoveBtn = document.querySelector('[data-banner-remove-btn]');
    const bannerFileInput = document.querySelector('[data-banner-file-input]');

    const maxFileSize = parseInt(form?.dataset.maxUpload || '10485760', 10);

    // --- Shared mutable state ---
    let previewVisible = false;
    let debounceTimer = null;

    // --- Shared utilities ---

    /**
     * Safely set preview HTML from server-rendered markdown.
     * Uses DOMParser instead of innerHTML.
     */
    function setPreviewHTML(html) {
        const doc = new DOMParser().parseFromString(html, 'text/html');
        previewContent.textContent = '';
        while (doc.body.firstChild) {
            previewContent.appendChild(doc.body.firstChild);
        }
    }

    function fetchPreview() {
        const content = textarea.value;
        if (!content.trim()) {
            setPreviewHTML('<p class="preview-placeholder">Start typing to see a preview...</p>');
            return;
        }

        setPreviewHTML('<p class="preview-loading">Rendering preview...</p>');

        const formData = new FormData();
        formData.append('content', content);

        authenticatedFetch('/compose/preview', {
            method: 'POST',
            body: formData,
        })
            .then((res) => {
                if (!res.ok) throw new Error('Preview failed');
                return res.text();
            })
            .then((html) => {
                setPreviewHTML(html);
                if (typeof hljs !== 'undefined') {
                    previewContent.querySelectorAll('pre code').forEach((block) => {
                        hljs.highlightElement(block);
                    });
                }
            })
            .catch((err) => {
                setPreviewHTML('<p class="preview-error">Preview unavailable' + (err.message ? ': ' + err.message : '') + '</p>');
            });
    }

    function debouncedPreview() {
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(fetchPreview, 500);
    }

    function getSlug() {
        return form?.dataset.slug || '';
    }

    function formatFileSize(bytes) {
        if (bytes >= 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(0) + ' MB';
        if (bytes >= 1024) return (bytes / 1024).toFixed(0) + ' KB';
        return bytes + ' bytes';
    }

    function setUploadStatus(msg, isError) {
        if (!uploadStatus) return;
        uploadStatus.textContent = msg;
        uploadStatus.className = 'upload-status' + (isError ? ' upload-error' : msg ? ' upload-success' : '');
    }

    function insertAtCursor(text) {
        const start = textarea.selectionStart;
        const end = textarea.selectionEnd;
        const before = textarea.value.substring(0, start);
        const after = textarea.value.substring(end);
        textarea.value = before + text + after;
        textarea.selectionStart = textarea.selectionEnd = start + text.length;
        textarea.focus();
        if (previewVisible) debouncedPreview();
    }

    function uploadFile(file) {
        if (!file) return;

        const slug = getSlug();
        if (!slug) {
            setUploadStatus('Save as draft first to upload files', false);
            return;
        }

        if (file.size > maxFileSize) {
            setUploadStatus(`File too large. Maximum size is ${formatFileSize(maxFileSize)}.`, true);
            return;
        }

        setUploadStatus('Uploading...', false);

        const formData = new FormData();
        formData.append('file', file);

        authenticatedFetch(`/compose/upload/${encodeURIComponent(slug)}`, {
            method: 'POST',
            body: formData,
        })
            .then((res) => {
                if (!res.ok) {
                    return res.json()
                        .catch(() => { throw new Error('Upload failed (server error)'); })
                        .then((data) => { throw new Error(data.error || 'Upload failed'); });
                }
                return res.json();
            })
            .then((data) => {
                insertAtCursor(data.markdown + '\n');
                setUploadStatus('Uploaded: ' + data.filename, false);
            })
            .catch((err) => {
                setUploadStatus(err.message || 'Upload failed', true);
            });
    }

    function updateUploadButton() {
        const slug = getSlug();
        if (uploadBtn) uploadBtn.disabled = !slug;
        // Mirror the same discipline on the banner button so both upload
        // affordances communicate "save draft first" via disabled state at
        // init, not just at click time.
        const bannerBtn = document.querySelector('[data-banner-upload-btn]');
        if (bannerBtn) bannerBtn.disabled = !slug;
        if (!slug) {
            setUploadStatus('Save as draft first to upload files', false);
        }
    }

    // Called from initAutoSave (discard, empty-on-save) and initFormSubmit
    // (success/redirect). Lives at outer scope so editing-mode submit still
    // clears any stale FULL_DRAFT_KEY and the saveTimer.
    function clearDraft() {
        clearTimeout(saveTimer);
        try { localStorage.removeItem(FULL_DRAFT_KEY); } catch (err) { console.warn('Failed to clear draft:', err.message); }
    }

    // --- Section helpers ---

    function initPreview() {
        previewBtn.addEventListener('click', () => {
            previewVisible = !previewVisible;
            form.classList.toggle('preview-visible', previewVisible);
            previewBtn.textContent = previewVisible ? 'Edit' : 'Preview';
            previewBtn.setAttribute('aria-pressed', previewVisible);

            if (previewVisible) {
                fetchPreview();
                textarea.addEventListener('input', debouncedPreview);
            } else {
                textarea.removeEventListener('input', debouncedPreview);
                clearTimeout(debounceTimer);
            }
        }, { signal });
    }

    function initAutoSave() {
        function getFormFields() {
            return {
                content: textarea?.value || '',
                title: titleInput?.value || '',
                description: descInput?.value || '',
                link_url: linkInput?.value || '',
                tags: tagsInput?.value || '',
                categories: catsInput?.value || '',
            };
        }

        function hasContent(fields) {
            return Object.values(fields).some((v) => v.trim() !== '');
        }

        function saveDraft() {
            const fields = getFormFields();
            if (!hasContent(fields)) {
                clearDraft();
                return;
            }
            try {
                localStorage.setItem(FULL_DRAFT_KEY, JSON.stringify({ ...fields, ts: Date.now() }));
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
                const raw = localStorage.getItem(FULL_DRAFT_KEY);
                if (!raw) return null;
                return JSON.parse(raw);
            } catch (err) {
                console.warn('Failed to load draft:', err.message);
                return null;
            }
        }

        // Recover draft on page load
        const draft = loadDraft();
        if (draft && hasContent(draft)) {
            if (textarea && draft.content) textarea.value = draft.content;
            if (titleInput && draft.title) titleInput.value = draft.title;
            if (descInput && draft.description) descInput.value = draft.description;
            if (linkInput && draft.link_url) linkInput.value = draft.link_url;
            if (tagsInput && draft.tags) tagsInput.value = draft.tags;
            if (catsInput && draft.categories) catsInput.value = draft.categories;
            if (draftNotice) draftNotice.hidden = false;
        }

        // Discard button
        if (draftDiscard) {
            draftDiscard.addEventListener('click', () => {
                clearDraft();
                if (textarea) textarea.value = '';
                if (titleInput) titleInput.value = '';
                if (descInput) descInput.value = '';
                if (linkInput) linkInput.value = '';
                if (tagsInput) tagsInput.value = '';
                if (catsInput) catsInput.value = '';
                if (draftNotice) draftNotice.hidden = true;
                textarea.focus();
            }, { signal });
        }

        // Attach input listeners for auto-save
        const fields = [textarea, titleInput, descInput, linkInput, tagsInput, catsInput];
        for (const el of fields) {
            if (el) el.addEventListener('input', scheduleSave, { signal });
        }
    }

    function initFormSubmit() {
        if (!form) return;

        function resetButtons() {
            if (saveDraftBtn) {
                saveDraftBtn.disabled = false;
                saveDraftBtn.textContent = 'Save Draft';
            }
            if (publishBtn) {
                publishBtn.disabled = false;
                publishBtn.textContent = publishBtn.dataset.label || 'Publish';
            }
        }

        // Store original label for the publish button (Update vs Publish)
        if (publishBtn) publishBtn.dataset.label = publishBtn.textContent;

        form.addEventListener('submit', async (e) => {
            e.preventDefault();

            const submitter = e.submitter;
            const isDraft = submitter?.value === 'on';

            // Disable both buttons during submit
            if (saveDraftBtn) saveDraftBtn.disabled = true;
            if (publishBtn) publishBtn.disabled = true;

            if (submitter) {
                submitter.textContent = isDraft ? 'Saving\u2026' : 'Publishing\u2026';
            }

            try {
                const response = await authenticatedFetch(form.action, {
                    method: 'POST',
                    body: new FormData(form, submitter),
                    headers: { Accept: 'text/html' },
                });

                if (response.status === 401 || response.status === 403) {
                    showToast('Please sign in to publish', 'warning');
                    resetButtons();
                    return;
                }

                if (response.redirected) {
                    clearDraft();
                    window.location.href = response.url;
                    return;
                }

                if (response.ok) {
                    clearDraft();
                    window.location.href = response.url;
                } else {
                    const html = await response.text();
                    const doc = new DOMParser().parseFromString(html, 'text/html');
                    const serverError = doc.querySelector('.compose-error p');
                    const msg = serverError?.textContent || 'Something went wrong. Please try again.';
                    showToast(msg, 'error');
                    resetButtons();
                }
            } catch (err) {
                console.error('Compose submit failed:', err);
                showToast('Network error \u2014 please try again', 'error');
                resetButtons();
            }
        }, { signal });
    }

    function initImageUpload() {
        updateUploadButton();

        if (uploadBtn && fileInput) {
            uploadBtn.addEventListener('click', () => fileInput.click(), { signal });
            fileInput.addEventListener('change', () => {
                if (fileInput.files.length > 0) {
                    uploadFile(fileInput.files[0]);
                    fileInput.value = '';
                }
            }, { signal });
        }
    }

    function initBannerUpload() {
        function setBannerValue(url) {
            if (!bannerInput) return;
            bannerInput.value = url;
            if (bannerPreview) {
                if (url) {
                    bannerPreview.src = url;
                    bannerPreview.hidden = false;
                } else {
                    bannerPreview.removeAttribute('src');
                    bannerPreview.hidden = true;
                }
            }
            if (bannerRemoveBtn) bannerRemoveBtn.hidden = !url;
            // banner_alt has no meaning without a banner — clear it on remove so
            // the form doesn't submit orphaned alt text. Server-side CreatePost
            // / UpdateArticle also drop banner_alt when banner is empty, but the
            // UI should reflect the rule.
            if (!url) {
                const altInput = document.querySelector('.compose-banner-alt');
                if (altInput) altInput.value = '';
            }
        }

        function uploadBanner(file) {
            if (!file) return;
            const slug = getSlug();
            if (!slug) {
                setUploadStatus('Save as draft first to upload a banner', false);
                return;
            }
            if (file.size > maxFileSize) {
                setUploadStatus(`Banner too large. Maximum size is ${formatFileSize(maxFileSize)}.`, true);
                return;
            }

            setUploadStatus('Uploading banner...', false);

            const formData = new FormData();
            formData.append('file', file);

            authenticatedFetch(`/compose/upload/${encodeURIComponent(slug)}`, {
                method: 'POST',
                body: formData,
            })
                .then((res) => {
                    if (!res.ok) {
                        return res.json()
                            .catch(() => { throw new Error('Banner upload failed (server error)'); })
                            .then((data) => { throw new Error(data.error || 'Banner upload failed'); });
                    }
                    return res.json();
                })
                .then((data) => {
                    setBannerValue(data.url);
                    setUploadStatus('Banner uploaded: ' + data.filename, false);
                })
                .catch((err) => {
                    setUploadStatus(err.message || 'Banner upload failed', true);
                });
        }

        if (bannerUploadBtn && bannerFileInput) {
            bannerUploadBtn.addEventListener('click', () => bannerFileInput.click(), { signal });
            bannerFileInput.addEventListener('change', () => {
                if (bannerFileInput.files.length > 0) {
                    uploadBanner(bannerFileInput.files[0]);
                    bannerFileInput.value = '';
                }
            }, { signal });
        }
        if (bannerRemoveBtn) {
            bannerRemoveBtn.addEventListener('click', () => setBannerValue(''), { signal });
        }
    }

    function initDragDrop() {
        textarea.addEventListener('dragover', (e) => {
            e.preventDefault();
            if (getSlug()) textarea.classList.add('compose-textarea-dragover');
        }, { signal });

        textarea.addEventListener('dragleave', () => {
            textarea.classList.remove('compose-textarea-dragover');
        }, { signal });

        textarea.addEventListener('drop', (e) => {
            e.preventDefault();
            textarea.classList.remove('compose-textarea-dragover');
            const files = e.dataTransfer.files;
            if (files.length > 0) uploadFile(files[0]);
        }, { signal });
    }

    // --- Wire up (matches original section order) ---
    initPreview();
    if (!isEditing) initAutoSave();
    initFormSubmit();
    initImageUpload();
    initBannerUpload();
    initDragDrop();
}

export function destroy() {
    clearTimeout(saveTimer);
    if (ac) { ac.abort(); ac = null; }
}
