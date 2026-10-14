import { LogHighlighter } from './log_highlighter';
import { showToast } from './toast';

export interface LogLineData {
  index: number;
  content: string;
}

export type FetchLinesCallback = (start: number, count: number) => Promise<LogLineData[]>;

export class VirtualScroller {
  private viewport: HTMLElement;
  private contentContainer: HTMLElement;
  // Empty flow element that gives the scroll viewport its (capped) scroll
  // range. The rows themselves live in `contentContainer`, which is pinned to
  // the visible viewport via CSS `position: sticky`, so rows are always
  // positioned at small coordinates (0..viewportHeight) and never hit the
  // browser's per-element pixel cap (which would blur/clip them).
  private scrollSpacer: HTMLElement;
  private lineHeight = 20;
  private totalLines = 0;
  private visibleCount = 0;
  private bufferCount = 25;
  private fetchCallback: FetchLinesCallback;

  /** Full text wrap with variable row height (no clipping). */
  private wordWrap = false;
  private heightCache = new Map<number, number>();
  private avgHeight = 20;
  private charWidthPx = 8;

  private scrollSpacerHeight = 0;
  private contentWidthPx = 0;
  private bottomInset = 0;
  private static readonly MAX_SAFE_PIXELS = 15_000_000;

  private fetchInFlight = false;
  private pendingRender = false;

  private renderedNodes = new Map<number, HTMLElement>();
  private poolNodes: HTMLElement[] = [];
  private isRequestPending = false;
  private generation = 0;
  private currentScrollTop = 0;
  private highlightedLineIndex: number | null = null;
  private currentSearchQuery = '';
  private onRowClickCallback?: (index: number) => void;
  private onScrollPauseCallback?: () => void;
  private onScrollPositionCallback?: (anchor: { index: number; fraction: number }) => void;
  /** Pixel scroll to re-apply after layout (derived from pendingAnchor). */
  private pendingScrollTop: number | null = null;
  /** Content anchor to re-apply after setTotalLines / async fetch (wrap-safe). */
  private pendingAnchor: { index: number; fraction: number } | null = null;
  /** Ignore scroll events caused by programmatic scroll restores. */
  private suppressScrollEvent = false;
  private scrollPosRaf = 0;
  private measureWidthRaf = 0;


  private selectionStart: number | null = null;
  private selectionEnd: number | null = null;
  private isSelecting = false;
  private selectionAnchor: number | null = null;
  /** Non-contiguous selection (Ctrl/Cmd+click/drag). When non-empty, takes priority over the range. */
  private selectedLines = new Set<number>();
  /** Ctrl/Cmd drag: base set at mousedown + range being added on top. */
  private ctrlAddBase: Set<number> | null = null;
  private ctrlAddStart: number | null = null;
  private ctrlAddMoved = false;
  /** Pending plain mousedown on text — promote to line-range once the pointer leaves the start row. */
  private pendingLineSelect: { startIdx: number; startY: number } | null = null;

  private rowRawContent = new WeakMap<HTMLElement, string>();
  private rowHighlightKey = new WeakMap<HTMLElement, string>();

  constructor(viewportId: string, fetchCallback: FetchLinesCallback) {
    this.viewport = document.getElementById(viewportId)!;
    this.fetchCallback = fetchCallback;
    if (this.viewport.tabIndex < 0) this.viewport.tabIndex = 0;

    this.contentContainer = document.createElement('div');
    this.contentContainer.className = 'log-content-container';
    this.viewport.appendChild(this.contentContainer);

    this.scrollSpacer = document.createElement('div');
    this.scrollSpacer.className = 'log-scroll-spacer';
    this.viewport.appendChild(this.scrollSpacer);

    this.viewport.addEventListener('scroll', () => this.onScroll(), { passive: true });
    window.addEventListener('resize', () => this.onResize());
    // Panel/splitter size changes do not fire window.resize — observe the viewport.
    if (typeof ResizeObserver !== 'undefined') {
      let roT = 0;
      const ro = new ResizeObserver(() => {
        if (roT) cancelAnimationFrame(roT);
        roT = requestAnimationFrame(() => {
          roT = 0;
          this.onResize();
        });
      });
      ro.observe(this.viewport);
    }

    this.updateLayout();
    this.recalculateVisible();
    this.attachSelectionListeners();
  }

  /** Force layout + soft render after panel becomes visible (tab switch / session restore). */
  public relayout() {
    this.measureLineHeight();
    this.measureCharWidth();
    this.updateLayout();
    this.recalculateVisible();
    this.applyPendingAnchor();
    this.render(false);
  }

  private fractionFromScrollTop(scrollTop: number): number {
    const maxScroll = this.scrollSpacerHeight - this.effectiveClientHeight();
    if (maxScroll <= 0) return 0;
    return Math.max(0, Math.min(1, scrollTop / maxScroll));
  }

  private updateLayout() {
    const unit = this.wordWrap ? Math.max(this.lineHeight, this.avgHeight) : this.lineHeight;
    const full = this.totalLines * unit;
    this.scrollSpacerHeight = Math.min(full, VirtualScroller.MAX_SAFE_PIXELS);
    this.scrollSpacer.style.height = `${Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight())}px`;
    this.contentContainer.style.height = `${this.effectiveClientHeight()}px`;
    this.applyContentWidth();
    document.documentElement.style.setProperty('--line-height', `${this.lineHeight}px`);
  }

