// Package main is the GoFLE desktop log viewer backend (Wails app). Its exported
// App methods are bound to the frontend and form the backend API surface.
package main

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/ftp"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/search"
	"GoFLE/pkg/ssh"
	"GoFLE/pkg/version"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ErrTabNotFound is returned by tab-scoped operations when the tab id does not
// exist (e.g. it was closed concurrently).
var ErrTabNotFound = errors.New("tab not found")

// FileStatsResponse carries file/tab statistics returned to the frontend after any open/restore, including the SSH summary needed to persist the tab.
type FileStatsResponse struct {
	TabID        string `json:"tab_id"`
	FilePath     string `json:"file_path"`
	FileName     string `json:"file_name"`
	FileSize     int64  `json:"file_size"`
	TotalLines   int64  `json:"total_lines"`
	MatchedLines int64  `json:"matched_lines"`
	IsSSH        bool   `json:"is_ssh"`
	// IsFTP is true when the backing stream is FTP (plain or FTPS). IsSSH stays
	// true for BOTH ssh and ftp remote streams: the frontend keys remote-stream
	// handling off IsSSH, so a ftp tab must also report IsSSH=true. IsFTP lets
	// the UI distinguish the transport (e.g. for the trust flow).
	IsFTP bool `json:"is_ftp"`
	// SSH is returned for SSH-backed tabs so the frontend can persist the
	// connection in the session snapshot and restore the tab after a restart.
	SSH *config.SSHConfigSummary `json:"ssh,omitempty"`
}

// LogLineResponse is one line returned to the virtualized scroller, identified by its absolute line index.
type LogLineResponse struct {
	Index   int64  `json:"index"`
	Content string `json:"content"`
	// IsMatch is false for context-only neighbor lines expanded around a hit.
	IsMatch bool `json:"is_match"`
}

// RegexValidationResult reports whether a user pattern is compilable and, if not, a human-readable reason for the UI.
type RegexValidationResult struct {
	IsValid      bool   `json:"is_valid"`
	ErrorMessage string `json:"error_message"`
}

// TabState is the in-memory state of one open tab (local or SSH): its indexer, searcher, matched-line set, and live-tail bookkeeping (searchGen, pollSkip).
type TabState struct {
	ID       string
	FilePath string
	FileName string
	// gzCleanup removes the decompressed temp file backing a .gz tab; nil for
	// plain files. Invoked from CloseTab/Shutdown.
	gzCleanup    func()
	Indexer      *indexer.FileIndexer
	Searcher     *search.SearchEngine
	MatchedLines []int64
	MatchSet     map[int64]struct{} // true search hits (before context expand)
	// PrimaryMatches is the last full-file (primary) result before SecondaryQuery
	// was applied. Used to refine the AND-filter in O(matches) via FilterLines
	// instead of rescanning the whole file.
	PrimaryMatches []int64
	AutoScroll     bool
	LastSearchOpts search.SearchOptions
	// searchCancel cancels an in-flight SearchLines for this tab when the user
	// types a new query (debounce fires again). Guarded by searchCancelMu.
	searchCancel     context.CancelFunc
	searchCancelMu   sync.Mutex
	IsSSH            bool
	IsFTP            bool
	TempFile         string
	SSHStreamer      *ssh.Streamer
	FTPStreamer      *ftp.Streamer
	SSHConfigSummary *config.SSHConfigSummary
	mu               sync.RWMutex
	// pollFailures counts consecutive UpdateIndex failures for the tab so the
	// first failure can be surfaced (event + log) without spamming on every
	// 500 ms tick afterwards. Guarded by t.mu.
	pollFailures int
	// searchGen is bumped on every UI-initiated search so the live-tail poll
	// can detect (and discard) stale appends that raced a newer full search.
	searchGen int64
	// pollSkip counts ticker ticks to skip before polling this tab again. It
	// implements adaptive backoff (O3): an idle tab (no new lines) is polled
	// less frequently so the 500 ms ticker doesn't waste CPU re-statting files
	// that never change, while an active tab is polled every tick.
	pollSkip int
	// statsMu serializes GetLevelStats for this tab and guards levels. It is
	// deliberately separate from mu: the counting pass reads the whole file on
	// its first run, and blocking rename/close/poll bookkeeping for that long
	// would stall the UI.
	statsMu sync.Mutex
	// levels is the incrementally maintained level-marker count for this tab
	// (see GetLevelStats). Guarded by statsMu.
	levels levelStats
}

