import { t } from './i18n';
import { showToast } from './toast';
import type { AppContext } from './app_context';

export class ContextMenu {
    private ctx: AppContext;
    private selectedLineText: string = '';
    private selectedLineIndex: number = -1;

    constructor(ctx: AppContext) {
        this.ctx = ctx;
        this.initHandlers();
    }

    private hideContextMenus = () => {
        this.ctx.ctxMenuMain.classList.add('hidden');
        this.ctx.ctxMenuFilter.classList.add('hidden');
    };

    private initHandlers = () => {
        const { ctxMenuMain, ctxMenuFilter, mainScroller, filterScroller, searchInput } = this.ctx;

        document.getElementById('viewport-main')?.addEventListener('contextmenu', (e) => {
            // Ctrl/Shift+right-click passes through: native menu hosts the
            // WebKit inspector entry in -debug builds.
            if (e.ctrlKey || e.shiftKey) return;
            e.preventDefault();
            e.stopPropagation();
            const target = e.target as HTMLElement;
            const rowEl = target.closest('.log-row') as HTMLElement;
            if (rowEl) {
                this.selectedLineText = rowEl.querySelector('.log-text')?.textContent || '';
                this.selectedLineIndex = parseInt(rowEl.dataset.index || '-1', 10);
                // If the scroller has an active range, use it for copy (single or multi).
                // Otherwise treat the right-clicked row as a one-line selection.
                const sel = mainScroller.getSelectedRange();
                if (sel) {
                    (this as any)._selRange = sel;
                    (this as any)._selScroller = mainScroller;
                } else if (!isNaN(this.selectedLineIndex) && this.selectedLineIndex >= 0) {
                    mainScroller.setSelection(this.selectedLineIndex, this.selectedLineIndex);
                    (this as any)._selRange = { start: this.selectedLineIndex, end: this.selectedLineIndex };
                    (this as any)._selScroller = mainScroller;
                } else {
                    (this as any)._selRange = null;
                    (this as any)._selScroller = null;
                }
                ctxMenuMain.style.left = `${e.clientX}px`;
                ctxMenuMain.style.top = `${e.clientY}px`;
                ctxMenuMain.classList.remove('hidden');
                ctxMenuFilter.classList.add('hidden');
            }
        });

        document.getElementById('viewport-filter')?.addEventListener('contextmenu', (e) => {
            // Ctrl/Shift+right-click passes through: native menu hosts the
            // WebKit inspector entry in -debug builds.
            if (e.ctrlKey || e.shiftKey) return;
            e.preventDefault();
            e.stopPropagation();
            const target = e.target as HTMLElement;
            const rowEl = target.closest('.log-row') as HTMLElement;
            if (rowEl) {
                this.selectedLineText = rowEl.querySelector('.log-text')?.textContent || '';
                this.selectedLineIndex = parseInt(rowEl.dataset.index || '-1', 10);
                if (!isNaN(this.selectedLineIndex) && this.selectedLineIndex >= 0) {
                    const sel = filterScroller.getSelectedRange();
                    if (sel) {
                        (this as any)._selRange = sel;
                    } else {
                        filterScroller.setSelection(this.selectedLineIndex, this.selectedLineIndex);
                        (this as any)._selRange = { start: this.selectedLineIndex, end: this.selectedLineIndex };
                    }
                    (this as any)._selScroller = filterScroller;
                } else {
                    (this as any)._selRange = null;
                    (this as any)._selScroller = null;
                }
                ctxMenuFilter.style.left = `${e.clientX}px`;
                ctxMenuFilter.style.top = `${e.clientY}px`;
                ctxMenuFilter.classList.remove('hidden');
                ctxMenuMain.classList.add('hidden');
            }
        });

        // Use pointerdown (not click): on some platforms a right-click also
        // synthesizes a click that would close the menu in the same gesture.
        // Ignore right-button and clicks inside any open context menu.
        document.addEventListener('pointerdown', (e) => {
            if (e.button === 2) return;
            const t = e.target as HTMLElement | null;
            if (t?.closest('.context-menu')) return;
            this.hideContextMenus();
        });

        document.getElementById('ctx-main-copy')?.addEventListener('click', async () => {
            const scroller = ((this as any)._selScroller as typeof mainScroller) || mainScroller;
            const range = (this as any)._selRange as { start: number; end: number } | null;
            if (range) {
                const ok = await scroller.copySelection();
                if (ok) showToast(t('ctx_copy_line'), 'info', 1500);
                else if (this.selectedLineText) {
                    await navigator.clipboard.writeText(this.selectedLineText);
                    showToast(t('ctx_copy_line'), 'info', 1500);
                }
                return;
            }
            if (this.selectedLineText) {
                await navigator.clipboard.writeText(this.selectedLineText);
                showToast(t('ctx_copy_line'), 'info', 1500);
            }
        });

        document.getElementById('ctx-main-copy-numbered')?.addEventListener('click', async () => {
            if (this.selectedLineText && (this as any)._selRange?.start === (this as any)._selRange?.end) {
                await navigator.clipboard.writeText(`${this.selectedLineIndex + 1}: ${this.selectedLineText}`);
                showToast(t('ctx_copy_numbered'), 'info', 2000);
                return;
            }
            const scroller = ((this as any)._selScroller as typeof mainScroller) || mainScroller;
            const ok = await scroller.copySelection();
            if (ok) showToast(t('ctx_copy_numbered'), 'info', 1500);
        });

        document.getElementById('ctx-main-add-search')?.addEventListener('click', () => {
            const selection = window.getSelection()?.toString();
            const textToAdd = selection && selection.length > 0 ? selection : this.selectedLineText;
            if (textToAdd) {
                searchInput.value = textToAdd;
                this.ctx.triggerSearch();
            }
        });

        document.getElementById('ctx-main-save-log')?.addEventListener('click', async () => {
            if (!this.ctx.activeTabId) return;
            const api = this.ctx.getApi();
            if (api && api.SaveFullLogAs) {
                try {
                    const savedPath = await api.SaveFullLogAs(this.ctx.activeTabId);
                    if (savedPath) {
                        showToast(t('msg_full_log_saved', { path: savedPath }), 'info');
                    }
                } catch (err) {
                    console.error('Save full log error:', err);
                    showToast(String((err as any)?.message ?? err), 'error');
                }
            }
        });

        document.getElementById('ctx-filter-copy')?.addEventListener('click', async () => {
            const scroller = ((this as any)._selScroller as typeof filterScroller) || filterScroller;
            if ((this as any)._selRange) {
                const ok = await scroller.copySelection();
                if (ok) {
                    showToast(t('ctx_copy_line'), 'info', 1500);
                    return;
                }
            }
            if (this.selectedLineText) {
                await navigator.clipboard.writeText(this.selectedLineText);
                showToast(t('ctx_copy_line'), 'info', 1500);
            }
        });

        document.getElementById('ctx-filter-goto')?.addEventListener('click', () => {
            if (this.selectedLineIndex >= 0) {
                mainScroller.scrollToIndex(this.selectedLineIndex);
            }
        });

        document.getElementById('ctx-filter-save')?.addEventListener('click', () => this.ctx.saveSearchResults());
    };
}