  private applyContentWidth() {
    if (this.wordWrap) {
      const w = this.viewport.clientWidth || 0;
      this.scrollSpacer.style.width = `${w}px`;
      this.contentContainer.style.minWidth = `${w}px`;
      this.contentContainer.style.width = `${w}px`;
      this.contentContainer.style.maxWidth = `${w}px`;
      return;
    }
    const w = Math.max(this.contentWidthPx, this.viewport.clientWidth || 0);
    this.scrollSpacer.style.width = `${w}px`;
    this.contentContainer.style.minWidth = `${w}px`;
    this.contentContainer.style.width = '';
    this.contentContainer.style.maxWidth = '';
  }

  private resetHorizontalExtent() {
    this.contentWidthPx = 0;
    this.viewport.scrollLeft = 0;
    this.applyContentWidth();
  }

  private scheduleMeasureContentWidth() {
    if (this.measureWidthRaf) return;
    this.measureWidthRaf = requestAnimationFrame(() => {
      this.measureWidthRaf = 0;
      this.measureContentWidth();
    });
  }

  private measureContentWidth() {
    if (this.wordWrap) {
      this.contentWidthPx = this.viewport.clientWidth || 0;
      this.applyContentWidth();
      return;
    }
    let maxW = this.viewport.clientWidth || 0;
    for (const node of this.renderedNodes.values()) {
      const sw = node.scrollWidth;
      if (sw > maxW) maxW = sw;
    }
    if (maxW !== this.contentWidthPx) {
      this.contentWidthPx = maxW;
      this.applyContentWidth();
    }
  }

  public setBottomInset(px: number) {
    this.bottomInset = Math.max(0, px);
    this.updateLayout();
  }

  private effectiveClientHeight(): number {
    return Math.max(1, this.viewport.clientHeight - this.bottomInset);
  }

  public setWordWrap(enabled: boolean) {
    if (this.wordWrap === enabled) return;
    this.wordWrap = enabled;
    this.heightCache.clear();
    this.avgHeight = this.lineHeight;
    this.contentContainer.classList.toggle('word-wrap', enabled);
    this.contentWidthPx = 0;
    this.applyContentWidth();
    this.updateLayout();
    this.recalculateVisible();
    this.render(true);
  }

  public getWordWrap(): boolean {
    return this.wordWrap;
  }

  public refreshMetrics() {
    this.heightCache.clear();
    this.avgHeight = this.lineHeight;
    this.measureLineHeight();
    this.measureCharWidth();
    this.updateLayout();
    this.recalculateVisible();
    this.render(true);
  }

  private measureCharWidth() {
    const sample = document.createElement('span');
    sample.className = 'log-text';
    sample.style.position = 'absolute';
    sample.style.visibility = 'hidden';
    sample.style.whiteSpace = 'pre';
    sample.textContent = 'MMMMMMMMMM';
    this.contentContainer.appendChild(sample);
    const w = sample.getBoundingClientRect().width;
    sample.remove();
    if (w > 0) this.charWidthPx = w / 10;
  }

  private textWidthPx(): number {
    const gutter = !document.body.classList.contains('hide-line-numbers') ? 78 : 8;
    return Math.max(40, (this.viewport.clientWidth || 800) - gutter);
  }

  private estimateHeight(content: string): number {
    if (!this.wordWrap) return this.lineHeight;
    const cw = Math.max(4, this.charWidthPx);
    const cols = Math.max(1, Math.floor(this.textWidthPx() / cw));
    const parts = (content || '').split('\n');
    let lines = 0;
    for (const p of parts) {
      lines += Math.max(1, Math.ceil(Math.max(1, p.length) / cols));
    }
    return Math.max(this.lineHeight, lines * this.lineHeight);
  }

  private heightOf(index: number): number {
    if (!this.wordWrap) return this.lineHeight;
    return this.heightCache.get(index) ?? this.avgHeight;
  }

  private rememberHeight(index: number, h: number) {
    const prev = this.heightCache.get(index);
    if (prev === h) return false;
    this.heightCache.set(index, h);
    // Recompute average from a bounded sample of cache values.
    let sum = 0;
    let n = 0;
    for (const v of this.heightCache.values()) {
      sum += v;
      n++;
      if (n >= 200) break;
    }
    if (n > 0) this.avgHeight = Math.max(this.lineHeight, Math.round(sum / n));
    return true;
  }

  private positionRows(firstVisibleIndex: number, fractional: number) {
    if (!this.wordWrap) {
      const fracPx = fractional * this.lineHeight;
      for (const [idx, node] of this.renderedNodes) {
        node.style.top = `${(idx - firstVisibleIndex) * this.lineHeight - fracPx}px`;
        node.style.height = `${this.lineHeight}px`;
        node.style.minHeight = '';
        node.style.overflow = '';
      }
      return;
    }

    const indices = [...this.renderedNodes.keys()].sort((a, b) => a - b);
    if (indices.length === 0) return;

    const minIdx = indices[0];
    let cursor = -fractional * this.heightOf(firstVisibleIndex);
    for (let i = firstVisibleIndex - 1; i >= minIdx; i--) {
      cursor -= this.heightOf(i);
    }

    let expected = minIdx;
    for (const idx of indices) {
      while (expected < idx) {
        cursor += this.heightOf(expected);
        expected++;
      }
      const node = this.renderedNodes.get(idx)!;
      const h = this.heightOf(idx);
      node.style.top = `${cursor}px`;
      node.style.height = 'auto';
      node.style.minHeight = `${h}px`;
      node.style.overflow = 'visible';
      cursor += h;
      expected = idx + 1;
    }
  }

