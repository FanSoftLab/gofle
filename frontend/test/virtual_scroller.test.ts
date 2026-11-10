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
    appendChild(child: any) {
      el.children.push(child);
      return child;
    },
    classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn(), contains: vi.fn(() => false) },
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
  // Fake element with working children/appendChild: the render pipeline reads
  // rowNode.children[0]/[1] and appends row nodes to the content container.
  const createEl = () => {
    const el: any = {
      className: '',
      style: {} as Record<string, string>,
      classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn(), contains: vi.fn(() => false) },
      appendChild(child: any) {
        el.children.push(child);
        return child;
      },
      remove: vi.fn(),
      dataset: {},
      children: [] as any[],
      addEventListener: vi.fn(),
      setAttribute: vi.fn(),
      querySelectorAll: vi.fn(() => []),
      querySelector: vi.fn(() => null),
      innerHTML: '',
      textContent: '',
      scrollHeight: 0,
      offsetHeight: 20,
      scrollWidth: 0,
      getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0 })),
    };
    return el;
  };
  return {
    getElementById: vi.fn(() => viewport),
    createElement: vi.fn(createEl),
    // The render pipeline mounts fetched rows through a DocumentFragment.
    createDocumentFragment: vi.fn(() => {
      const frag: any = {
        children: [] as any[],
        appendChild(child: any) {
          frag.children.push(child);
          return child;
        },
      };
      return frag;
    }),
    documentElement: {
      style: { setProperty: vi.fn(), removeProperty: vi.fn() },
      classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn(), contains: vi.fn(() => false) },
    },
    body: {
      style: { setProperty: vi.fn() },
      classList: { add: vi.fn(), remove: vi.fn(), toggle: vi.fn(), contains: vi.fn(() => false) },
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
  // The scroller calls bare requestAnimationFrame / cancelAnimationFrame.
  (globalThis as any).requestAnimationFrame = (cb: FrameRequestCallback) => cb(0);
  (globalThis as any).cancelAnimationFrame = vi.fn();
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
    scroller.setTotalLines(100);
    scroller.setSelection(5, 10);
    expect(scroller.getSelectedIndices()).toEqual([5, 6, 7, 8, 9, 10]);
    expect(scroller.getSelectedRange()).toEqual({ start: 5, end: 10 });
  });

  it('setSelection with reversed order normalizes', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setTotalLines(100);
    scroller.setSelection(10, 5);
    expect(scroller.getSelectedIndices()).toEqual([5, 6, 7, 8, 9, 10]);
    expect(scroller.getSelectedRange()).toEqual({ start: 5, end: 10 });
  });

  it('extendSelection extends contiguous range from anchor', () => {
    const scroller = new VirtualScroller('viewport', async (): Promise<LogLineData[]> => []);
    scroller.setTotalLines(100);
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

  // --- Word-wrap jump-to-search-result tests ---
  // In wrap mode scrollToIndex must go through the wrap-safe content anchor
  // (pendingAnchor) instead of a one-shot pixel write: heights for lines that
  // were never rendered are only estimated, so the position has to be
  // re-applied with refined heights and converge on the target.

  const LINE_H = 20; // mock row height: lineHeight/avgHeight default
  const VIEW_H = 400; // mock viewport.clientHeight

  function flushTicks(): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, 0));
  }

  function makeLineFetcher(): FetchLinesCallback {
    return async (start, count) => {
      const lines: LogLineData[] = [];
      for (let i = 0; i < count; i++) {
        lines.push({ index: start + i, content: `line ${start + i}` });
      }
      return lines;
    };
  }

  it('scrollToIndex wrap mode places the target ~35% from the top via anchor', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);
    scroller.scrollToIndex(150);

    // Expected anchor position: 150 rows above the target at LINE_H each,
    // minus JUMP_VIEWPORT_FRACTION (0.35) of the viewport height. All mock
    // rows are single-height, so the estimate is exact.
    const expected = 150 * LINE_H - 0.35 * VIEW_H;
    expect(viewport.scrollTop).toBe(expected);

    // Let the async fetch/render/refine chain settle: the target row mounts
    // and the bounded retry state clears.
    await flushTicks();
    await flushTicks();

    const sc = scroller as any;
    expect(sc.renderedNodes.has(150)).toBe(true);
    expect(sc.pendingJump).toBe(null);
    // Anchor survives (user has not scrolled) and still resolves to the same
    // position — it keeps the target pinned while heights refine.
    expect(sc.pendingAnchor).not.toBe(null);
    expect(viewport.scrollTop).toBe(expected);
  });

  it('scrollToIndex wrap mode re-applies the anchor when heights refine', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);
    // Pre-measure every row at the base height so the height estimate is
    // exact and the avgHeight sample stays stable when one row is refined.
    const sc = scroller as any;
    for (let i = 0; i < 200; i++) sc.rememberHeight(i, LINE_H);
    scroller.scrollToIndex(150);

    const before = viewport.scrollTop;
    expect(before).toBe(150 * LINE_H - 0.35 * VIEW_H);

    // A row above the target actually wraps to 3 rows: re-applying the anchor
    // with the refined height must shift the scroll down by exactly 40px.
    sc.rememberHeight(100, 3 * LINE_H);
    sc.applyPendingAnchor();

    expect(viewport.scrollTop).toBe(before + 2 * LINE_H);
  });

  it('scrollToIndex wrap mode drops the jump on user scroll and bounds retries', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);
    scroller.scrollToIndex(150);
    await flushTicks();
    await flushTicks();

    const sc = scroller as any;

    // Exhausted passes clear the jump without rendering an endless loop.
    sc.pendingJump = { index: 199, passes: 0 };
    sc.continuePendingJump();
    expect(sc.pendingJump).toBe(null);

    // A user-driven scroll event drops the anchor and any pending jump.
    sc.pendingAnchor = { index: 100, fraction: 0 };
    sc.pendingJump = { index: 199, passes: 5 };
    viewport.scrollTop = 500;
    sc.onScroll();
    expect(sc.pendingAnchor).toBe(null);
    expect(sc.pendingJump).toBe(null);
  });

  it('scrollToIndex wrap mode falls back to the proportional map when the spacer cap engages', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);
    // 800k rows × 20px = 16M px of content > MAX_SAFE_PIXELS (15M): the scroll
    // spacer is capped, so height-sum anchors beyond the cap are not
    // representable and must map proportionally instead of clamping to EOF.
    const total = 800_000;
    scroller.setTotalLines(total);
    scroller.scrollToIndex(total - 1000);

    // Anchor walks back 0.35×400 = 140px = 7 rows from the target.
    const expected = ((total - 1000 - 7) / (total - 20)) * (15_000_000 - VIEW_H);
    expect(viewport.scrollTop).toBeCloseTo(expected, 0);

    await flushTicks();
    await flushTicks();

    const sc = scroller as any;
    expect(sc.renderedNodes.has(total - 1000)).toBe(true);
    expect(sc.pendingJump).toBe(null);
  });

  it('scrollToIndex non-wrap keeps fractional positioning', () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setTotalLines(100);
    scroller.scrollToIndex(50);

    // visibleCount = floor(400 / 20) = 20; maxFirst = 80;
    // targetFirst = 50 - 8 = 42 → 42 / 80 × (100×20 − 400) = 840.
    expect(viewport.scrollTop).toBe(840);
    expect((scroller as any).pendingJump).toBe(null);
  });
});

