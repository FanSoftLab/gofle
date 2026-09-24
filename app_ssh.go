package main

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"GoFLE/pkg/ssh"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// sshBrowseMu serializes ListDir calls on the cached *ssh.Lister. ssh.Client
// (the transport the lister wraps) is not safe for concurrent use, so two
// overlapping ListSSHFiles calls must not issue sessions on the same client at
// once. The lister cache itself (a.sshBrowseLister) stays guarded by a.mu; this
// mutex is held only around the blocking ListDir and the drop-on-error block,
// never while a.mu is held, so there is no lock-ordering cycle.
var sshBrowseMu sync.Mutex

// SSHConfig is the JSON contract the frontend sends to open a remote log over SSH (host/port/credentials/remote path).
type SSHConfig struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	AuthType       string `json:"auth_type"`
	Password       string `json:"password"`
	KeyPath        string `json:"key_path"`
	KeyPassphrase  string `json:"key_passphrase"`
	RemoteFilePath string `json:"remote_file_path"`
}

// newSSHSummary converts the frontend SSHConfig contract into the persisted
// config.SSHConfigSummary shape shared by the open and accept flows.
func newSSHSummary(cfg SSHConfig) *config.SSHConfigSummary {
	return &config.SSHConfigSummary{
		Host:           cfg.Host,
		Port:           cfg.Port,
		Username:       cfg.Username,
		RemoteFilePath: cfg.RemoteFilePath,
		AuthType:       cfg.AuthType,
		Password:       cfg.Password,
		KeyPath:        cfg.KeyPath,
		KeyPassphrase:  cfg.KeyPassphrase,
	}
}

// OpenSSHFile streams a remote log over SSH, indexes it locally, and registers
// a tab. Reopening the same remote path focuses the existing tab. If the server's
// host key is unknown, it returns a *ssh.HostKeyUnknownError and stashes the
// connection so AcceptSSHHost can complete it after the user trusts the host.
// The recent entry is only recorded after a successful open.
func (a *App) OpenSSHFile(sshCfg SSHConfig) (*FileStatsResponse, error) {
	// Rate limiting check
	if err := a.checkConnRateLimit(sshCfg.Host, sshCfg.Port); err != nil {
		return nil, fmt.Errorf("ssh connection rate limited: %w", err)
	}

	sshPathDisplay := fmt.Sprintf("ssh://%s:%d%s", sshCfg.Host, sshCfg.Port, sshCfg.RemoteFilePath)

	// 1 tab = 1 file: reopening the same remote file focuses the existing tab
	// (and avoids opening a second SSH connection / temp file).
	if existing := a.findOpenTab(sshPathDisplay); existing != nil {
		return a.reuseOpenTab(existing)
	}

	sshSummary := newSSHSummary(sshCfg)

	stats, err := a.openSSHStream(sshCfg, sshPathDisplay, sshSummary, "")
	if err != nil {
		a.recordConnFailure(sshCfg.Host, sshCfg.Port)
		if hkErr, ok := errors.AsType[*ssh.HostKeyUnknownError](err); ok {
			// Stash the connection + the known_hosts line so AcceptSSHHost can
			// persist the host and retry once the user trusts it.
			a.mu.Lock()
			a.pendingSSHHost[hkErr.Host] = hkErr.Line
			a.pendingSSHConfig[hkErr.Host] = sshCfg
			a.mu.Unlock()
		}
		// Failures are already logged inside openSSHStream.
		return nil, err
	}

	a.recordConnSuccess(sshCfg.Host, sshCfg.Port)

	// Only record the entry in the recent list after a real, successful open.
	// This avoids persisting connections that failed (e.g. an unknown host the
	// user declined to trust, or credentials that did not authenticate).
	// The open itself succeeded, so a failed recent-list persist is logged, not
	// returned — it must not turn into a phantom connection error in the UI.
	if err := a.cfg.AddRecentFile(sshPathDisplay, 0, true, sshSummary); err != nil {
		slog.Warn("failed to persist recent SSH entry", "path", sshPathDisplay, "err", err)
	}

	return stats, nil
}

