
import { getPrefs } from './settings_manager';
import {
  HELP_SECTIONS,
  LICENSE_ENTRIES,
  helpLangFromApp,
  type HelpLang,
  type HelpSection,
} from './help_content';
import { t } from './i18n';
import logoUrl from './assets/logo.svg';

function escapeHtml(s: string): string {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function highlightQuery(text: string, q: string): string {
  const esc = escapeHtml(text);
  if (!q) return esc;
  try {
    const re = new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
    return esc.replace(re, (m) => `<mark class="help-mark">${m}</mark>`);
  } catch {
    return esc;
  }
}

async function resolveAppVersion(): Promise<string> {
  try {
    const go = (window as unknown as { go?: { main?: { App?: { GetVersion?: () => Promise<string> } } } }).go;
    const fn = go?.main?.App?.GetVersion;
    if (typeof fn === 'function') {
      const v = await fn();
      if (v && String(v).trim()) return String(v).trim().split('\n')[0];
    }
  } catch {
    /* offline / no binding */
  }
  return '2.0.0';
}

export class HelpModal {
  private modal: HTMLElement;
  private tabsEl: HTMLElement;
  private panelsEl: HTMLElement;
  private searchInput: HTMLInputElement;
  private activeId = 'overview';
  private query = '';
  private cachedVersion = '2.0.0';

  constructor() {
    this.modal = document.getElementById('help-modal')!;
    this.tabsEl = document.getElementById('help-tabs')!;
    this.panelsEl = document.getElementById('help-panels')!;
    this.searchInput = document.getElementById('help-search-input') as HTMLInputElement;

    document.getElementById('btn-help')?.addEventListener('click', () => this.open());
    document.getElementById('btn-close-help')?.addEventListener('click', () => this.close());
    this.modal.addEventListener('click', (e) => {
      if (e.target === this.modal) this.close();
    });
    this.searchInput?.addEventListener('input', () => {
      this.query = (this.searchInput.value || '').trim();
      this.render();
    });
    window.addEventListener('keydown', (e) => {
      if (e.key === 'F1') {
        e.preventDefault();
        this.open();
      }
      if (e.key === 'Escape' && !this.modal.classList.contains('hidden')) {
        this.close();
      }
    });

    void resolveAppVersion().then((v) => {
      this.cachedVersion = v;
    });
  }

  open() {
    this.modal.classList.remove('hidden');
    void resolveAppVersion().then((v) => {
      this.cachedVersion = v;
      this.render();
    });
    this.render();
    setTimeout(() => this.searchInput?.focus(), 50);
  }

  close() {
    this.modal.classList.add('hidden');
  }

  private lang(): HelpLang {
    return helpLangFromApp(getPrefs().language === 'system'
      ? (navigator.language || 'uk')
      : getPrefs().language);
  }

  private sectionMatches(sec: HelpSection, q: string, lang: HelpLang): boolean {
    if (!q) return true;
    const ql = q.toLowerCase();
    if (sec.title[lang].toLowerCase().includes(ql)) return true;
    if (sec.id === 'licenses') {
      for (const e of LICENSE_ENTRIES) {
        if (
          e.name.toLowerCase().includes(ql) ||
          e.license.toLowerCase().includes(ql) ||
          e.url.toLowerCase().includes(ql)
        ) return true;
      }
    }
    for (const item of sec.items) {
      if (item.title[lang].toLowerCase().includes(ql)) return true;
      if (item.body[lang].some((l) => l.toLowerCase().includes(ql))) return true;
      if (item.example?.[lang]?.some((l) => l.toLowerCase().includes(ql))) return true;
    }
    return false;
  }

  private itemMatches(item: HelpSection['items'][0], q: string, lang: HelpLang): boolean {
    if (!q) return true;
    const ql = q.toLowerCase();
    if (item.title[lang].toLowerCase().includes(ql)) return true;
    if (item.body[lang].some((l) => l.toLowerCase().includes(ql))) return true;
    if (item.example?.[lang]?.some((l) => l.toLowerCase().includes(ql))) return true;
    return false;
  }

  private renderAbout(lang: HelpLang, q: string): string {
    const name = 'Go Fast Log Explorer';
    const short = lang === 'uk'
      ? 'Швидкий інтерактивний переглядач дуже великих лог-файлів. Локальні шляхи, SSH і FTP/FTPS, кілька вкладок, live-follow, пошук і фільтри.'
      : 'Fast interactive viewer for very large log files. Local paths, SSH and FTP/FTPS, multiple tabs, live follow, search and filters.';
    const licLabel = lang === 'uk' ? 'Ліцензія' : 'License';
    const verLabel = lang === 'uk' ? 'Версія' : 'Version';
    return `
      <div class="help-panel active help-about">
        <div class="help-about-header">
          <img class="help-about-logo" src="${escapeHtml(logoUrl)}" alt="GoFLE" width="72" height="72" />
          <div class="help-about-meta">
            <h4 class="help-section-title help-about-name">${highlightQuery(name, q)}</h4>
            <p class="help-about-line"><span class="help-about-label">${escapeHtml(verLabel)}:</span> <span class="help-about-value">${highlightQuery(this.cachedVersion, q)}</span></p>
            <p class="help-about-line"><span class="help-about-label">${escapeHtml(licLabel)}:</span> <span class="help-about-value">${highlightQuery('MIT', q)}</span></p>
          </div>
        </div>
        <p class="help-about-desc">${highlightQuery(short, q)}</p>
      </div>`;
  }

  private renderLicenses(lang: HelpLang, q: string): string {
    const ql = q.toLowerCase();
    const rows = LICENSE_ENTRIES.filter((e) => {
      if (!q) return true;
      return (
        e.name.toLowerCase().includes(ql) ||
        e.license.toLowerCase().includes(ql) ||
        e.url.toLowerCase().includes(ql)
      );
    });
    const note = lang === 'uk'
      ? 'Повний опис — у файлі LICENSES.md у корені проєкту.'
      : 'Full descriptions are in LICENSES.md at the project root.';
    const colName = lang === 'uk' ? 'Назва' : 'Name';
    const colLink = lang === 'uk' ? 'Посилання' : 'Link';
    const colLic = lang === 'uk' ? 'Ліцензія' : 'License';
    return `
      <div class="help-panel active">
        <h4 class="help-section-title">${highlightQuery(lang === 'uk' ? 'Ліцензії' : 'Licenses', q)}</h4>
        <p class="help-licenses-note">${highlightQuery(note, q)}</p>
        <table class="help-licenses-table">
          <thead>
            <tr>
              <th>${escapeHtml(colName)}</th>
              <th>${escapeHtml(colLink)}</th>
              <th>${escapeHtml(colLic)}</th>
            </tr>
          </thead>
          <tbody>
            ${rows.map((e) => `
              <tr>
                <td class="help-lic-name">${highlightQuery(e.name, q)}</td>
                <td class="help-lic-url"><a href="${escapeHtml(e.url)}" target="_blank" rel="noopener noreferrer">${highlightQuery(e.url, q)}</a></td>
                <td class="help-lic-spdx">${highlightQuery(e.license, q)}</td>
              </tr>`).join('')}
          </tbody>
        </table>
      </div>`;
  }

  private render() {
    const lang = this.lang();
    const q = this.query;
    const visible = HELP_SECTIONS.filter((s) => this.sectionMatches(s, q, lang));
    if (visible.length && !visible.some((s) => s.id === this.activeId)) {
      this.activeId = visible[0].id;
    }

    this.tabsEl.innerHTML = visible.map((s) =>
      `<button type="button" class="settings-tab${s.id === this.activeId ? ' active' : ''}" data-help-tab="${s.id}">${escapeHtml(s.title[lang])}</button>`
    ).join('') || `<span class="help-empty">${escapeHtml(t('help_no_results') || 'Nothing found')}</span>`;

    this.tabsEl.querySelectorAll('[data-help-tab]').forEach((btn) => {
      btn.addEventListener('click', () => {
        this.activeId = (btn as HTMLElement).dataset.helpTab || 'overview';
        this.render();
      });
    });

    const sec = visible.find((s) => s.id === this.activeId) || visible[0];
    if (!sec) {
      this.panelsEl.innerHTML = `<div class="help-empty">${escapeHtml(t('help_no_results') || 'Nothing found')}</div>`;
      return;
    }

    if (sec.id === 'about') {
      this.panelsEl.innerHTML = this.renderAbout(lang, q);
      return;
    }
    if (sec.id === 'licenses') {
      this.panelsEl.innerHTML = this.renderLicenses(lang, q);
      return;
    }

    const items = sec.items.filter((it) => this.itemMatches(it, q, lang));
    this.panelsEl.innerHTML = `
      <div class="help-panel active">
        <h4 class="help-section-title">${highlightQuery(sec.title[lang], q)}</h4>
        ${items.map((it) => `
          <article class="help-item">
            <h5 class="help-item-title">${highlightQuery(it.title[lang], q)}</h5>
            <ul class="help-body">
              ${it.body[lang].map((line) => `<li>${highlightQuery(line, q)}</li>`).join('')}
            </ul>
            ${it.example?.[lang]?.length ? `
              <div class="help-example">
                <div class="help-example-label">${escapeHtml(t('help_example') || 'Example')}</div>
                <pre>${it.example[lang].map((l) => highlightQuery(l, q)).join('\n')}</pre>
              </div>` : ''}
          </article>`).join('') || `<div class="help-empty">${escapeHtml(t('help_no_results') || 'Nothing found')}</div>`}
      </div>`;
  }
}