  private refineMeasuredHeights(anchorIndex: number, anchorFrac: number) {
    if (!this.wordWrap) return;
    let changed = false;
    for (const [idx, node] of this.renderedNodes) {
      const h = Math.max(this.lineHeight, Math.round(node.scrollHeight || node.offsetHeight || this.lineHeight));
      if (this.rememberHeight(idx, h)) changed = true;
    }
    if (!changed) return;
    const maxFirst = Math.max(0, this.totalLines - this.visibleCount);
    const exact = Math.min(maxFirst, anchorIndex + anchorFrac);
    const targetFrac = maxFirst <= 0 ? 0 : exact / maxFirst;
    this.updateLayout();
    const maxScroll = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    const desired = targetFrac * maxScroll;
    if (Math.abs(this.viewport.scrollTop - desired) > 1) {
      this.viewport.scrollTop = desired;
    }
    this.positionRows(anchorIndex, anchorFrac);
  }

  public setTotalLines(total: number) {
    if (total === this.totalLines && total > 0) {
      this.updateLayout();
      this.recalculateVisible();
      this.applyPendingAnchor();
      this.render();
      return;
    }
    this.totalLines = total;
    this.heightCache.clear();
    this.avgHeight = this.lineHeight;
    this.measureLineHeight();
    this.measureCharWidth();
    this.resetHorizontalExtent();
    this.updateLayout();
    this.recalculateVisible();
    this.clear();
    // Re-map content anchor → pixels after spacer metrics change.
    this.applyPendingAnchor();
    this.render();
  }

  private highlightTerms: { q: string; cls: string; isRegex?: boolean }[] = [];

  public setSearchQuery(query: string) {
    if (this.currentSearchQuery === query && this.highlightTerms.length === 0) return;
    this.currentSearchQuery = query;
    this.rehighlightVisibleRows();
    this.scheduleMeasureContentWidth();
  }

  /** Extra highlight terms (secondary / nearby / exclude). Empty = primary only. */
  public setHighlightTerms(terms: { q: string; cls: string; isRegex?: boolean }[]) {
    this.highlightTerms = terms || [];
    this.rehighlightVisibleRows();
  }

  private rehighlightVisibleRows() {
    const q = this.currentSearchQuery;
    const terms = this.highlightTerms;
    const termKey = terms.map((t) => t.q + t.cls).join('|');
    for (const [, node] of this.renderedNodes) {
      const raw = this.rowRawContent.get(node);
      if (raw === undefined) continue;
      const textEl = node.children[1] as HTMLElement | undefined;
      if (!textEl) continue;
      const sourceIndex = node.dataset.sourceIndex || '';
      const key = `${sourceIndex}\0${q}\0${termKey}`;
      if (this.rowHighlightKey.get(node) === key) continue;
      if (terms.length > 0) {
        const all = [{ q, cls: 'search-highlight', isRegex: false }, ...terms].filter((t) => t.q);
        textEl.innerHTML = LogHighlighter.highlightLineMulti(raw, all);
      } else {
        textEl.innerHTML = LogHighlighter.highlightLine(raw, q);
      }
      this.rowHighlightKey.set(node, key);
    }
  }

  public setHighlightedLine(index: number | null) {
    this.highlightedLineIndex = index;
    this.applyHighlighting();
    this.applySelectionStyle();
  }

  public setOnRowClick(cb: (index: number) => void) {
    this.onRowClickCallback = cb;
  }

  public setOnScrollPause(cb: () => void) {
    this.onScrollPauseCallback = cb;
  }

  public setAutoScroll(_enabled: boolean) {
    // host-owned
  }

  /** Current first-visible line + fractional progress within that line. */
  public getViewAnchor(): { index: number; fraction: number } {
    const w = this.windowFromScrollTop(this.viewport.scrollTop || 0);
    return { index: w.firstVisibleIndex, fraction: w.fractional };
  }

  /**
   * Restore view so `index + fraction` is at the top of the viewport.
   * Survives setTotalLines / word-wrap height changes (re-applied via pendingAnchor).
   */
  public setViewAnchor(index: number, fraction = 0) {
    const frac = Math.max(0, Math.min(1, fraction || 0));
    const idx = Math.max(0, Math.floor(index || 0));
    this.pendingAnchor = { index: idx, fraction: frac };
    this.applyPendingAnchor();
    this.render();
  }

  public setOnScrollPosition(cb: (anchor: { index: number; fraction: number }) => void) {
    this.onScrollPositionCallback = cb;
  }