// AcceptSSHHost is invoked from the UI after the user confirms a previously
// unknown SSH host key. It writes the host key into known_hosts and re-runs the
// full open path for the stored connection config.
func (a *App) AcceptSSHHost(host string) (*FileStatsResponse, error) {
	a.mu.Lock()
	line, hasLine := a.pendingSSHHost[host]

	cfg, hasCfg := a.pendingSSHConfig[host]
	if hasLine {
		delete(a.pendingSSHHost, host)
	}

	if hasCfg {
		delete(a.pendingSSHConfig, host)
	}

	a.mu.Unlock()

	if !hasLine || !hasCfg {
		return nil, fmt.Errorf("no pending SSH host to accept: %s", host)
	}

	// Rate limiting check (use host from config which includes port)
	if err := a.checkConnRateLimit(cfg.Host, cfg.Port); err != nil {
		return nil, fmt.Errorf("ssh connection rate limited: %w", err)
	}

	if err := ssh.WriteKnownHost(a.cfg.KnownHostsPath(), line); err != nil {
		return nil, fmt.Errorf("failed to persist trusted host key: %w", err)
	}

	slog.Info("ssh: user trusted host key", "host", host)

	sshPathDisplay := fmt.Sprintf("ssh://%s:%d%s", cfg.Host, cfg.Port, cfg.RemoteFilePath)
	sshSummary := newSSHSummary(cfg)

	stats, err := a.openSSHStream(cfg, sshPathDisplay, sshSummary, "")
	if err != nil {
		a.recordConnFailure(cfg.Host, cfg.Port)
		return nil, fmt.Errorf("failed to reopen %s after trusting host key: %w", sshPathDisplay, err)
	}

	a.recordConnSuccess(cfg.Host, cfg.Port)

	// Persist only after a successful open (B4). AddRecentFile already writes
	// recent.json, so the redundant full Save() is dropped. A failed recent-list
	// persist is logged, not returned (see OpenSSHFile).
	if err := a.cfg.AddRecentFile(sshPathDisplay, 0, true, sshSummary); err != nil {
		slog.Warn("failed to persist recent SSH entry", "path", sshPathDisplay, "err", err)
	}

	return stats, nil
}

