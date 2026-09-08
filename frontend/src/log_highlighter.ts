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

  public static highlightLine(text: string, searchQuery?: string, isRegex?: boolean): string {
    text = this.sanitize(text);
    const tokens = this.tokenize(text);
    const searchRe = this.buildSearchRegex(searchQuery, isRegex);

    let html = '';
    for (const token of tokens) {
      const inner = this.renderTokenValue(token.value, searchRe);
      if (token.type === 'text') {
        html += inner;
      } else {
        const rule = this.rules.find((r) => r.name === token.type);
        const className = rule ? rule.className : token.type;
        html += `<span class="${className}">${inner}</span>`;
      }
    }
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
