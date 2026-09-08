package main

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/ftp"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/search"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ftpBrowseMu / ftpBrowseLister / ftpBrowseListerKey mirror the SSH browse
// cache (a.sshBrowseLister / a.sshBrowseListerKey) for the FTP/FTPS path. They
// are process-global because the App struct lives in app.go and must stay
// untouched; a single App instance is the only caller in practice. ftp.Lister
// wraps one non-concurrency-safe *ftp.Client, so ftpBrowseMu is held across the
// cache lookup and the blocking ListDir to ensure at most one in-flight listing
// on the shared client. It is never held while a.mu is held, so no deadlock.
var (
	ftpBrowseMu        sync.Mutex
	ftpBrowseLister    *ftp.Lister
	ftpBrowseListerKey string
)

// remotePathDisplay builds the canonical display URL for a remote file, e.g.
// "ssh://host:22/var/log/app.log" or "ftp://host:21/var/log/app.log". It omits
// any user@ prefix already present in host (matching the existing buildSSHURL
// convention: no credentials in the URL), so the path doubles as the tab's
// dedup key.
func remotePathDisplay(proto, host string, port int, path string) string {
	return fmt.Sprintf("%s://%s:%d%s", proto, host, port, path)
}

// resolveFTPCertStore returns the FTPS certificate trust store path to use for a
// connection: cfg.KnownHostsPath when set, otherwise the same default location
// pkg/ftp uses (~/.config/GoFLE/ftp_certs, or the temp dir if UserConfigDir is
// unavailable). It is mirrored here so AckFTPCert can write to the exact path
// the dial pinned against, without reaching into pkg/ftp internals.
func resolveFTPCertStore(cfg remote.Config) string {
	if cfg.KnownHostsPath != "" {
		return cfg.KnownHostsPath
	}

	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "GoFLE", "ftp_certs")
	}

	return filepath.Join(os.TempDir(), "GoFLE", "ftp_certs")
}

// OpenRemoteFile streams a remote log (ssh or ftp) into a local index and
// registers a tab. It is the protocol-agnostic entry point the frontend calls.
//
// The ssh path is delegated to OpenSSHFile, which keeps the existing host-key
// stash + recent-persist behavior (and its singleflight) byte-for-byte intact.
// The ftp path mirrors that flow: it dials via ftp.NewStreamer, indexes the
// streamed temp file, registers a tab, and on an unknown FTPS certificate stashes
// the connection so AckFTPCert can persist the trust and the frontend can retry.
func (a *App) OpenRemoteFile(cfg remote.Config) (*FileStatsResponse, error) {
	proto := cfg.Protocol
	if proto == "" {
		proto = remote.ProtocolSSH
	}

	switch proto {
	case remote.ProtocolSSH:
		// Reuse the existing, tested SSH open path unchanged.
		return a.OpenSSHFile(SSHConfig{
			Host:           cfg.Host,
			Port:           cfg.Port,
			Username:       cfg.Username,
			AuthType:       cfg.AuthType,
			Password:       cfg.Password,
			KeyPath:        cfg.KeyPath,
			KeyPassphrase:  cfg.KeyPassphrase,
			RemoteFilePath: cfg.RemoteFilePath,
		})
	case remote.ProtocolFTP:
		displayPath := remotePathDisplay(remote.ProtocolFTP, cfg.Host, cfg.Port, cfg.RemoteFilePath)
		if existing := a.findOpenTab(displayPath); existing != nil {
			return a.reuseOpenTab(existing)
		}

		ftpCfg := ftp.Config{
			Host:           cfg.Host,
			Port:           cfg.Port,
			Username:       cfg.Username,
			AuthType:       cfg.AuthType,
			Password:       cfg.Password,
			KeyPath:        cfg.KeyPath,
			KeyPassphrase:  cfg.KeyPassphrase,
			RemoteFilePath: cfg.RemoteFilePath,
			TLSMode:        cfg.TLSMode,
			KnownHostsPath: resolveFTPCertStore(cfg),
			Progress:       cfg.Progress,
			StallTimeout:   cfg.StallTimeout,
		}
		summary := &config.SSHConfigSummary{
			Host:           cfg.Host,
			Port:           cfg.Port,
			Username:       cfg.Username,
			RemoteFilePath: cfg.RemoteFilePath,
			AuthType:       cfg.AuthType,
			Password:       cfg.Password,
			KeyPath:        cfg.KeyPath,
			KeyPassphrase:  cfg.KeyPassphrase,
			Protocol:       remote.ProtocolFTP,
			TLSMode:        cfg.TLSMode,
		}

		stats, err := a.openFTPStream(ftpCfg, displayPath, summary, "")
		if err != nil {
			var certErr *ftp.CertUnknownError
			if errors.As(err, &certErr) {
				// Stash the connection + the cert store line so AckFTPCert can
				// persist the certificate and the frontend can re-open/re-list.
				a.mu.Lock()
				a.pendingFTPHost[certErr.Host] = certErr.Line
				a.pendingFTPConfig[certErr.Host] = cfg
				a.mu.Unlock()
			}

			return nil, err
		}

		// Only record the entry in the recent list after a real, successful
		// open (same policy as OpenSSHFile).
		if err := a.cfg.AddRecentFile(displayPath, 0, true, summary); err != nil {
			slog.Warn("failed to persist recent FTP entry", "path", displayPath, "err", err)
		}

		return stats, nil
	default:
		return nil, fmt.Errorf("unsupported remote protocol %q", proto)
	}
}

