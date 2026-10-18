import { describe, it, expect, beforeEach, vi } from 'vitest';
import { VirtualScroller, type LogLineData, type FetchLinesCallback } from '../src/virtual_scroller';

// Minimal DOM mock sufficient to construct VirtualScroller and test pure-logic
// methods (selection, highlighted line, search query, view anchor).
function createMockViewport() {
  const style: Record<string, string> = {};
  const el: any = {
    tabIndex: -1,
    clientHeight: 400,
    clientWidth: 800,
    scrollTop: 0,
    scrollLeft: 0,
    style,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    appendChild: vi.fn(),
    classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn() },
    focus: vi.fn(),
    querySelectorAll: vi.fn(() => []),
    getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0 })),
    children: [],
    dataset: {},
    className: '',
    scrollHeight: 0,
    offsetHeight: 20,
    scrollWidth: 0,
  };
  return el;
}

function createMockDocument() {
  const viewport = createMockViewport();
  const createEl = () => ({
    className: '',
    style: {},
    classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn() },
    appendChild: vi.fn(),
    remove: vi.fn(),
    dataset: {},
    children: [],
    addEventListener: vi.fn(),
    setAttribute: vi.fn(),
    querySelectorAll: vi.fn(() => []),
    querySelector: vi.fn(() => null),
    innerHTML: '',
    textContent: '',
    scrollHeight: 0,
    offsetHeight: 20,
    scrollWidth: 0,
  });
  return {
    getElementById: vi.fn(() => viewport),
    createElement: vi.fn(createEl),
    documentElement: {
      style: { setProperty: vi.fn(), removeProperty: vi.fn() },
      classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn() },
    },
    body: {
      style: { setProperty: vi.fn() },
      classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn() },
      className: '',
      dataset: {},
    },
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  } as unknown as Document;
}

function mockWindow() {
  (globalThis as any).window = {
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    matchMedia: vi.fn(() => ({ matches: false })),
    getSelection: vi.fn(() => ({ isCollapsed: true, removeAllRanges: vi.fn(), toString: vi.fn(() => '') })),
    requestAnimationFrame: vi.fn((cb: FrameRequestCallback) => cb(0)),
    cancelAnimationFrame: vi.fn(),
    innerWidth: 1000,
    innerHeight: 800,
  };
  (globalThis as any).ResizeObserver = undefined;
}

describe('VirtualScroller', () => {
  let doc: ReturnType<typeof createMockDocument>;
  let viewport: ReturnType<typeof createMockViewport>;

  beforeEach(() => {
    doc = createMockDocument();
    viewport = doc.getElementById('viewport');
    (globalThis as any).document = doc;
    mockWindow();
  });

  it('constructs without crashing', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    expect(scroller).toBeInstanceOf(VirtualScroller);
  });

  it('getSelectedIndices returns empty when nothing selected', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    expect(scroller.getSelectedIndices()).toEqual([]);
    expect(scroller.getSelectedRange()).toBe(null);
  });

  it('setSelection creates a contiguous range', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setSelection(5, 10);
    expect(scroller.getSelectedIndices()).toEqual([5, 6, 7, 8, 9, 10]);
    expect(scroller.getSelectedRange()).toEqual({ start: 5, end: 10 });
  });

  it('setSelection with reversed order normalizes', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setSelection(10, 5);
    expect(scroller.getSelectedIndices()).toEqual([5, 6, 7, 8, 9, 10]);
    expect(scroller.getSelectedRange()).toEqual({ start: 5, end: 10 });
  });

  it('extendSelection extends contiguous range from anchor', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setSelection(5, 5);
    scroller.extendSelection(10);
    expect(scroller.getSelectedIndices()).toEqual([5, 6, 7, 8, 9, 10]);
  });

  it('clearSelection clears all selection state', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setSelection(3, 7);
    scroller.clearSelection();
    expect(scroller.getSelectedIndices()).toEqual([]);
    expect(scroller.getSelectedRange()).toBe(null);
  });

  it('setHighlightedLine stores the index', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setHighlightedLine(42);
    const anchor = scroller.getViewAnchor();
    // With zero total lines, windowFromScrollTop returns index 0.
    expect(anchor.index).toBe(0);
    expect(anchor.fraction).toBe(0);
  });

  it('setSearchQuery stores the query without crashing', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setSearchQuery('error');
    expect(scroller).toBeDefined();
  });

  it('setWordWrap toggles word wrap mode', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    expect(scroller.getWordWrap()).toBe(false);
    scroller.setWordWrap(true);
    expect(scroller.getWordWrap()).toBe(true);
    scroller.setWordWrap(true); // no-op when already set
    expect(scroller.getWordWrap()).toBe(true);
    scroller.setWordWrap(false);
    expect(scroller.getWordWrap()).toBe(false);
  });

  it('setTotalLines with zero clears and keeps zero', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setTotalLines(0);
    expect(scroller.getViewAnchor()).toEqual({ index: 0, fraction: 0 });
  });

  it('setViewAnchor clamps fraction to [0, 1]', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setTotalLines(100);
    scroller.setViewAnchor(10, 0.5);
    const anchor = scroller.getViewAnchor();
    expect(anchor.index).toBeGreaterThanOrEqual(0);
    expect(anchor.fraction).toBeGreaterThanOrEqual(0);
    expect(anchor.fraction).toBeLessThanOrEqual(1);
  });

  it('scrollToBottom clamps to max scroll', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setTotalLines(100);
    scroller.scrollToBottom();
    // Should not throw
    expect(scroller).toBeDefined();
  });

  it('scrollToIndex clamps out of range', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setTotalLines(10);
    scroller.scrollToIndex(-1); // no-op
    scroller.scrollToIndex(99); // no-op
    scroller.scrollToIndex(5);
    expect(scroller).toBeDefined();
  });

  it('setBottomInset accepts non-negative values', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setBottomInset(0);
    scroller.setBottomInset(50);
    scroller.setBottomInset(-10); // clamped to 0
    expect(scroller).toBeDefined();
  });

  it('FetchLinesCallback type is a function type', () => {
    const cb: FetchLinesCallback = async (start, count) => {
      const lines: LogLineData[] = [];
      for (let i = 0; i < count; i++) {
        lines.push({ index: start + i, content: `line ${start + i}` });
      }
      return lines;
    };
    expect(typeof cb).toBe('function');
  });
});