// levelStats is the cached level-marker count for one tab: how far the count
// has consumed the file (scanned lines, exclusive) and which indexer generation
// those line indices belong to, so a rotation can invalidate it instead of
// silently mixing counts from two different files.
type levelStats struct {
	gen     uint64
	scanned int64
	errors  int64
	warns   int64
}

// App is the Wails backend: it owns all tabs, the 500 ms tail-poller, and the SSH host-key trust handshake. Exported methods are the frontend API.
//
//nolint:containedctx // App is a long-lived Wails model holding the request context
type App struct {
	ctx          context.Context
	cfg          *config.AppConfig
	tabs         map[string]*TabState
	activeTab    string
	ticker       *time.Ticker
	stopTicker   chan struct{}
	startupArgs  []string
	mu           sync.RWMutex
	shutdownOnce sync.Once
	// pendingSSHHost / pendingSSHConfig bridge the two-phase SSH open flow.
	// When a host key is unknown, OpenSSHFile stores the (host -> known_hosts
	// line) and (host -> connection config) here under a mutex; AcceptSSHHost
	// consumes them after the user trusts the host.
	pendingSSHHost   map[string]string
	pendingSSHConfig map[string]SSHConfig
	// pendingFTPHost / pendingFTPConfig mirror the SSH pair for the FTP/FTPS
	// trust flow: when an explicit/implicit FTPS cert is unknown, OpenRemoteFile
	// / ListRemoteFiles store the (host -> cert store line) and (host ->
	// connection config) here; AckFTPCert consumes them after the user trusts
	// the server.
	pendingFTPHost   map[string]string
	pendingFTPConfig map[string]remote.Config
	// sshBrowseLister caches a single *ssh.Lister for the active remote-directory
	// browser session so navigating directories does not re-dial (and re-auth)
	// the server on every listing. It is reused while the (host,port) matches and
	// dropped (and the connection closed) on error or when a different host is
	// browsed. Guarded by mu.
	sshBrowseLister    *ssh.Lister
	sshBrowseListerKey string
	// sshInflight collapses concurrent opens of the SAME remote target into one
	// connection: findOpenTab only sees already-registered tabs, so overlapping
	// opens (double-click on a recent entry, session restore racing a manual
	// open, AcceptSSHHost after an interim success) used to register two tabs.
	sshInflight singleflight.Group
	// ftpInflight collapses concurrent ftp opens of the SAME remote target into
	// one connection, exactly like sshInflight does for ssh.
	ftpInflight singleflight.Group
	// connAttempts tracks failed connection attempts per host for rate limiting.
	// Key is "host:port", value is attempt info.
	connAttemptsMu sync.Mutex
	connAttempts   map[string]*connAttemptInfo
	// hub broadcasts events to web UI WebSocket clients. Always non-nil.
	hub *EventHub
	// webMode is true when running as HTTP server (no Wails runtime).
	webMode bool
}

// connAttemptInfo tracks connection attempts for rate limiting.
type connAttemptInfo struct {
	count       int
	lastAttempt time.Time
	lockedUntil time.Time
}

func (a *App) connLimitParams() (maxAttempts int, window, lockout time.Duration) {
	maxAttempts = 5
	window = 15 * time.Minute
	lockout = 5 * time.Minute
	if a.cfg == nil {
		return
	}
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()
	if a.cfg.ConnMaxAttempts > 0 {
		maxAttempts = a.cfg.ConnMaxAttempts
	}
	if a.cfg.ConnAttemptWindowMin > 0 {
		window = time.Duration(a.cfg.ConnAttemptWindowMin) * time.Minute
	}
	if a.cfg.ConnLockoutMin > 0 {
		lockout = time.Duration(a.cfg.ConnLockoutMin) * time.Minute
	}
	return
}

// NewApp constructs the application, loading persisted configuration. Returns
// an error if the config directory or secret key cannot be initialized.
func NewApp() (*App, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	return &App{
		cfg:              cfg,
		tabs:             make(map[string]*TabState),
		stopTicker:       make(chan struct{}),
		pendingSSHHost:   make(map[string]string),
		pendingSSHConfig: make(map[string]SSHConfig),
		pendingFTPHost:   make(map[string]string),
		pendingFTPConfig: make(map[string]remote.Config),
		connAttempts:     make(map[string]*connAttemptInfo),
		hub:              NewEventHub(),
	}, nil
}

