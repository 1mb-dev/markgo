/**
 * Contact Form — captcha generation, inline validation, and submission.
 * Validation errors shown inline (field-level). Network/server errors use toasts.
 */

import { showToast } from './modules/toast.js';
import { authenticatedJSON } from './modules/auth-fetch.js';

let ac = null;

export function init() {
    ac = new AbortController();
    const { signal } = ac;
    const contactForm = document.getElementById('contactForm');
    const captchaInput = document.getElementById('captcha');
    const captchaQuestionField = document.getElementById('captchaQuestionField');
    const num1Element = document.getElementById('num1');
    const num2Element = document.getElementById('num2');
    const refreshBtn = document.getElementById('refreshCaptcha');

    if (!contactForm) return;

    let captchaAnswer = 0;
    let isSubmitting = false;

    function generateCaptcha() {
        const num1 = Math.floor(Math.random() * 10) + 1;
        const num2 = Math.floor(Math.random() * 10) + 1;
        captchaAnswer = num1 + num2;

        num1Element.textContent = num1;
        num2Element.textContent = num2;
        captchaQuestionField.value = `${num1} + ${num2}`;
        captchaInput.value = '';
    }

    /** Show inline error below a field. Uses existing .field-error + .error classes from components.css. */
    function showFieldError(fieldId, message) {
        const field = document.getElementById(fieldId);
        if (!field) return;

        field.classList.add('error');

        // Find or create the error element after the field (or its parent for captcha)
        const parent = field.closest('.form-group') || field.parentElement;
        let errorEl = parent.querySelector('.field-error');
        if (!errorEl) {
            errorEl = document.createElement('div');
            errorEl.className = 'field-error';
            parent.appendChild(errorEl);
        }
        errorEl.textContent = message;
    }

    /** Clear all inline errors */
    function clearErrors() {
        contactForm.querySelectorAll('.error').forEach(el => el.classList.remove('error'));
        contactForm.querySelectorAll('.field-error').forEach(el => el.remove());
    }

    /** Clear error on a single field when user starts typing */
    function clearFieldError(e) {
        const field = e.target;
        if (!field.classList.contains('error')) return;
        field.classList.remove('error');
        const parent = field.closest('.form-group') || field.parentElement;
        const errorEl = parent.querySelector('.field-error');
        if (errorEl) errorEl.remove();
    }

    function handleSubmit(e) {
        e.preventDefault();
        if (isSubmitting) return;
        clearErrors();

        const submitBtn = contactForm.querySelector('.contact-submit');
        const formData = new FormData(contactForm);

        const name = (formData.get('name') || '').trim();
        const email = (formData.get('email') || '').trim();
        const subject = (formData.get('subject') || '').trim();
        const message = (formData.get('message') || '').trim();
        const captchaValue = (formData.get('captcha_answer') || '').trim();

        // Inline validation — required + min-length. The form is novalidate, so
        // the browser skips the inputs' minlength attrs; mirror them here (read
        // from the attr itself, the single source the server's bounds match) so a
        // short field gets a field-level message instead of a generic server 400.
        let hasError = false;
        const fields = [
            { id: 'name', value: name, label: 'Name' },
            { id: 'email', value: email, label: 'Email' },
            { id: 'subject', value: subject, label: 'Subject' },
            { id: 'message', value: message, label: 'Message' },
        ];
        for (const f of fields) {
            if (!f.value) {
                showFieldError(f.id, `${f.label} is required`);
                hasError = true;
                continue;
            }
            const min = document.getElementById(f.id)?.minLength ?? -1;
            if (min > 0 && f.value.length < min) {
                showFieldError(f.id, `${f.label} must be at least ${min} characters`);
                hasError = true;
            }
        }
        if (!captchaValue) { showFieldError('captcha', 'Please solve the math problem'); hasError = true; }

        if (hasError) {
            // Focus the first field with an error
            const firstError = contactForm.querySelector('.error');
            if (firstError) firstError.focus();
            return;
        }

        if (parseInt(captchaValue) !== captchaAnswer) {
            showFieldError('captcha', 'Incorrect answer — try again');
            generateCaptcha();
            captchaInput.focus();
            return;
        }

        isSubmitting = true;
        submitBtn.disabled = true;
        const spinner = document.createElement('span');
        spinner.className = 'spinner';
        submitBtn.textContent = ' Sending...';
        submitBtn.prepend(spinner);

        authenticatedJSON('/contact', {
            method: 'POST',
            body: { name, email, subject, message, captcha_question: formData.get('captcha_question'), captcha_answer: captchaValue },
            skipCSRF: true,
        })
            .then((result) => {
                if (result.ok && result.data.success) {
                    showToast(result.data.message || 'Message sent!', 'success');
                    contactForm.reset();
                } else {
                    showToast(result.error || result.data?.message || 'Failed to send message. Please try again.', 'error');
                }
                generateCaptcha();
            })
            .catch((error) => {
                console.error('Contact form error:', error);
                showToast('An error occurred. Please try again later.', 'error');
                generateCaptcha();
            })
            .finally(() => {
                isSubmitting = false;
                submitBtn.disabled = false;
                submitBtn.textContent = 'Send';
            });
    }

    // Clear individual field errors on input (calm: error disappears as user fixes it)
    contactForm.addEventListener('input', clearFieldError, { signal });
    refreshBtn.addEventListener('click', generateCaptcha, { signal });
    contactForm.addEventListener('submit', handleSubmit, { signal });
    contactForm.addEventListener('reset', () => { clearErrors(); setTimeout(generateCaptcha, 0); }, { signal });

    generateCaptcha();
}

export function destroy() {
    if (ac) { ac.abort(); ac = null; }
}
