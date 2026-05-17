/**
 * AMA Sheet — reader question submission overlay.
 *
 * Shell module: runs once, persists across SPA navigations.
 * Opens on fab:ama event, closes on backdrop/Escape/navigation.
 * Submits to POST /ama/submit as JSON (public, no CSRF).
 * Client-side math challenge for spam prevention.
 */

import { showToast } from './toast.js';
import { authenticatedJSON } from './auth-fetch.js';
import { trapFocus, releaseFocus } from './focus-trap.js';

let overlay = null;
let isOpen = false;
let isSubmitting = false;
let captchaAnswer = 0;
let viewportHandler = null;

// Operator-overridable copy. Read from <meta> tags emitted by base.html so the
// server is the single source of truth (AMA_* env vars → config.AMA → meta tags).
// Defaults preserve pre-v3.11.0 behavior when meta tags are missing (e.g. tests).
function metaCopy(name, fallback) {
    const el = document.querySelector(`meta[name="${name}"]`);
    return el && el.content ? el.content : fallback;
}
function readAmaCopy() {
    return {
        heading:     metaCopy('markgo-ama-heading',     'Ask me anything'),
        intro:       metaCopy('markgo-ama-intro',       "Curious about something? Submit a question and I'll answer it publicly."),
        placeholder: metaCopy('markgo-ama-placeholder', 'What would you like to know?'),
        submit:      metaCopy('markgo-ama-submit',      'Submit Question'),
        thankyou:    metaCopy('markgo-ama-thankyou',    'Question submitted! It will appear once answered.'),
    };
}