  /** Map a content anchor to pixel scrollTop for the current spacer metrics. */
  private scrollTopForAnchor(index: number, fraction: number): number {
    const maxScroll = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    if (maxScroll <= 0) return 0;
    if (this.wordWrap) {
      let acc = 0;
      const idx = Math.max(0, Math.min(this.totalLines - 1, Math.floor(index)));
      for (let i = 0; i < idx; i++) acc += this.heightOf(i);
      acc += Math.max(0, Math.min(1, fraction)) * this.heightOf(idx);
      return Math.max(0, Math.min(maxScroll, acc));
    }
    const maxFirst = Math.max(0, this.totalLines - this.visibleCount);
    if (maxFirst <= 0) return 0;
    const exact = Math.min(maxFirst, Math.max(0, index + fraction));
    const targetFrac = exact / maxFirst;
    return targetFrac * maxScroll;
  }

  private applyPendingAnchor() {
    if (!this.pendingAnchor) return;
    const y = this.scrollTopForAnchor(this.pendingAnchor.index, this.pendingAnchor.fraction);
    this.pendingScrollTop = y;
    this.applyPendingScroll();
  }

  private applyPendingScroll() {
    if (this.pendingScrollTop === null) return;
    const maxScroll = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    const y = Math.max(0, Math.min(maxScroll, this.pendingScrollTop));
    if (Math.abs((this.viewport.scrollTop || 0) - y) > 0.5) {
      this.suppressScrollEvent = true;
      this.viewport.scrollTop = y;
      queueMicrotask(() => { this.suppressScrollEvent = false; });
    }
  }

  /** @deprecated pixel API — prefer getViewAnchor / setViewAnchor */
  public getScrollTop(): number {
    return this.viewport.scrollTop || 0;
  }

  public setScrollTop(px: number) {
    this.pendingAnchor = null;
    this.pendingScrollTop = Math.max(0, px || 0);
    this.applyPendingScroll();
    this.render();
  }

  public scrollToBottom() {
    if (this.totalLines > 0) {
      this.viewport.scrollTop = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    }
  }

  public scrollToIndex(lineIndex: number) {
    if (lineIndex < 0 || lineIndex >= this.totalLines) return;
    const maxScroll = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    let y: number;
    if (this.wordWrap) {
      // Approximate cumulative height — heightCache / avgHeight (no full force-recycle).
      let acc = 0;
      const target = Math.max(0, lineIndex);
      for (let i = 0; i < target; i++) acc += this.heightOf(i);
      y = acc - this.effectiveClientHeight() * 0.35;
    } else {
      const maxFirst = Math.max(1, this.totalLines - this.visibleCount);
      const targetFirst = Math.max(0, Math.min(maxFirst, lineIndex - Math.floor(this.visibleCount * 0.4)));
      const frac = maxFirst > 0 ? targetFirst / maxFirst : 0;
      y = frac * maxScroll;
    }
    this.suppressScrollEvent = true;
    this.viewport.scrollTop = Math.max(0, Math.min(maxScroll, y));
    queueMicrotask(() => { this.suppressScrollEvent = false; });
    this.setHighlightedLine(lineIndex);
    // Soft render — keep existing rows to avoid flicker
    this.render(false);
  }

  private windowFromScrollTop(scrollTop: number) {
    const maxFirst = Math.max(0, this.totalLines - this.visibleCount);
    const exact = this.fractionFromScrollTop(scrollTop) * maxFirst;
    const firstVisibleIndex = Math.floor(exact);
    const fractional = exact - firstVisibleIndex;
    return { firstVisibleIndex, fractional };
  }

  /** Mount enough rows to cover the viewport in pixels (wrap-aware). */
  private indexWindow(firstVisibleIndex: number, fractional: number): { startIdx: number; endIdx: number } {
    const vh = this.effectiveClientHeight();
    const bufPx = this.wordWrap ? Math.max(vh, 400) : this.bufferCount * this.lineHeight;

    if (!this.wordWrap) {
      return {
        startIdx: Math.max(0, firstVisibleIndex - this.bufferCount),
        endIdx: Math.min(this.totalLines - 1, firstVisibleIndex + this.visibleCount + this.bufferCount * 2),
      };
    }

    let covered = -fractional * this.heightOf(firstVisibleIndex);
    let end = firstVisibleIndex;
    while (end < this.totalLines && covered < vh + bufPx) {
      covered += this.heightOf(end);
      end++;
    }

    let start = firstVisibleIndex;
    let up = fractional * this.heightOf(firstVisibleIndex);
    while (start > 0 && up < bufPx) {
      start--;
      up += this.heightOf(start);
    }

    const maxMount = Math.max(120, this.visibleCount + this.bufferCount * 6);
    if (end - start > maxMount) {
      end = Math.min(this.totalLines, start + maxMount);
    }
    return {
      startIdx: Math.max(0, start),
      endIdx: Math.min(this.totalLines - 1, Math.max(start, end - 1)),
    };
  }

