package tui

import (
	"GoFLE/pkg/search"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// searchProgressMsg is emitted while an async search is running.
type searchProgressMsg struct {
	gen   int
	count int64
}

// searchDoneMsg carries the finished result of an async search.
type searchDoneMsg struct {
	gen            int
	matches        []int64
	primaryMatches []int64
	primaryKey     string
	err            error
}

// startSearchCmd runs the full search pipeline off the UI thread and streams
// partial match counts via searchProgressMsg (ticker reads an atomic counter
// updated by SearchWithProgress).
func (m *model) startSearchCmd() tea.Cmd {
	m.query = m.searchInput.Value()
	m.secondaryQuery = strings.TrimSpace(m.secondaryInput.Value())

	opts := m.effectiveSearchOptions()
	hasAdv := m.hasAdvancedFilters()

	if opts.Query == "" && !hasAdv && opts.SecondaryQuery == "" {
		m.matches = nil
		m.primaryMatches = nil
		m.primaryKey = ""
		m.matchCursor = 0
		m.searching = false
		return nil
	}

	if opts.Query == "" {
		opts.MatchAll = true
		opts.IsRegex = false
		opts.InvertMatch = false
	}

	// Cancel any in-flight search.
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.searchCancel = cancel

	m.searchGen++
	gen := m.searchGen
	m.searching = true
	m.status = "searching…"

	prog := &atomic.Int64{}
	m.searchProg = prog

	idx := m.idx
	se := m.se
	ctxBefore := m.advCtxBefore
	ctxAfter := m.advCtxAfter
	expand := m.hasContextExpand()

	key := searchPrimaryKey(opts)
	secondaryOnly := m.primaryMatches != nil && key == m.primaryKey && !hasAdv && opts.SecondaryQuery != ""
	primarySnap := append([]int64(nil), m.primaryMatches...)

	searchCmd := func() tea.Msg {
		var res, primary []int64
		var err error
		primaryKey := key

		if secondaryOnly {
			res, err = se.FilterLines(ctx, opts, primarySnap)
			primary = primarySnap
		} else {
			primaryOpts := opts
			primaryOpts.SecondaryQuery = ""

			if opts.LineStart > 0 || opts.LineEnd > 0 {
				end := opts.LineEnd
				if end <= 0 {
					end = idx.TotalLines()
				}
				res, err = se.SearchRange(ctx, primaryOpts, opts.LineStart, end)
			} else {
				res, err = se.SearchWithProgress(ctx, primaryOpts, func(n int64) {
					prog.Store(n)
				})
			}
			if err != nil {
				return searchDoneMsg{gen: gen, err: err}
			}
			primary = append([]int64(nil), res...)
			if opts.SecondaryQuery != "" {
				res, err = se.FilterLines(ctx, opts, res)
				if err != nil {
					return searchDoneMsg{gen: gen, err: err}
				}
			}
		}

		if err == nil {
			res, err = se.ApplyAdvancedFilters(ctx, opts, res)
		}
		if err != nil {
			return searchDoneMsg{gen: gen, err: err}
		}
		if expand {
			res = search.ExpandMatchesWithContext(res, ctxBefore, ctxAfter, idx.TotalLines())
		}
		return searchDoneMsg{
			gen:            gen,
			matches:        res,
			primaryMatches: primary,
			primaryKey:     primaryKey,
		}
	}

	tickCmd := tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return searchProgressMsg{gen: gen, count: prog.Load()}
	})

	return tea.Batch(searchCmd, tickCmd)
}

// applySearchProgress updates the status line while a search is in flight.
func (m *model) applySearchProgress(msg searchProgressMsg) tea.Cmd {
	if !m.searching || msg.gen != m.searchGen {
		return nil
	}
	if msg.count > 0 {
		m.status = fmt.Sprintf("searching… %d matches so far", msg.count)
	} else {
		m.status = "searching…"
	}
	// Keep ticking until searchDoneMsg arrives.
	prog := m.searchProg
	gen := msg.gen
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		var c int64
		if prog != nil {
			c = prog.Load()
		}
		return searchProgressMsg{gen: gen, count: c}
	})
}

// applySearchDone finalizes an async search result.
func (m *model) applySearchDone(msg searchDoneMsg) {
	if msg.gen != m.searchGen {
		return // stale
	}
	m.searching = false
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
	m.searchProg = nil
	if msg.err != nil {
		m.err = msg.err
		m.status = "search failed: " + msg.err.Error()
		return
	}
	m.primaryMatches = msg.primaryMatches
	m.primaryKey = msg.primaryKey
	m.applyMatchResult(msg.matches)
}
