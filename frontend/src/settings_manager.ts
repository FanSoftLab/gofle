import { resolveLang, resolveTheme, setLang, applyI18n } from './i18n';
import { showToast } from './toast';
import type { AppContext } from './app_context';

export type AppPrefs = {
    theme: string;
    language: string;
    saveTabsExit: boolean;
    viewMode: string;
    logFontSize: number;
    logFontFamily: string;
    lineDensity: string;
    wordWrap: boolean;
    showLineNumbers: boolean;
    defaultFollow: boolean;
    defaultSearchCase: boolean;
    defaultSearchRegex: boolean;
    defaultSearchInvert: boolean;
    maxRecentFiles: number;
    maxHistorySize: number;
    confirmCloseTab: boolean;
    startupBehavior: string;
    searchOrientation: string;
    searchSide: string;
    sshConnectTimeoutSec: number;
    remoteStallTimeoutSec: number;
    autoAcceptHostKey: boolean;
    connMaxAttempts: number;
    connAttemptWindowMin: number;
    connLockoutMin: number;
    defaultSshPort: number;
    defaultFtpPort: number;
};

const defaultPrefs: AppPrefs = {
    theme: 'system', language: 'system', saveTabsExit: true, viewMode: 'split',
    logFontSize: 13, logFontFamily: 'monospace', lineDensity: 'normal',
    wordWrap: false, showLineNumbers: true, defaultFollow: true,
    defaultSearchCase: false, defaultSearchRegex: false, defaultSearchInvert: false,
    maxRecentFiles: 20, maxHistorySize: 100, confirmCloseTab: true,
    startupBehavior: 'session', searchOrientation: 'horizontal', searchSide: 'bottom',
    sshConnectTimeoutSec: 10, remoteStallTimeoutSec: 60, autoAcceptHostKey: false,
    connMaxAttempts: 5, connAttemptWindowMin: 15, connLockoutMin: 5,
    defaultSshPort: 22, defaultFtpPort: 21,
};

let currentPrefs: AppPrefs = { ...defaultPrefs };

export function getPrefs(): AppPrefs { return currentPrefs; }

export function applyLogFontSize(px: string | number) {
    const n = typeof px === 'number' ? px : parseInt(String(px), 10) || 13;
    document.documentElement.style.setProperty('--log-font-size', `${n}px`);
}

export function applyLogFontFamily(family: string) {
    const v = (family || 'monospace').trim() || 'monospace';
    document.documentElement.style.setProperty('--log-font-family', v);
    // Also set on body so any non-var rules still inherit a concrete family.
    document.body.style.setProperty('--log-font-family', v);
}

export function applyLineDensity(density: string) {
    const map: Record<string, string> = { compact: '1.15', normal: '1.35', comfortable: '1.55' };
    document.documentElement.style.setProperty('--line-density', map[density] || map.normal);
    document.body.dataset.lineDensity = density || 'normal';
}

export function applyWordWrap(enabled: boolean) {
    document.body.classList.toggle('word-wrap', !!enabled);
}

export function applyShowLineNumbers(show: boolean) {
    document.body.classList.toggle('hide-line-numbers', !show);
}

export class SettingsManager {
    private ctx: AppContext;
    private autosaveTimer: ReturnType<typeof setTimeout> | undefined;
    private onPrefsApplied?: (prefs: AppPrefs) => void;

    constructor(ctx: AppContext, onPrefsApplied?: (prefs: AppPrefs) => void) {
        this.ctx = ctx;
        this.onPrefsApplied = onPrefsApplied;
        this.initHandlers();
    }

    public async loadAndApply(): Promise<AppPrefs> {
        const api = this.ctx.getApi();
        if (!api?.GetSettings) return currentPrefs;
        try {
            const cfg = await api.GetSettings();
            currentPrefs = this.mapFromBackend(cfg);
            this.applyPrefs(currentPrefs);
            return currentPrefs;
        } catch (err) {
            console.error('loadAndApply settings failed:', err);
            return currentPrefs;
        }
    }