function buildOverlay() {
    const copy = readAmaCopy();

    const el = document.createElement('div');
    el.className = 'ama-sheet-overlay';
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-modal', 'true');
    el.setAttribute('aria-label', 'Ask a question');
    el.hidden = true;

    // Backdrop
    const backdrop = document.createElement('div');
    backdrop.className = 'ama-sheet-backdrop';
    backdrop.addEventListener('click', close);

    // Sheet
    const sheet = document.createElement('div');
    sheet.className = 'ama-sheet';

    // Header
    const header = document.createElement('div');
    header.className = 'ama-sheet-header';

    const heading = document.createElement('span');
    heading.className = 'ama-sheet-heading';
    heading.textContent = copy.heading;

    const closeBtn = document.createElement('button');
    closeBtn.className = 'ama-sheet-close';
    closeBtn.setAttribute('aria-label', 'Close');
    closeBtn.textContent = '\u00D7';
    closeBtn.addEventListener('click', close);

    header.appendChild(heading);
    header.appendChild(closeBtn);

    // Intro paragraph (operator-overridable via AMA_PAGE_INTRO)
    const intro = document.createElement('p');
    intro.className = 'ama-sheet-intro';
    intro.textContent = copy.intro;

    // Form
    const form = document.createElement('form');
    form.className = 'ama-sheet-form';
    form.noValidate = true;

    // Name field
    const nameGroup = createFormGroup('ama-name', 'Name', 'text', 'Your name', true);
    nameGroup.input.minLength = 2;
    nameGroup.input.maxLength = 50;

    // Email field (optional)
    const emailGroup = createFormGroup('ama-email', 'Email (optional)', 'email', 'you@example.com', false);
    emailGroup.input.maxLength = 100;

    // Question field
    const questionGroup = document.createElement('div');
    questionGroup.className = 'ama-form-group';

    const questionLabel = document.createElement('label');
    questionLabel.htmlFor = 'ama-question';
    questionLabel.className = 'ama-form-label';
    questionLabel.textContent = 'Question';

    const questionTextarea = document.createElement('textarea');
    questionTextarea.id = 'ama-question';
    questionTextarea.className = 'ama-form-input ama-form-textarea';
    questionTextarea.placeholder = copy.placeholder;
    questionTextarea.required = true;
    questionTextarea.minLength = 20;
    questionTextarea.maxLength = 500;
    questionTextarea.rows = 4;

    const charCounter = document.createElement('span');
    charCounter.className = 'ama-char-counter';
    charCounter.setAttribute('aria-live', 'polite');
    charCounter.textContent = '0 / 500';
    questionTextarea.setAttribute('aria-describedby', 'ama-char-counter');
    charCounter.id = 'ama-char-counter';

    questionTextarea.addEventListener('input', () => {
        const len = questionTextarea.value.length;
        charCounter.textContent = `${len} / 500`;
        charCounter.classList.toggle('ama-char-near-limit', len > 400);
        charCounter.classList.toggle('ama-char-over-limit', len > 500);
    });

    questionGroup.appendChild(questionLabel);
    questionGroup.appendChild(questionTextarea);
    questionGroup.appendChild(charCounter);

    // Honeypot (hidden from users)
    const honeypot = document.createElement('input');
    honeypot.type = 'text';
    honeypot.name = 'website';
    honeypot.autocomplete = 'off';
    honeypot.tabIndex = -1;
    honeypot.setAttribute('aria-hidden', 'true');
    honeypot.style.cssText = 'position:absolute;left:-9999px;opacity:0;height:0;width:0;';

    // Captcha
    const captchaGroup = document.createElement('div');
    captchaGroup.className = 'ama-form-group';

    const captchaLabel = document.createElement('label');
    captchaLabel.htmlFor = 'ama-captcha';
    captchaLabel.className = 'ama-form-label';
    captchaLabel.textContent = 'Security check';

    const captchaContainer = document.createElement('div');
    captchaContainer.className = 'ama-captcha-container';

    const captchaQuestion = document.createElement('span');
    captchaQuestion.className = 'ama-captcha-question';

    const captchaInput = document.createElement('input');
    captchaInput.type = 'text';
    captchaInput.id = 'ama-captcha';
    captchaInput.className = 'ama-form-input ama-captcha-input';
    captchaInput.placeholder = 'Answer';
    captchaInput.required = true;
    captchaInput.pattern = '[0-9]+';
    captchaInput.inputMode = 'numeric';

    captchaContainer.appendChild(captchaQuestion);
    captchaContainer.appendChild(captchaInput);

    captchaGroup.appendChild(captchaLabel);
    captchaGroup.appendChild(captchaContainer);

    // Submit button
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.className = 'ama-submit-btn';
    submitBtn.textContent = copy.submit;

    // Assemble form
    form.appendChild(nameGroup.group);
    form.appendChild(emailGroup.group);
    form.appendChild(questionGroup);
    form.appendChild(honeypot);
    form.appendChild(captchaGroup);
    form.appendChild(submitBtn);

    form.addEventListener('submit', (e) => {
        e.preventDefault();
        handleSubmit(form, nameGroup.input, emailGroup.input, questionTextarea, honeypot, captchaInput, captchaQuestion, submitBtn);
    });

    // Assemble sheet
    sheet.appendChild(header);
    sheet.appendChild(intro);
    sheet.appendChild(form);

    el.appendChild(backdrop);
    el.appendChild(sheet);

    // Store refs for captcha generation
    el._captchaQuestion = captchaQuestion;
    el._captchaInput = captchaInput;

    return el;
}

function createFormGroup(id, labelText, type, placeholder, required) {
    const group = document.createElement('div');
    group.className = 'ama-form-group';

    const label = document.createElement('label');
    label.htmlFor = id;
    label.className = 'ama-form-label';
    label.textContent = labelText;

    const input = document.createElement('input');
    input.type = type;
    input.id = id;
    input.className = 'ama-form-input';
    input.placeholder = placeholder;
    input.required = required;

    group.appendChild(label);
    group.appendChild(input);

    return { group, input };
}