// openFTPStream mirrors openSSHStream: it collapses concurrent opens of the same
// remote target and delegates the actual connection+registration to
// openFTPStreamOnce. tabID may be empty (generate a fresh id) or explicit.
//
//nolint:dupl // symmetric SSH/FTP stream openers
func (a *App) openFTPStream(ftpCfg ftp.Config, displayPath string, summary *config.SSHConfigSummary, tabID string) (*FileStatsResponse, error) {
	if existing := a.findOpenTab(displayPath); existing != nil {
		return a.reuseOpenTab(existing)
	}

	v, err, sharedCall := a.ftpInflight.Do(displayPath, func() (any, error) {
		return a.openFTPStreamOnce(ftpCfg, displayPath, summary, tabID)
	})
	if err != nil {
		return nil, err
	}

	stats, ok := v.(*FileStatsResponse)
	if !ok || stats == nil {
		return nil, fmt.Errorf("ftp: unexpected inflight result for %s", displayPath)
	}

	if sharedCall {
		if existing := a.findOpenTab(displayPath); existing != nil {
			return a.reuseOpenTab(existing)
		}
	}

	return stats, nil
}

// openFTPStreamOnce performs the connection+registration for an FTP open. It
// mirrors openSSHStreamOnce exactly, differing only in the concrete streamer
// type, the tab-id prefix, the IsFTP flag, and the host shown in the progress
// overlay.
func (a *App) openFTPStreamOnce(ftpCfg ftp.Config, displayPath string, summary *config.SSHConfigSummary, tabID string) (*FileStatsResponse, error) {
	progress := func(stage string, done, total int64) {
		runtime.EventsEmit(a.ctx, "ssh:progress", map[string]any{
			"stage": stage,
			"done":  done,
			"total": total,
			"host":  ftpCfg.Host,
		})
	}

	streamer, idx, err := a.openFTPResources(ftpCfg, displayPath, progress)
	if err != nil {
		return nil, err
	}

	tmpPath := streamer.LocalPath()

	tabID = newTabID("tab_ftp", tabID)
	fileName := fmt.Sprintf("[%s] %s", ftpCfg.Host, filepath.Base(ftpCfg.RemoteFilePath))

	tab := &TabState{
		ID:               tabID,
		FilePath:         displayPath,
		FileName:         fileName,
		Indexer:          idx,
		Searcher:         search.NewSearchEngine(idx),
		MatchedLines:     make([]int64, 0),
		AutoScroll:       true,
		IsSSH:            true, // frontend treats any remote stream as a remote tab
		IsFTP:            true,
		TempFile:         tmpPath,
		FTPStreamer:      streamer,
		SSHConfigSummary: summary,
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
		IsFTP:        true,
		SSH:          summary,
	}, nil
}

