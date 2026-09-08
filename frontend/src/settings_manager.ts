import { resolveLang, resolveTheme, setLang, applyI18n } from './i18n';
import { showToast } from './toast';
import type { AppContext } from './app_context';

export class SettingsManager {
    private ctx: AppContext;
    private autosaveTimer: ReturnType<typeof setTimeout> | null = null;

    constructor(ctx: AppContext) {
        this.ctx = ctx;
        this.initHandlers();
    }

    private initHandlers = () => {
        const { btnSettingsHeader, btnCancelSettings, btnSaveSettings } = this.ctx;

        btnSettingsHeader.addEventListener('click', async () => {
            const api = this.ctx.getApi();
            if (api) {
                try {
                    const cfg = await Promise.race([
                        api.GetSettings(),
                        new Promise<never>((_, reject) => setTimeout(() => reject(new Error('GetSettings timeout')), 3000))
                    ]);
                    if (cfg) {
                        this.ctx.settingsThemeSelect.value = cfg.theme || 'system';
                        this.ctx.settingsLangSelect.value = cfg.language || 'system';
                        this.ctx.settingsSaveTabsCheck.checked = !!cfg.save_tabs_exit;
                        this.ctx.settingsMaxHistoryInput.value = String(cfg.max_history_size || 100);
                        const fontSel = document.getElementById('settings-font-size') as HTMLSelectElement | null;
                        if (fontSel) {
                            const px = String(cfg.log_font_size || localStorage.getItem('gofle.logFontSize') || '13');
                            fontSel.value = px;
                        }
                    }
                } catch (err) {
                    console.error('GetSettings failed:', err);
                    showToast('Failed to load settings', 'error');
                }
            }
            this.ctx.settingsModal.classList.remove('hidden');
        });

        btnCancelSettings.addEventListener('click', () => {
            this.ctx.settingsModal.classList.add('hidden');
        });

        // Explicit Save still works; also closes the modal.
        btnSaveSettings.addEventListener('click', async () => {
            await this.persistSettings(true);
            this.ctx.settingsModal.classList.add('hidden');
        });

        // Auto-save on any change — no need to click Save for theme/lang/font/history.
        const schedule = () => this.scheduleAutosave();
        this.ctx.settingsThemeSelect.addEventListener('change', schedule);
        this.ctx.settingsLangSelect.addEventListener('change', schedule);
        this.ctx.settingsSaveTabsCheck.addEventListener('change', schedule);
        this.ctx.settingsMaxHistoryInput.addEventListener('change', schedule);
        document.getElementById('settings-font-size')?.addEventListener('change', () => {
            const fontSel = document.getElementById('settings-font-size') as HTMLSelectElement | null;
            if (fontSel) applyLogFontSize(fontSel.value);
            schedule();
        });
    };

    private scheduleAutosave = () => {
        if (this.autosaveTimer) clearTimeout(this.autosaveTimer);
        this.autosaveTimer = setTimeout(() => {
            void this.persistSettings(false);
        }, 350);
    };

    /** Write current settings form into the shared backend config. */
    private persistSettings = async (showError: boolean) => {
        const theme = this.ctx.settingsThemeSelect.value;
        const lang = this.ctx.settingsLangSelect.value;
        const saveTabs = this.ctx.settingsSaveTabsCheck.checked;
        let maxHistory = parseInt(this.ctx.settingsMaxHistoryInput.value, 10);
        if (!Number.isFinite(maxHistory) || maxHistory < 1) {
            maxHistory = 1;
        }

        const fontSel = document.getElementById('settings-font-size') as HTMLSelectElement | null;
        const fontSize = fontSel?.value || '13';
        applyLogFontSize(fontSize);
        localStorage.setItem('gofle.logFontSize', fontSize);

        const api = this.ctx.getApi();
        if (!api) return;

        try {
            await api.SaveSettings(theme, lang, saveTabs);
            if (api.SetMaxHistorySize) {
                await api.SetMaxHistorySize(maxHistory);
            }
            if (api.SaveLogFontSize) {
                await api.SaveLogFontSize(parseInt(fontSize, 10) || 13);
            }
            if (!saveTabs && api.ClearSavedSession) {
                await api.ClearSavedSession();
            } else if (saveTabs) {
                this.ctx.saveSession();
            }
            setLang(resolveLang(lang));
            document.body.className = `theme-${resolveTheme(theme)}`;
            applyI18n();
            this.ctx.updateUIState();
        } catch (err) {
            console.error('Failed to save settings:', err);
            if (showError) {
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }
    };
}

/** Apply log font size + derived line-height (CSS vars consumed by VirtualScroller). */
export function applyLogFontSize(px: string) {
    const size = Math.max(11, Math.min(22, parseInt(px, 10) || 13));
    const line = Math.round(size * 1.55);
    document.documentElement.style.setProperty('--log-font-size', `${size}px`);
    document.documentElement.style.setProperty('--line-height', `${line}px`);
    // Nudge scrollers to remeasure row height after font change.
    window.dispatchEvent(new Event('resize'));
}
