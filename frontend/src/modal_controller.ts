import { t } from './i18n';
import { showToast } from './toast';
import type { AppContext, RecentFileItem } from './app_context';
import type { SSHHandler } from './ssh_handler';

export class ModalController {
    private recentActiveTab: 'local' | 'ssh' | 'ftp' = 'local';
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

        document.querySelectorAll<HTMLElement>('[data-recent-tab]').forEach((tab) => {
            tab.addEventListener('click', () => {
                const id = tab.dataset.recentTab as 'local' | 'ssh' | 'ftp' | undefined;
                if (!id) return;
                this.recentActiveTab = id;
                document.querySelectorAll('[data-recent-tab]').forEach((t) =>
                    t.classList.toggle('active', t === tab));
                document.querySelectorAll('[data-recent-panel]').forEach((p) => {
                    p.classList.toggle('active', (p as HTMLElement).dataset.recentPanel === id);
                });
            });
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

    private classifyRecent(f: RecentFileItem): 'local' | 'ssh' | 'ftp' {
        if (!f.is_ssh) return 'local';
        const proto = (f.ssh?.protocol || 'ssh').toLowerCase();
        return proto === 'ftp' || proto === 'ftps' ? 'ftp' : 'ssh';
    }

    private renderRecentList(listEl: HTMLElement, files: RecentFileItem[], emptyKey: string) {
        const { getApi, recentModal } = this.ctx;
        const api = getApi();
        listEl.innerHTML = '';
        if (!files.length) {
            const empty = document.createElement('li');
            empty.className = 'recent-item recent-item-empty';
            empty.textContent = t(emptyKey);
            listEl.appendChild(empty);
            return;
        }
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
            const kind = this.classifyRecent(f);
            const sizeStr = kind === 'local'
                ? `${(f.file_size / 1024 / 1024).toFixed(2)} MB`
                : (kind === 'ftp' ? 'FTP' : t('recent_ssh_badge'));
            const dir = (f.path || '').replace(/[/\\][^/\\]*$/, '') || f.path;
            infoDiv.textContent = `${sizeStr} · ${f.last_opened}`;
            infoDiv.title = dir;

            main.appendChild(pathDiv);
            main.appendChild(infoDiv);

            const actions = document.createElement('div');
            actions.className = 'recent-item-actions';

            if (kind !== 'local') {
                const badge = document.createElement('span');
                badge.className = 'recent-badge';
                badge.textContent = kind === 'ftp' ? 'FTP' : t('recent_ssh_badge');
                actions.appendChild(badge);
            }

            if (f.is_ssh && f.ssh) {
                const browseBtn = document.createElement('button');
                browseBtn.type = 'button';
                browseBtn.className = 'recent-browse-btn';
                browseBtn.textContent = t('recent_browse_server');
                browseBtn.title = t('recent_browse_server_title');
                browseBtn.setAttribute('aria-label', t('recent_browse_server_title'));
                browseBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    recentModal.classList.add('hidden');
                    await this.sshHandler.browseOnServerFromRecent(f);
                });
                actions.appendChild(browseBtn);
            }

            const removeBtn = document.createElement('button');
            removeBtn.type = 'button';
            removeBtn.className = 'recent-remove-btn';
            removeBtn.title = t('recent_remove_title');
            removeBtn.setAttribute('aria-label', t('recent_remove_title'));
            removeBtn.innerHTML = '<svg class="icon" viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="M6 6l12 12"/></svg>';
            removeBtn.addEventListener('click', async (e) => {
                e.stopPropagation();
                try {
                    if (api) await api.RemoveRecentFile(f.path);
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
                } else if (api) {
                    const stats = await api.OpenFile(f.path);
                    if (stats) this.ctx.addOrUpdateTab(stats);
                }
            });
            listEl.appendChild(li);
        });
    }

    public loadRecent = async () => {
        const { getApi, recentModal } = this.ctx;
        const api = getApi();
        if (!api) return;
        const files: RecentFileItem[] = (await api.GetRecentFiles()) || [];

        const local = files.filter((f) => this.classifyRecent(f) === 'local');
        const ssh = files.filter((f) => this.classifyRecent(f) === 'ssh');
        const ftp = files.filter((f) => this.classifyRecent(f) === 'ftp');

        const localEl = document.getElementById('recent-files-list-local');
        const sshEl = document.getElementById('recent-files-list-ssh');
        const ftpEl = document.getElementById('recent-files-list-ftp');
        if (localEl) this.renderRecentList(localEl, local, 'no_recent_local');
        if (sshEl) this.renderRecentList(sshEl, ssh, 'no_recent_ssh');
        if (ftpEl) this.renderRecentList(ftpEl, ftp, 'no_recent_ftp');

        // Sync tab UI with last active category (prefer one that has items).
        let tab = this.recentActiveTab;
        if (tab === 'local' && local.length === 0) {
            if (ssh.length) tab = 'ssh';
            else if (ftp.length) tab = 'ftp';
        }
        this.recentActiveTab = tab;
        document.querySelectorAll('[data-recent-tab]').forEach((t) =>
            t.classList.toggle('active', (t as HTMLElement).dataset.recentTab === tab));
        document.querySelectorAll('[data-recent-panel]').forEach((p) =>
            p.classList.toggle('active', (p as HTMLElement).dataset.recentPanel === tab));

        recentModal.classList.remove('hidden');
    };
}
