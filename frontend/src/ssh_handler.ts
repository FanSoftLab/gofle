import { t } from './i18n';
import type { AppContext, RecentFileItem, RemoteConfig } from './app_context';

interface FieldRef {
    input: HTMLInputElement | HTMLSelectElement;
    err: HTMLElement;
}

// fmtBytes renders a byte count as a compact human string for the progress line.
function fmtBytes(n: number): string {
    if (!Number.isFinite(n) || n < 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
}

export class SSHHandler {
    private ctx: AppContext;
    private pendingHost: string | null = null;
    // pendingIsFTP marks the in-flight trust prompt as an FTPS certificate
    // (vs an SSH host key); the accept button then routes to AckFTPCert.
    private pendingIsFTP: boolean = false;
    // pendingOpenConfig is the RemoteConfig to re-open after an FTP trust
    // prompt (ftp has no AcceptSSHHost equivalent that re-opens for us).
    private pendingOpenConfig: any = null;

    // Remote directory browser state.
    private browsePath: string = '~';
    private browseEntries: any[] = [];
    private browseCursor: number = 0;
    private browseLoading: boolean = false;
    // browseAfterTrust re-opens the browser (instead of opening the file) once
    // the user trusts an unknown host key encountered during listing.
    private browseAfterTrust: boolean = false;

    constructor(ctx: AppContext) {
        this.ctx = ctx;
        this.initHandlers();
    }

    // getFieldRefs returns the per-field input + error-span pairs. It resolves
    // elements by id (rather than extending AppContext) so the validation UI can
    // live entirely in this file.
    private getFieldRefs(): FieldRef[] {
        const ref = (inputId: string, errId: string): FieldRef => ({
            input: document.getElementById(inputId) as HTMLInputElement,
            err: document.getElementById(errId) as HTMLElement,
        });
        return [
            ref('ssh-host', 'err-ssh-host'),
            ref('ssh-port', 'err-ssh-port'),
            ref('ssh-username', 'err-ssh-username'),
            ref('ssh-password', 'err-ssh-password'),
            ref('ssh-key-path', 'err-ssh-key-path'),
            ref('ssh-remote-path', 'err-ssh-remote-path'),
        ];
    }

    private clearFieldState() {
        for (const { input, err } of this.getFieldRefs()) {
            input.classList.remove('is-invalid', 'is-valid');
            if (err) err.classList.add('hidden');
        }
    }

    private markInvalid(ref: FieldRef, msg: string) {
        ref.input.classList.add('is-invalid');
        ref.input.classList.remove('is-valid');
        if (ref.err) {
            ref.err.textContent = msg;
            ref.err.classList.remove('hidden');
        }
        ref.input.focus();
    }

    private markValid(ref: FieldRef) {
        ref.input.classList.add('is-valid');
        ref.input.classList.remove('is-invalid');
    }

    // applyAuthVisibility shows the credential fields matching the selected auth
    // type. Called on every select change AND once at startup: the markup's
    // default visibility must never drift from the select's default value, or
    // the password field stays hidden and submit silently fails validation.
    private applyAuthVisibility() {
        const { sshAuthSelect, groupPassword, groupKeyPath, groupKeyPass } = this.ctx;
        if (sshAuthSelect.value === 'password') {
            groupPassword.classList.remove('hidden');
            groupKeyPath.classList.add('hidden');
            groupKeyPass.classList.add('hidden');
        } else {
            groupPassword.classList.add('hidden');
            groupKeyPath.classList.remove('hidden');
            groupKeyPass.classList.remove('hidden');
        }
    }

    // applyProtocolVisibility shows the credential/TLS fields matching the
    // selected protocol. Called on every protocol change AND at startup: like
    // applyAuthVisibility, the markup's default visibility must track the
    // select's default value or the TLS/group fields drift out of sync.
    //   ssh -> password/key groups as now, TLS mode hidden
    //   ftp -> key groups hidden (password only), TLS mode shown
    private applyProtocolVisibility() {
        const { sshProtocolSelect, sshAuthSelect, groupPassword, groupKeyPath, groupKeyPass } = this.ctx;
        const tlsGroup = document.getElementById('group-ssh-tls');
        const portEl = document.getElementById('ssh-port') as HTMLInputElement | null;
        if (sshProtocolSelect.value === 'ftp') {
            groupKeyPath.classList.add('hidden');
            groupKeyPass.classList.add('hidden');
            groupPassword.classList.remove('hidden');
            // ftp has no key auth — collapse it to password only.
            sshAuthSelect.value = 'password';
            if (tlsGroup) tlsGroup.classList.remove('hidden');
            // Default the port to 21 only when it still holds the ssh default.
            if (portEl && portEl.value === '22') portEl.value = '21';
        } else {
            if (tlsGroup) tlsGroup.classList.add('hidden');
            if (portEl && portEl.value === '21') portEl.value = '22';
            this.applyAuthVisibility();
        }
    }

    // ---- SSH open progress overlay -------------------------------------
    // The backend streams the remote file in stages (connecting / downloading
    // / indexing) and emits `ssh:progress` events; this overlay turns them
    // into a spinner + bar so the wait is visible. It also blocks pointer
    // input, which incidentally prevents double-open clicks.

    private showProgress() {
        const overlay = document.getElementById('ssh-progress-overlay');
        if (!overlay) return;
        const stage = document.getElementById('ssh-progress-stage');
        if (stage) stage.textContent = t('prog_connecting');
        const bytes = document.getElementById('ssh-progress-bytes');
        if (bytes) bytes.innerHTML = '&nbsp;';
        overlay.classList.remove('hidden');
    }

    private hideProgress() {
        document.getElementById('ssh-progress-overlay')?.classList.add('hidden');
    }

    private onSSHProgress = (data: any) => {
        const overlay = document.getElementById('ssh-progress-overlay');
        if (!overlay || overlay.classList.contains('hidden')) return;
        const stageEl = document.getElementById('ssh-progress-stage');
        const bar = document.getElementById('ssh-progress-bar') as HTMLDivElement | null;
        const bytesEl = document.getElementById('ssh-progress-bytes');
        if (!stageEl || !bar || !bytesEl) return;

        const stage = String(data?.stage ?? '');
        if (stage === 'downloading') {
            stageEl.textContent = t('prog_downloading');
        } else if (stage === 'indexing') {
            stageEl.textContent = t('prog_indexing');
        }

        const done = Number(data?.done ?? 0);
        const total = Number(data?.total ?? -1);
        if (stage === 'downloading' && total > 0) {
            const pct = Math.min(100, Math.round((done / total) * 100));
            bar.classList.remove('ssh-progress-bar-indeterminate');
            bar.style.width = `${pct}%`;
            bytesEl.textContent = `${fmtBytes(done)} / ${fmtBytes(total)} · ${pct}%`;
        } else if (stage === 'downloading') {
            // Total unknown: keep an indeterminate slide, still show volume.
            bar.classList.add('ssh-progress-bar-indeterminate');
            bar.style.width = '';
            bytesEl.textContent = fmtBytes(done);
        } else {
            bar.classList.add('ssh-progress-bar-indeterminate');
            bar.style.width = '';
            bytesEl.innerHTML = '&nbsp;';
        }
    };

    private initHandlers = () => {
        const { sshModal, sshAuthSelect, sshProtocolSelect, errSSH, btnOpenSSH } = this.ctx;

        this.applyProtocolVisibility();

        // Backend open-stage events drive the progress overlay.
        (window as any).runtime?.EventsOn?.('ssh:progress', this.onSSHProgress);

        btnOpenSSH.addEventListener('click', () => {
            sshModal.classList.remove('hidden');
            errSSH.classList.add('hidden');
            this.clearFieldState();
        });

        sshAuthSelect.addEventListener('change', () => {
            this.applyAuthVisibility();
            this.clearFieldState();
        });

        sshProtocolSelect.addEventListener('change', () => {
            this.applyProtocolVisibility();
            this.clearFieldState();
        });

        // Live feedback: clearing a field's invalid state as the user edits it.
        for (const { input, err } of this.getFieldRefs()) {
            const clear = () => {
                input.classList.remove('is-invalid', 'is-valid');
                if (err) err.classList.add('hidden');
            };
            input.addEventListener('input', clear);
            input.addEventListener('change', clear);
        }

        document.getElementById('btn-submit-ssh')?.addEventListener('click', () => this.submit());

        document.getElementById('btn-cancel-ssh')?.addEventListener('click', () => {
            sshModal.classList.add('hidden');
        });

        document.getElementById('btn-accept-hostkey')?.addEventListener('click', () => this.acceptHostKey());
        document.getElementById('btn-cancel-hostkey')?.addEventListener('click', () => {
            document.getElementById('ssh-hostkey-modal')?.classList.add('hidden');
        });

        document.getElementById('btn-browse-ssh')?.addEventListener('click', () => this.openBrowse());
        document.getElementById('btn-browse-up')?.addEventListener('click', () => this.browseUp());
        document.getElementById('btn-browse-open')?.addEventListener('click', () => this.browseOpenSelected());
        document.getElementById('btn-browse-goto')?.addEventListener('click', () => this.browseGotoPath());
        document.getElementById('btn-browse-home')?.addEventListener('click', () => {
            this.browsePath = '~';
            this.browseCursor = 0;
            this.listRemoteDir();
        });
        document.getElementById('btn-browse-cancel')?.addEventListener('click', () => {
            document.getElementById('ssh-browse-modal')?.classList.add('hidden');
        });

        // Keyboard navigation inside the remote directory browser.
        const pathInput = document.getElementById('ssh-browse-path') as HTMLInputElement | null;
        pathInput?.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                this.browseGotoPath();
            }
        });
        const listEl = document.getElementById('ssh-browse-list');
        listEl?.addEventListener('keydown', (e) => this.browseKeyDown(e));
        // Capture arrows while the modal is open even if focus is elsewhere in it.
        document.getElementById('ssh-browse-modal')?.addEventListener('keydown', (e) => {
            const modal = document.getElementById('ssh-browse-modal');
            if (!modal || modal.classList.contains('hidden')) return;
            const tag = (e.target as HTMLElement)?.tagName;
            if (tag === 'INPUT' || tag === 'TEXTAREA') return;
            this.browseKeyDown(e);
        });
    };

    // parseSSHError detects the structured "unknown host key" error raised by the
    // backend and returns [host, fingerprint] when present, or null otherwise.
    // The message reaches us double-wrapped ("failed to connect via SSH to …:
    // ssh: handshake failed: unknown host key for …"), so the sentinel is
    // matched anywhere in the string, not anchored at the start.
    private parseSSHError(err: unknown): [string, string] | null {
        const msg = String((err as any)?.message ?? err);
        const m = /unknown host key for (.+?) \(fingerprint (.+?)\)/.exec(msg);
        if (m) return [m[1], m[2]];
        return null;
    }

    // parseFTPError detects the structured "unknown FTP server certificate" error
    // raised by the backend for explicit FTPS and returns [host, fingerprint].
    private parseFTPError(err: unknown): [string, string] | null {
        const msg = String((err as any)?.message ?? err);
        const m = /unknown FTP server certificate for (.+?) \(fingerprint (.+?)\)/.exec(msg);
        if (m) return [m[1], m[2]];
        return null;
    }

    // handleTrustResult centralizes the open/accept error handling so the submit
    // and reopen paths behave identically: an unknown SSH host key OR an unknown
    // FTPS certificate opens the trust modal (adapted copy), everything else
    // surfaces as a plain connection error.
    private handleTrustResult(err: unknown) {
        const { errSSH } = this.ctx;
        const sshU = this.parseSSHError(err);
        if (sshU) {
            this.showTrustModal(sshU[0], sshU[1], false);
            return;
        }
        const ftpU = this.parseFTPError(err);
        if (ftpU) {
            this.showTrustModal(ftpU[0], ftpU[1], true);
            return;
        }
        errSSH.textContent = String(err);
        errSSH.classList.remove('hidden');
    }

    // showTrustModal opens the shared #ssh-hostkey-modal for either an SSH host
    // key (isFTP=false) or an FTPS certificate (isFTP=true). The title and
    // warning copy are swapped to match the protocol.
    private showTrustModal(host: string, fingerprint: string, isFTP: boolean) {
        this.pendingHost = host;
        this.pendingIsFTP = isFTP;
        const hostEl = document.getElementById('ssh-hostkey-host');
        const fpEl = document.getElementById('ssh-hostkey-fingerprint');
        const titleEl = document.getElementById('ssh-hostkey-modal-title');
        const warnEl = document.getElementById('ssh-hostkey-warning');
        if (hostEl) hostEl.textContent = host;
        if (fpEl) fpEl.textContent = fingerprint;
        if (titleEl) titleEl.textContent = isFTP ? t('ssh_cert_modal_title') : t('ssh_hostkey_modal_title');
        if (warnEl) warnEl.textContent = isFTP ? t('ssh_cert_warning') : t('ssh_hostkey_warning');
        document.getElementById('ssh-hostkey-modal')?.classList.remove('hidden');
    }

    private submit = async () => {
        const { sshModal, errSSH } = this.ctx;
        errSSH.classList.add('hidden');
        this.clearFieldState();

        const host = (document.getElementById('ssh-host') as HTMLInputElement).value.trim();
        const portRaw = (document.getElementById('ssh-port') as HTMLInputElement).value.trim();
        const username = (document.getElementById('ssh-username') as HTMLInputElement).value.trim();
        const authType = this.ctx.sshAuthSelect.value;
        const password = (document.getElementById('ssh-password') as HTMLInputElement).value;
        const keyPath = (document.getElementById('ssh-key-path') as HTMLInputElement).value.trim();
        const keyPassphrase = (document.getElementById('ssh-key-pass') as HTMLInputElement).value;
        const remoteFilePath = (document.getElementById('ssh-remote-path') as HTMLInputElement).value.trim();

        const refs = this.getFieldRefs();
        const byId = (id: string) => refs.find(r => r.input.id === id)!;

        // Step through the fields in order; mark each valid one green, and on the
        // first failure mark it red, surface its message, focus it, and stop.
        if (!host) {
            this.markInvalid(byId('ssh-host'), t('err_ssh_host_required'));
            return;
        }
        this.markValid(byId('ssh-host'));

        let port: number;
        if (portRaw === '') {
            port = 22;
        } else {
            port = parseInt(portRaw, 10);
            if (isNaN(port) || port < 1 || port > 65535) {
                this.markInvalid(byId('ssh-port'), t('err_ssh_port_invalid'));
                return;
            }
        }
        this.markValid(byId('ssh-port'));

        if (!username) {
            this.markInvalid(byId('ssh-username'), t('err_ssh_username_required'));
            return;
        }
        this.markValid(byId('ssh-username'));

        if (authType === 'password') {
            if (!password) {
                this.markInvalid(byId('ssh-password'), t('err_ssh_password_required'));
                return;
            }
            this.markValid(byId('ssh-password'));
        } else if (authType === 'key') {
            // key_passphrase is optional.
            if (!keyPath) {
                this.markInvalid(byId('ssh-key-path'), t('err_ssh_key_path_required'));
                return;
            }
            this.markValid(byId('ssh-key-path'));
        } else {
            errSSH.textContent = t('err_field_required');
            errSSH.classList.remove('hidden');
            return;
        }

        if (!remoteFilePath) {
            // The remote file path is optional: with no path, open the interactive
            // directory browser so the user can pick a log on the server.
            await this.openBrowse();
            return;
        }
        this.markValid(byId('ssh-remote-path'));

        const api = this.ctx.getApi();
        if (api) {
            try {
                this.showProgress();
                const cfg = this.buildRemoteConfig();
                this.pendingOpenConfig = cfg;
                const stats = await api.OpenRemoteFile(cfg);
                if (stats) {
                    this.ctx.addOrUpdateTab(stats);
                    sshModal.classList.add('hidden');
                }
            } catch (err: any) {
                this.handleTrustResult(err);
            } finally {
                this.hideProgress();
            }
        }
    };

    // acceptHostKey is invoked from the unknown-host trust modal. It asks the
    // backend to persist the host key / certificate and re-run the open path.
    // For SSH the open is re-run by the backend via AcceptSSHHost; for FTP we
    // re-call OpenRemoteFile ourselves (ftp has no Accept* wrapper). When the
    // unknown host/cert was hit while browsing, it re-opens the browser instead.
    public acceptHostKey = async () => {
        const host = this.pendingHost;
        if (!host) return;
        const api = this.ctx.getApi();
        if (!api) return;

        document.getElementById('ssh-hostkey-modal')?.classList.add('hidden');

        if (this.pendingIsFTP) {
            try {
                await api.AckFTPCert(host);
            } catch (err: any) {
                this.handleTrustResult(err);
                return;
            }
            if (this.browseAfterTrust) {
                this.browseAfterTrust = false;
                this.browsePath = '~';
                this.browseCursor = 0;
                document.getElementById('ssh-browse-modal')?.classList.remove('hidden');
                await this.listRemoteDir();
                return;
            }
            if (this.pendingOpenConfig) {
                const cfg = this.pendingOpenConfig;
                try {
                    this.showProgress();
                    const stats = await api.OpenRemoteFile(cfg);
                    if (stats) {
                        this.ctx.addOrUpdateTab(stats);
                        this.ctx.sshModal.classList.add('hidden');
                    }
                } catch (err: any) {
                    this.handleTrustResult(err);
                } finally {
                    this.hideProgress();
                }
            }
            return;
        }

        if (this.browseAfterTrust) {
            this.browseAfterTrust = false;
            try {
                await api.AckSSHHost(host);
                this.browsePath = '~';
                this.browseCursor = 0;
                document.getElementById('ssh-browse-modal')?.classList.remove('hidden');
                await this.listRemoteDir();
            } catch (err: any) {
                this.handleTrustResult(err);
            }
            return;
        }

        try {
            this.showProgress();
            const stats = await api.AcceptSSHHost(host);
            if (stats) {
                this.ctx.addOrUpdateTab(stats);
                this.ctx.sshModal.classList.add('hidden');
            }
        } catch (err: any) {
            this.handleTrustResult(err);
        } finally {
            this.hideProgress();
        }
    };

    // ---- Remote directory browser ---------------------------------------

    private getFieldValue(id: string): string {
        const el = document.getElementById(id) as HTMLInputElement | null;
        return el ? el.value.trim() : '';
    }

    // buildRemoteConfig assembles the protocol-agnostic RemoteConfig contract
    // from the current form fields (used by both submit and the browser). The
    // port defaults to 22 for ssh and 21 for ftp when left blank; the TLS mode
    // is included only for ftp.
    private buildRemoteConfig(): RemoteConfig {
        const host = this.getFieldValue('ssh-host');
        const portRaw = this.getFieldValue('ssh-port');
        const username = this.getFieldValue('ssh-username');
        const authType = this.ctx.sshAuthSelect.value;
        const password = (document.getElementById('ssh-password') as HTMLInputElement).value;
        const keyPath = this.getFieldValue('ssh-key-path');
        const keyPassphrase = (document.getElementById('ssh-key-pass') as HTMLInputElement).value;
        const remoteFilePath = this.getFieldValue('ssh-remote-path');
        const protocol = this.ctx.sshProtocolSelect.value;
        const tlsMode = protocol === 'ftp' ? (this.ctx.sshTLSSelect.value || 'none') : 'none';
        const defaultPort = protocol === 'ftp' ? 21 : 22;
        return {
            protocol,
            host,
            port: portRaw === '' ? defaultPort : parseInt(portRaw, 10),
            username,
            auth_type: authType,
            password,
            key_path: keyPath,
            key_passphrase: keyPassphrase,
            remote_file_path: remoteFilePath,
            tls_mode: tlsMode,
        } as RemoteConfig;
    }

    // validateConnectBasics checks host/port/username/auth without requiring a
    // remote file path — that is what makes the path optional in the browser.
    private validateConnectBasics(): boolean {
        const host = this.getFieldValue('ssh-host');
        const portRaw = this.getFieldValue('ssh-port');
        const username = this.getFieldValue('ssh-username');
        const authType = this.ctx.sshAuthSelect.value;
        const refs = this.getFieldRefs();
        const byId = (id: string) => refs.find(r => r.input.id === id)!;

        if (!host) {
            this.markInvalid(byId('ssh-host'), t('err_ssh_host_required'));
            return false;
        }
        this.markValid(byId('ssh-host'));

        let port: number;
        if (portRaw === '') {
            port = 22;
        } else {
            port = parseInt(portRaw, 10);
            if (isNaN(port) || port < 1 || port > 65535) {
                this.markInvalid(byId('ssh-port'), t('err_ssh_port_invalid'));
                return false;
            }
        }
        this.markValid(byId('ssh-port'));

        if (!username) {
            this.markInvalid(byId('ssh-username'), t('err_ssh_username_required'));
            return false;
        }
        this.markValid(byId('ssh-username'));

        if (authType === 'password') {
            const pw = (document.getElementById('ssh-password') as HTMLInputElement).value;
            if (!pw) {
                this.markInvalid(byId('ssh-password'), t('err_ssh_password_required'));
                return false;
            }
            this.markValid(byId('ssh-password'));
        } else if (authType === 'key') {
            const kp = this.getFieldValue('ssh-key-path');
            if (!kp) {
                this.markInvalid(byId('ssh-key-path'), t('err_ssh_key_path_required'));
                return false;
            }
            this.markValid(byId('ssh-key-path'));
        } else {
            this.ctx.errSSH.textContent = t('err_field_required');
            this.ctx.errSSH.classList.remove('hidden');
            return false;
        }
        return true;
    }

    // openBrowse validates the connection fields (path stays optional) and opens
    // the remote directory browser starting at the remote home (~).
    private openBrowse = async () => {
        const { errSSH, sshModal } = this.ctx;
        errSSH.classList.add('hidden');
        this.clearFieldState();
        this.browseAfterTrust = false;
        if (!this.validateConnectBasics()) {
            return;
        }
        this.browsePath = '~';
        this.browseCursor = 0;
        this.browseEntries = [];
        this.browseLoading = false;
        document.getElementById('ssh-browse-modal')?.classList.remove('hidden');
        await this.listRemoteDir();
    };

    // listRemoteDir calls the backend, rendering results (or a host-key / cert
    // trust prompt / error) into the browser modal.
    private async listRemoteDir() {
        const api = this.ctx.getApi();
        if (!api) return;
        this.browseLoading = true;
        const pathEl = document.getElementById('ssh-browse-path') as HTMLInputElement | null;
        if (pathEl) pathEl.value = this.browsePath;
        const errEl = document.getElementById('ssh-browse-error');
        if (errEl) errEl.classList.add('hidden');
        document.getElementById('ssh-browse-loading')?.classList.add('hidden');
        this.renderBrowse();

        try {
            const cfg = this.buildRemoteConfig();
            this.pendingOpenConfig = cfg;
            const entries = await api.ListRemoteFiles(cfg, this.browsePath);
            this.browseEntries = entries || [];
            // Backend resolves "~"/"" to the remote home, but returns only the
            // entries. Promote browsePath to the absolute parent so "Up" can
            // leave the home directory (previously stuck on "~").
            if ((this.browsePath === '~' || this.browsePath === '') && this.browseEntries.length > 0) {
                this.browsePath = this.parentOfPath(this.browseEntries[0].path);
                if (pathEl) pathEl.value = this.browsePath;
            }
            this.browseCursor = 0;
        } catch (err: any) {
            const sshU = this.parseSSHError(err);
            const ftpU = this.parseFTPError(err);
            const unknown = sshU || ftpU;
            if (unknown) {
                this.browseAfterTrust = true;
                this.pendingIsFTP = !!ftpU;
                document.getElementById('ssh-browse-modal')?.classList.add('hidden');
                this.showTrustModal(unknown[0], unknown[1], !!ftpU);
                return;
            }
            const e = document.getElementById('ssh-browse-error');
            if (e) {
                e.textContent = t('ssh_browse_error') + ': ' + String(err);
                e.classList.remove('hidden');
            }
        } finally {
            this.browseLoading = false;
            this.renderBrowse();
            // Keep keyboard focus on the list for arrow navigation.
            document.getElementById('ssh-browse-list')?.focus({ preventScroll: true });
        }
    }

    // parentOfPath returns the directory containing path, or "/" at the root.
    private parentOfPath(path: string): string {
        let p = path || '/';
        if (p.endsWith('/') && p.length > 1) p = p.slice(0, -1);
        const idx = p.lastIndexOf('/');
        if (idx <= 0) return '/';
        return p.slice(0, idx) || '/';
    }

    // browseGotoPath jumps to the path typed in the path bar (Enter / Go).
    private browseGotoPath = () => {
        const pathEl = document.getElementById('ssh-browse-path') as HTMLInputElement | null;
        let next = (pathEl?.value || '').trim();
        if (!next) next = '~';
        // Normalize trailing slash except for root.
        if (next.length > 1 && next.endsWith('/')) next = next.slice(0, -1);
        this.browsePath = next;
        this.browseCursor = 0;
        this.listRemoteDir();
    };

    // browseUp navigates one directory level up (unless already at filesystem root).
    private browseUp = () => {
        if (this.browsePath === '/') return;
        // "~" is treated as home: resolve first list already promotes it to an
        // absolute path; if somehow still "~", go to "/".
        if (this.browsePath === '~' || this.browsePath === '') {
            this.browsePath = '/';
        } else {
            this.browsePath = this.parentOfPath(this.browsePath);
        }
        this.browseCursor = 0;
        this.listRemoteDir();
    };

    // browseOpenSelected opens the highlighted entry: a dir navigates into it, a
    // file fills the remote-path field and closes the browser.
    private browseOpenSelected = () => {
        const entry = this.browseEntries[this.browseCursor];
        if (!entry) return;
        if (entry.kind === 'dir') {
            this.browsePath = entry.path;
            this.browseCursor = 0;
            this.listRemoteDir();
            return;
        }
        this.selectRemoteFile(entry.path);
    };

    private selectRemoteFile(path: string) {
        (document.getElementById('ssh-remote-path') as HTMLInputElement).value = path;
        document.getElementById('ssh-browse-modal')?.classList.add('hidden');
    }

    // browseKeyDown handles arrow / enter / backspace while the browser is open.
    private browseKeyDown = (e: KeyboardEvent) => {
        const modal = document.getElementById('ssh-browse-modal');
        if (!modal || modal.classList.contains('hidden') || this.browseLoading) return;

        switch (e.key) {
            case 'ArrowUp':
                e.preventDefault();
                if (this.browseEntries.length === 0) return;
                this.browseCursor = (this.browseCursor + this.browseEntries.length - 1) % this.browseEntries.length;
                this.renderBrowse();
                this.scrollBrowseCursorIntoView();
                break;
            case 'ArrowDown':
                e.preventDefault();
                if (this.browseEntries.length === 0) return;
                this.browseCursor = (this.browseCursor + 1) % this.browseEntries.length;
                this.renderBrowse();
                this.scrollBrowseCursorIntoView();
                break;
            case 'Enter':
                e.preventDefault();
                this.browseOpenSelected();
                break;
            case 'Backspace':
                // Don't steal Backspace from the path input.
                if ((e.target as HTMLElement)?.tagName === 'INPUT') return;
                e.preventDefault();
                this.browseUp();
                break;
            case 'Home':
                if (e.ctrlKey || e.metaKey) {
                    e.preventDefault();
                    this.browsePath = '~';
                    this.browseCursor = 0;
                    this.listRemoteDir();
                }
                break;
        }
    };

    private scrollBrowseCursorIntoView() {
        const listEl = document.getElementById('ssh-browse-list');
        const sel = listEl?.querySelector('.ssh-browse-selected') as HTMLElement | null;
        sel?.scrollIntoView({ block: 'nearest' });
    }

    // renderBrowse paints the current directory listing: an "up" pseudo-row
    // (except at filesystem root), a loading/empty placeholder, then dirs-first
    // entries with the cursor highlighted.
    private renderBrowse() {
        const listEl = document.getElementById('ssh-browse-list');
        if (!listEl) return;

        const frag = document.createDocumentFragment();
        // Always offer ".." unless we are already at the filesystem root. Home
        // is no longer a hard ceiling — users can walk up to /var/log etc.
        if (this.browsePath !== '/') {
            const li = document.createElement('li');
            li.className = 'ssh-browse-up';
            li.textContent = '▸ ..';
            li.addEventListener('click', () => this.browseUp());
            frag.appendChild(li);
        }

        if (this.browseLoading) {
            const li = document.createElement('li');
            li.className = 'ssh-browse-loading';
            li.textContent = t('ssh_browse_loading');
            frag.appendChild(li);
        } else if (this.browseEntries.length === 0) {
            const li = document.createElement('li');
            li.className = 'ssh-browse-empty';
            li.textContent = t('ssh_browse_empty');
            frag.appendChild(li);
        } else {
            this.browseEntries.forEach((e, i) => {
                const li = document.createElement('li');
                const icon = e.kind === 'dir' ? '▸' : (e.kind === 'symlink' ? '→' : '·');
                li.textContent = `${icon} ${e.name}` + (e.kind === 'file' ? `  (${fmtBytes(e.size)})` : '');
                if (i === this.browseCursor) li.classList.add('ssh-browse-selected');
                // A single click both records the cursor (so "Open selected"
                // acts on the right entry) and performs the action. We do NOT
                // re-render on mouseenter: rebuilding the list on every hover
                // made the app sluggish and replaced the node under the cursor
                // so clicks frequently failed to register.
                li.addEventListener('click', () => {
                    this.browseCursor = i;
                    if (e.kind === 'dir') {
                        this.browsePath = e.path;
                        this.listRemoteDir();
                    } else {
                        this.selectRemoteFile(e.path);
                    }
                });
                frag.appendChild(li);
            });
        }

        listEl.innerHTML = '';
        listEl.appendChild(frag);

        const spinner = document.getElementById('ssh-browse-loading');
        if (spinner) spinner.classList.toggle('hidden', !this.browseLoading);
    }

    // fillRemoteFormFromRecent writes saved SSH/FTP credentials into the connect
    // form without opening a file. Shared by reopen and "other file on server".
    private fillRemoteFormFromRecent(f: RecentFileItem) {
        if (!f.ssh) return;
        const protocol = f.ssh.protocol || 'ssh';
        const tlsMode = f.ssh.tls_mode || 'none';
        (document.getElementById('ssh-host') as HTMLInputElement).value = f.ssh.host || '';
        (document.getElementById('ssh-port') as HTMLInputElement).value = String(f.ssh.port || (protocol === 'ftp' ? 21 : 22));
        (document.getElementById('ssh-username') as HTMLInputElement).value = f.ssh.username || '';
        (document.getElementById('ssh-protocol') as HTMLSelectElement).value = protocol;
        (document.getElementById('ssh-protocol') as HTMLSelectElement).dispatchEvent(new Event('change'));
        (document.getElementById('ssh-auth-type') as HTMLSelectElement).value = f.ssh.auth_type || 'password';
        (document.getElementById('ssh-auth-type') as HTMLSelectElement).dispatchEvent(new Event('change'));
        (document.getElementById('ssh-password') as HTMLInputElement).value = f.ssh.password || '';
        (document.getElementById('ssh-key-path') as HTMLInputElement).value = f.ssh.key_path || '';
        (document.getElementById('ssh-key-pass') as HTMLInputElement).value = f.ssh.key_passphrase || '';
        (document.getElementById('ssh-remote-path') as HTMLInputElement).value = f.ssh.remote_file_path || '';
        if (protocol === 'ftp') {
            (document.getElementById('ssh-tls-mode') as HTMLSelectElement).value = tlsMode;
        }
        this.clearFieldState();
    }

    // startPathForRecent picks a sensible browser start directory for a saved
    // remote entry: the parent of the last opened file, or "~" when unknown.
    private startPathForRecent(f: RecentFileItem): string {
        const remote = (f.ssh?.remote_file_path || '').trim();
        if (!remote || remote === '~') return '~';
        // Strip trailing slash then drop the last segment.
        let p = remote.endsWith('/') && remote.length > 1 ? remote.slice(0, -1) : remote;
        const idx = p.lastIndexOf('/');
        if (idx <= 0) return '/';
        return p.slice(0, idx) || '/';
    }

    // browseOnServerFromRecent opens the remote directory browser on a saved
    // SSH/FTP server so the user can pick a different log without re-entering
    // credentials. Starts in the directory of the previously opened file.
    public browseOnServerFromRecent = async (f: RecentFileItem) => {
        const { sshModal, errSSH } = this.ctx;
        if (!f.ssh) return;
        errSSH?.classList.add('hidden');
        this.fillRemoteFormFromRecent(f);
        // Clear the file path field: we are choosing a new one.
        (document.getElementById('ssh-remote-path') as HTMLInputElement).value = '';
        sshModal.classList.remove('hidden');
        this.browseAfterTrust = false;
        if (!this.validateConnectBasics()) {
            return;
        }
        this.browsePath = this.startPathForRecent(f);
        this.browseCursor = 0;
        this.browseEntries = [];
        this.browseLoading = false;
        document.getElementById('ssh-browse-modal')?.classList.remove('hidden');
        await this.listRemoteDir();
    };

    // browseOnServerFromTab opens the browser using credentials stored on an
    // already-open remote tab (same host, different log file).
    public browseOnServerFromTab = async (ssh: any) => {
        if (!ssh) return;
        // RecentFileItem is a Wails-generated class (requires convertValues); build
        // via constructor so tsc accepts the value without a structural mismatch.
        const item = {
            path: '',
            file_size: 0,
            last_opened: '',
            is_ssh: true,
            ssh,
        } as unknown as RecentFileItem;
        await this.browseOnServerFromRecent(item);
    };

    public reopenFromRecent = async (f: RecentFileItem) => {

        const { sshModal, getApi } = this.ctx;
        const api = getApi();
        if (!api || !f.ssh) return;

        const protocol = f.ssh.protocol || 'ssh';
        const tlsMode = f.ssh.tls_mode || 'none';
        const buildCfg = (): RemoteConfig => ({
            protocol,
            host: f.ssh!.host,
            port: f.ssh!.port,
            username: f.ssh!.username,
            auth_type: f.ssh!.auth_type,
            password: f.ssh!.password || '',
            key_path: f.ssh!.key_path || '',
            key_passphrase: f.ssh!.key_passphrase || '',
            remote_file_path: f.ssh!.remote_file_path || '',
            tls_mode: tlsMode,
        } as RemoteConfig);

        // For key auth there is no password, so the stored key_path is what makes
        // the entry eligible for auto-reopen (F8).
        const hasCreds = f.ssh.auth_type === 'key'
            ? !!(f.ssh.username && f.ssh.key_path)
            : !!(f.ssh.username && f.ssh.password);
        if (hasCreds && f.ssh.remote_file_path) {
            try {
                this.showProgress();
                const stats = await api.OpenRemoteFile(buildCfg());
                if (stats) {
                    this.ctx.addOrUpdateTab(stats);
                    return;
                }
            } catch (e) {
                const sshU = this.parseSSHError(e);
                const ftpU = this.parseFTPError(e);
                const unknown = sshU || ftpU;
                if (unknown) {
                    // Even a "Recent" reopen can hit an unknown host / cert (e.g.
                    // the trust store entry was removed). Surface the trust modal.
                    this.pendingOpenConfig = buildCfg();
                    this.showTrustModal(unknown[0], unknown[1], !!ftpU);
                    return;
                }
                // other failure — fall through to the form prefilled with the
                // saved credentials so the user can retry immediately
            } finally {
                this.hideProgress();
            }
        }

        // Fill the form with the stored credentials, choosing the right protocol.
        this.fillRemoteFormFromRecent(f);
        sshModal.classList.remove('hidden');

        if (f.ssh.remote_file_path === '') {
            // No saved file path: open the browser with the stored credentials so
            // the user can pick a log interactively instead of erroring out.
            await this.openBrowse();
            return;
        }
    };
}
