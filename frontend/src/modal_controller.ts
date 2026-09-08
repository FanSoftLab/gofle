import { t } from './i18n';
import { showToast } from './toast';
import type { AppContext, RecentFileItem } from './app_context';
import type { SSHHandler } from './ssh_handler';

export class ModalController {
    private ctx: AppContext;
    private sshHandler: SSHHandler;

    constructor(ctx: AppContext, sshHandler: SSHHandler) {
        this.ctx = ctx;
        this.sshHandler = sshHandler;
        this.initHandlers();
    }

    private initHandlers = () => {
        const {
            btnRecent, btnCloseRecent, btnClearRecent,
            gotoModal, gotoLineInput, errGotoLine, btnSubmitGoto, btnCancelGoto,
            mainScroller,
        } = this.ctx;

        btnRecent.addEventListener('click', () => this.loadRecent());

        btnCloseRecent.addEventListener('click', () => {
            this.ctx.recentModal.classList.add('hidden');
        });

        btnClearRecent.addEventListener('click', async () => {
            const api = this.ctx.getApi();
            if (api) {
                try {
                    await api.ClearRecentFiles();
                    this.loadRecent();
                } catch (err) {
                    showToast(String((err as any)?.message ?? err), 'error');
                }
            }
        });

        btnCancelGoto.addEventListener('click', () => {
            gotoModal.classList.add('hidden');
        });

        btnSubmitGoto.addEventListener('click', () => {
            if (!this.ctx.activeTabId || !this.ctx.tabs.has(this.ctx.activeTabId)) return;
            const tab = this.ctx.tabs.get(this.ctx.activeTabId)!;
            const lineNum = parseInt(gotoLineInput.value.trim(), 10);

            if (isNaN(lineNum) || lineNum < 1 || lineNum > tab.totalLines) {
                errGotoLine.textContent = t('err_invalid_line_number', { max: tab.totalLines });
                errGotoLine.classList.remove('hidden');
                return;
            }

            gotoModal.classList.add('hidden');
            mainScroller.scrollToIndex(lineNum - 1);
        });
    };

    public openGotoModal = () => {
        if (!this.ctx.activeTabId) return;
        this.ctx.errGotoLine.classList.add('hidden');
        this.ctx.gotoLineInput.value = '';
        this.ctx.gotoModal.classList.remove('hidden');
        this.ctx.gotoLineInput.focus();
    };

    public loadRecent = async () => {
        const { getApi, recentFilesList, recentModal } = this.ctx;
        const api = getApi();
        if (!api) return;
        const files = await api.GetRecentFiles();
        recentFilesList.innerHTML = '';
        if (files && files.length > 0) {
            files.forEach((f: RecentFileItem) => {
                const li = document.createElement('li');
                li.className = 'recent-item';

                const main = document.createElement('div');
                main.className = 'recent-item-main';

                const pathDiv = document.createElement('div');
                pathDiv.className = 'recent-path';
                const base = (f.path || '').split(/[/\\]/).filter(Boolean).pop() || f.path;
                pathDiv.textContent = base;
                pathDiv.title = f.path;

                const infoDiv = document.createElement('div');
                infoDiv.className = 'recent-info';
                const sizeStr = f.is_ssh ? t('recent_ssh_badge') : `${(f.file_size / 1024 / 1024).toFixed(2)} MB`;
                const dir = (f.path || '').replace(/[/\\][^/\\]*$/, '') || f.path;
                infoDiv.textContent = `${sizeStr} · ${f.last_opened}`;
                infoDiv.title = dir;

                main.appendChild(pathDiv);
                main.appendChild(infoDiv);

                const actions = document.createElement('div');
                actions.className = 'recent-item-actions';

                // Fixed-width badge slot so SSH / local rows share the same metrics.
                const badgeSlot = document.createElement('div');
                badgeSlot.className = 'recent-badge-slot';
                if (f.is_ssh) {
                    const badge = document.createElement('span');
                    badge.className = 'recent-badge';
                    badge.textContent = t('recent_ssh_badge');
                    badgeSlot.appendChild(badge);
                }
                actions.appendChild(badgeSlot);

                const removeBtn = document.createElement('button');
                removeBtn.type = 'button';
                removeBtn.className = 'recent-remove-btn';
                removeBtn.title = t('recent_remove_title');
                removeBtn.setAttribute('aria-label', t('recent_remove_title'));
                removeBtn.innerHTML = '<svg class="icon" viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="M6 6l12 12"/></svg>';
                removeBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    try {
                        await api.RemoveRecentFile(f.path);
                        this.loadRecent();
                    } catch (err) {
                        showToast(String((err as any)?.message ?? err), 'error');
                    }
                });
                actions.appendChild(removeBtn);

                li.appendChild(main);
                li.appendChild(actions);

                li.addEventListener('click', async () => {
                    recentModal.classList.add('hidden');
                    if (f.is_ssh && f.ssh) {
                        await this.sshHandler.reopenFromRecent(f);
                    } else {
                        const stats = await api.OpenFile(f.path);
                        if (stats) this.ctx.addOrUpdateTab(stats);
                    }
                });
                recentFilesList.appendChild(li);
            });
        } else {
            const empty = document.createElement('li');
            empty.className = 'recent-item';
            empty.textContent = t('no_recent_files');
            recentFilesList.appendChild(empty);
        }
        recentModal.classList.remove('hidden');
    };
}
