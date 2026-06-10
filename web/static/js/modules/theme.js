/**
 * Theme popover — light/dark/auto mode + color presets.
 *
 * Two independent axes:
 * 1. Color theme: data-color-theme attribute (server-rendered from BLOG_THEME config)
 * 2. Light/dark mode: data-theme attribute ("dark", "light", or absent for auto)
 */

function applyTheme(mode) {
    if (mode === 'dark' || mode === 'light') {
        document.documentElement.setAttribute('data-theme', mode);
    } else {
        document.documentElement.removeAttribute('data-theme');
    }
    updateThemeColor();
}

function updateThemeColor() {
    const meta = document.querySelector('meta[name="theme-color"]');
    if (!meta) return;
    const bg = getComputedStyle(document.body).backgroundColor;
    if (bg) meta.content = bg;
}

function getSavedMode() {
    try {
        const saved = localStorage.getItem('theme');
        if (saved === 'dark' || saved === 'light') return saved;
    } catch (e) {
        // ignore
    }
    return null;
}

// ---------------------------------------------------------------------------
// Color theme
// ---------------------------------------------------------------------------

// Split intentionally: applyColorThemeToDOM mutates the document only;
// setColorTheme persists to localStorage too. Hover/focus preview calls the
// apply-only helper so the document reflects the previewed preset without
// overwriting the user's saved choice; mouseleave/popover-close restores
// the persisted value. Do not collapse these back into one function.
function applyColorThemeToDOM(preset) {
    const html = document.documentElement;
    if (preset && preset !== 'default') {
        html.setAttribute('data-color-theme', preset);
    } else {
        html.removeAttribute('data-color-theme');
    }
    requestAnimationFrame(updateThemeColor);
}

function setColorTheme(preset) {
    applyColorThemeToDOM(preset);
    try { localStorage.setItem('colorTheme', preset || 'default'); } catch (e) { /* ignore */ }
}

function persistedColorTheme() {
    try { return localStorage.getItem('colorTheme') || 'default'; } catch (e) { return 'default'; }
}

function updateSwatchActive(container, preset) {
    container.querySelectorAll('.color-swatch').forEach((btn) => {
        const isActive = btn.dataset.color === (preset || 'default');
        btn.classList.toggle('active', isActive);
        btn.setAttribute('aria-checked', isActive);
    });
}

// ---------------------------------------------------------------------------
// Mode buttons
// ---------------------------------------------------------------------------

function updateModeActive(container, mode) {
    container.querySelectorAll('.theme-mode-btn[data-mode]').forEach((btn) => {
        const isActive = btn.dataset.mode === (mode || 'auto');
        btn.classList.toggle('active', isActive);
        btn.setAttribute('aria-checked', isActive);
    });
}

// ---------------------------------------------------------------------------
// Reader font-size — scales prose only via data-font-size on <html> (see
// main.css --reading-scale + article.css). "m" is the default, stored as the
// absence of the attribute so the calc() fallback of 1 keeps prose untouched.
// The bare "fontSize" key matches theme/colorTheme so the FOUC inline script can
// apply it before render. Apply-on-click only (no hover preview — reflow jars);
// the role=radio aria-checked toggle is the screen-reader announcement.
// ---------------------------------------------------------------------------

function applyFontSize(size) {
    const html = document.documentElement;
    if (size === 's' || size === 'l') {
        html.setAttribute('data-font-size', size);
    } else {
        html.removeAttribute('data-font-size');
    }
}

function setFontSize(size) {
    applyFontSize(size);
    try { localStorage.setItem('fontSize', size || 'm'); } catch (e) { /* ignore */ }
}

function persistedFontSize() {
    try { return localStorage.getItem('fontSize') || 'm'; } catch (e) { return 'm'; }
}