function generateCaptcha() {
    if (!overlay) return;
    const num1 = Math.floor(Math.random() * 10) + 1;
    const num2 = Math.floor(Math.random() * 10) + 1;
    captchaAnswer = num1 + num2;
    overlay._captchaQuestion.textContent = `What is ${num1} + ${num2}?`;
    overlay._captchaInput.value = '';
}

function open() {
    if (isOpen) return;
    if (!overlay) {
        overlay = buildOverlay();
        document.body.appendChild(overlay);
    }

    generateCaptcha();
    overlay.hidden = false;
    isOpen = true;
    document.body.style.overflow = 'hidden';

    // Focus first input after animation, trap focus inside dialog
    requestAnimationFrame(() => {
        const nameInput = overlay.querySelector('#ama-name');
        if (nameInput) nameInput.focus();
    });

    trapFocus(overlay);
    document.addEventListener('keydown', handleKeydown);

    // Visual viewport handling for iOS virtual keyboard
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

    overlay.hidden = true;
    overlay.style.height = '';
    overlay.style.top = '';
    document.body.style.overflow = '';
    releaseFocus();
    document.removeEventListener('keydown', handleKeydown);

    if (viewportHandler && window.visualViewport) {
        window.visualViewport.removeEventListener('resize', viewportHandler);
        window.visualViewport.removeEventListener('scroll', viewportHandler);
        viewportHandler = null;
    }

    // Reset form
    const form = overlay.querySelector('.ama-sheet-form');
    if (form) form.reset();
    const counter = overlay.querySelector('.ama-char-counter');
    if (counter) {
        counter.textContent = '0 / 500';
        counter.classList.remove('ama-char-near-limit', 'ama-char-over-limit');
    }
    isSubmitting = false;
}

function handleKeydown(e) {
    if (e.key === 'Escape') {
        e.preventDefault();
        close();
    }
}

async function handleSubmit(form, nameInput, emailInput, questionTextarea, honeypot, captchaInput, captchaQuestion, submitBtn) {
    if (isSubmitting) return;

    const name = nameInput.value.trim();
    const email = emailInput.value.trim();
    const question = questionTextarea.value.trim();
    const website = honeypot.value;
    const captchaValue = captchaInput.value.trim();

    // Validation
    if (!name || name.length < 2) {
        showToast('Please enter your name (at least 2 characters)', 'warning');
        nameInput.focus();
        return;
    }
    if (!question || question.length < 20) {
        showToast('Question must be at least 20 characters', 'warning');
        questionTextarea.focus();
        return;
    }
    if (question.length > 500) {
        showToast('Question must be 500 characters or less', 'warning');
        questionTextarea.focus();
        return;
    }
    if (!captchaValue || parseInt(captchaValue, 10) !== captchaAnswer) {
        showToast('Incorrect answer \u2014 try again', 'warning');
        generateCaptcha();
        captchaInput.focus();
        return;
    }

    const copy = readAmaCopy();

    isSubmitting = true;
    submitBtn.disabled = true;
    submitBtn.textContent = 'Submitting\u2026';

    try {
        const result = await authenticatedJSON('/ama/submit', {
            method: 'POST',
            body: { name, email, question, website },
            skipCSRF: true,
        });

        if (result.ok) {
            close();
            showToast(copy.thankyou, 'success');
        } else {
            showToast(result.error || 'Failed to submit question', 'error');
            generateCaptcha();
        }
    } catch {
        showToast('Network error \u2014 please try again', 'error');
        generateCaptcha();
    } finally {
        isSubmitting = false;
        submitBtn.disabled = false;
        submitBtn.textContent = copy.submit;
    }
}

export function init() {
    // Listen for AMA triggers (FAB, bottom nav, about page CTA)
    document.addEventListener('fab:ama', open);

    // Also handle clicks on .ama-trigger buttons (about page CTA)
    document.addEventListener('click', (e) => {
        if (e.target.closest('.ama-trigger')) {
            e.preventDefault();
            open();
        }
    });

    // Close on SPA navigation
    document.addEventListener('router:navigate-start', close);
}
