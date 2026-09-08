// Package tui implements the Bubbletea-based interactive log viewer launched by
// `gofle <target>`. It renders only the visible window of a (possibly huge) log
// via the indexer, supports scrolling, live search, match navigation, a
// results-only filter view, follow/tail mode, goto-line and export — without
// ever materializing the whole file.
package tui

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"context"
	"log/slog"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
)

// RunRecentPicker launches the interactive recent-files picker and returns the
// target to open (local path or ssh:// URL). An empty string means the user
// canceled (esc/q). It is the interactive equivalent of the in-viewer `o` key.
func RunRecentPicker(items []config.RecentFileItem) (string, error) {
	m := recentPickerModel{items: items, cursor: 0}
	p := tea.NewProgram(m, tea.WithAltScreen())

	final, err := p.Run()
	if err != nil {
		return "", err
	}

	mm, ok := final.(recentPickerModel)
	if !ok || mm.canceled {
		return "", nil
	}

	if mm.cursor < 0 || mm.cursor >= len(mm.items) {
		return "", nil
	}

	it := mm.items[mm.cursor]

	target := it.Path
	if it.IsSSH && it.SSH != nil {
		// Canonical no-userinfo target; its scheme (ssh:// or ftp://) follows the
		// stored Protocol, and credentials resolve from the stored encrypted
		// summary, never through the URL.
		target = remoteTargetFromRecent(it)
	}

	return target, nil
}

// RunView constructs the viewer model and runs the Bubbletea program. cleanup is
// always called on exit (both on quit and on any error).
//
// parent is the command's context, which carries the interrupt cancellation
// installed by main. Deriving the viewer context from it (rather than from
// context.Background()) is what lets an interrupt actually reach the blocking work
// the viewer starts — notably an in-viewer reopen of an ssh:// entry, which
// resolves synchronously and would otherwise hold the terminal for the full
// dial/handshake/stall watchdogs with no way to give up.
func RunView(parent context.Context, idx *indexer.FileIndexer, se *search.SearchEngine, cleanup func(), cfg *config.AppConfig) error {
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)

	defer func() {
		cancel()
		// Tear down asynchronously but still give a healthy connection time to
		// release its resources (temp file, SSH session) so we don't leak. The
		// wait is bounded: a stuck SSH tail can make Streamer.Close block up to
		// its close timeout, and we must never let that freeze application exit.
		if cleanup != nil {
			done := make(chan struct{})

			go func() {
				cleanup()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				slog.Warn("gofle: viewer cleanup did not finish promptly; exiting anyway",
					"target", idx.FilePath())
			}
		}
	}()

	si := textinput.New()
	si.Placeholder = "query ( /re, !inv, i:ci )"
	seci := textinput.New()
	seci.Placeholder = "AND filter within results"
	gi := textinput.New()
	gi.Placeholder = "line number"
	ei := textinput.New()
	ei.Placeholder = "path (~ expands)"

	// Remote connect form fields (protocol-agnostic): Protocol, Host, Port,
	// User, Auth, Password, Key path, Remote path. The password field is masked
	// and never enters any command/input history (the TUI keeps no such history).
	rfProto := textinput.New()
	rfProto.Placeholder = "ssh"
	rfHost := textinput.New()
	rfHost.Placeholder = "host (ip/domain)"
	rfPort := textinput.New()
	rfPort.Placeholder = "22"
	rfUser := textinput.New()
	rfUser.Placeholder = "user"
	rfAuth := textinput.New()
	rfAuth.Placeholder = "password"
	rfPass := textinput.New()
	rfPass.Placeholder = "password"
	rfPass.EchoMode = textinput.EchoPassword
	rfKey := textinput.New()
	rfKey.Placeholder = "private key path (~ expands)"
	rfPath := textinput.New()
	rfPath.Placeholder = "remote file path (optional — leave empty to browse)"
	remoteForm := []textinput.Model{rfProto, rfHost, rfPort, rfUser, rfAuth, rfPass, rfKey, rfPath}

	m := model{
		idx:            idx,
		se:             se,
		cleanup:        cleanup,
		cfg:            cfg,
		height:         40,
		width:          100,
		searchInput:    si,
		secondaryInput: seci,
		gotoInput:      gi,
		exportInput:    ei,
		remoteForm:     remoteForm,
		// Default: case-insensitive search. Layout comes from shared settings
		// (view_mode: normal | results | split; default split).
		searchOpts: search.SearchOptions{CaseSensitive: false},
		ctx:        ctx,
		cancel:     cancel,
	}
	m.applyViewModeFromConfig()

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()

	return err
}
