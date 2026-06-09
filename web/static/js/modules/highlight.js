/**
 * Syntax highlighting — hljs init + copy buttons on code blocks.
 */

import { copyToClipboard } from './clipboard.js';
import { loadHighlighter, hasCodeBlocks } from './highlighter-loader.js';

function createCopyIcon() {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('width', '16');
    svg.setAttribute('height', '16');
    svg.setAttribute('fill', 'currentColor');
    svg.setAttribute('viewBox', '0 0 16 16');
    const paths = [
        'M4 1.5H3a2 2 0 0 0-2 2V14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V3.5a2 2 0 0 0-2-2h-1v1h1a1 1 0 0 1 1 1V14a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1h1v-1z',
        'M9.5 1a.5.5 0 0 1 .5.5v1a.5.5 0 0 1-.5.5h-3a.5.5 0 0 1-.5-.5v-1a.5.5 0 0 1 .5-.5h3zm-3-1A1.5 1.5 0 0 0 5 1.5v1A1.5 1.5 0 0 0 6.5 4h3A1.5 1.5 0 0 0 11 2.5v-1A1.5 1.5 0 0 0 9.5 0h-3z',
    ];
    paths.forEach((d) => {
        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', d);
        svg.appendChild(path);
    });
    return svg;
}

function createCheckIcon() {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('width', '16');
    svg.setAttribute('height', '16');
    svg.setAttribute('fill', 'currentColor');
    svg.setAttribute('viewBox', '0 0 16 16');
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('d', 'M10.97 4.97a.75.75 0 0 1 1.07 1.05l-3.99 4.99a.75.75 0 0 1-1.08.02L4.324 8.384a.75.75 0 1 1 1.06-1.06l2.094 2.093 3.473-4.425a.267.267 0 0 1 .02-.022z');
    svg.appendChild(path);
    return svg;
}

function addCopyButton(preElement) {
    const button = document.createElement('button');
    button.className = 'copy-code-btn';
    button.appendChild(createCopyIcon());
    button.title = 'Copy code to clipboard';

    button.addEventListener('click', () => {
        const code = preElement.querySelector('code').textContent;
        copyToClipboard(code)
            .then(() => {
                button.textContent = '';
                button.appendChild(createCheckIcon());
                button.classList.add('success');
                setTimeout(() => {
                    button.textContent = '';
                    button.appendChild(createCopyIcon());
                    button.classList.remove('success');
                }, 2000);
            })
            .catch((err) => console.error('Failed to copy code:', err));
    });

    preElement.style.position = 'relative';
    preElement.appendChild(button);
}

export async function init() {
    // Text pages (thoughts, links, feed) never pay for the 122 KB highlighter.
    if (!hasCodeBlocks()) return;

    const hljs = await loadHighlighter();
    if (!hljs) return;

    hljs.configure({
        languages: ['javascript', 'typescript', 'go', 'html', 'css', 'bash', 'json', 'yaml', 'markdown'],
    });

    hljs.highlightAll();

    document.querySelectorAll('pre code').forEach((codeBlock) => {
        addCopyButton(codeBlock.parentElement);
    });
}
