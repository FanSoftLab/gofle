// Minimal toast notifications: errors and non-blocking confirmations that must
// be visible without interrupting flow (no alert()).
let host: HTMLElement | null = null;

export function initToasts(): void {
    host = document.getElementById('toast-host');
}

export function showToast(message: string, kind: 'error' | 'info' = 'error', ms = 6000): void {
    if (!host) return;
    const el = document.createElement('div');
    el.className = `toast toast-${kind}`;
    
    // Wrap long lines so they don't overflow the toast container
    const lines = message.split('\n');
    const frag = document.createDocumentFragment();
    lines.forEach((line, idx) => {
        const span = document.createElement('span');
        span.textContent = line;
        span.style.display = 'block';
        span.style.whiteSpace = 'pre-wrap';
        span.style.wordBreak = 'break-word';
        frag.appendChild(span);
        if (idx < lines.length - 1) frag.appendChild(document.createElement('br'));
    });
    el.appendChild(frag);
    
    const close = () => {
        el.classList.add('toast-out');
        setTimeout(() => el.remove(), 200);
    };
    el.addEventListener('click', close);
    host.appendChild(el);
    setTimeout(close, ms);
}
