package tui

import (
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// Term highlight styles (Catppuccin-ish, distinct from each other).
var (
	hlPrimaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")) // gold
	hlAndStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")) // green
	hlNearbyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#89dceb")) // cyan
	hlExcludeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")) // red
)

type hlSpan struct {
	start, end int // byte offsets in the segment
	priority   int // lower = higher priority (primary wins over exclude)
	style      lipgloss.Style
}

// paintSearchTerms colors substrings matching primary / AND / nearby / exclude
// patterns inside seg. When no patterns are active, returns seg unchanged.
// Overlaps are resolved by priority: primary < AND < nearby < exclude.
func (m model) paintSearchTerms(seg string) string {
	if seg == "" {
		return seg
	}
	opts := m.effectiveSearchOptions()
	hlPrimary, hlAnd, hlNearby, hlExclude := true, true, true, false
	if m.cfg != nil {
		hlPrimary = m.cfg.HlPrimary
		hlAnd = m.cfg.HlSecondary
		hlNearby = m.cfg.HlNearby
		hlExclude = m.cfg.HlExclude
	}

	var spans []hlSpan
	caseSens := opts.CaseSensitive

	if hlPrimary && opts.Query != "" {
		spans = append(spans, findSpans(seg, opts.Query, opts.IsRegex, caseSens, 0, hlPrimaryStyle)...)
	}
	if hlAnd && opts.SecondaryQuery != "" {
		spans = append(spans, findSpans(seg, opts.SecondaryQuery, opts.IsRegex, caseSens, 1, hlAndStyle)...)
	}
	if hlNearby && m.advNearbyQuery != "" {
		spans = append(spans, findSpans(seg, m.advNearbyQuery, opts.IsRegex, caseSens, 2, hlNearbyStyle)...)
	}
	if hlExclude {
		for _, p := range m.advExclude {
			if p == "" {
				continue
			}
			spans = append(spans, findSpans(seg, p, m.advExcludeRegex, caseSens, 3, hlExcludeStyle)...)
		}
	}

	if len(spans) == 0 {
		return seg
	}

	// Sort by start, then priority (so primary wins on same start).
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].priority < spans[j].priority
	})

	// Greedy non-overlapping merge (keep higher priority on overlap).
	merged := make([]hlSpan, 0, len(spans))
	for _, s := range spans {
		if s.start >= s.end || s.start < 0 || s.end > len(seg) {
			continue
		}
		if len(merged) == 0 {
			merged = append(merged, s)
			continue
		}
		last := &merged[len(merged)-1]
		if s.start >= last.end {
			merged = append(merged, s)
			continue
		}
		// Overlap: keep the one with better priority; if equal, keep longer.
		if s.priority < last.priority || (s.priority == last.priority && s.end > last.end) {
			if s.start <= last.start {
				*last = s
			} else {
				// Truncate last and append s if remaining.
				last.end = s.start
				if last.start >= last.end {
					merged = merged[:len(merged)-1]
				}
				merged = append(merged, s)
			}
		}
		// else drop s
	}

	var b strings.Builder
	b.Grow(len(seg) + len(merged)*16)
	pos := 0
	for _, s := range merged {
		if s.start > pos {
			b.WriteString(seg[pos:s.start])
		}
		if s.end > s.start {
			b.WriteString(s.style.Render(seg[s.start:s.end]))
		}
		pos = s.end
	}
	if pos < len(seg) {
		b.WriteString(seg[pos:])
	}
	return b.String()
}

// findSpans returns all non-empty match ranges of pattern in s.
func findSpans(s, pattern string, isRegex, caseSensitive bool, priority int, style lipgloss.Style) []hlSpan {
	if pattern == "" || s == "" {
		return nil
	}
	var out []hlSpan
	if isRegex {
		// Serving the compiled pattern from the cache avoids one
		// regexp.Compile per rendered row — compiling cost ~25x the match
		// itself (~30 allocs + ~34µs per row, measured on view_bench) — so
		// each distinct pattern is compiled once and reused every frame.
		re, err := cachedRegex(pattern, caseSensitive)
		if err != nil {
			// Fall back to literal on bad regex.
			return findSpans(s, pattern, false, caseSensitive, priority, style)
		}
		idxs := re.FindAllStringIndex(s, -1)
		for _, ix := range idxs {
			if ix[1] > ix[0] {
				out = append(out, hlSpan{start: ix[0], end: ix[1], priority: priority, style: style})
			}
		}
		return out
	}

	// Literal substring search.
	if caseSensitive {
		from := 0
		for {
			i := strings.Index(s[from:], pattern)
			if i < 0 {
				break
			}
			start := from + i
			end := start + len(pattern)
			out = append(out, hlSpan{start: start, end: end, priority: priority, style: style})
			from = end
			if from >= len(s) {
				break
			}
		}
		return out
	}

	// Case-insensitive literal (ASCII-oriented; good enough for log viewers).
	// For ASCII patterns the match index comes from a byte-wise ASCII fold
	// scan that folds 'A'-'Z' on the fly (indexFoldASCII), which avoids the
	// strings.ToLower copy of the whole segment — one full-line alloc + copy
	// per rendered row per frame — since only the index is needed to re-slice
	// the original string. Non-ASCII patterns keep the lowered-copy path so
	// Unicode folding semantics are unchanged.
	if isASCII(pattern) {
		from := 0
		for {
			i := indexFoldASCII(s[from:], pattern)
			if i < 0 {
				break
			}
			start := from + i
			end := start + len(pattern)
			// Align end to rune boundary if needed.
			for end < len(s) && !utf8Start(s, end) {
				end++
			}
			out = append(out, hlSpan{start: start, end: end, priority: priority, style: style})
			from = end
			if from >= len(s) {
				break
			}
		}
		return out
	}

	// Non-ASCII pattern: fold via lowered copies (unchanged semantics).
	lowerS := strings.ToLower(s)
	lowerP := strings.ToLower(pattern)
	from := 0
	for {
		i := strings.Index(lowerS[from:], lowerP)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(pattern)
		// Align end to rune boundary if needed.
		for end < len(s) && !utf8Start(s, end) {
			end++
		}
		out = append(out, hlSpan{start: start, end: end, priority: priority, style: style})
		from = end
		if from >= len(s) {
			break
		}
	}
	return out
}

func utf8Start(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	return !unicode.In(rune(s[i]), unicode.Mn) && (s[i]&0xC0) != 0x80
}

// isASCII reports whether s contains only ASCII bytes.
func isASCII(s string) bool {
	for i := range s {
		if s[i] >= 0x80 {
			return false
		}
	}

	return true
}

// indexFoldASCII returns the byte index of the first case-insensitive
// occurrence of sub in s, or -1. Both arguments are assumed ASCII and folded
// on the fly ('A'-'Z' to lower, same style as pkg/search's
// containsFoldASCIIBytes, folding sub too so callers pass the raw pattern).
// Returning just the index keeps the per-rendered-row scan allocation-free:
// the caller re-slices the original string instead of lowering a copy of it.
func indexFoldASCII(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}

	if m > n {
		return -1
	}

	for i := 0; i <= n-m; i++ {
		ok := true

		for j := range m {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}

			p := sub[j]
			if 'A' <= p && p <= 'Z' {
				p += 'a' - 'A'
			}

			if c != p {
				ok = false
				break
			}
		}

		if ok {
			return i
		}
	}

	return -1
}
