/** App-styled confirm dialog (replaces window.confirm). */
export function showConfirm(message: string, opts?: { title?: string; ok?: string; cancel?: string }): Promise<boolean> {
  return new Promise((resolve) => {
    const existing = document.getElementById('app-confirm-modal');
    if (existing) existing.remove();

    const overlay = document.createElement('div');
    overlay.id = 'app-confirm-modal';
    overlay.className = 'modal app-confirm-modal';
    overlay.innerHTML = `
      <div class="modal-content app-confirm-content" role="dialog" aria-modal="true">
        <div class="app-confirm-body">
          ${opts?.title ? `<h3 class="app-confirm-title">${escapeHtml(opts.title)}</h3>` : ''}
          <p class="app-confirm-message">${escapeHtml(message)}</p>
        </div>
        <div class="modal-actions app-confirm-actions">
          <button type="button" class="btn-secondary" data-act="cancel">${escapeHtml(opts?.cancel || 'Cancel')}</button>
          <button type="button" class="btn-primary" data-act="ok">${escapeHtml(opts?.ok || 'OK')}</button>
        </div>
      </div>`;
    document.body.appendChild(overlay);

    const close = (val: boolean) => {
      overlay.remove();
      document.removeEventListener('keydown', onKey);
      resolve(val);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close(false);
      if (e.key === 'Enter') close(true);
    };
    document.addEventListener('keydown', onKey);
    overlay.querySelector('[data-act="cancel"]')?.addEventListener('click', () => close(false));
    overlay.querySelector('[data-act="ok"]')?.addEventListener('click', () => close(true));
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(false); });
    (overlay.querySelector('[data-act="ok"]') as HTMLButtonElement)?.focus();
  });
}

function escapeHtml(s: string): string {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
