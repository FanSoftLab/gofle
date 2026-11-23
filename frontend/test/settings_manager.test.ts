import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getPrefs, type AppPrefs } from '../src/settings_manager';

// Minimal DOM mock for settings_manager functions that touch document.
const mockStyleSetProperty = vi.fn();
const mockStyleRemoveProperty = vi.fn();
const mockClassListAdd = vi.fn();
const mockClassListRemove = vi.fn();
const mockClassListToggle = vi.fn();
const mockClassListContains = vi.fn(() => false);

beforeEach(() => {
  vi.resetAllMocks();
  (globalThis as any).document = {
    documentElement: {
      style: { setProperty: mockStyleSetProperty, removeProperty: mockStyleRemoveProperty },
      classList: { add: mockClassListAdd, remove: mockClassListRemove, contains: mockClassListContains, toggle: mockClassListToggle },
    },
    body: {
      style: { setProperty: mockStyleSetProperty },
      classList: { add: mockClassListAdd, remove: mockClassListRemove, toggle: mockClassListToggle },
      dataset: {},
      className: '',
    },
    getElementById: vi.fn(() => null),
    querySelectorAll: vi.fn(() => []),
  };
});

describe('getPrefs', () => {
  it('returns default preferences', () => {
    const prefs = getPrefs();
    expect(prefs.theme).toBe('system');
    expect(prefs.language).toBe('system');
    expect(prefs.saveTabsExit).toBe(true);
    expect(prefs.viewMode).toBe('split');
    expect(prefs.logFontSize).toBe(13);
    expect(prefs.logFontFamily).toBe('monospace');
    expect(prefs.lineDensity).toBe('normal');
    expect(prefs.wordWrap).toBe(false);
    expect(prefs.showLineNumbers).toBe(true);
    expect(prefs.defaultFollow).toBe(true);
    expect(prefs.defaultSearchCase).toBe(false);
    expect(prefs.defaultSearchRegex).toBe(false);
    expect(prefs.defaultSearchInvert).toBe(false);
    expect(prefs.defaultSearchCombine).toBe('or');
    expect(prefs.ctxBefore).toBe(0);
    expect(prefs.ctxAfter).toBe(0);
    expect(prefs.nearbyMode).toBe('either');
    expect(prefs.nearbyWindow).toBe(10);
    expect(prefs.excludeRegexDefault).toBe(false);
    expect(prefs.timeRequireStamp).toBe(false);
    expect(prefs.topLimit).toBe(100);
    expect(prefs.hlPrimary).toBe(true);
    expect(prefs.hlSecondary).toBe(true);
    expect(prefs.hlNearby).toBe(true);
    expect(prefs.hlExclude).toBe(false);
    expect(prefs.maxRecentFiles).toBe(20);
    expect(prefs.maxHistorySize).toBe(100);
    expect(prefs.confirmCloseTab).toBe(true);
    expect(prefs.startupBehavior).toBe('session');
    expect(prefs.searchOrientation).toBe('horizontal');
    expect(prefs.searchSide).toBe('bottom');
    expect(prefs.sshConnectTimeoutSec).toBe(10);
    expect(prefs.remoteStallTimeoutSec).toBe(60);
    expect(prefs.autoAcceptHostKey).toBe(false);
    expect(prefs.connMaxAttempts).toBe(20);
    expect(prefs.connAttemptWindowMin).toBe(5);
    expect(prefs.connLockoutMin).toBe(1);
    expect(prefs.defaultSshPort).toBe(22);
    expect(prefs.defaultFtpPort).toBe(21);
  });

  it('returns a reference to the same currentPrefs object (not a copy)', () => {
    const p1 = getPrefs();
    const p2 = getPrefs();
    expect(p1).toBe(p2);
  });
});

describe('AppPrefs type', () => {
  it('has all required fields', () => {
    const prefs: AppPrefs = getPrefs();
    // Security-relevant defaults must be preserved
    expect(prefs.autoAcceptHostKey).toBe(false);
    expect(prefs.connMaxAttempts).toBeGreaterThan(0);
    expect(prefs.connLockoutMin).toBeGreaterThan(0);
    expect(prefs.sshConnectTimeoutSec).toBeGreaterThan(0);
  });
});
