// Regression test: LogHighlighter.highlightLine must never emit live HTML for
// attacker-controlled log content. Every token is escaped via escapeHtml before
// being wrapped in <span>/<mark>, so log lines containing <img onerror>, <script>,
// quotes, etc. cannot inject markup into the rendered DOM (the result is set via
// textEl.innerHTML in virtual_scroller.ts).
import { describe, it, expect } from 'vitest';
import { LogHighlighter } from '../src/log_highlighter';

describe('LogHighlighter XSS escaping', () => {
  it('escapes an img onerror payload with no search query', () => {
    const out = LogHighlighter.highlightLine('<img src=x onerror=alert(1)>');
    // There must be no live <img> tag; the angle brackets are escaped. Any
    // generated markup is a safe <span>/<mark> wrapper, never the raw payload.
    expect(out).not.toContain('<img');
    expect(out).toContain('&lt;img');
    expect(out).toContain('&gt;');
  });

  it('escapes a script tag payload', () => {
    const out = LogHighlighter.highlightLine('</span><script>alert(1)</script>');
    expect(out).not.toContain('<script>');
    expect(out).toContain('&lt;/span&gt;');
    expect(out).toContain('&lt;script&gt;');
  });

  it('escapes quotes and angle brackets in a plain line', () => {
    const out = LogHighlighter.highlightLine('a "b" <c> & d');
    expect(out).toBe('a &quot;b&quot; &lt;c&gt; &amp; d');
  });

  it('escapes content even inside a search highlight match', () => {
    const out = LogHighlighter.highlightLine('payload <b>INJECT</b> here', '<b>INJECT</b>');
    // The match is wrapped in <mark>, but the angle brackets in the literal text
    // are escaped — there must be no raw <b> tag in the output.
    expect(out).not.toContain('<b>');
    expect(out).toContain('<mark class="search-highlight">');
    expect(out).toContain('&lt;b&gt;INJECT&lt;/b&gt;');
  });

  it('never emits an unescaped < that is not part of a generated tag', () => {
    const payloads = [
      '<svg/onload=alert(1)>',
      '<<script>>',
      '<a href="javascript:alert(1)">x</a>',
      '1 < 2 && 3 > 2',
    ];
    for (const p of payloads) {
      const out = LogHighlighter.highlightLine(p);
      // strip our own generated tags, what remains must contain no raw '<'
      const stripped = out.replace(/<\/?span[^>]*>/g, '').replace(/<\/?mark[^>]*>/g, '');
      expect(stripped).not.toContain('<');
    }
  });
});
