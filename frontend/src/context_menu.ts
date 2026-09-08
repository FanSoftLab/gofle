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
            const target = e.target as HTMLElement;
            const rowEl = target.closest('.log-row') as HTMLElement;
            if (rowEl) {
                this.selectedLineText = rowEl.querySelector('.log-text')?.textContent || '';
                this.selectedLineIndex = parseInt(rowEl.dataset.index || '-1', 10);
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
            const target = e.target as HTMLElement;
            const rowEl = target.closest('.log-row') as HTMLElement;
            if (rowEl) {
                this.selectedLineText = rowEl.querySelector('.log-text')?.textContent || '';
                this.selectedLineIndex = parseInt(rowEl.dataset.index || '-1', 10);
                ctxMenuFilter.style.left = `${e.clientX}px`;
                ctxMenuFilter.style.top = `${e.clientY}px`;
                ctxMenuFilter.classList.remove('hidden');
                ctxMenuMain.classList.add('hidden');
            }
        });

        document.addEventListener('click', () => this.hideContextMenus());

        document.getElementById('ctx-main-copy')?.addEventListener('click', () => {
            if (this.selectedLineText) navigator.clipboard.writeText(this.selectedLineText);
        });

        document.getElementById('ctx-main-copy-numbered')?.addEventListener('click', () => {
            if (!this.selectedLineText) return;
            navigator.clipboard.writeText(`${this.selectedLineIndex + 1}: ${this.selectedLineText}`);
            showToast(t('ctx_copy_numbered'), 'info', 2000);
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

        document.getElementById('ctx-filter-copy')?.addEventListener('click', () => {
            if (this.selectedLineText) navigator.clipboard.writeText(this.selectedLineText);
        });

        document.getElementById('ctx-filter-goto')?.addEventListener('click', () => {
            if (this.selectedLineIndex >= 0) {
                mainScroller.scrollToIndex(this.selectedLineIndex);
            }
        });

        document.getElementById('ctx-filter-save')?.addEventListener('click', () => this.ctx.saveSearchResults());
    };
}
