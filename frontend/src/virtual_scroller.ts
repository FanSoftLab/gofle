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
  private bufferCount = 15;
  private fetchCallback: FetchLinesCallback;

  private scrollSpacerHeight = 0;
  // Max measured content width of currently rendered rows — drives horizontal
  // scroll so lines longer than the viewport are reachable.
  private contentWidthPx = 0;
  // Height (px) of a panel overlaid on the bottom of the viewport (e.g. the
  // status bar) that would otherwise cover the last visible row. The rows
  // layer is shrunk by this so rows never sit behind it.
  private bottomInset = 0;
  private static readonly MAX_SAFE_PIXELS = 15_000_000;

  // IPC coalescing: at most one fetch is in flight; a further render() request
  // while one is pending is collapsed into a single retry.
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

  // Selection state
  private selectionStart: number | null = null;
  private selectionEnd: number | null = null;
  private isSelecting = false;
  private selectionAnchor: number | null = null;

  constructor(viewportId: string, fetchCallback: FetchLinesCallback) {
    this.viewport = document.getElementById(viewportId)!;
    this.fetchCallback = fetchCallback;
    // Keyboard focus target for Shift+Arrow selection and Ctrl+C.
    if (this.viewport.tabIndex < 0) this.viewport.tabIndex = 0;

    this.contentContainer = document.createElement('div');
    this.contentContainer.className = 'log-content-container';
    this.viewport.appendChild(this.contentContainer);

    this.scrollSpacer = document.createElement('div');
    this.scrollSpacer.className = 'log-scroll-spacer';
    this.viewport.appendChild(this.scrollSpacer);

    this.viewport.addEventListener('scroll', () => this.onScroll(), { passive: true });
    window.addEventListener('resize', () => this.onResize());

    this.updateLayout();
    this.recalculateVisible();
    this.attachSelectionListeners();
  }

  // Maps the native scroll position (0..spacerHeight) to a 0..1 fraction of
  // the file. Using a fraction (instead of absolute pixels) keeps the scroll
  // math independent of the pixel cap, so every line stays reachable.
  private fractionFromScrollTop(scrollTop: number): number {
    const maxScroll = this.scrollSpacerHeight - this.effectiveClientHeight();
    if (maxScroll <= 0) return 0;
    return Math.max(0, Math.min(1, scrollTop / maxScroll));
  }

  private updateLayout() {
    const full = this.totalLines * this.lineHeight;
    this.scrollSpacerHeight = Math.min(full, VirtualScroller.MAX_SAFE_PIXELS);
    // Spacer provides the scroll range; rows layer is pinned via sticky and
    // sized to the viewport minus the bottom inset so its rows stay at small
    // coordinates and never sit behind an overlaid panel.
    this.scrollSpacer.style.height = `${Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight())}px`;
    this.contentContainer.style.height = `${this.effectiveClientHeight()}px`;
    this.applyContentWidth();
    document.documentElement.style.setProperty('--line-height', `${this.lineHeight}px`);
  }

  /** Expand spacer/content min-width so the viewport can scroll horizontally. */
  private applyContentWidth() {
    const w = Math.max(this.contentWidthPx, this.viewport.clientWidth || 0);
    this.scrollSpacer.style.width = `${w}px`;
    this.contentContainer.style.minWidth = `${w}px`;
  }

  /** Reset horizontal extent (call on tab switch / clear). */
  private resetHorizontalExtent() {
    this.contentWidthPx = 0;
    this.viewport.scrollLeft = 0;
    this.applyContentWidth();
  }

  /** Measure visible rows and update the horizontal scroll extent. */
  private measureContentWidth() {
    let maxW = this.viewport.clientWidth || 0;
    for (const node of this.renderedNodes.values()) {
      const sw = node.scrollWidth;
      if (sw > maxW) maxW = sw;
    }
    // Always apply: tab switches must shrink as well as grow the scroll range.
    if (maxW !== this.contentWidthPx) {
      this.contentWidthPx = maxW;
      this.applyContentWidth();
    } else {
      this.applyContentWidth();
    }
  }

  public setBottomInset(px: number) {
    this.bottomInset = Math.max(0, px);
    this.updateLayout();
  }

  // Usable height of the rows layer: the viewport height minus any overlaid
  // panel at the bottom (e.g. the status bar). The content layer, the visible
  // row count and the scroll range all derive from this so the bottom rows are
  // never clipped or pushed out of reach.
  private effectiveClientHeight(): number {
    return Math.max(1, this.viewport.clientHeight - this.bottomInset);
  }

  public setTotalLines(total: number) {
    this.totalLines = total;
    this.measureLineHeight();
    // Drop previous tab's width/scroll before loading new rows.
    this.resetHorizontalExtent();
    this.updateLayout();
    this.clear();
    this.render();
  }

  public setSearchQuery(query: string) {
    this.currentSearchQuery = query;
    // Query change rewrites row HTML widths — remeasure after paint.
    this.render(true);
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
    // Auto-scroll state is owned by the host; nothing to track here.
  }

  public scrollToBottom() {
    if (this.totalLines > 0) {
      this.viewport.scrollTop = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    }
  }

  public scrollToIndex(lineIndex: number) {
    if (lineIndex < 0 || lineIndex >= this.totalLines) return;
    const maxFirst = Math.max(1, this.totalLines - this.visibleCount);
    const targetFirst = Math.max(0, Math.min(maxFirst, lineIndex - Math.floor(this.visibleCount * 0.4)));
    const frac = targetFirst / maxFirst;
    const maxScroll = Math.max(0, this.scrollSpacerHeight - this.effectiveClientHeight());
    this.viewport.scrollTop = Math.max(0, Math.min(maxScroll, frac * maxScroll));
    this.setHighlightedLine(lineIndex);
    this.render(true);
  }

  private windowFromScrollTop(scrollTop: number) {
    const maxFirst = Math.max(0, this.totalLines - this.visibleCount);
    const exact = this.fractionFromScrollTop(scrollTop) * maxFirst;
    const firstVisibleIndex = Math.floor(exact);
    const fractional = exact - firstVisibleIndex;
    return { firstVisibleIndex, fractional };
  }

  public render(force = false) {
    if (this.totalLines === 0) {
      this.clear();
      return;
    }

    this.recalculateVisible();
    this.currentScrollTop = this.viewport.scrollTop;
    const { firstVisibleIndex, fractional } = this.windowFromScrollTop(this.viewport.scrollTop);
    const startIdx = Math.max(0, firstVisibleIndex - this.bufferCount);
    const endIdx = Math.min(this.totalLines - 1, firstVisibleIndex + this.visibleCount + this.bufferCount * 2);

    const gen = ++this.generation;

    // Recycle rows that left the window, and reposition the rows that remain so
    // they track the current scroll position. Repositioning must run on EVERY
    // scroll pass (even while a fetch is in flight) — otherwise kept rows keep
    // their old offset and the view flickers as you wheel-scroll.
    for (const [idx, node] of this.renderedNodes.entries()) {
      if (force || idx < startIdx || idx > endIdx) {
        this.recycleRowNode(idx, node);
      } else {
        const rowTop = (idx - firstVisibleIndex) * this.lineHeight - fractional * this.lineHeight;
        node.style.top = `${rowTop}px`;
        node.style.height = `${this.lineHeight}px`;
      }
    }

    if (this.fetchInFlight) {
      this.pendingRender = true;
      return;
    }

    const missingIndices: number[] = [];
    for (let i = startIdx; i <= endIdx; i++) {
      if (!this.renderedNodes.has(i)) missingIndices.push(i);
    }
    if (missingIndices.length === 0) {
      this.applyHighlighting();
      this.applySelectionStyle();
      // Rows were only repositioned — still refresh horizontal extent (e.g. after
      // a tab switch that recycled everything then hit a warm cache path).
      requestAnimationFrame(() => this.measureContentWidth());
      return;
    }

    const fetchStart = missingIndices[0];
    const fetchCount = missingIndices[missingIndices.length - 1] - fetchStart + 1;

    this.fetchInFlight = true;
    this.fetchCallback(fetchStart, fetchCount).then((lines) => {
      if (gen !== this.generation) return;
      const fragment = document.createDocumentFragment();

      const win = this.windowFromScrollTop(this.viewport.scrollTop);

      const startIdx2 = Math.max(0, win.firstVisibleIndex - this.bufferCount);
      const endIdx2 = Math.min(this.totalLines - 1, win.firstVisibleIndex + this.visibleCount + this.bufferCount * 2);

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        // The fetch callback returns lines for the logical range
        // [fetchStart, fetchStart+count) in order, so the i-th returned line
        // occupies the logical (virtual) index fetchStart + i. For the main log
        // this equals the global line number; for the filter panel it equals
        // the match index. We must position/recycle by the logical index, not
        // by line.index (which for filtered results is the *source* line and
        // would otherwise push rows far outside the scroll area).
        const logicalIndex = fetchStart + i;
        if (logicalIndex < startIdx2 || logicalIndex > endIdx2) continue;
        if (this.renderedNodes.has(logicalIndex)) continue;

        const rowNode = this.createRowNode();
        // Small coordinates relative to the pinned rows layer: the first
        // visible line sits at the top (minus a partial-line offset), so rows
        // never exceed the viewport height and avoid the browser pixel cap.
        const rowTop = (logicalIndex - win.firstVisibleIndex) * this.lineHeight - win.fractional * this.lineHeight;
        rowNode.style.top = `${rowTop}px`;
        rowNode.style.height = `${this.lineHeight}px`;

        if (this.highlightedLineIndex === logicalIndex) {
          rowNode.classList.add('highlighted');
        } else {
          rowNode.classList.remove('highlighted');
        }

        const numEl = rowNode.children[0] as HTMLElement;
        const textEl = rowNode.children[1] as HTMLElement;

        numEl.textContent = (line.index + 1).toString();
        textEl.innerHTML = LogHighlighter.highlightLine(line.content, this.currentSearchQuery);

        // Logical index for selection / context menu (NOT line.index — on the
        // filter pane line.index is the source log line, while the virtual
        // position is the match ordinal).
        rowNode.dataset.index = String(logicalIndex);

        const range = this.getSelectedRange();
        if (range && logicalIndex >= range.start && logicalIndex <= range.end) {
          rowNode.classList.add('selected');
        } else {
          rowNode.classList.remove('selected');
        }

        rowNode.onclick = () => {
          this.highlightedLineIndex = logicalIndex;
          this.applyHighlighting();
          this.applySelectionStyle();
          if (this.onRowClickCallback) this.onRowClickCallback(line.index);
        };

        this.renderedNodes.set(logicalIndex, rowNode);
        fragment.appendChild(rowNode);
      }
      this.contentContainer.appendChild(fragment);
      this.applyHighlighting();
      this.applySelectionStyle();
      // After rows are in the DOM, expand horizontal scroll for long lines.
      requestAnimationFrame(() => this.measureContentWidth());
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
    this.visibleCount = Math.floor(this.effectiveClientHeight() / this.lineHeight);
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

  // Selection API
  public getSelectedRange(): { start: number; end: number } | null {
    if (this.selectionStart === null || this.selectionEnd === null) return null;
    const start = Math.min(this.selectionStart, this.selectionEnd);
    const end = Math.max(this.selectionStart, this.selectionEnd);
    return { start, end };
  }

  public clearSelection() {
    if (this.selectionStart !== null || this.selectionEnd !== null) {
      this.selectionStart = null;
      this.selectionEnd = null;
      this.selectionAnchor = null;
      this.applySelectionStyle();
    }
  }

  /** Copy the current line selection to the clipboard. Returns true if anything was copied. */
  public async copySelection(): Promise<boolean> {
    const range = this.getSelectedRange();
    if (!range) return false;

    const count = range.end - range.start + 1;
    try {
      // fetchCallback is defined to return lines for the logical window
      // [start, start+count) in order (main log: absolute lines; filter pane:
      // match ordinals). Do NOT key by line.index — on the filter pane that is
      // the source log line number and would leave almost every part empty.
      const lines = await this.fetchCallback(range.start, count);
      const parts: string[] = new Array(count).fill('');
      for (let i = 0; i < lines.length && i < count; i++) {
        parts[i] = lines[i]?.content ?? '';
      }
      // Prefer any content already rendered in the DOM for visible rows (avoids
      // a second IPC when the selection is on-screen).
      for (let i = range.start; i <= range.end; i++) {
        const node = this.renderedNodes.get(i);
        if (!node) continue;
        const textEl = node.querySelector('.log-text') as HTMLElement | null;
        const domText = textEl?.textContent;
        if (domText != null && domText.length > 0) {
          parts[i - range.start] = domText;
        }
      }
      const text = parts.join('\n');
      if (!text) return false;
      await navigator.clipboard.writeText(text);
      return true;
    } catch (err) {
      console.error('copySelection failed:', err);
      return false;
    }
  }

  public setSelection(start: number, end: number) {
    this.selectionStart = Math.max(0, Math.min(start, this.totalLines - 1));
    this.selectionEnd = Math.max(0, Math.min(end, this.totalLines - 1));
    this.selectionAnchor = this.selectionStart;
    this.applySelectionStyle();
  }

  public extendSelection(end: number) {
    if (this.selectionAnchor === null) {
      this.setSelection(end, end);
      return;
    }
    this.selectionEnd = Math.max(0, Math.min(end, this.totalLines - 1));
    this.applySelectionStyle();
  }

  private applySelectionStyle() {
    const range = this.getSelectedRange();
    for (const [idx, node] of this.renderedNodes) {
      if (range && idx >= range.start && idx <= range.end) {
        node.classList.add('selected');
      } else {
        node.classList.remove('selected');
      }
    }
  }

  // Mouse event handlers for selection
  private handleMouseDown = (e: MouseEvent, logicalIndex: number) => {
    if (e.button !== 0) return; // Only left click — right-click is context menu

    // Focus viewport so Shift+Arrow / Ctrl+C work without clicking an input first.
    if (this.viewport.tabIndex < 0) this.viewport.tabIndex = 0;
    this.viewport.focus({ preventScroll: true });

    if (e.shiftKey && this.selectionAnchor !== null) {
      this.extendSelection(logicalIndex);
    } else {
      this.setSelection(logicalIndex, logicalIndex);
    }
    this.isSelecting = true;
    // Prevent native text selection while dragging line ranges; single-line
    // still shows our .selected style which is more reliable in a virtual list.
    e.preventDefault();
  }

  private handleMouseMove = (e: MouseEvent) => {
    if (!this.isSelecting) return;
    // Prefer elementFromPoint so selection still extends when the pointer is
    // over padding / between rows / on the sticky content layer.
    const under = document.elementFromPoint(e.clientX, e.clientY) as HTMLElement | null;
    const rowEl = (under?.closest('.log-row') || (e.target as HTMLElement)?.closest?.('.log-row')) as HTMLElement | null;
    if (rowEl) {
      const idx = parseInt(rowEl.dataset.index || '', 10);
      if (!isNaN(idx)) {
        this.extendSelection(idx);
      }
    }
  }

  private handleMouseUp = () => {
    this.isSelecting = false;
  }

  // Keyboard support for selection
  public handleKeyDown = (e: KeyboardEvent) => {
    if (e.shiftKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      e.preventDefault();
      const current = this.highlightedLineIndex;
      if (current === null) return;
      
      const direction = e.key === 'ArrowUp' ? -1 : 1;
      const newIndex = Math.max(0, Math.min(this.totalLines - 1, current + direction));
      
      if (this.selectionAnchor === null) {
        this.setSelection(current, newIndex);
      } else {
        this.extendSelection(newIndex);
      }
      this.scrollToIndex(newIndex);
    } else if (e.key === 'Escape') {
      this.clearSelection();
    }
  }

  // Attach event listeners for selection
  public attachSelectionListeners() {
    this.viewport.addEventListener('mousedown', (e) => {
      const target = e.target as HTMLElement;
      const rowEl = target.closest('.log-row') as HTMLElement;
      if (rowEl) {
        const idx = parseInt(rowEl.dataset.index || '', 10);
        if (!isNaN(idx)) {
          this.handleMouseDown(e, idx);
        }
      }
    });

    this.viewport.addEventListener('mousemove', this.handleMouseMove.bind(this));
    document.addEventListener('mouseup', this.handleMouseUp.bind(this));
    this.viewport.addEventListener('keydown', this.handleKeyDown.bind(this));
  }
}