    private mapFromBackend(cfg: any): AppPrefs {
        return {
            theme: cfg.theme || 'system', language: cfg.language || 'system',
            saveTabsExit: !!cfg.save_tabs_exit, viewMode: cfg.view_mode || 'split',
            logFontSize: cfg.log_font_size || 13, logFontFamily: cfg.log_font_family || 'monospace',
            lineDensity: cfg.line_density || 'normal', wordWrap: !!cfg.word_wrap,
            showLineNumbers: cfg.show_line_numbers !== false, defaultFollow: cfg.default_follow !== false,
            defaultSearchCase: !!cfg.default_search_case, defaultSearchRegex: !!cfg.default_search_regex,
            defaultSearchInvert: !!cfg.default_search_invert, maxRecentFiles: cfg.max_recent_files || 20,
            maxHistorySize: cfg.max_history_size || 100, confirmCloseTab: cfg.confirm_close_tab !== false,
            startupBehavior: cfg.startup_behavior || 'session',
            searchOrientation: cfg.search_orientation || 'horizontal', searchSide: cfg.search_side || 'bottom',
            sshConnectTimeoutSec: cfg.ssh_connect_timeout_sec || 10,
            remoteStallTimeoutSec: cfg.remote_stall_timeout_sec || 60,
            autoAcceptHostKey: !!cfg.auto_accept_host_key, connMaxAttempts: cfg.conn_max_attempts || 5,
            connAttemptWindowMin: cfg.conn_attempt_window_min || 15, connLockoutMin: cfg.conn_lockout_min || 5,
            defaultSshPort: cfg.default_ssh_port || 22, defaultFtpPort: cfg.default_ftp_port || 21,
        };
    }

    private applyPrefs(p: AppPrefs) {
        applyLogFontSize(p.logFontSize);
        applyLogFontFamily(p.logFontFamily);
        applyLineDensity(p.lineDensity);
        applyWordWrap(p.wordWrap);
        applyShowLineNumbers(p.showLineNumbers);
        const classes = document.body.className.split(/\s+/).filter((c) => c && !c.startsWith('theme-') && c !== 'word-wrap' && c !== 'hide-line-numbers');
        document.body.className = classes.join(' ');
        document.body.classList.add(`theme-${resolveTheme(p.theme)}`);
        if (p.wordWrap) document.body.classList.add('word-wrap');
        if (!p.showLineNumbers) document.body.classList.add('hide-line-numbers');
        setLang(resolveLang(p.language));
        applyI18n();
        this.onPrefsApplied?.(p);
    }

    private readForm(): AppPrefs {
        const g = (id: string) => document.getElementById(id);
        const sel = (id: string) => (g(id) as HTMLSelectElement | null)?.value ?? '';
        const num = (id: string, fallback: number) => {
            const v = parseInt((g(id) as HTMLInputElement | null)?.value || '', 10);
            return Number.isFinite(v) ? v : fallback;
        };
        const chk = (id: string) => !!(g(id) as HTMLInputElement | null)?.checked;
        return {
            theme: sel('settings-theme') || 'system', language: sel('settings-lang') || 'system',
            saveTabsExit: chk('settings-save-tabs'), viewMode: sel('settings-view-mode') || 'split',
            logFontSize: num('settings-font-size', 13), logFontFamily: sel('settings-font-family') || 'monospace',
            lineDensity: sel('settings-line-density') || 'normal', wordWrap: chk('settings-word-wrap'),
            showLineNumbers: chk('settings-show-line-numbers'), defaultFollow: chk('settings-default-follow'),
            defaultSearchCase: chk('settings-default-search-case'), defaultSearchRegex: chk('settings-default-search-regex'),
            defaultSearchInvert: chk('settings-default-search-invert'), maxRecentFiles: num('settings-max-recent', 20),
            maxHistorySize: num('settings-max-history', 100), confirmCloseTab: chk('settings-confirm-close-tab'),
            startupBehavior: sel('settings-startup-behavior') || 'session',
            searchOrientation: sel('settings-search-orientation') || 'horizontal',
            searchSide: sel('settings-search-side') || 'bottom',
            sshConnectTimeoutSec: num('settings-ssh-timeout', 10), remoteStallTimeoutSec: num('settings-stall-timeout', 60),
            autoAcceptHostKey: chk('settings-auto-accept-host-key'), connMaxAttempts: num('settings-conn-max-attempts', 5),
            connAttemptWindowMin: num('settings-conn-window', 15), connLockoutMin: num('settings-conn-lockout', 5),
            defaultSshPort: 22, defaultFtpPort: 21,
        };
    }