  public render(force = false) {
    if (this.totalLines === 0) {
      this.clear();
      return;
    }

    this.recalculateVisible();
    this.currentScrollTop = this.viewport.scrollTop;
    const { firstVisibleIndex, fractional } = this.windowFromScrollTop(this.viewport.scrollTop);
    const { startIdx, endIdx } = this.indexWindow(firstVisibleIndex, fractional);

    const gen = ++this.generation;

    for (const [idx, node] of this.renderedNodes.entries()) {
      if (force || idx < startIdx || idx > endIdx) {
        this.recycleRowNode(idx, node);
      }
    }
    this.positionRows(firstVisibleIndex, fractional);

    if (this.fetchInFlight) {
      this.pendingRender = true;
      return;
    }

    const missingIndices: number[] = [];
    for (let i = startIdx; i <= endIdx; i++) {
      if (!this.renderedNodes.has(i)) missingIndices.push(i);
    }
    if (missingIndices.length === 0) {
      this.applyPendingAnchor();
      const w = this.windowFromScrollTop(this.viewport.scrollTop);
      this.positionRows(w.firstVisibleIndex, w.fractional);
      this.applyHighlighting();
      this.applySelectionStyle();
      this.scheduleMeasureContentWidth();
      return;
    }

    const fetchStart = missingIndices[0];
    const fetchCount = missingIndices[missingIndices.length - 1] - fetchStart + 1;

    this.fetchInFlight = true;
    this.fetchCallback(fetchStart, fetchCount).then((lines) => {
      if (gen !== this.generation) return;

      const fragment = document.createDocumentFragment();
      const win = this.windowFromScrollTop(this.viewport.scrollTop);
      const { startIdx: startIdx2, endIdx: endIdx2 } = this.indexWindow(win.firstVisibleIndex, win.fractional);

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        const logicalIndex = fetchStart + i;
        if (logicalIndex < startIdx2 || logicalIndex > endIdx2) continue;
        if (this.renderedNodes.has(logicalIndex)) continue;

        const rowNode = this.createRowNode();
        const h = this.wordWrap ? this.estimateHeight(line.content ?? '') : this.lineHeight;
        if (this.wordWrap) this.rememberHeight(logicalIndex, h);

        const unit = this.wordWrap ? this.avgHeight : this.lineHeight;
        const rowTop = (logicalIndex - win.firstVisibleIndex) * unit - win.fractional * unit;
        rowNode.style.top = `${rowTop}px`;
        rowNode.style.height = this.wordWrap ? 'auto' : `${this.lineHeight}px`;
        rowNode.style.minHeight = `${h}px`;
        rowNode.style.overflow = this.wordWrap ? 'visible' : '';

        const numEl = rowNode.children[0] as HTMLElement;
        const textEl = rowNode.children[1] as HTMLElement;
        const sourceIndex = line.index;
        numEl.textContent = String(sourceIndex + 1);
        // Logical index in this scroller (match index in filter pane, line index in main).
        rowNode.dataset.index = String(logicalIndex);
        rowNode.dataset.sourceIndex = String(sourceIndex);
        // Only explicit false marks context neighbors (Go zero-value must not dim main log).
        const isContext = (line as any).is_match === false || (line as any).isMatch === false;
        rowNode.classList.toggle('context-line', isContext);

        const raw = line.content ?? '';
        this.rowRawContent.set(rowNode, raw);
        const hlKey = `${sourceIndex}\0${this.currentSearchQuery}`;
        if (this.highlightTerms.length > 0) {
          const all = [{ q: this.currentSearchQuery, cls: 'search-highlight', isRegex: false }, ...this.highlightTerms].filter((t) => t.q);
          textEl.innerHTML = LogHighlighter.highlightLineMulti(raw, all);
        } else {
          textEl.innerHTML = LogHighlighter.highlightLine(raw, this.currentSearchQuery);
        }
        this.rowHighlightKey.set(rowNode, hlKey);

        rowNode.onclick = (e) => {
          // Modifier clicks are handled by attachSelectionListeners (range / toggle).
          if (e.shiftKey || e.ctrlKey || e.metaKey) return;
          this.onRowClickCallback?.(logicalIndex);
        };

        this.renderedNodes.set(logicalIndex, rowNode);
        fragment.appendChild(rowNode);
      }

      this.contentContainer.appendChild(fragment);
      this.applyPendingAnchor();
      const win2 = this.windowFromScrollTop(this.viewport.scrollTop);
      this.positionRows(win2.firstVisibleIndex, win2.fractional);

      if (this.wordWrap) {
        const anchorIdx = win.firstVisibleIndex;
        const anchorFrac = win.fractional;
        requestAnimationFrame(() => {
          if (gen !== this.generation) return;
          this.refineMeasuredHeights(anchorIdx, anchorFrac);
        });
      }

      this.applyHighlighting();
      this.applySelectionStyle();
      this.scheduleMeasureContentWidth();
    }).catch((err) => {
      if (gen === this.generation) {
        console.error('Failed to fetch log lines for virtual render:', err);
        showToast(String((err as any)?.message ?? err), 'error');
      }
    }).finally(() => {
      this.fetchInFlight = false;
      if (this.pendingRender) {
        this.pendingRender = false;
        this.render();
      }
    });
  }

  private recalculateVisible() {
    const unit = this.wordWrap ? Math.max(this.lineHeight, this.avgHeight) : this.lineHeight;
    this.visibleCount = Math.max(1, Math.floor(this.effectiveClientHeight() / unit));
  }

  private measureLineHeight() {
    const sample = document.createElement('div');
    sample.className = 'log-row';
    sample.style.position = 'absolute';
    sample.style.visibility = 'hidden';
    sample.style.left = '-9999px';
    sample.style.top = '0';
    sample.style.height = 'auto';
    sample.style.lineHeight = 'normal';
    sample.innerHTML = '<div class="log-num">1</div><div class="log-text">M</div>';
    this.contentContainer.appendChild(sample);
    const height = sample.getBoundingClientRect().height;
    sample.remove();
    if (height > 0) {
      this.lineHeight = Math.round(height);
      document.documentElement.style.setProperty('--line-height', `${this.lineHeight}px`);
    }
  }

  private onResize() {
    this.measureLineHeight();
    this.measureCharWidth();
    if (this.wordWrap) this.heightCache.clear();
    this.updateLayout();
    this.recalculateVisible();
    this.render(true);
  }

  private onScroll() {
    const maxScroll = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    const isAtBottom = maxScroll <= 0 || this.viewport.scrollTop >= maxScroll - 2;
    if (!isAtBottom && this.onScrollPauseCallback) {
      this.onScrollPauseCallback();
    }
    if (this.suppressScrollEvent) return;
    // User-driven scroll — drop pending restore and report content anchor.
    this.pendingScrollTop = null;
    this.pendingAnchor = null;
    if (this.onScrollPositionCallback) {
      if (this.scrollPosRaf) cancelAnimationFrame(this.scrollPosRaf);
      this.scrollPosRaf = requestAnimationFrame(() => {
        this.scrollPosRaf = 0;
        this.onScrollPositionCallback?.(this.getViewAnchor());
      });
    }
    if (!this.isRequestPending) {
      this.isRequestPending = true;
      requestAnimationFrame(() => {
        this.render();
        this.isRequestPending = false;
      });
    }
  }

  private createRowNode(): HTMLElement {
    if (this.poolNodes.length > 0) {
      return this.poolNodes.pop()!;
    }
    const row = document.createElement('div');
    row.className = 'log-row';
    const numEl = document.createElement('div');
    numEl.className = 'log-num';
    const textEl = document.createElement('div');
    textEl.className = 'log-text';
    row.appendChild(numEl);
    row.appendChild(textEl);
    return row;
  }

  private recycleRowNode(idx: number, node: HTMLElement) {
    node.remove();
    this.renderedNodes.delete(idx);
    this.poolNodes.push(node);
  }

  private clear() {
    this.heightCache.clear();
    for (const [idx, node] of this.renderedNodes) {
      this.recycleRowNode(idx, node);
    }
    this.contentContainer.innerHTML = '';
    this.resetHorizontalExtent();
  }

  private applyHighlighting() {
    for (const [idx, node] of this.renderedNodes) {
      if (this.highlightedLineIndex !== null && idx === this.highlightedLineIndex) {
        node.classList.add('highlighted');
      } else {
        node.classList.remove('highlighted');
      }
    }
  }

  /** Sorted logical indices currently selected (multi-select or contiguous range). */
  public getSelectedIndices(): number[] {
    if (this.selectedLines.size > 0) {
      return Array.from(this.selectedLines).sort((a, b) => a - b);
    }
    if (this.selectionStart === null || this.selectionEnd === null) return [];
    const start = Math.min(this.selectionStart, this.selectionEnd);
    const end = Math.max(this.selectionStart, this.selectionEnd);
    const out: number[] = [];
    for (let i = start; i <= end; i++) out.push(i);
    return out;
  }

  public getSelectedRange(): { start: number; end: number } | null {
    const idxs = this.getSelectedIndices();
    if (idxs.length === 0) return null;
    return { start: idxs[0], end: idxs[idxs.length - 1] };
  }

  public clearSelection() {
    const had =
      this.selectedLines.size > 0 ||
      this.selectionStart !== null ||
      this.selectionEnd !== null;
    this.selectedLines.clear();
    this.selectionStart = null;
    this.selectionEnd = null;
    this.selectionAnchor = null;
    if (had) this.applySelectionStyle();
  }

  /** Move a contiguous range into the multi-select set (clears range fields). */
  private promoteRangeToSet() {
    if (this.selectionStart === null || this.selectionEnd === null) return;
    const lo = Math.min(this.selectionStart, this.selectionEnd);
    const hi = Math.max(this.selectionStart, this.selectionEnd);
    for (let i = lo; i <= hi; i++) this.selectedLines.add(i);
    this.selectionStart = null;
    this.selectionEnd = null;
  }

  /** Replace multi-select with a contiguous range (also used by context menu). */
  private setRangeSelection(start: number, end: number) {
    this.selectedLines.clear();
    this.ctrlAddBase = null;
    this.ctrlAddStart = null;
    this.ctrlAddMoved = false;
    const lo = Math.max(0, Math.min(start, this.totalLines - 1));
    const hi = Math.max(0, Math.min(end, this.totalLines - 1));
    this.selectionStart = lo;
    this.selectionEnd = hi;
    this.selectionAnchor = lo;
  }

  /** Snapshot current selection into a Set (range and/or multi). */
  private snapshotSelection(): Set<number> {
    const snap = new Set<number>(this.selectedLines);
    if (this.selectionStart !== null && this.selectionEnd !== null) {
      const lo = Math.min(this.selectionStart, this.selectionEnd);
      const hi = Math.max(this.selectionStart, this.selectionEnd);
      for (let i = lo; i <= hi; i++) snap.add(i);
    }
    return snap;
  }

  /** Add a contiguous range onto a base set → becomes the active multi-select. */
  private applyCtrlAddRange(base: Set<number>, start: number, end: number) {
    const lo = Math.max(0, Math.min(start, end, this.totalLines - 1));
    const hi = Math.min(
      this.totalLines - 1,
      Math.max(0, start, end),
    );
    const next = new Set<number>(base);
    for (let i = lo; i <= hi; i++) next.add(i);
    this.selectedLines = next;
    this.selectionStart = null;
    this.selectionEnd = null;
    this.selectionAnchor = end;
  }

  /** Toggle a single line in the multi-select set (Ctrl+click without drag). */
  private toggleLineInSelection(idx: number) {
    this.promoteRangeToSet();
    if (this.selectedLines.has(idx)) {
      this.selectedLines.delete(idx);
    } else {
      this.selectedLines.add(idx);
    }
    this.selectionAnchor = idx;
    if (this.selectedLines.size === 0) {
      this.selectionAnchor = null;
    }
  }

  private async selectionLineTexts(): Promise<{ texts: string[]; indices: number[] } | null> {
    const indices = this.getSelectedIndices();
    if (indices.length === 0) return null;

    // Fetch contiguous spans to minimize round-trips when the set is sparse.
    const parts = new Map<number, string>();
    let i = 0;
    while (i < indices.length) {
      const spanStart = indices[i];
      let spanEnd = spanStart;
      while (i + 1 < indices.length && indices[i + 1] === spanEnd + 1) {
        i++;
        spanEnd = indices[i];
      }
      const count = spanEnd - spanStart + 1;
      const lines = await this.fetchCallback(spanStart, count);
      for (let j = 0; j < lines.length && j < count; j++) {
        parts.set(spanStart + j, lines[j]?.content ?? '');
      }
      i++;
    }

    // Prefer live DOM text for visible rows (already highlighted/sanitized).
    for (const idx of indices) {
      const node = this.renderedNodes.get(idx);
      if (!node) continue;
      const textEl = node.querySelector('.log-text') as HTMLElement | null;
      const domText = textEl?.textContent;
      if (domText != null && domText.length > 0) {
        parts.set(idx, domText);
      }
    }

    return {
      indices,
      texts: indices.map((idx) => parts.get(idx) ?? ''),
    };
  }

  public async copySelection(): Promise<boolean> {
    try {
      const data = await this.selectionLineTexts();
      if (!data) return false;
      const text = data.texts.join('\n');
      if (!text) return false;
      await navigator.clipboard.writeText(text);
      return true;
    } catch (err) {
      console.error('copySelection failed:', err);
      return false;
    }
  }

  public async copySelectionNumbered(): Promise<boolean> {
    try {
      const data = await this.selectionLineTexts();
      if (!data) return false;
      const text = data.texts
        .map((line, i) => `${data.indices[i] + 1}: ${line}`)
        .join('\n');
      if (!text) return false;
      await navigator.clipboard.writeText(text);
      return true;
    } catch (err) {
      console.error('copySelectionNumbered failed:', err);
      return false;
    }
  }

  public setSelection(start: number, end: number) {
    this.setRangeSelection(start, end);
    this.applySelectionStyle();
  }

  public extendSelection(end: number) {
    if (this.selectionAnchor === null && this.selectedLines.size === 0) {
      this.setSelection(end, end);
      return;
    }
    // Contiguous extend from anchor; drop multi-set in favour of a range.
    const anchor = this.selectionAnchor ?? end;
    this.setRangeSelection(anchor, end);
    this.selectionAnchor = anchor;
    this.applySelectionStyle();
  }

  private applySelectionStyle() {
    const selected = new Set(this.getSelectedIndices());
    for (const [idx, node] of this.renderedNodes) {
      if (selected.has(idx)) {
        node.classList.add('selected');
      } else {
        node.classList.remove('selected');
      }
    }
  }

  private indexFromPoint(clientY: number): number | null {
    for (const [idx, node] of this.renderedNodes) {
      const r = node.getBoundingClientRect();
      if (clientY >= r.top && clientY <= r.bottom) return idx;
    }
    // nearest
    let best: number | null = null;
    let bestDist = Infinity;
    for (const [idx, node] of this.renderedNodes) {
      const r = node.getBoundingClientRect();
      const mid = (r.top + r.bottom) / 2;
      const d = Math.abs(clientY - mid);
      if (d < bestDist) {
        bestDist = d;
        best = idx;
      }
    }
    return best;
  }

  private attachSelectionListeners() {
    this.viewport.addEventListener('mousedown', (e) => {
      if (e.button !== 0) return;
      const target = e.target as HTMLElement;
      if (!target.closest('.log-row')) return;

      const inGutter = !!target.closest('.log-num');
      const withShift = e.shiftKey;
      const withCtrl = e.ctrlKey || e.metaKey;
      const idx = this.indexFromPoint(e.clientY);
      if (idx === null) return;

      this.viewport.focus();

      // Ctrl/Cmd+click or drag: add line/range to the existing selection.
      if (withCtrl && !withShift) {
        e.preventDefault();
        this.pendingLineSelect = null;
        this.ctrlAddBase = this.snapshotSelection();
        this.ctrlAddStart = idx;
        this.ctrlAddMoved = false;
        this.isSelecting = true; // drag will extend the added range
        this.selectionAnchor = idx;
        // Preview: add the start line immediately.
        this.applyCtrlAddRange(this.ctrlAddBase, idx, idx);
        this.applySelectionStyle();
        return;
      }

      // Shift+click: extend contiguous range from anchor / highlighted line.
      if (withShift) {
        e.preventDefault();
        this.pendingLineSelect = null;
        this.ctrlAddBase = null;
        this.ctrlAddStart = null;
        this.isSelecting = false;
        const anchor =
          this.selectionAnchor !== null
            ? this.selectionAnchor
            : this.highlightedLineIndex !== null
              ? this.highlightedLineIndex
              : idx;
        this.setRangeSelection(anchor, idx);
        this.selectionAnchor = anchor;
        this.applySelectionStyle();
        return;
      }

      // Gutter click: start line-range selection immediately; drag extends.
      if (inGutter) {
        e.preventDefault();
        this.pendingLineSelect = null;
        this.ctrlAddBase = null;
        this.ctrlAddStart = null;
        this.isSelecting = true;
        this.setRangeSelection(idx, idx);
        this.applySelectionStyle();
        return;
      }

      // Plain mousedown on text: keep native character selection for a
      // single-row drag; promote to line-range selection once the pointer
      // moves onto another row (or a clear vertical drag).
      this.ctrlAddBase = null;
      this.ctrlAddStart = null;
      this.pendingLineSelect = { startIdx: idx, startY: e.clientY };
      this.isSelecting = false;
    });

    window.addEventListener('mousemove', (e) => {
      // Promote pending text-drag into multi-line selection.
      if (this.pendingLineSelect && !this.isSelecting) {
        const idx = this.indexFromPoint(e.clientY);
        const dy = Math.abs(e.clientY - this.pendingLineSelect.startY);
        const leftStartRow =
          idx !== null && idx !== this.pendingLineSelect.startIdx;
        // ~half a line height — enough to mean "select lines", not "select chars".
        if (leftStartRow || dy >= Math.max(8, this.lineHeight * 0.45)) {
          const startIdx = this.pendingLineSelect.startIdx;
          this.pendingLineSelect = null;
          // Clear any native character selection that started on the first row.
          const native = window.getSelection();
          if (native && !native.isCollapsed) native.removeAllRanges();
          this.isSelecting = true;
          this.setRangeSelection(startIdx, idx ?? startIdx);
          this.applySelectionStyle();
        }
        return;
      }

      if (!this.isSelecting) return;
      const idx = this.indexFromPoint(e.clientY);
      if (idx === null) return;

      // Ctrl/Cmd drag: union base selection with the drag range.
      if (this.ctrlAddBase !== null && this.ctrlAddStart !== null) {
        this.ctrlAddMoved = true;
        this.applyCtrlAddRange(this.ctrlAddBase, this.ctrlAddStart, idx);
        this.applySelectionStyle();
        return;
      }

      // Plain drag-extend: contiguous range (replaces multi-set).
      const anchor = this.selectionAnchor ?? idx;
      this.setRangeSelection(anchor, idx);
      this.selectionAnchor = anchor;
      this.applySelectionStyle();
    });

    window.addEventListener('mouseup', (e) => {
      // Ctrl/Cmd click without drag: toggle the line on the base set.
      if (this.ctrlAddBase !== null && this.ctrlAddStart !== null && !this.ctrlAddMoved) {
        const idx = this.ctrlAddStart;
        const base = this.ctrlAddBase;
        this.selectedLines = new Set(base);
        this.selectionStart = null;
        this.selectionEnd = null;
        if (this.selectedLines.has(idx)) {
          this.selectedLines.delete(idx);
        } else {
          this.selectedLines.add(idx);
        }
        this.selectionAnchor = idx;
        if (this.selectedLines.size === 0) this.selectionAnchor = null;
        this.applySelectionStyle();
      }

      // Plain click (no drag) on a log line → select that one line so Ctrl+C
      // and the context menu have a range. Horizontal char-selection drag
      // that stayed on one row leaves pendingLineSelect set until here without
      // promoting — do not overwrite an intentional native text selection.
      if (this.pendingLineSelect && !this.isSelecting) {
        const idx = this.pendingLineSelect.startIdx;
        const dy = Math.abs(e.clientY - this.pendingLineSelect.startY);
        const native = window.getSelection();
        const hasNative = !!(native && !native.isCollapsed && (native.toString()?.length ?? 0) > 0);
        // Click without meaningful native selection → select the line.
        if (!hasNative && dy < Math.max(8, this.lineHeight * 0.45)) {
          this.setRangeSelection(idx, idx);
          this.applySelectionStyle();
        }
      }
      this.pendingLineSelect = null;
      this.ctrlAddBase = null;
      this.ctrlAddStart = null;
      this.ctrlAddMoved = false;
      this.isSelecting = false;
    });
  }
}