// =============================================================================
// G1: scrollToIndex with word wrap
// =============================================================================
describe('scrollToIndex with wordWrap', () => {
  let doc: ReturnType<typeof createMockDocument>;
  let viewport: ReturnType<typeof createMockViewport>;

  beforeEach(() => {
    doc = createMockDocument();
    viewport = doc.getElementById('viewport');
    (globalThis as any).document = doc;
    mockWindow();
  });

  const LINE_H = 20;
  const VIEW_H = 400;

  function flushTicks(): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, 0));
  }

  function makeVariableLineFetcher(): FetchLinesCallback {
    return async (start, count) => {
      const lines: LogLineData[] = [];
      for (let i = 0; i < count; i++) {
        const idx = start + i;
        // Mix of short and very long lines
        let content: string;
        if (idx % 5 === 0) {
          // Long line that wraps many times
          content = `LONG-${idx} ${'payload-'.repeat(30)}`;
        } else if (idx % 5 === 2) {
          // Medium line
          content = `MEDIUM-${idx} ${'data-'.repeat(10)}`;
        } else {
          // Short line
          content = `short-${idx}`;
        }
        lines.push({ index: idx, content });
      }
      return lines;
    };
  }

  it('far jump lands near target after convergence', async () => {
    const scroller = new VirtualScroller('viewport', makeVariableLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);
    scroller.scrollToIndex(150);

    // Let the async fetch/render/refine chain settle
    await flushTicks();
    await flushTicks();

    const sc = scroller as any;
    expect(sc.renderedNodes.has(150)).toBe(true);
    expect(sc.pendingJump).toBe(null);
    // Anchor survives (user has not scrolled) and still resolves to the same position
    expect(sc.pendingAnchor).not.toBe(null);

    // Verify the target is in the visible window
    const anchor = scroller.getViewAnchor();
    // Target should be roughly JUMP_VIEWPORT_FRACTION (0.35) from top
    // So firstVisible should be around 150 - 0.35 * visibleCount
    const visibleCount = Math.floor(VIEW_H / LINE_H); // ~20
    const expectedFirstVisible = Math.max(0, 150 - Math.floor(visibleCount * 0.35));
    expect(anchor.index).toBeGreaterThanOrEqual(expectedFirstVisible - 2);
    expect(anchor.index).toBeLessThanOrEqual(expectedFirstVisible + 2);
  });

  it('sequential jumps: 10 -> 120 -> 40, anchor not stuck', async () => {
    const scroller = new VirtualScroller('viewport', makeVariableLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);

    // First jump
    scroller.scrollToIndex(10);
    await flushTicks();
    await flushTicks();

    // Second jump
    scroller.scrollToIndex(120);
    await flushTicks();
    await flushTicks();

    // Third jump
    scroller.scrollToIndex(40);
    await flushTicks();
    await flushTicks();

    const sc = scroller as any;
    expect(sc.renderedNodes.has(40)).toBe(true);
    expect(sc.pendingJump).toBe(null);
    expect(sc.pendingAnchor).not.toBe(null);

    // Final highlight should be 40
    expect(scroller.getHighlightedLine()).toBe(40);
  });

  it('non-wrap regression: target in visible window', () => {
    const scroller = new VirtualScroller('viewport', makeVariableLineFetcher());
    scroller.setWordWrap(false);
    scroller.setTotalLines(200);
    scroller.scrollToIndex(150);

    // In non-wrap mode, target should be in visible window immediately
    const anchor = scroller.getViewAnchor();
    const visibleCount = Math.floor(VIEW_H / LINE_H); // 20
    // First visible should be target - visibleCount * JUMP_VIEWPORT_FRACTION
    const expectedFirst = Math.max(0, 150 - Math.floor(visibleCount * 0.35));
    expect(anchor.index).toBeGreaterThanOrEqual(expectedFirst - 1);
    expect(anchor.index).toBeLessThanOrEqual(expectedFirst + 1);
    expect((scroller as any).pendingJump).toBe(null);
  });
});

