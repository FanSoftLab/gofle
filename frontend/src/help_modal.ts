
import { getPrefs } from './settings_manager';
import { HELP_SECTIONS, helpLangFromApp, type HelpLang, type HelpSection } from './help_content';
import { t } from './i18n';

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

export class HelpModal {
  private modal: HTMLElement;
  private tabsEl: HTMLElement;
  private panelsEl: HTMLElement;
  private searchInput: HTMLInputElement;
  private activeId = 'overview';
  private query = '';

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
  }

  open() {
    this.modal.classList.remove('hidden');
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
