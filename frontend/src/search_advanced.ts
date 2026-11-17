import { t } from './i18n';
import { getPrefs } from './settings_manager';

export type AdvFilterState = {
  nearbyQuery: string;
  nearbyMode: 'before' | 'after' | 'either';
  nearbyWindow: number;
  levels: string[];
  exclude: string[];
  excludeRegex: boolean;
  timeFrom: string;
  timeTo: string;
  timeRequire: boolean;
  rangeMode: 'all' | 'from_cursor' | 'last_n';
  lastN: number;
  topMode: boolean;
};

const defaultState = (): AdvFilterState => {
  const p = getPrefs() as any;
  return {
    nearbyQuery: '',
    nearbyMode: (p.nearbyMode as any) || 'either',
    nearbyWindow: p.nearbyWindow ?? 10,
    levels: [],
    exclude: [],
    excludeRegex: !!p.excludeRegexDefault,
    timeFrom: '',
    timeTo: '',
    timeRequire: !!p.timeRequireStamp,
    rangeMode: 'all',
    lastN: 5000,
    topMode: false,
  };
};

export function splitStoredTime(v: string): { date: string; hh: string; mm: string } {
  if (!v) return { date: '', hh: '', mm: '' };
  const m = v.trim().match(/^(\d{4}-\d{2}-\d{2})(?:[ T](\d{2}):(\d{2})(?::\d{2})?)?/);
  if (!m) return { date: '', hh: '', mm: '' };
  // Validate the date is actually a real calendar date (check month/day bounds)
  const dateStr = m[1];
  const year = parseInt(dateStr.slice(0, 4), 10);
  const month = parseInt(dateStr.slice(5, 7), 10);
  const day = parseInt(dateStr.slice(8, 10), 10);
  if (month < 1 || month > 12 || day < 1 || day > 31) {
    return { date: '', hh: '', mm: '' };
  }
  // Check days in month
  const daysInMonth = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month === 2) {
    const isLeap = (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
    if (day > (isLeap ? 29 : 28)) return { date: '', hh: '', mm: '' };
  } else if (day > daysInMonth[month - 1]) {
    return { date: '', hh: '', mm: '' };
  }
  return { date: m[1], hh: m[2] || '', mm: m[3] || '' };
}

/** Date required; time optional. empty time → from 00:00:00 / to 23:59:59 */
export function joinBackendTime(date: string, hh: string, mm: string, endOfDay: boolean): string {
  date = (date || '').trim();
  if (!date) return '';
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return '';
  // Validate the date is actually a real calendar date (check month/day bounds)
  const year = parseInt(date.slice(0, 4), 10);
  const month = parseInt(date.slice(5, 7), 10);
  const day = parseInt(date.slice(8, 10), 10);
  if (month < 1 || month > 12 || day < 1 || day > 31) return '';
  const daysInMonth = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month === 2) {
    const isLeap = (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
    if (day > (isLeap ? 29 : 28)) return '';
  } else if (day > daysInMonth[month - 1]) {
    return '';
  }
  hh = (hh || '').trim();
  mm = (mm || '').trim();
  if (!hh && !mm) {
    return endOfDay ? `${date} 23:59:59` : `${date} 00:00:00`;
  }
  const h = Math.max(0, Math.min(23, parseInt(hh || '0', 10) || 0));
  const mi = Math.max(0, Math.min(59, parseInt(mm || '0', 10) || 0));
  return `${date} ${String(h).padStart(2, '0')}:${String(mi).padStart(2, '0')}:00`;
}

export class AdvancedSearchBar {
  state: AdvFilterState = defaultState();
  private panel: HTMLElement;
  private activePanel: string | null = null;
  private onChange: () => void;
  private getCursorLine: () => number;