// emitEvent delivers an event to the Wails runtime (desktop) and/or the web hub.
func (a *App) emitEvent(name string, data any) {
	if a == nil {
		return
	}
	if a.hub != nil {
		a.hub.Broadcast(name, data)
	}
	if !a.webMode && a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// onSecondInstanceLaunch is invoked by Wails SingleInstanceLock when the user
// launches GoFLE again (e.g. "Open with" on a log file) while an instance is
// already running. Paths from the second process are opened as tabs in this
// window; a second OS window is never created.
//
// Wails sets SecondInstanceData.Args to os.Args[1:] (no executable name) on
// all platforms. Relative paths are resolved against WorkingDirectory when set.
func (a *App) onSecondInstanceLaunch(data options.SecondInstanceData) {
	if a == nil || a.ctx == nil {
		return
	}

	// Bring the existing window forward (Wails does not focus automatically).
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)

	paths := extractFilePathArgsIn(data.Args, data.WorkingDirectory)
	slog.Info("second instance launch",
		"args", data.Args,
		"cwd", data.WorkingDirectory,
		"resolved_paths", paths,
	)
	if len(paths) == 0 {
		return
	}

	// Open on the backend first so tab state exists even if the UI event is delayed.
	responses, err := a.OpenFiles(paths)
	if err != nil {
		slog.Warn("second-instance open failed", "err", err, "paths", paths)
	}

	// Prefer paths so the frontend can call OpenFiles (same path as drag-drop);
	// include any stats already produced as a fallback.
	a.emitEvent("app:open-files", map[string]any{
		"paths": paths,
		"files": responses,
	})
}

// Startup is the Wails lifecycle hook: it captures the runtime context, parses
// any file arguments from the OS, and starts the 500 ms tail-poller goroutine.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.startupArgs = extractFilePathArgs(os.Args)
	a.ticker = time.NewTicker(500 * time.Millisecond)

	go func() {
		for {
			select {
			case <-a.ticker.C:
				// A panic in one poll tick (untrusted file/SSH input below)
				// must not kill the whole GUI: recover per tick so the poller
				// keeps serving the remaining tabs.
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("panic in file poller tick", "panic", r, "stack", string(debug.Stack()))
						}
					}()

					a.pollFileUpdates()
				}()
			case <-a.stopTicker:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// closeRemoteStreamerAsync tears down a remote (SSH or FTP) streamer in the
// background. Streamer.Close can block up to its close timeout waiting for a
// stuck tail supervisor to release a dead connection, so running it
// synchronously on the UI thread would freeze tab close / app shutdown while an
// open remote log is being torn down. The streamer is idempotent, so a
// concurrent or repeated call is harmless. It accepts the transport-agnostic
// remote.Streamer so both SSH and FTP tabs route through the same teardown.
func closeRemoteStreamerAsync(s remote.Streamer) {
	if s == nil {
		return
	}

	go func() {
		if err := s.Close(); err != nil {
			slog.Warn("closeRemoteStreamerAsync: streamer close failed", "err", err)
		}
	}()
}

// GetVersion returns the application version.
func (a *App) GetVersion() string {
	return version.Get().String()
}

// remoteTimeouts returns connect and stall timeouts from settings (with defaults).
func (a *App) remoteTimeouts() (connect, stall time.Duration) {
	connect = 10 * time.Second
	stall = 60 * time.Second
	if a.cfg == nil {
		return
	}
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()
	if a.cfg.SSHConnectTimeoutSec > 0 {
		connect = time.Duration(a.cfg.SSHConnectTimeoutSec) * time.Second
	}
	if a.cfg.RemoteStallTimeoutSec > 0 {
		stall = time.Duration(a.cfg.RemoteStallTimeoutSec) * time.Second
	}
	return
}

// autoAcceptHostKey reports whether unknown host keys/certs should be trusted automatically.
func (a *App) autoAcceptHostKey() bool {
	if a.cfg == nil {
		return false
	}
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()
	return a.cfg.AutoAcceptHostKey
}

