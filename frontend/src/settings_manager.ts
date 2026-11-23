import { resolveLang, resolveTheme, setLang, setOnLangChange } from './i18n';
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
    /** Default AND/OR for secondary search field */
    defaultSearchCombine: 'and' | 'or';
    ctxBefore: number;
    ctxAfter: number;
    nearbyMode: string;
    nearbyWindow: number;
    excludeRegexDefault: boolean;
    timeRequireStamp: boolean;
    topLimit: number;
    hlPrimary: boolean;
    hlSecondary: boolean;
    hlNearby: boolean;
    hlExclude: boolean;
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
    defaultSearchCase: false, defaultSearchRegex: false, defaultSearchInvert: false, defaultSearchCombine: 'or',
    ctxBefore: 0, ctxAfter: 0, nearbyMode: 'either', nearbyWindow: 10,
    excludeRegexDefault: false, timeRequireStamp: false, topLimit: 100,
    hlPrimary: true, hlSecondary: true, hlNearby: true, hlExclude: false,
    maxRecentFiles: 20, maxHistorySize: 100, confirmCloseTab: true,
    startupBehavior: 'session', searchOrientation: 'horizontal', searchSide: 'bottom',
    sshConnectTimeoutSec: 10, remoteStallTimeoutSec: 60, autoAcceptHostKey: false,
    connMaxAttempts: 20, connAttemptWindowMin: 5, connLockoutMin: 1,
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
        // Register callback to refresh status bar when language changes
        setOnLangChange(() => {
            this.ctx.updateUIState?.();
        });
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
        // Spread defaults so AppPrefs stays complete when backend omits new keys.
        return {
            ...defaultPrefs,
            theme: cfg.theme || 'system',
            language: cfg.language || 'system',
            saveTabsExit: !!cfg.save_tabs_exit,
            viewMode: cfg.view_mode || 'split',
            logFontSize: cfg.log_font_size || 13,
            logFontFamily: cfg.log_font_family || 'monospace',
            lineDensity: cfg.line_density || 'normal',
            wordWrap: !!cfg.word_wrap,
            showLineNumbers: cfg.show_line_numbers !== false,
            defaultFollow: cfg.default_follow !== false,
            defaultSearchCase: !!cfg.default_search_case,
            defaultSearchRegex: !!cfg.default_search_regex,
            defaultSearchInvert: !!cfg.default_search_invert,
            defaultSearchCombine: (cfg.default_search_combine === 'and' ? 'and' : 'or') as 'and' | 'or',
            ctxBefore: cfg.ctx_before ?? (() => { try { return JSON.parse(localStorage.getItem("gofle.appPrefs")||"{}").ctxBefore; } catch { return undefined; } })() ?? defaultPrefs.ctxBefore,
            ctxAfter: cfg.ctx_after ?? (() => { try { return JSON.parse(localStorage.getItem("gofle.appPrefs")||"{}").ctxAfter; } catch { return undefined; } })() ?? defaultPrefs.ctxAfter,
            nearbyMode: cfg.nearby_mode || defaultPrefs.nearbyMode,
            nearbyWindow: cfg.nearby_window ?? defaultPrefs.nearbyWindow,
            excludeRegexDefault: cfg.exclude_regex_default != null ? !!cfg.exclude_regex_default : defaultPrefs.excludeRegexDefault,
            timeRequireStamp: cfg.time_require_stamp != null ? !!cfg.time_require_stamp : defaultPrefs.timeRequireStamp,
            topLimit: cfg.top_limit ?? defaultPrefs.topLimit,
            hlPrimary: cfg.hl_primary !== false,
            hlSecondary: cfg.hl_secondary !== false,
            hlNearby: cfg.hl_nearby !== false,
            hlExclude: !!cfg.hl_exclude,
            maxRecentFiles: cfg.max_recent_files || 20,
            maxHistorySize: cfg.max_history_size || 100,
            confirmCloseTab: cfg.confirm_close_tab !== false,
            startupBehavior: cfg.startup_behavior || 'session',
            searchOrientation: cfg.search_orientation || 'horizontal',
            searchSide: cfg.search_side || 'bottom',
            sshConnectTimeoutSec: cfg.ssh_connect_timeout_sec || 10,
            remoteStallTimeoutSec: cfg.remote_stall_timeout_sec || 60,
            autoAcceptHostKey: !!cfg.auto_accept_host_key,
            connMaxAttempts: cfg.conn_max_attempts || 20,
            connAttemptWindowMin: cfg.conn_attempt_window_min || 5,
            connLockoutMin: cfg.conn_lockout_min || 1,
            defaultSshPort: cfg.default_ssh_port || 22,
            defaultFtpPort: cfg.default_ftp_port || 21,
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
            ctxBefore: num('settings-ctx-before', 0),
            ctxAfter: num('settings-ctx-after', 0),
            nearbyMode: sel('settings-nearby-mode') || 'either',
            nearbyWindow: num('settings-nearby-window', 10),
            excludeRegexDefault: chk('settings-exclude-regex'),
            timeRequireStamp: chk('settings-time-require'),
            topLimit: num('settings-top-limit', 100),
            hlPrimary: chk('settings-hl-primary'),
            hlSecondary: chk('settings-hl-secondary'),
            hlNearby: chk('settings-hl-nearby'),
            hlExclude: chk('settings-hl-exclude'),
            defaultSearchInvert: chk('settings-default-search-invert'),
            defaultSearchCombine: (sel('settings-default-search-combine') === 'and' ? 'and' : 'or') as 'and' | 'or', maxRecentFiles: num('settings-max-recent', 20),
            maxHistorySize: num('settings-max-history', 100), confirmCloseTab: chk('settings-confirm-close-tab'),
            startupBehavior: sel('settings-startup-behavior') || 'session',
            searchOrientation: sel('settings-search-orientation') || 'horizontal',
            searchSide: sel('settings-search-side') || 'bottom',
            sshConnectTimeoutSec: num('settings-ssh-timeout', 10), remoteStallTimeoutSec: num('settings-stall-timeout', 60),
            autoAcceptHostKey: chk('settings-auto-accept-host-key'), connMaxAttempts: num('settings-conn-max-attempts', 20),
            connAttemptWindowMin: num('settings-conn-window', 5), connLockoutMin: num('settings-conn-lockout', 1),
            defaultSshPort: 22, defaultFtpPort: 21,
        };
    }

    private fillForm(p: AppPrefs) {
        const setSel = (id: string, v: string) => { const el = document.getElementById(id) as HTMLSelectElement | null; if (el) el.value = v; };
        const setNum = (id: string, v: number) => { const el = document.getElementById(id) as HTMLInputElement | null; if (el) el.value = String(v); };
        const setChk = (id: string, v: boolean) => { const el = document.getElementById(id) as HTMLInputElement | null; if (el) el.checked = v; };
        setSel('settings-theme', p.theme); setSel('settings-lang', p.language);
        setNum('settings-ctx-before', p.ctxBefore ?? 0);
        setNum('settings-ctx-after', p.ctxAfter ?? 0);
        setSel('settings-nearby-mode', p.nearbyMode || 'either');
        setNum('settings-nearby-window', p.nearbyWindow ?? 10);
        setChk('settings-exclude-regex', !!p.excludeRegexDefault);
        setChk('settings-time-require', !!p.timeRequireStamp);
        setNum('settings-top-limit', p.topLimit ?? 100);
        setChk('settings-hl-primary', p.hlPrimary !== false);
        setChk('settings-hl-secondary', p.hlSecondary !== false);
        setChk('settings-hl-nearby', p.hlNearby !== false);
        setChk('settings-hl-exclude', !!p.hlExclude);
        setChk('settings-save-tabs', p.saveTabsExit); setSel('settings-view-mode', p.viewMode);
        setSel('settings-font-family', p.logFontFamily); setSel('settings-font-size', String(p.logFontSize));
        setSel('settings-line-density', p.lineDensity); setChk('settings-word-wrap', p.wordWrap);
        setChk('settings-show-line-numbers', p.showLineNumbers); setChk('settings-default-follow', p.defaultFollow);
        setChk('settings-default-search-case', p.defaultSearchCase); setChk('settings-default-search-regex', p.defaultSearchRegex);
        setChk('settings-default-search-invert', p.defaultSearchInvert);
        setSel('settings-default-search-combine', p.defaultSearchCombine || 'or');
        setNum('settings-max-recent', p.maxRecentFiles);
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
            default_search_invert: p.defaultSearchInvert,
            default_search_combine: p.defaultSearchCombine || 'or',
            ctx_before: p.ctxBefore, ctx_after: p.ctxAfter,
            nearby_mode: p.nearbyMode, nearby_window: p.nearbyWindow,
            exclude_regex_default: p.excludeRegexDefault,
            time_require_stamp: p.timeRequireStamp, top_limit: p.topLimit,
            hl_primary: p.hlPrimary, hl_secondary: p.hlSecondary,
            hl_nearby: p.hlNearby, hl_exclude: p.hlExclude, max_recent_files: p.maxRecentFiles,
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
        try { this.ctx.syncSearchCombineUI?.(); } catch { /* ignore */ }
        try {
            localStorage.setItem('gofle.appPrefs', JSON.stringify({
                ctxBefore: prefs.ctxBefore, ctxAfter: prefs.ctxAfter,
                nearbyMode: prefs.nearbyMode, nearbyWindow: prefs.nearbyWindow,
                excludeRegexDefault: prefs.excludeRegexDefault,
                timeRequireStamp: prefs.timeRequireStamp, topLimit: prefs.topLimit,
            }));
        } catch { /* ignore quota */ }
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
            // Context ±N and search-related defaults take effect on next search.
            try { this.ctx.triggerSearch?.(); } catch { /* ignore */ }
        });
        const schedule = () => this.scheduleAutosave();
        const ids = [
            'settings-theme', 'settings-lang', 'settings-save-tabs', 'settings-max-history',
            'settings-ctx-before', 'settings-ctx-after', 'settings-nearby-mode', 'settings-nearby-window',
            'settings-exclude-regex', 'settings-time-require', 'settings-top-limit',
            'settings-hl-primary', 'settings-hl-secondary', 'settings-hl-nearby', 'settings-hl-exclude',
            'settings-font-size', 'settings-font-family', 'settings-line-density',
            'settings-word-wrap', 'settings-show-line-numbers', 'settings-view-mode',
            'settings-default-follow', 'settings-default-search-case', 'settings-default-search-regex',
            'settings-default-search-invert', 'settings-default-search-combine', 'settings-max-recent', 'settings-confirm-close-tab',
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