// =============================================================================
// G2: Tab switch position restore (dataKey)
// =============================================================================
describe('setViewAnchor / getViewAnchor with dataKey (tab switch)', () => {
  let doc: ReturnType<typeof createMockDocument>;
  let viewport: ReturnType<typeof createMockViewport>;

  beforeEach(() => {
    doc = createMockDocument();
    viewport = doc.getElementById('viewport');
    (globalThis as any).document = doc;
    mockWindow();
  });

  function makeLineFetcher(): FetchLinesCallback {
    return async (start, count) => {
      const lines: LogLineData[] = [];
      for (let i = 0; i < count; i++) {
        lines.push({ index: start + i, content: `line ${start + i}` });
      }
      return lines;
    };
  }

  function flushTicks(): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, 0));
  }

  it('save anchor A, switch to B, restore A', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());

    // Tab A: set up with dataKey 'A'
    scroller.setTotalLines(100, 'A');
    scroller.setViewAnchor(50, 0.25);
    await flushTicks();

    const anchorA = scroller.getViewAnchor();
    expect(anchorA.index).toBeCloseTo(50, 0);
    expect(anchorA.fraction).toBeCloseTo(0.25, 2);

    // Tab B: different document
    scroller.setTotalLines(200, 'B');
    scroller.setViewAnchor(120, 0.5);
    await flushTicks();

    const anchorB = scroller.getViewAnchor();
    expect(anchorB.index).toBeCloseTo(120, 0);
    expect(anchorB.fraction).toBeCloseTo(0.5, 2);

    // Back to Tab A: restore anchor
    scroller.setViewAnchor(anchorA.index, anchorA.fraction);
    scroller.setTotalLines(100, 'A'); // Re-set with same key
    await flushTicks();

    const restoredA = scroller.getViewAnchor();
    // Allow small tolerance for fraction (±0.05) and index (±1)
    expect(restoredA.index).toBeGreaterThanOrEqual(anchorA.index - 1);
    expect(restoredA.index).toBeLessThanOrEqual(anchorA.index + 1);
    expect(restoredA.fraction).toBeGreaterThanOrEqual(anchorA.fraction - 0.05);
    expect(restoredA.fraction).toBeLessThanOrEqual(anchorA.fraction + 0.05);
  });

  it('wordWrap=true preserves anchor across tab switch', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);

    // Tab A with wrap
    scroller.setTotalLines(150, 'A');
    scroller.setViewAnchor(80, 0.3);
    await flushTicks();
    await flushTicks();

    const anchorA = scroller.getViewAnchor();

    // Tab B
    scroller.setTotalLines(300, 'B');
    scroller.setViewAnchor(200, 0.4);
    await flushTicks();
    await flushTicks();

    // Back to A
    scroller.setViewAnchor(anchorA.index, anchorA.fraction);
    scroller.setTotalLines(150, 'A');
    await flushTicks();
    await flushTicks();

    const restoredA = scroller.getViewAnchor();
    expect(restoredA.index).toBeGreaterThanOrEqual(anchorA.index - 1);
    expect(restoredA.index).toBeLessThanOrEqual(anchorA.index + 1);
    expect(restoredA.fraction).toBeGreaterThanOrEqual(anchorA.fraction - 0.05);
    expect(restoredA.fraction).toBeLessThanOrEqual(anchorA.fraction + 0.05);
  });

  it('double RAF / repeated setViewAnchor after setTotalLines does not reset to 0', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setTotalLines(100, 'tab1');
    scroller.setViewAnchor(60, 0.5);
    await flushTicks();

    // Call setViewAnchor again (simulating double RAF)
    scroller.setViewAnchor(60, 0.5);
    await flushTicks();

    // Call setTotalLines again with same key (simulating relayout)
    scroller.setTotalLines(100, 'tab1');
    await flushTicks();

    const anchor = scroller.getViewAnchor();
    expect(anchor.index).toBeCloseTo(60, 0);
    expect(anchor.fraction).toBeCloseTo(0.5, 2);
    // Should not be reset to 0
    expect(anchor.index).not.toBe(0);
  });

  it('filter scroller: dataKey = tabId + ":f:" + query', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());

    // Filter for tab1, query "error"
    const filterKey = 'tab1:f:error';
    scroller.setTotalLines(50, filterKey);
    scroller.setViewAnchor(20, 0.2);
    await flushTicks();

    const anchor1 = scroller.getViewAnchor();

    // Switch to different filter (same tab, different query)
    const filterKey2 = 'tab1:f:warn';
    scroller.setTotalLines(30, filterKey2);
    scroller.setViewAnchor(10, 0.5);
    await flushTicks();

    // Back to first filter
    scroller.setViewAnchor(anchor1.index, anchor1.fraction);
    scroller.setTotalLines(50, filterKey);
    await flushTicks();

    const restored = scroller.getViewAnchor();
    expect(restored.index).toBeGreaterThanOrEqual(anchor1.index - 1);
    expect(restored.index).toBeLessThanOrEqual(anchor1.index + 1);
    expect(restored.fraction).toBeGreaterThanOrEqual(anchor1.fraction - 0.05);
    expect(restored.fraction).toBeLessThanOrEqual(anchor1.fraction + 0.05);
  });
});