// checkConnRateLimit checks if a connection to the given host:port is allowed.
// Returns an error if rate limited, otherwise returns nil and records the attempt start.
func (a *App) checkConnRateLimit(host string, port int) error {
	maxAttempts, window, lockout := a.connLimitParams()
	key := fmt.Sprintf("%s:%d", host, port)
	now := time.Now()

	a.connAttemptsMu.Lock()
	defer a.connAttemptsMu.Unlock()

	info, exists := a.connAttempts[key]
	if !exists {
		a.connAttempts[key] = &connAttemptInfo{
			count:       1,
			lastAttempt: now,
		}
		return nil
	}

	// Check if locked out
	if now.Before(info.lockedUntil) {
		return fmt.Errorf("connection to %s rate limited: try again after %v",
			key, time.Until(info.lockedUntil).Round(time.Second))
	}

	// Reset count if outside the window
	if now.Sub(info.lastAttempt) > window {
		info.count = 1
		info.lastAttempt = now
		return nil
	}

	// Increment count
	info.count++
	info.lastAttempt = now

	if info.count > maxAttempts {
		info.lockedUntil = now.Add(lockout)
		return fmt.Errorf("too many failed connection attempts to %s: locked for %v",
			key, lockout)
	}

	return nil
}

// recordConnFailure records a failed connection attempt for rate limiting.
func (a *App) recordConnFailure(host string, port int) {
	maxAttempts, _, lockout := a.connLimitParams()
	key := fmt.Sprintf("%s:%d", host, port)
	now := time.Now()

	a.connAttemptsMu.Lock()
	defer a.connAttemptsMu.Unlock()

	info, exists := a.connAttempts[key]
	if !exists {
		a.connAttempts[key] = &connAttemptInfo{
			count:       1,
			lastAttempt: now,
		}
		return
	}

	info.count++
	info.lastAttempt = now
	if info.count > maxAttempts {
		info.lockedUntil = now.Add(lockout)
	}
}

// recordConnSuccess resets the attempt count on successful connection.
func (a *App) recordConnSuccess(host string, port int) {
	key := fmt.Sprintf("%s:%d", host, port)

	a.connAttemptsMu.Lock()
	defer a.connAttemptsMu.Unlock()

	delete(a.connAttempts, key)
}

// Shutdown is the Wails lifecycle hook: it stops the tail poller (idempotent via
// sync.Once), closes every tab's indexer and SSH streamer, and clears the saved
// session when "save tabs on exit" is disabled. It does not overwrite the
// frontend-owned session snapshot.
func (a *App) Shutdown(_ context.Context) {
	// close on a channel is not idempotent — a second Shutdown would panic.
	// Guard with Once so duplicate lifecycle calls can't crash the app.
	a.shutdownOnce.Do(func() { close(a.stopTicker) })

	if a.ticker != nil {
		a.ticker.Stop()
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, tab := range a.tabs {
		tab.mu.Lock()

		if err := tab.Indexer.Close(); err != nil {
			slog.Warn("shutdown: failed to close indexer", "tab_id", tab.ID, "err", err)
		}

		if tab.gzCleanup != nil {
			tab.gzCleanup()
		}

		if tab.SSHStreamer != nil {
			// Close asynchronously: a stuck SSH tail can make Streamer.Close block
			// up to its close timeout, which must not freeze app shutdown while
			// holding the tab/app locks.
			s := tab.SSHStreamer
			tab.SSHStreamer = nil

			closeRemoteStreamerAsync(s)
		}

		tab.mu.Unlock()
	}

	// The frontend owns the session snapshot (order, groups, collapsed state) and
	// persists it on every structural change, so Go must NOT overwrite it here
	// with a map-derived, group-less list. The only responsibility left is to
	// honour the "save tabs on exit" opt-out.
	if !a.cfg.SaveTabsExit {
		if err := a.cfg.ClearSavedSession(); err != nil {
			slog.Warn("shutdown: failed to clear saved session", "err", err)
		}
	}
}

// maxRegexLen bounds the size of a user-supplied regular expression. Go's
// regexp is RE2 (no catastrophic backtracking), but an unbounded pattern still
// costs CPU on every line of a multi-million-line log, so we reject anything
// larger than this up front (ReDoS hardening).
const maxRegexLen = 256

// tabIDSeq is an atomic counter that disambiguates tab ids created within the same clock tick so back-to-back opens never collide.
var tabIDSeq atomic.Uint64
