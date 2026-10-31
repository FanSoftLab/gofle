interface HighlighterRule {
  name: string;
  regex: RegExp;
  className: string;
}

interface Token {
  type: string;
  value: string;
}

interface SearchSegment {
  text: string;
  isMatch: boolean;
}

/**
 * LogHighlighter — syntax + search highlighting for log lines.
 *
 * Performance notes (2026-10):
 * - tokenize() is the hot path. Results are cached by (text + search key) so
 *   re-renders / soft setTotalLines do not re-run 16 sticky regexes per visible
 *   line on every poll tick.
 * - Cache is bounded (LRU-ish via Map insertion order) to avoid unbounded growth
 *   when scrolling huge files.
 */
export class LogHighlighter {
  private static readonly rules: HighlighterRule[] = [
    { name: 'date', className: 'hl-date', regex: /\b(\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?)\b/y },
    { name: 'ip', className: 'hl-ip', regex: /\b(?:\d{1,3}\.){3}\d{1,3}\b/y },
    { name: 'url', className: 'hl-url', regex: /\b(https?:\/\/[^\s"']+)\b/y },
    { name: 'jsonKey', className: 'hl-json-key', regex: /("[\w_\-]+")(?=\s*:)/y },
    { name: 'http2xx', className: 'hl-http-2xx', regex: /\b(2\d{2})\b/y },
    { name: 'http4xx', className: 'hl-http-4xx', regex: /\b([45]\d{2})\b/y },
    { name: 'fatal', className: 'hl-fatal', regex: /\b(FATAL|CRITICAL|EMERGENCY)\b/yi },
    { name: 'error', className: 'hl-error', regex: /\b(ERROR|ERR|FAIL|FAILURE|FAILED)\b/yi },
    { name: 'warn', className: 'hl-warn', regex: /\b(WARN|WARNING|ALERT)\b/yi },
    { name: 'info', className: 'hl-info', regex: /\b(INFO|INFORMATIONAL|NOTICE|NOTE|SUCCESS)\b/yi },
    { name: 'debug', className: 'hl-debug', regex: /\b(DEBUG|DBG)\b/yi },
    { name: 'trace', className: 'hl-trace', regex: /\b(TRACE)\b/yi },
    { name: 'uuid', className: 'hl-uuid', regex: /\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b/y },
    { name: 'hex', className: 'hl-hex', regex: /#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?\b/y },
    { name: 'metric', className: 'hl-metric', regex: /\b\d+(?:\.\d+)?\s?(?:ms|µs|us|ns|s|kb|mb|gb|tb|b|%|x)\b/yi },
    { name: 'pid', className: 'hl-pid', regex: /\b(?:pid|tid)=(\d+)\b/yi },
  ];

  /** Bounded HTML cache: key = text + '\\0' + queryKey → highlighted HTML. */
  private static readonly htmlCache = new Map<string, string>();
  private static readonly HTML_CACHE_MAX = 4000;

  private static escapeHtml(str: string): string {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  /** Strip C0/C1 controls and bidi/zero-width junk before rendering. */
  private static sanitize(text: string): string {
    if (!text) return text;
    // Keep tab; drop other C0, DEL, C1, ZW*/bidi format chars, U+FFFD.
    return text.replace(
      // eslint-disable-next-line no-control-regex
      /[\u0000-\u0008\u000A-\u001F\u007F-\u009F\u200B-\u200D\u202A-\u202E\u2060\u2066-\u2069\uFEFF\uFE00-\uFE0F\uFFFD]/g,
      '',
    );
  }

  private static cacheGet(key: string): string | undefined {
    const v = this.htmlCache.get(key);
    if (v === undefined) return undefined;
    // Refresh insertion order for crude LRU.
    this.htmlCache.delete(key);
    this.htmlCache.set(key, v);
    return v;
  }

  private static cacheSet(key: string, html: string): void {
    if (this.htmlCache.has(key)) this.htmlCache.delete(key);
    this.htmlCache.set(key, html);
    while (this.htmlCache.size > this.HTML_CACHE_MAX) {
      const first = this.htmlCache.keys().next().value;
      if (first === undefined) break;
      this.htmlCache.delete(first);
    }
  }

  /** Drop cached HTML (e.g. after theme / rule changes). */
  public static clearCache(): void {
    this.htmlCache.clear();
  }

  public static highlightLineMulti(
    text: string,
    terms: { q: string; cls: string; isRegex?: boolean }[],
  ): string {
    text = this.sanitize(text);
    const termKey = terms.map((t) => `${t.cls}:${t.isRegex ? 'r' : 's'}:${t.q}`).join('|');
    const cacheKey = text + '\0m\0' + termKey;
    const cached = this.cacheGet(cacheKey);
    if (cached !== undefined) return cached;

    // Multi-term path: escape once, then apply each term as a replace.
    let html = this.escapeHtml(text);
    for (const term of terms) {
      if (!term.q) continue;
      try {
        const re = term.isRegex
          ? new RegExp(term.q, 'gi')
          : new RegExp(term.q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
        html = html.replace(re, (m) => `<span class="${term.cls}">${m}</span>`);
      } catch {
        /* ignore bad regex */
      }
    }
    this.cacheSet(cacheKey, html);
    return html;
  }

  public static highlightLine(text: string, searchQuery?: string, isRegex?: boolean): string {
    text = this.sanitize(text);
    const q = searchQuery ?? '';
    const cacheKey = text + '\0s\0' + (isRegex ? 'r:' : 's:') + q;
    const cached = this.cacheGet(cacheKey);
    if (cached !== undefined) return cached;

    const tokens = this.tokenize(text);
    const searchRe = this.buildSearchRegex(searchQuery, isRegex);

    let html = '';
    for (const token of tokens) {
      const className = this.rules.find((r) => r.name === token.type)?.className;
      const inner = this.renderTokenValue(token.value, searchRe);
      if (className && token.type !== 'text') {
        html += `<span class="${className}">${inner}</span>`;
      } else {
        html += inner;
      }
    }
    this.cacheSet(cacheKey, html);
    return html;
  }

  private static tokenize(text: string): Token[] {
    const tokens: Token[] = [];
    let pos = 0;
    let buffer = '';

    const flush = () => {
      if (buffer.length > 0) {
        tokens.push({ type: 'text', value: buffer });
        buffer = '';
      }
    };

    while (pos < text.length) {
      let best: { rule: HighlighterRule; length: number } | null = null;
      for (const rule of this.rules) {
        rule.regex.lastIndex = pos;
        const m = rule.regex.exec(text);
        if (m && m.index === pos) {
          const len = m[0].length;
          if (!best || len > best.length) {
            best = { rule, length: len };
          }
        }
      }

      if (best) {
        flush();
        tokens.push({ type: best.rule.name, value: text.substr(pos, best.length) });
        pos += best.length;
      } else {
        buffer += text[pos];
        pos++;
      }
    }
    flush();
    return tokens;
  }

  private static buildSearchRegex(searchQuery?: string, isRegex?: boolean): RegExp | null {
    if (!searchQuery || searchQuery.trim() === '') return null;
    try {
      const pattern = isRegex ? searchQuery : searchQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      return new RegExp(`(${pattern})`, 'gi');
    } catch {
      return null;
    }
  }

  private static splitSearch(value: string, searchRe: RegExp): SearchSegment[] {
    const segments: SearchSegment[] = [];
    let last = 0;
    let match: RegExpExecArray | null;
    searchRe.lastIndex = 0;
    while ((match = searchRe.exec(value)) !== null) {
      if (match.index > last) {
        segments.push({ text: value.slice(last, match.index), isMatch: false });
      }
      segments.push({ text: match[0], isMatch: true });
      last = match.index + match[0].length;
      if (match[0].length === 0) {
        searchRe.lastIndex++;
      }
    }
    if (last < value.length) {
      segments.push({ text: value.slice(last), isMatch: false });
    }
    return segments;
  }

  private static renderTokenValue(value: string, searchRe: RegExp | null): string {
    if (!searchRe) {
      return this.escapeHtml(value);
    }
    const segments = this.splitSearch(value, searchRe);
    let out = '';
    for (const seg of segments) {
      const escaped = this.escapeHtml(seg.text);
      out += seg.isMatch ? `<mark class="search-highlight">${escaped}</mark>` : escaped;
    }
    return out;
  }
}