  constructor(onChange: () => void, getCursorLine: () => number) {
    this.onChange = onChange;
    this.getCursorLine = getCursorLine;

    // Prefer docked panel; create if missing
    let panel = document.getElementById('adv-dock-panel');
    if (!panel) {
      const bar = document.getElementById('advanced-search-bar');
      panel = document.createElement('div');
      panel.id = 'adv-dock-panel';
      panel.className = 'adv-dock-panel hidden';
      bar?.appendChild(panel);
    }
    this.panel = panel;

    // Hide legacy popover if present
    document.getElementById('adv-popover')?.classList.add('hidden');

    this.bindChips();
    this.syncChipActive();
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && this.activePanel) this.closePanel();
    });
  }

  private bindChips() {
    const map: Record<string, () => void> = {
      'btn-adv-nearby': () => this.togglePanel('nearby', () => this.renderNearby()),
      'btn-adv-levels': () => this.togglePanel('levels', () => this.renderLevels()),
      'btn-adv-time': () => this.togglePanel('time', () => this.renderTime()),
      'btn-adv-exclude': () => this.togglePanel('exclude', () => this.renderExclude()),
      'btn-adv-range': () => this.togglePanel('range', () => this.renderRange()),
      'btn-adv-top': () => this.toggleTop(),
      'btn-adv-presets': () => this.togglePanel('presets', () => this.renderPresets()),
    };
    Object.entries(map).forEach(([id, fn]) => {
      const el = document.getElementById(id);
      if (!el) return;
      el.classList.add('adv-chip', 'search-opt-btn');
      el.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        fn();
      });
    });
  }

  private togglePanel(id: string, render: () => void) {
    if (this.activePanel === id) {
      this.closePanel();
      return;
    }
    this.activePanel = id;
    render();
    this.panel.classList.remove('hidden');
    this.syncChipActive();
  }

  closePanel() {
    this.activePanel = null;
    this.panel.classList.add('hidden');
    this.panel.innerHTML = '';
    this.syncChipActive();
  }

  private syncChipActive() {
    const s = this.state;
    const set = (id: string, on: boolean, open: boolean) => {
      const el = document.getElementById(id);
      if (!el) return;
      el.classList.toggle('active', on);
      el.classList.toggle('adv-open', open);
    };
    set('btn-adv-nearby', !!s.nearbyQuery, this.activePanel === 'nearby');
    set('btn-adv-levels', s.levels.length > 0, this.activePanel === 'levels');
    set('btn-adv-time', !!(s.timeFrom || s.timeTo), this.activePanel === 'time');
    set('btn-adv-exclude', s.exclude.length > 0, this.activePanel === 'exclude');
    set('btn-adv-range', s.rangeMode !== 'all', this.activePanel === 'range');
    set('btn-adv-top', s.topMode, false);
    set('btn-adv-presets', false, this.activePanel === 'presets');

    // Summary badge text on chips when active
    this.updateChipLabels();
  }

  private updateChipLabels() {
    const s = this.state;
    const lab = (id: string, baseKey: string, extra: string) => {
      const el = document.getElementById(id);
      if (!el) return;
      const base = t(baseKey);
      el.textContent = extra ? `${base}: ${extra}` : base;
    };
    lab('btn-adv-nearby', 'adv_nearby', s.nearbyQuery ? s.nearbyQuery.slice(0, 12) : '');
    lab('btn-adv-levels', 'adv_levels', s.levels.length ? s.levels.join(',') : '');
    lab('btn-adv-time', 'adv_time', s.timeFrom || s.timeTo ? '✓' : '');
    lab('btn-adv-exclude', 'adv_exclude', s.exclude.length ? String(s.exclude.length) : '');
    lab('btn-adv-range', 'adv_range', s.rangeMode === 'all' ? '' : (s.rangeMode === 'last_n' ? `N=${s.lastN}` : 'cursor'));
    lab('btn-adv-top', 'adv_top', s.topMode ? 'ON' : '');
  }

  private applyAndClose(keepOpen = false) {
    this.syncChipActive();
    this.onChange();
    if (!keepOpen) this.closePanel();
  }

  private actionsHtml(): string {
    return `<div class="adv-actions">
      <button type="button" class="search-opt-btn" data-adv-act="clear">${t('adv_clear')}</button>
      <button type="button" class="search-opt-btn active" data-adv-act="apply">${t('adv_apply')}</button>
    </div>`;
  }

  private renderNearby() {
    this.panel.innerHTML = `
      <div class="adv-form">
        <div class="adv-field">
          <span class="adv-label">${t('adv_nearby_pattern')}</span>
          <input type="text" id="adv-nearby-q" class="adv-input" value="${escapeAttr(this.state.nearbyQuery)}" placeholder="…" autofocus />
        </div>
        <div class="adv-field-row">
          <div class="adv-field">
            <span class="adv-label">${t('adv_nearby_mode')}</span>
            <select id="adv-nearby-mode" class="adv-input">
              <option value="either" ${this.state.nearbyMode === 'either' ? 'selected' : ''}>${t('adv_mode_either')}</option>
              <option value="before" ${this.state.nearbyMode === 'before' ? 'selected' : ''}>${t('adv_mode_before')}</option>
              <option value="after" ${this.state.nearbyMode === 'after' ? 'selected' : ''}>${t('adv_mode_after')}</option>
            </select>
          </div>
          <div class="adv-field">
            <span class="adv-label">${t('adv_nearby_window')}</span>
            <input type="number" id="adv-nearby-win" class="adv-input" min="1" max="200" value="${this.state.nearbyWindow}" />
          </div>
        </div>
        ${this.actionsHtml()}
      </div>`;
    this.bindFormActions(
      () => {
        this.state.nearbyQuery = '';
      },
      () => {
        this.state.nearbyQuery = (document.getElementById('adv-nearby-q') as HTMLInputElement).value.trim();
        this.state.nearbyMode = (document.getElementById('adv-nearby-mode') as HTMLSelectElement).value as any;
        this.state.nearbyWindow = parseInt((document.getElementById('adv-nearby-win') as HTMLInputElement).value, 10) || 10;
      },
    );
  }

  private renderLevels() {
    const all = ['ERROR', 'WARN', 'INFO', 'DEBUG', 'OTHER'];
    const checks = all.map((l) => {
      const on = this.state.levels.includes(l);
      return `<label class="adv-check ${on ? 'on' : ''}"><input type="checkbox" data-level="${l}" ${on ? 'checked' : ''}/> <span>${l}</span></label>`;
    }).join('');
    this.panel.innerHTML = `
      <div class="adv-form">
        <div class="adv-hint">${t('adv_levels_hint')}</div>
        <div class="level-checks">${checks}</div>
        ${this.actionsHtml()}
      </div>`;
    this.panel.querySelectorAll('.adv-check input').forEach((inp) => {
      inp.addEventListener('change', () => {
        (inp.closest('.adv-check') as HTMLElement)?.classList.toggle('on', (inp as HTMLInputElement).checked);
      });
    });
    this.bindFormActions(
      () => { this.state.levels = []; },
      () => {
        this.state.levels = all.filter((l) =>
          (this.panel.querySelector(`input[data-level="${l}"]`) as HTMLInputElement)?.checked);
      },
    );
  }

  private renderTime() {
    const from = splitStoredTime(this.state.timeFrom);
    const to = splitStoredTime(this.state.timeTo);
    this.panel.innerHTML = `
      <div class="adv-form">
        <div class="adv-hint">${t('adv_time_keyboard_hint')}</div>
        <div class="adv-field-row">
          <div class="adv-field">
            <span class="adv-label">${t('adv_time_from')}</span>
            <div class="adv-dt-grid">
              <input type="text" id="adv-from-d" class="adv-input adv-date" inputmode="numeric" placeholder="YYYY-MM-DD" maxlength="10" value="${escapeAttr(from.date)}" autocomplete="off" />
              <input type="text" id="adv-from-hh" class="adv-input adv-hh" inputmode="numeric" placeholder="HH" maxlength="2" value="${escapeAttr(from.hh)}" autocomplete="off" />
              <span class="adv-dt-sep">:</span>
              <input type="text" id="adv-from-mm" class="adv-input adv-mm" inputmode="numeric" placeholder="MM" maxlength="2" value="${escapeAttr(from.mm)}" autocomplete="off" />
            </div>
          </div>
          <div class="adv-field">
            <span class="adv-label">${t('adv_time_to')}</span>
            <div class="adv-dt-grid">
              <input type="text" id="adv-to-d" class="adv-input adv-date" inputmode="numeric" placeholder="YYYY-MM-DD" maxlength="10" value="${escapeAttr(to.date)}" autocomplete="off" />
              <input type="text" id="adv-to-hh" class="adv-input adv-hh" inputmode="numeric" placeholder="HH" maxlength="2" value="${escapeAttr(to.hh)}" autocomplete="off" />
              <span class="adv-dt-sep">:</span>
              <input type="text" id="adv-to-mm" class="adv-input adv-mm" inputmode="numeric" placeholder="MM" maxlength="2" value="${escapeAttr(to.mm)}" autocomplete="off" />
            </div>
          </div>
        </div>
        <div class="adv-hint">${t('adv_time_optional_hint')}</div>
        <label class="adv-check ${this.state.timeRequire ? 'on' : ''}">
          <input type="checkbox" id="adv-time-req" ${this.state.timeRequire ? 'checked' : ''}/>
          <span>${t('adv_time_require')}</span>
        </label>
        ${this.actionsHtml()}
      </div>`;
    this.wireDateMask('adv-from-d');
    this.wireDateMask('adv-to-d');
    this.wireTimeAutoTab('adv-from-hh', 'adv-from-mm');
    this.wireTimeAutoTab('adv-to-hh', 'adv-to-mm');
    this.bindFormActions(
      () => { this.state.timeFrom = ''; this.state.timeTo = ''; },
      () => {
        this.state.timeFrom = joinBackendTime(
          (document.getElementById('adv-from-d') as HTMLInputElement).value,
          (document.getElementById('adv-from-hh') as HTMLInputElement).value,
          (document.getElementById('adv-from-mm') as HTMLInputElement).value,
          false,
        );
        this.state.timeTo = joinBackendTime(
          (document.getElementById('adv-to-d') as HTMLInputElement).value,
          (document.getElementById('adv-to-hh') as HTMLInputElement).value,
          (document.getElementById('adv-to-mm') as HTMLInputElement).value,
          true,
        );
        this.state.timeRequire = (document.getElementById('adv-time-req') as HTMLInputElement).checked;
      },
    );
  }

  private wireDateMask(id: string) {
    const el = document.getElementById(id) as HTMLInputElement | null;
    if (!el) return;
    el.addEventListener('input', () => {
      let d = el.value.replace(/[^\d]/g, '').slice(0, 8);
      if (d.length > 4) d = d.slice(0, 4) + '-' + d.slice(4);
      if (d.length > 7) d = d.slice(0, 7) + '-' + d.slice(7);
      el.value = d;
    });
  }

  private wireTimeAutoTab(hhId: string, mmId: string) {
    const hh = document.getElementById(hhId) as HTMLInputElement | null;
    const mm = document.getElementById(mmId) as HTMLInputElement | null;
    if (!hh || !mm) return;
    hh.addEventListener('input', () => {
      hh.value = hh.value.replace(/\D/g, '').slice(0, 2);
      if (hh.value.length >= 2) mm.focus();
    });
    mm.addEventListener('input', () => {
      mm.value = mm.value.replace(/\D/g, '').slice(0, 2);
    });
  }

  private renderExclude() {
    this.panel.innerHTML = `
      <div class="adv-form">
        <div class="adv-field">
          <span class="adv-label">${t('adv_exclude_patterns')}</span>
          <textarea id="adv-exclude-ta" class="adv-input adv-textarea" rows="3" placeholder="…">${escapeAttr(this.state.exclude.join('\n'))}</textarea>
        </div>
        <label class="adv-check ${this.state.excludeRegex ? 'on' : ''}">
          <input type="checkbox" id="adv-exclude-re" ${this.state.excludeRegex ? 'checked' : ''}/>
          <span>${t('adv_exclude_as_regex')}</span>
        </label>
        ${this.actionsHtml()}
      </div>`;
    this.bindFormActions(
      () => { this.state.exclude = []; },
      () => {
        const raw = (document.getElementById('adv-exclude-ta') as HTMLTextAreaElement).value;
        this.state.exclude = raw.split('\n').map((s) => s.trim()).filter(Boolean);
        this.state.excludeRegex = (document.getElementById('adv-exclude-re') as HTMLInputElement).checked;
      },
    );
  }

  private renderRange() {
    this.panel.innerHTML = `
      <div class="adv-form">
        <div class="adv-seg" id="adv-range-seg">
          <button type="button" class="search-opt-btn ${this.state.rangeMode === 'all' ? 'active' : ''}" data-range="all">${t('adv_range_all')}</button>
          <button type="button" class="search-opt-btn ${this.state.rangeMode === 'from_cursor' ? 'active' : ''}" data-range="from_cursor">${t('adv_range_from_cursor')}</button>
          <button type="button" class="search-opt-btn ${this.state.rangeMode === 'last_n' ? 'active' : ''}" data-range="last_n">${t('adv_range_last_n')}</button>
        </div>
        <div class="adv-field" id="adv-range-n-wrap" style="${this.state.rangeMode === 'last_n' ? '' : 'display:none'}">
          <span class="adv-label">${t('adv_range_last_n_label')}</span>
          <input type="number" id="adv-range-last-n" class="adv-input" min="100" max="5000000" step="100" value="${this.state.lastN}" />
        </div>
        <div class="adv-hint">${t('adv_range_hint')}</div>
        ${this.actionsHtml()}
      </div>`;
    let mode = this.state.rangeMode;
    this.panel.querySelectorAll('[data-range]').forEach((btn) => {
      btn.addEventListener('click', () => {
        mode = (btn as HTMLElement).dataset.range as any;
        this.panel.querySelectorAll('[data-range]').forEach((b) => b.classList.toggle('active', b === btn));
        const wrap = document.getElementById('adv-range-n-wrap') as HTMLElement;
        if (wrap) wrap.style.display = mode === 'last_n' ? '' : 'none';
      });
    });
    this.bindFormActions(
      () => { this.state.rangeMode = 'all'; this.state.lastN = 5000; },
      () => {
        this.state.rangeMode = mode;
        this.state.lastN = parseInt((document.getElementById('adv-range-last-n') as HTMLInputElement)?.value || '5000', 10) || 5000;
      },
    );
  }

  private toggleTop() {
    this.state.topMode = !this.state.topMode;
    // Never hide the advanced bar — top only affects results viewport.
    this.syncChipActive();
    this.onChange();
  }

  private renderPresets() {
    const raw = localStorage.getItem('gofle.searchPresets') || '[]';
    let presets: any[] = [];
    try { presets = JSON.parse(raw); } catch { presets = []; }
    const list = presets.map((p, i) =>
      `<div class="adv-preset-row">
        <button type="button" class="search-opt-btn adv-preset-load" data-load="${i}">${escapeAttr(p.name || 'preset')}</button>
        <button type="button" class="search-opt-btn adv-preset-del" data-del="${i}">×</button>
      </div>`).join('') || `<div class="adv-hint">${t('adv_preset_empty')}</div>`;
    this.panel.innerHTML = `
      <div class="adv-form">
        ${list}
        <div class="adv-field-row">
          <div class="adv-field" style="flex:1">
            <span class="adv-label">${t('adv_preset_name')}</span>
            <input type="text" id="adv-preset-name" class="adv-input" />
          </div>
          <button type="button" class="search-opt-btn active" id="adv-preset-save" style="align-self:flex-end">${t('adv_preset_save')}</button>
        </div>
      </div>`;
    this.panel.querySelectorAll('[data-load]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const i = parseInt((btn as HTMLElement).dataset.load || '0', 10);
        const p = presets[i];
        if (p?.filter) {
          this.state = {
            ...defaultState(),
            ...p.filter,
            rangeMode: (p.filter.rangeMode === 'lines' || p.filter.rangeMode === 'to_cursor') ? 'all' : (p.filter.rangeMode || 'all'),
            lastN: p.filter.lastN ?? 5000,
          };
        }
        this.closePanel();
        this.syncChipActive();
        this.onChange();
      });
    });
    this.panel.querySelectorAll('[data-del]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const i = parseInt((btn as HTMLElement).dataset.del || '0', 10);
        presets.splice(i, 1);
        localStorage.setItem('gofle.searchPresets', JSON.stringify(presets));
        this.renderPresets();
      });
    });
    document.getElementById('adv-preset-save')?.addEventListener('click', () => {
      const name = (document.getElementById('adv-preset-name') as HTMLInputElement).value.trim() || 'preset';
      presets.push({ name, filter: { ...this.state } });
      localStorage.setItem('gofle.searchPresets', JSON.stringify(presets));
      this.renderPresets();
    });
  }

  private bindFormActions(onClear: () => void, onApply: () => void) {
    this.panel.querySelector('[data-adv-act="clear"]')?.addEventListener('click', () => {
      onClear();
      this.applyAndClose();
    });
    this.panel.querySelector('[data-adv-act="apply"]')?.addEventListener('click', () => {
      onApply();
      this.applyAndClose();
    });
  }

  toFilterMap(totalLines: number): Record<string, unknown> {
    const s = this.state;
    let lineStart = 0;
    let lineEnd = 0;
    const cursor = this.getCursorLine();
    if (s.rangeMode === 'from_cursor') {
      lineStart = Math.max(0, cursor);
    } else if (s.rangeMode === 'last_n') {
      const n = Math.max(1, s.lastN || 5000);
      lineStart = Math.max(0, totalLines - n);
    }
    const prefs = getPrefs() as any;
    return {
      exclude_patterns: s.exclude,
      exclude_is_regex: s.excludeRegex,
      nearby_query: s.nearbyQuery,
      nearby_mode: s.nearbyMode,
      nearby_window: s.nearbyWindow,
      levels: s.levels,
      line_start: lineStart,
      line_end: lineEnd,
      time_from: s.timeFrom,
      time_to: s.timeTo,
      time_require_stamp: s.timeRequire,
      context_before: Math.max(0, Math.min(50, prefs.ctxBefore ?? 0)),
      context_after: Math.max(0, Math.min(50, prefs.ctxAfter ?? 0)),
    };
  }

  hasAdvanced(): boolean {
    const s = this.state;
    return !!(s.nearbyQuery || s.levels.length || s.exclude.length ||
      s.timeFrom || s.timeTo || s.rangeMode !== 'all' || s.topMode);
  }
}

export function escapeAttr(s: string): string {
  return String(s || '')
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}