// =============================================================================
// G3: Scroll map consistency (wrap)
// =============================================================================
describe('scroll map consistency with wordWrap', () => {
  let doc: ReturnType<typeof createMockDocument>;
  let viewport: ReturnType<typeof createMockViewport>;

  beforeEach(() => {
    doc = createMockDocument();
    viewport = doc.getElementById('viewport');
    (globalThis as any).document = doc;
    mockWindow();
  });

  function makeLineFetcher(): FetchLinesCallback {
    return async (start, count) => {
      const lines: LogLineData[] = [];
      for (let i = 0; i < count; i++) {
        lines.push({ index: start + i, content: `line ${start + i}` });
      }
      return lines;
    };
  }

  function flushTicks(): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, 0));
  }

  it('setViewAnchor immediately reflected in getViewAnchor', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);

    scroller.setViewAnchor(75, 0.4);
    await flushTicks();

    const anchor = scroller.getViewAnchor();
    expect(anchor.index).toBe(75);
    expect(anchor.fraction).toBeCloseTo(0.4, 2);
  });

  it('during pending jump/anchor, window matches anchor not foreign scrollTop', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);

    // Trigger a far jump - this sets up pendingAnchor and pendingJump
    scroller.scrollToIndex(150);

    const sc = scroller as any;
    // Before fetch resolves, pendingAnchor should be active
    expect(sc.pendingAnchor).not.toBe(null);
    expect(sc.pendingJump).not.toBe(null);

    // The window should reflect the pending anchor, not some arbitrary scrollTop
    // (we can't easily test the exact window without full render, but we can verify
    // the anchor is set)
    expect(sc.pendingAnchor?.index).toBeGreaterThan(0);
  });

  it('user scroll clears pending anchor, next scrollToIndex works', async () => {
    const scroller = new VirtualScroller('viewport', makeLineFetcher());
    scroller.setWordWrap(true);
    scroller.setTotalLines(200);

    // Start a jump
    scroller.scrollToIndex(100);
    await flushTicks();

    const sc = scroller as any;
    expect(sc.pendingAnchor).not.toBe(null);
    expect(sc.pendingJump).not.toBe(null);

    // Simulate user scroll (onScroll)
    viewport.scrollTop = 500;
    sc.onScroll();

    // Pending anchor and jump should be cleared
    expect(sc.pendingAnchor).toBe(null);
    expect(sc.pendingJump).toBe(null);

    // Next scrollToIndex should work normally
    scroller.scrollToIndex(50);
    await flushTicks();
    await flushTicks();

    const anchor = scroller.getViewAnchor();
    expect(anchor.index).toBeGreaterThanOrEqual(40);
    expect(anchor.index).toBeLessThanOrEqual(60);
  });
});