// ListSSHFiles browses a remote host's directory so the user can pick a log file
// interactively when no path is supplied. It reuses a cached *ssh.Lister for the
// active browser session (so navigating directories does not re-dial and re-auth
// the server on every listing), dialing a fresh one only for a new host or after
// a connection drop. On an unknown host key it stashes the connection in the same
// pendingSSH* maps OpenSSHFile uses, so AckSSHHost can persist the key and the
// frontend can re-list afterwards.
func (a *App) ListSSHFiles(sshCfg SSHConfig, path string) ([]ssh.DirEntry, error) {
	// Rate limiting check
	if err := a.checkConnRateLimit(sshCfg.Host, sshCfg.Port); err != nil {
		return nil, fmt.Errorf("ssh connection rate limited: %w", err)
	}

	cfg := ssh.Config{
		Host:           sshCfg.Host,
		Port:           sshCfg.Port,
		Username:       sshCfg.Username,
		AuthType:       sshCfg.AuthType,
		Password:       sshCfg.Password,
		KeyPath:        sshCfg.KeyPath,
		KeyPassphrase:  sshCfg.KeyPassphrase,
		RemoteFilePath: sshCfg.RemoteFilePath,
		KnownHostsPath: a.cfg.KnownHostsPath(),
	}
	key := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Reuse the cached lister for this host if we have one.
	a.mu.Lock()

	var lister *ssh.Lister
	if a.sshBrowseLister != nil && a.sshBrowseListerKey == key {
		lister = a.sshBrowseLister
	}

	a.mu.Unlock()

	if lister == nil {
		l, err := ssh.NewLister(cfg)
		if err != nil {
			var hkErr *ssh.HostKeyUnknownError
			if errors.As(err, &hkErr) {
				// Stash the connection + the known_hosts line so AckSSHHost can
				// persist the host; the frontend then re-calls ListSSHFiles.
				a.mu.Lock()
				a.pendingSSHHost[hkErr.Host] = hkErr.Line
				a.pendingSSHConfig[hkErr.Host] = sshCfg
				a.mu.Unlock()
			}

			return nil, err
		}

		lister = l
		// Replace any stale cached lister for a different host.
		a.mu.Lock()

		if a.sshBrowseLister != nil && a.sshBrowseLister != lister {
			_ = a.sshBrowseLister.Close()
		}

		a.sshBrowseLister = lister
		a.sshBrowseListerKey = key
		a.mu.Unlock()
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	sshBrowseMu.Lock()

	entries, err := lister.ListDir(ctx, path)
	if err != nil {
		// The connection died mid-session: drop the cache so the next listing
		// reconnects instead of failing repeatedly on a dead client.
		a.mu.Lock()

		if a.sshBrowseLister == lister {
			a.sshBrowseLister = nil
			a.sshBrowseListerKey = ""
		}

		a.mu.Unlock()
		sshBrowseMu.Unlock()

		_ = lister.Close()

		return nil, fmt.Errorf("failed to list remote directory %q: %w", path, err)
	}

	sshBrowseMu.Unlock()

	return entries, nil
}

// AckSSHHost is invoked from the browser (or the open flow) after the user
// confirms a previously unknown SSH host key. It writes the host key into
// known_hosts and clears the pending maps. Unlike AcceptSSHHost it does NOT open
// or index anything — the browser simply re-lists afterwards.
func (a *App) AckSSHHost(host string) error {
	a.mu.Lock()
	line, hasLine := a.pendingSSHHost[host]

	_, hasCfg := a.pendingSSHConfig[host]
	if hasLine {
		delete(a.pendingSSHHost, host)
	}

	if hasCfg {
		delete(a.pendingSSHConfig, host)
	}

	a.mu.Unlock()

	if !hasLine || !hasCfg {
		return fmt.Errorf("no pending SSH host to accept: %s", host)
	}

	if err := ssh.WriteKnownHost(a.cfg.KnownHostsPath(), line); err != nil {
		return fmt.Errorf("failed to persist trusted host key: %w", err)
	}

	slog.Info("ssh: user trusted host key (browser)", "host", host)

	return nil
}

// openSSHStream opens the SSH connection, indexes the streamed remote file, and
// registers a tab. It is the shared core of OpenSSHFile, AcceptSSHHost and
// RestoreTab. tabID may be empty (generate a fresh id) or explicit (restore).
//
//nolint:dupl // symmetric SSH/FTP stream openers
func (a *App) openSSHStream(sshCfg SSHConfig, sshPathDisplay string, sshSummary *config.SSHConfigSummary, tabID string) (*FileStatsResponse, error) {
	// 1 tab = 1 file, for ALL callers (open / accept-host / restore): a tab
	// registered while another open of the same remote is still dialing is not
	// visible to findOpenTab yet, so concurrent opens must be collapsed here.
	if existing := a.findOpenTab(sshPathDisplay); existing != nil {
		return a.reuseOpenTab(existing)
	}

	v, err, sharedCall := a.sshInflight.Do(sshPathDisplay, func() (any, error) {
		return a.openSSHStreamOnce(sshCfg, sshPathDisplay, sshSummary, tabID)
	})
	if err != nil {
		return nil, err
	}

	stats, ok := v.(*FileStatsResponse)
	if !ok || stats == nil {
		return nil, fmt.Errorf("ssh: unexpected inflight result for %s", sshPathDisplay)
	}

	if sharedCall {
		// A follower may have raced past the fast path while the winner was
		// still dialing and registered nothing itself; reuse keeps ids stable.
		if existing := a.findOpenTab(sshPathDisplay); existing != nil {
			return a.reuseOpenTab(existing)
		}
	}

	return stats, nil
}

// openSSHStreamOnce performs the actual connection+registration. It runs at
// most once concurrently per sshPathDisplay (see sshInflight).
func (a *App) openSSHStreamOnce(sshCfg SSHConfig, sshPathDisplay string, sshSummary *config.SSHConfigSummary, tabID string) (*FileStatsResponse, error) {
	// Progress bridge: pkg/ssh reports stages through this callback and the app
	// forwards them to the frontend overlay as Wails events.
	progress := func(stage string, done, total int64) {
		runtime.EventsEmit(a.ctx, "ssh:progress", map[string]any{
			"stage": stage,
			"done":  done,
			"total": total,
			"host":  sshCfg.Host,
		})
	}

	streamer, idx, err := a.openSSHResources(sshCfg, sshPathDisplay, progress)
	if err != nil {
		return nil, err
	}

	tmpPath := streamer.LocalPath()

	tabID = newTabID("tab_ssh", tabID)
	fileName := fmt.Sprintf("[%s] %s", sshCfg.Host, filepath.Base(sshCfg.RemoteFilePath))

	tab := &TabState{
		ID:               tabID,
		FilePath:         sshPathDisplay,
		FileName:         fileName,
		Indexer:          idx,
		Searcher:         search.NewSearchEngine(idx),
		MatchedLines:     make([]int64, 0),
		AutoScroll:       true,
		IsSSH:            true,
		TempFile:         tmpPath,
		SSHStreamer:      streamer,
		SSHConfigSummary: sshSummary,
	}

	a.mu.Lock()
	a.tabs[tabID] = tab
	a.activeTab = tabID
	a.mu.Unlock()

	return &FileStatsResponse{
		TabID:        tabID,
		FilePath:     tab.FilePath,
		FileName:     fileName,
		FileSize:     idx.FileSize(),
		TotalLines:   idx.TotalLines(),
		MatchedLines: 0,
		IsSSH:        true,
		SSH:          sshSummary,
	}, nil
}

// openSSHResources performs the blocking half of an SSH open — dial, handshake,
// full remote copy into a temp file, then the local index build — on its own
// goroutine bound to the app lifecycle context.
//
//nolint:dupl // symmetric SSH/FTP stream openers
func (a *App) openSSHResources(
	sshCfg SSHConfig,
	sshPathDisplay string,
	progress func(stage string, done, total int64),
) (*ssh.Streamer, *indexer.FileIndexer, error) {
	// Headless callers (tests, CLI paths) have no Wails context yet.
	base := a.ctx
	if base == nil {
		base = context.Background()
	}

	ctx, cancel := context.WithCancel(base)
	defer cancel()

	type opened struct {
		streamer *ssh.Streamer
		idx      *indexer.FileIndexer
		err      error
	}

	done := make(chan opened, 1)

	go func() {
		streamer, err := ssh.NewStreamerContext(ctx, ssh.Config{
			Host:           sshCfg.Host,
			Port:           sshCfg.Port,
			Username:       sshCfg.Username,
			AuthType:       sshCfg.AuthType,
			Password:       sshCfg.Password,
			KeyPath:        sshCfg.KeyPath,
			KeyPassphrase:  sshCfg.KeyPassphrase,
			RemoteFilePath: sshCfg.RemoteFilePath,
			KnownHostsPath: a.cfg.KnownHostsPath(),
			Progress:       progress,
		})
		if err != nil {
			// NewStreamer errors are already fully contextualized ("failed to
			// connect via SSH to %s: …", *HostKeyUnknownError, …); an extra
			// capitalized prefix would only duplicate context. Logged here — not
			// per-caller — because RestoreTab failures are otherwise invisible:
			// a silent skip looks exactly like "tabs are not restored".
			slog.Warn("ssh: stream open failed", "remote", sshPathDisplay, "auth_type", sshCfg.AuthType, "err", err)

			done <- opened{err: err}

			return
		}

		progress("indexing", streamer.Downloaded(), streamer.RemoteTotal())

		idx, err := indexer.NewFileIndexer(streamer.LocalPath())
		if err != nil {
			// Close asynchronously: a stuck SSH tail can make Streamer.Close block up
			// to its close timeout, and this runs while the UI waits on the open.
			closeRemoteStreamerAsync(streamer)

			done <- opened{err: err}

			return
		}

		done <- opened{streamer: streamer, idx: idx}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			return nil, nil, res.err
		}

		return res.streamer, res.idx, nil
	case <-base.Done():
		// Cancel the open and stop waiting for it. The goroutine keeps ownership
		// of whatever it produced and disposes of it, so shutting down mid-open
		// can never leak an SSH session, a temp file or an open fd.
		cancel()

		go func() {
			res := <-done
			if res.idx != nil {
				if cerr := res.idx.Close(); cerr != nil {
					slog.Warn("ssh: failed to close indexer of an abandoned open", "remote", sshPathDisplay, "err", cerr)
				}
			}

			closeRemoteStreamerAsync(res.streamer)
		}()

		slog.Warn("ssh: open abandoned because the app is shutting down", "remote", sshPathDisplay)

		return nil, nil, fmt.Errorf("ssh: open of %s aborted: %w", sshPathDisplay, base.Err())
	}
}