function updateFontSizeActive(container, size) {
    container.querySelectorAll('.font-size-btn').forEach((btn) => {
        const isActive = btn.dataset.size === (size || 'm');
        btn.classList.toggle('active', isActive);
        btn.setAttribute('aria-checked', isActive);
    });
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

export function init() {
    const trigger = document.querySelector('.theme-btn');
    const popover = document.getElementById('theme-popover');
    if (!trigger || !popover) return;

    try {
        const savedMode = getSavedMode();
        applyTheme(savedMode);
        requestAnimationFrame(updateThemeColor);

        // Restore saved color theme
        let savedColor = 'default';
        try { savedColor = localStorage.getItem('colorTheme') || 'default'; } catch (e) { /* ignore */ }
        setColorTheme(savedColor);

        // Restore saved reader font-size (FOUC script already applied s/l)
        const savedFontSize = persistedFontSize();
        applyFontSize(savedFontSize);

        // Set initial active states
        updateModeActive(popover, savedMode || 'auto');
        updateSwatchActive(popover, savedColor);
        updateFontSizeActive(popover, savedFontSize);

        // Popover toggle
        function openPopover() {
            document.dispatchEvent(new CustomEvent('popover:exclusive', { detail: 'theme-popover' }));
            popover.hidden = false;
            trigger.setAttribute('aria-expanded', 'true');
            const firstBtn = popover.querySelector('.theme-mode-btn');
            if (firstBtn) firstBtn.focus();
        }

        function closePopover() {
            popover.hidden = true;
            trigger.setAttribute('aria-expanded', 'false');
            // If a preview was active (hover/focus on a swatch), snap back to persisted
            applyColorThemeToDOM(persistedColorTheme());
        }

        trigger.addEventListener('click', (e) => {
            e.stopPropagation();
            if (popover.hidden) {
                openPopover();
            } else {
                closePopover();
            }
        });

        // Close on click outside
        document.addEventListener('click', (e) => {
            if (!popover.hidden && !popover.contains(e.target) && e.target !== trigger) {
                closePopover();
            }
        });

        // Close on Escape
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && !popover.hidden) {
                closePopover();
                trigger.focus();
            }
        });

        // Mutual exclusion — close when another popover opens
        document.addEventListener('popover:exclusive', (e) => {
            if (e.detail !== 'theme-popover' && !popover.hidden) closePopover();
        });

        // Live preview — hover/focus a swatch applies the preset to the document
        // without persisting; mouseleave/focusout-outside-popover and popover
        // close restore the persisted preset. Click still persists via setColorTheme.
        popover.addEventListener('mouseover', (e) => {
            const swatch = e.target.closest('.color-swatch');
            if (swatch) applyColorThemeToDOM(swatch.dataset.color);
        });
        popover.addEventListener('mouseleave', () => {
            applyColorThemeToDOM(persistedColorTheme());
        });
        popover.addEventListener('focusin', (e) => {
            const swatch = e.target.closest('.color-swatch');
            if (swatch) applyColorThemeToDOM(swatch.dataset.color);
        });
        popover.addEventListener('focusout', (e) => {
            // Restore whenever focus leaves the popover entirely, regardless of
            // which element was last focused. Tab-out from a mode button after
            // a swatch preview was applied would otherwise strand the preview.
            if (!popover.contains(e.relatedTarget)) {
                applyColorThemeToDOM(persistedColorTheme());
            }
        });

        // Mode / font-size / color selection
        popover.addEventListener('click', (e) => {
            // Font-size buttons share .theme-mode-btn styling — match them first
            // by .font-size-btn so the mode handler ([data-mode]) never sees them.
            const fontBtn = e.target.closest('.font-size-btn');
            if (fontBtn) {
                setFontSize(fontBtn.dataset.size);
                updateFontSizeActive(popover, fontBtn.dataset.size);
                return;
            }

            const modeBtn = e.target.closest('.theme-mode-btn[data-mode]');
            if (modeBtn) {
                const mode = modeBtn.dataset.mode;
                if (mode === 'auto') {
                    try { localStorage.removeItem('theme'); } catch (err) { /* ignore */ }
                    applyTheme(null);
                } else {
                    try { localStorage.setItem('theme', mode); } catch (err) { /* ignore */ }
                    applyTheme(mode);
                }
                updateModeActive(popover, mode);
                return;
            }

            // Color selection
            const swatch = e.target.closest('.color-swatch');
            if (swatch) {
                const preset = swatch.dataset.color;
                setColorTheme(preset);
                updateSwatchActive(popover, preset);
            }
        });

        // Respond to system preference changes when in auto mode
        if (window.matchMedia) {
            window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
                const saved = getSavedMode();
                if (!saved) {
                    applyTheme(null);
                }
                requestAnimationFrame(updateThemeColor);
            });
        }
    } catch (error) {
        console.error('Theme initialization failed:', error);
    }
}