    private fillForm(p: AppPrefs) {
        const setSel = (id: string, v: string) => { const el = document.getElementById(id) as HTMLSelectElement | null; if (el) el.value = v; };
        const setNum = (id: string, v: number) => { const el = document.getElementById(id) as HTMLInputElement | null; if (el) el.value = String(v); };
        const setChk = (id: string, v: boolean) => { const el = document.getElementById(id) as HTMLInputElement | null; if (el) el.checked = v; };
        setSel('settings-theme', p.theme); setSel('settings-lang', p.language);
        setChk('settings-save-tabs', p.saveTabsExit); setSel('settings-view-mode', p.viewMode);
        setSel('settings-font-family', p.logFontFamily); setSel('settings-font-size', String(p.logFontSize));
        setSel('settings-line-density', p.lineDensity); setChk('settings-word-wrap', p.wordWrap);
        setChk('settings-show-line-numbers', p.showLineNumbers); setChk('settings-default-follow', p.defaultFollow);
        setChk('settings-default-search-case', p.defaultSearchCase); setChk('settings-default-search-regex', p.defaultSearchRegex);
        setChk('settings-default-search-invert', p.defaultSearchInvert); setNum('settings-max-recent', p.maxRecentFiles);
        setNum('settings-max-history', p.maxHistorySize); setChk('settings-confirm-close-tab', p.confirmCloseTab);
        setSel('settings-startup-behavior', p.startupBehavior); setSel('settings-search-orientation', p.searchOrientation);
        setSel('settings-search-side', p.searchSide); setNum('settings-ssh-timeout', p.sshConnectTimeoutSec);
        setNum('settings-stall-timeout', p.remoteStallTimeoutSec); setChk('settings-auto-accept-host-key', p.autoAcceptHostKey);
        setNum('settings-conn-max-attempts', p.connMaxAttempts); setNum('settings-conn-window', p.connAttemptWindowMin);
        setNum('settings-conn-lockout', p.connLockoutMin);
    }

    private toBackend(p: AppPrefs): Record<string, unknown> {
        return {
            theme: p.theme, language: p.language, save_tabs_exit: p.saveTabsExit, view_mode: p.viewMode,
            log_font_size: p.logFontSize, log_font_family: p.logFontFamily, line_density: p.lineDensity,
            word_wrap: p.wordWrap, show_line_numbers: p.showLineNumbers, default_follow: p.defaultFollow,
            default_search_case: p.defaultSearchCase, default_search_regex: p.defaultSearchRegex,
            default_search_invert: p.defaultSearchInvert, max_recent_files: p.maxRecentFiles,
            max_history_size: p.maxHistorySize, confirm_close_tab: p.confirmCloseTab,
            startup_behavior: p.startupBehavior, search_orientation: p.searchOrientation, search_side: p.searchSide,
            ssh_connect_timeout_sec: p.sshConnectTimeoutSec, remote_stall_timeout_sec: p.remoteStallTimeoutSec,
            auto_accept_host_key: p.autoAcceptHostKey, conn_max_attempts: p.connMaxAttempts,
            conn_attempt_window_min: p.connAttemptWindowMin, conn_lockout_min: p.connLockoutMin,
            default_ssh_port: p.defaultSshPort, default_ftp_port: p.defaultFtpPort,
        };
    }

    private scheduleAutosave = () => {
        if (this.autosaveTimer) clearTimeout(this.autosaveTimer);
        this.autosaveTimer = setTimeout(() => { void this.persistSettings(false); }, 400);
    };

    private persistSettings = async (showError: boolean) => {
        const prefs = this.readForm();
        currentPrefs = prefs;
        this.applyPrefs(prefs);
        const api = this.ctx.getApi();
        if (!api) return;
        try {
            if (api.SaveAppSettings) await api.SaveAppSettings(this.toBackend(prefs));
            else if (api.SaveSettings) {
                await api.SaveSettings(prefs.theme, prefs.language, prefs.saveTabsExit);
                if (api.SaveLogFontSize) await api.SaveLogFontSize(prefs.logFontSize);
            }
            if (api.SetMaxHistorySize) await api.SetMaxHistorySize(prefs.maxHistorySize);
            if (api.SaveSearchPaneLayout) await api.SaveSearchPaneLayout(prefs.searchOrientation, prefs.searchSide);
        } catch (err) {
            console.error('Failed to save settings:', err);
            if (showError) showToast(String((err as any)?.message ?? err), 'error');
        }
    };

