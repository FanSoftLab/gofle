export type LangCode = 'uk' | 'en';

let translationsCache: Record<LangCode, Record<string, string>> | null = null;

async function loadTranslations(): Promise<Record<LangCode, Record<string, string>>> {
    if (translationsCache) return translationsCache;

    const [ukModule, enModule] = await Promise.all([
        import('./i18n/uk.json'),
        import('./i18n/en.json'),
    ]);

    translationsCache = {
        uk: ukModule.default,
        en: enModule.default,
    };

    return translationsCache;
}

let currentLang: LangCode = 'uk';

export async function initI18n(): Promise<void> {
    await loadTranslations();
    applyI18n();
}

export function setLang(lang: LangCode) {
    currentLang = lang;
    applyI18n();
}

export function getLang(): LangCode {
    return currentLang;
}

export function resolveLang(raw: string | undefined): LangCode {
    if (raw === 'uk' || raw === 'en') return raw;
    const nav = (navigator.language || 'en').toLowerCase();
    return nav.startsWith('uk') ? 'uk' : 'en';
}

export function resolveTheme(raw: string | undefined): 'dark' | 'light' {
    if (raw === 'light') return 'light';
    if (raw === 'dark') return 'dark';
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function t(key: string, params?: Record<string, string | number>): string {
    if (!translationsCache) return key;
    let str = translationsCache[currentLang]?.[key] || translationsCache['uk']?.[key] || key;
    if (params) {
        Object.keys(params).forEach(p => {
            str = str.replace(`{${p}}`, String(params[p]));
        });
    }
    return str;
}

export function applyI18n() {
    if (!translationsCache) return;

    document.querySelectorAll('[data-i18n]:not(#status-file-text):not(#status-size-text):not(#status-lines-text):not(#match-counter-text):not(#status-tail-text)').forEach(el => {
        const key = el.getAttribute('data-i18n');
        if (key) el.textContent = t(key);
    });
    document.querySelectorAll('[data-i18n-title]').forEach(el => {
        const key = el.getAttribute('data-i18n-title');
        if (key) {
            const text = t(key);
            const htmlEl = el as HTMLElement;
            htmlEl.title = text;
            // Icon-only buttons need an accessible name for screen readers.
            if (htmlEl.tagName === 'BUTTON' && !htmlEl.getAttribute('aria-label')) {
                htmlEl.setAttribute('aria-label', text);
            }
        }
    });
    document.querySelectorAll('button[data-i18n]:not([data-i18n-title])').forEach(el => {
        const key = el.getAttribute('data-i18n');
        if (key) {
            const text = t(key);
            (el as HTMLElement).title = text;
            if (!(el as HTMLElement).getAttribute('aria-label')) {
                (el as HTMLElement).setAttribute('aria-label', text);
            }
        }
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
        const key = el.getAttribute('data-i18n-placeholder');
        if (key) (el as HTMLInputElement).placeholder = t(key);
    });
}