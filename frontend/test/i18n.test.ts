import { describe, it, expect, beforeAll, beforeEach, afterEach, vi } from 'vitest';
import { t, setLang, getLang, resolveLang, resolveTheme, initI18n, type LangCode } from '../src/i18n';

// Mock window.matchMedia used by resolveTheme for the system theme branch.
const mockMatchMedia = vi.fn().mockReturnValue({ matches: false });

function ensureWindow() {
  if (typeof globalThis.window === 'undefined') {
    (globalThis as any).window = { matchMedia: mockMatchMedia };
  } else {
    (globalThis.window as any).matchMedia = mockMatchMedia;
  }
}

beforeEach(() => {
  setLang('uk');
  ensureWindow();
  mockMatchMedia.mockClear();
});

afterEach(() => {
  setLang('uk' as LangCode);
});

beforeAll(async () => {
  await initI18n();
});

describe('i18n t()', () => {
  it('translates existing key to Ukrainian', () => {
    expect(t('btn_open')).toBe('Відкрити');
  });

  it('translates existing key to English', () => {
    setLang('en');
    expect(t('btn_open')).toBe('Open');
  });

  it('returns the key itself when translation is missing', () => {
    expect(t('nonexistent.key')).toBe('nonexistent.key');
  });

  it('replaces a single {name} parameter', () => {
    expect(t('confirm_close_group', { name: 'Тестова група' }))
      .toBe('Закрити всі вкладки групи «Тестова група»?');
  });

  it('replaces multiple parameters', () => {
    expect(t('err_restore_tab_failed', { path: '/var/log/app.log', err: 'файл не знайдено' }))
      .toBe('Вкладку не відновлено: /var/log/app.log: файл не знайдено');
  });

  it('switches between uk and en', () => {
    expect(t('btn_open')).toBe('Відкрити');
    setLang('en');
    expect(t('btn_open')).toBe('Open');
    setLang('uk');
    expect(t('btn_open')).toBe('Відкрити');
  });
});

describe('setLang / getLang', () => {
  it('sets and gets language', () => {
    setLang('en');
    expect(getLang()).toBe('en');
    setLang('uk');
    expect(getLang()).toBe('uk');
  });
});

describe('resolveLang', () => {
  it('returns uk for explicit uk', () => {
    expect(resolveLang('uk')).toBe('uk');
  });

  it('returns en for explicit en', () => {
    expect(resolveLang('en')).toBe('en');
  });

  it('returns en for undefined', () => {
    expect(resolveLang(undefined)).toBe('en');
  });

  it('returns en for empty string', () => {
    expect(resolveLang('')).toBe('en');
  });

  it('returns uk for uk-prefixed navigator language', () => {
    expect(resolveLang('uk_UA')).toBe('uk');
    expect(resolveLang('uk')).toBe('uk');
  });

  it('returns en for other navigator languages', () => {
    expect(resolveLang('en_US')).toBe('en');
    expect(resolveLang('de_DE')).toBe('en');
    expect(resolveLang('pl_PL')).toBe('en');
  });
});

describe('resolveTheme', () => {
  it('returns light for explicit light', () => {
    expect(resolveTheme('light')).toBe('light');
  });

  it('returns dark for explicit dark', () => {
    expect(resolveTheme('dark')).toBe('dark');
  });

  it('returns dark when matchMedia matches dark', () => {
    mockMatchMedia.mockReturnValue({ matches: true });
    expect(resolveTheme(undefined)).toBe('dark');
  });

  it('returns light when matchMedia does not match dark', () => {
    mockMatchMedia.mockReturnValue({ matches: false });
    expect(resolveTheme(undefined)).toBe('light');
  });
});