    private initHandlers = () => {
        const { btnSettingsHeader, btnCancelSettings, btnSaveSettings } = this.ctx;

        // Settings sidebar tabs
        document.querySelectorAll<HTMLElement>('.settings-tab').forEach((tab) => {
            tab.addEventListener('click', () => {
                const id = tab.dataset.settingsTab;
                if (!id) return;
                document.querySelectorAll('.settings-tab').forEach((t) => t.classList.toggle('active', t === tab));
                document.querySelectorAll('.settings-panel').forEach((p) => {
                    p.classList.toggle('active', (p as HTMLElement).dataset.settingsPanel === id);
                });
            });
        });

        btnSettingsHeader.addEventListener('click', async () => {
            const api = this.ctx.getApi();
            if (api?.GetSettings) {
                try {
                    const cfg = await Promise.race([
                        api.GetSettings(),
                        new Promise<never>((_, reject) => setTimeout(() => reject(new Error('GetSettings timeout')), 3000)),
                    ]);
                    currentPrefs = this.mapFromBackend(cfg);
                    this.fillForm(currentPrefs);
                } catch (err) {
                    console.error('GetSettings failed:', err);
                    showToast('Failed to load settings', 'error');
                    this.fillForm(currentPrefs);
                }
            } else this.fillForm(currentPrefs);
            this.ctx.settingsModal.classList.remove('hidden');
        });
        btnCancelSettings.addEventListener('click', () => this.ctx.settingsModal.classList.add('hidden'));
        btnSaveSettings.addEventListener('click', async () => {
            await this.persistSettings(true);
            this.ctx.settingsModal.classList.add('hidden');
        });
        const schedule = () => this.scheduleAutosave();
        const ids = [
            'settings-theme', 'settings-lang', 'settings-save-tabs', 'settings-max-history',
            'settings-font-size', 'settings-font-family', 'settings-line-density',
            'settings-word-wrap', 'settings-show-line-numbers', 'settings-view-mode',
            'settings-default-follow', 'settings-default-search-case', 'settings-default-search-regex',
            'settings-default-search-invert', 'settings-max-recent', 'settings-confirm-close-tab',
            'settings-startup-behavior', 'settings-search-orientation', 'settings-search-side',
            'settings-ssh-timeout', 'settings-stall-timeout', 'settings-auto-accept-host-key',
            'settings-conn-max-attempts', 'settings-conn-window', 'settings-conn-lockout',
            
        ];
        for (const id of ids) {
            const el = document.getElementById(id);
            if (!el) continue;
            el.addEventListener('change', () => {
                if (id === 'settings-font-size') {
                    const fontSel = document.getElementById('settings-font-size') as HTMLSelectElement | null;
                    if (fontSel) applyLogFontSize(fontSel.value);
                }
                if (id === 'settings-font-family') {
                    const s = document.getElementById('settings-font-family') as HTMLSelectElement | null;
                    if (s) applyLogFontFamily(s.value);
                }
                if (id === 'settings-line-density') {
                    const s = document.getElementById('settings-line-density') as HTMLSelectElement | null;
                    if (s) applyLineDensity(s.value);
                }
                if (id === 'settings-word-wrap') {
                    const c = document.getElementById('settings-word-wrap') as HTMLInputElement | null;
                    if (c) applyWordWrap(c.checked);
                    // Immediately update scrollers (don't wait for autosave).
                    this.onPrefsApplied?.(this.readForm());
                }
                if (id === 'settings-show-line-numbers') {
                    const c = document.getElementById('settings-show-line-numbers') as HTMLInputElement | null;
                    if (c) applyShowLineNumbers(c.checked);
                }
                if (id === 'settings-font-size' || id === 'settings-font-family' || id === 'settings-line-density') {
                    this.onPrefsApplied?.(this.readForm());
                }
                schedule();
            });
        }
    };
}