// openFTPResources mirrors openSSHResources: it runs the blocking dial + initial
// sync + index build on a goroutine bound to the app lifecycle context, so a
// stalled connection can be abandoned on shutdown without leaking the client,
// temp file or fd. The context governs only the open; once the streamer exists
// its tail supervisor is driven by Streamer.Close.
//
//nolint:dupl // symmetric SSH/FTP stream openers
func (a *App) openFTPResources(
	ftpCfg ftp.Config,
	displayPath string,
	progress func(stage string, done, total int64),
) (*ftp.Streamer, *indexer.FileIndexer, error) {
	base := a.ctx
	if base == nil {
		base = context.Background()
	}

	ctx, cancel := context.WithCancel(base)
	defer cancel()

	type opened struct {
		streamer *ftp.Streamer
		idx      *indexer.FileIndexer
		err      error
	}

	done := make(chan opened, 1)

	go func() {
		streamer, err := ftp.NewStreamer(ctx, ftpCfg)
		if err != nil {
			slog.Warn("ftp: stream open failed", "remote", displayPath, "auth_type", ftpCfg.AuthType, "err", err)

			done <- opened{err: err}

			return
		}

		progress("indexing", streamer.Downloaded(), streamer.RemoteTotal())

		idx, err := indexer.NewFileIndexer(streamer.LocalPath())
		if err != nil {
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
		cancel()

		go func() {
			res := <-done
			if res.idx != nil {
				if cerr := res.idx.Close(); cerr != nil {
					slog.Warn("ftp: failed to close indexer of an abandoned open", "remote", displayPath, "err", cerr)
				}
			}

			closeRemoteStreamerAsync(res.streamer)
		}()

		slog.Warn("ftp: open abandoned because the app is shutting down", "remote", displayPath)

		return nil, nil, fmt.Errorf("ftp: open of %s aborted: %w", displayPath, base.Err())
	}
}

// ListRemoteFiles browses a remote host's directory (ssh or ftp) so the user can
// pick a log file interactively. It is the protocol-agnostic counterpart of
// ListSSHFiles. On an unknown host key (ssh) / unknown FTPS certificate (ftp) it
// stashes the connection in the matching pending* maps so the trust flow can
// persist it and the frontend can re-list.
//
//nolint:nestif // cached-lister reuse with dial/drop/else branch is inherently nested
func (a *App) ListRemoteFiles(cfg remote.Config, path string) ([]remote.DirEntry, error) {
	proto := cfg.Protocol
	if proto == "" {
		proto = remote.ProtocolSSH
	}

	switch proto {
	case remote.ProtocolSSH:
		return a.ListSSHFiles(SSHConfig{
			Host:           cfg.Host,
			Port:           cfg.Port,
			Username:       cfg.Username,
			AuthType:       cfg.AuthType,
			Password:       cfg.Password,
			KeyPath:        cfg.KeyPath,
			KeyPassphrase:  cfg.KeyPassphrase,
			RemoteFilePath: cfg.RemoteFilePath,
		}, path)
	case remote.ProtocolFTP:
		ftpCfg := ftp.Config{
			Host:           cfg.Host,
			Port:           cfg.Port,
			Username:       cfg.Username,
			AuthType:       cfg.AuthType,
			Password:       cfg.Password,
			KeyPath:        cfg.KeyPath,
			KeyPassphrase:  cfg.KeyPassphrase,
			RemoteFilePath: cfg.RemoteFilePath,
			TLSMode:        cfg.TLSMode,
			KnownHostsPath: resolveFTPCertStore(cfg),
		}
		key := fmt.Sprintf("%s:%d", ftpCfg.Host, ftpCfg.Port)

		// Reuse the cached lister for this host if we have one, mirroring the SSH
		// browse cache. ftpBrowseMu is held during the dial-free cache check and
		// the ListDir call so concurrent listings never share the client at once.
		ftpBrowseMu.Lock()

		var lister *ftp.Lister
		if ftpBrowseLister != nil && ftpBrowseListerKey == key {
			lister = ftpBrowseLister
		} else {
			// Drop the lock while dialing + logging in (may be slow), then re-check.
			ftpBrowseMu.Unlock()

			l, err := ftp.NewLister(ftpCfg)
			if err != nil {
				var certErr *ftp.CertUnknownError
				if errors.As(err, &certErr) {
					a.mu.Lock()
					a.pendingFTPHost[certErr.Host] = certErr.Line
					a.pendingFTPConfig[certErr.Host] = cfg
					a.mu.Unlock()
				}

				return nil, err
			}

			ftpBrowseMu.Lock()
			// Replace any stale cached lister for a different host.
			if ftpBrowseLister != nil && ftpBrowseListerKey != key {
				_ = ftpBrowseLister.Close()
			}

			ftpBrowseLister = l
			ftpBrowseListerKey = key
			lister = l
		}

		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}

		entries, err := lister.ListDir(ctx, path)
		if err != nil {
			// The connection died mid-session: drop the cache so the next
			// listing reconnects instead of failing on a dead client.
			if ftpBrowseLister == lister {
				ftpBrowseLister = nil
				ftpBrowseListerKey = ""
			}

			ftpBrowseMu.Unlock()

			_ = lister.Close()

			return nil, fmt.Errorf("failed to list remote directory %q: %w", path, err)
		}

		ftpBrowseMu.Unlock()

		return entries, nil
	default:
		return nil, fmt.Errorf("unsupported remote protocol %q", proto)
	}
}

// AckFTPCert is invoked from the UI after the user confirms a previously unknown
// FTPS server certificate. It writes the certificate into the trust store and
// clears the pending maps. Unlike AcceptSSHHost it does NOT open or index
// anything — the browser simply re-lists afterwards.
func (a *App) AckFTPCert(host string) error {
	a.mu.Lock()
	line, hasLine := a.pendingFTPHost[host]

	cfg, hasCfg := a.pendingFTPConfig[host]
	if hasLine {
		delete(a.pendingFTPHost, host)
	}

	if hasCfg {
		delete(a.pendingFTPConfig, host)
	}

	a.mu.Unlock()

	if !hasLine || !hasCfg {
		return fmt.Errorf("no pending FTP host to accept: %s", host)
	}

	storePath := resolveFTPCertStore(cfg)
	if err := ftp.WriteCertTrust(storePath, line); err != nil {
		return fmt.Errorf("failed to persist trusted FTP certificate: %w", err)
	}

	slog.Info("ftp: user trusted server certificate", "host", host)

	return nil
}
