import { describe, it, expect } from 'vitest';
import { splitStoredTime, joinBackendTime, escapeAttr } from '../src/search_advanced';

describe('splitStoredTime', () => {
  it('returns empty parts for empty string', () => {
    expect(splitStoredTime('')).toEqual({ date: '', hh: '', mm: '' });
  });

  it('returns empty parts for whitespace-only string', () => {
    expect(splitStoredTime('   ')).toEqual({ date: '', hh: '', mm: '' });
  });

  it('parses date only', () => {
    expect(splitStoredTime('2024-01-15')).toEqual({ date: '2024-01-15', hh: '', mm: '' });
  });

  it('parses date with space-separated time', () => {
    expect(splitStoredTime('2024-01-15 14:30')).toEqual({ date: '2024-01-15', hh: '14', mm: '30' });
  });

  it('parses date with T separator and seconds', () => {
    expect(splitStoredTime('2024-01-15T14:30:45')).toEqual({ date: '2024-01-15', hh: '14', mm: '30' });
  });

  it('parses time with leading zeros', () => {
    expect(splitStoredTime('2024-01-15T08:00')).toEqual({ date: '2024-01-15', hh: '08', mm: '00' });
  });

  it('returns empty parts for invalid date', () => {
    expect(splitStoredTime('not-a-date')).toEqual({ date: '', hh: '', mm: '' });
    expect(splitStoredTime('2024-13-45')).toEqual({ date: '', hh: '', mm: '' });
    expect(splitStoredTime('2024-02-30')).toEqual({ date: '', hh: '', mm: '' });
  });

  it('ignores trailing content after valid date-time', () => {
    expect(splitStoredTime('2024-01-15 14:30 extra')).toEqual({ date: '2024-01-15', hh: '14', mm: '30' });
  });
});

describe('joinBackendTime', () => {
  it('returns empty for empty date', () => {
    expect(joinBackendTime('', '', '', false)).toBe('');
    expect(joinBackendTime('', '', '', true)).toBe('');
  });

  it('returns empty for invalid date format', () => {
    expect(joinBackendTime('not-a-date', '', '', false)).toBe('');
    expect(joinBackendTime('2024-1-5', '', '', false)).toBe('');
    expect(joinBackendTime('2024-13-45', '', '', false)).toBe('');
  });

  it('returns start of day when no time given', () => {
    expect(joinBackendTime('2024-01-15', '', '', false)).toBe('2024-01-15 00:00:00');
  });

  it('returns end of day when no time given and endOfDay=true', () => {
    expect(joinBackendTime('2024-01-15', '', '', true)).toBe('2024-01-15 23:59:59');
  });

  it('parses hours and minutes', () => {
    expect(joinBackendTime('2024-01-15', '14', '30', false)).toBe('2024-01-15 14:30:00');
  });

  it('clamps hours to 0-23', () => {
    expect(joinBackendTime('2024-01-15', '99', '30', false)).toBe('2024-01-15 23:30:00');
    expect(joinBackendTime('2024-01-15', '-1', '30', false)).toBe('2024-01-15 00:30:00');
  });

  it('clamps minutes to 0-59', () => {
    expect(joinBackendTime('2024-01-15', '14', '99', false)).toBe('2024-01-15 14:59:00');
    expect(joinBackendTime('2024-01-15', '14', '-1', false)).toBe('2024-01-15 14:00:00');
  });

  it('handles missing hh or mm gracefully', () => {
    expect(joinBackendTime('2024-01-15', '14', '', false)).toBe('2024-01-15 14:00:00');
    expect(joinBackendTime('2024-01-15', '', '30', false)).toBe('2024-01-15 00:30:00');
  });
});

describe('escapeAttr', () => {
  it('escapes ampersand', () => {
    expect(escapeAttr('a & b')).toBe('a &amp; b');
  });

  it('escapes double quotes', () => {
    expect(escapeAttr('a "b" c')).toBe('a &quot;b&quot; c');
  });

  it('escapes less than', () => {
    expect(escapeAttr('a < b')).toBe('a &lt; b');
  });

  it('escapes a full XSS payload', () => {
    expect(escapeAttr('<img src=x onerror=alert(1)>'))
      .toBe('&lt;img src=x onerror=alert(1)&gt;');
  });

  it('returns empty string for nullish input', () => {
    expect(escapeAttr(null as unknown as string)).toBe('');
    expect(escapeAttr(undefined as unknown as string)).toBe('');
  });

  it('leaves safe text unchanged', () => {
    expect(escapeAttr('plain text 123')).toBe('plain text 123');
  });
});
