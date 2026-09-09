// Package ssh streams remote log files over SSH into a local temp file and
// tails them incrementally.
//
// Streamer opens an SSH session, copies the remote file to a local temp file
// (which the indexer then treats as a regular local file), and polls the remote
// for appended bytes every 2 seconds. Host keys are verified against an OpenSSH
// known_hosts file; unknown keys surface as a HostKeyUnknownError for the UI to
// confirm.
//
// Lister is the browse-only counterpart: it shares the dial/auth/host-key path
// but only runs one-shot listing commands, so the user can pick a remote log
// interactively when no path was supplied. It implements remote.Lister and
// returns remote.DirEntry values, so browsing is identical across transports.
package ssh

import (
	"GoFLE/pkg/remote"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/sync/singleflight"
)

// HostKeyUnknownError is returned by NewStreamer when the server's host key is
// not present in the local known_hosts file. The UI surfaces Host + Fingerprint
// to the user; on acceptance the caller writes Line via WriteKnownHost and
// retries the connection.
type HostKeyUnknownError struct {
	Host        string
	Fingerprint string
	Line        string
}

// Error implements the error interface.
func (e *HostKeyUnknownError) Error() string {
	return fmt.Sprintf("unknown host key for %s (fingerprint %s)", e.Host, e.Fingerprint)
}

// HostKeyMismatchError is returned when the server presents a key that differs
// from the one recorded in known_hosts — a potential man-in-the-middle attack.
// It wraps the underlying knownhosts error so callers can still inspect the
// expected key via errors.As(&knownhosts.KeyError).
type HostKeyMismatchError struct {
	Host string
	Err  error
}

// Error implements the error interface.
func (e *HostKeyMismatchError) Error() string {
	return fmt.Sprintf("host key mismatch for %s: possible MITM", e.Host)
}

// Unwrap returns the wrapped knownhosts error so callers can inspect the
// expected key via errors.As.
func (e *HostKeyMismatchError) Unwrap() error { return e.Err }

// Config holds the parameters for opening an SSH stream: server address, authentication, and the remote file path to copy and tail locally.
type Config struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	AuthType       string `json:"auth_type"` // "password" or "key"
	Password       string `json:"password,omitempty"`
	KeyPath        string `json:"key_path,omitempty"`
	KeyPassphrase  string `json:"key_passphrase,omitempty"`
	RemoteFilePath string `json:"remote_file_path"`
	// KnownHostsPath points at the OpenSSH known_hosts file used to verify the
	// server's host key. Defaults to ~/.config/GoFLE/known_hosts when empty.
	KnownHostsPath string `json:"known_hosts_path,omitempty"`
	// Progress, when set, receives open-stage updates: ("connecting",0,0)
	// before the dial and throttled ("downloading", doneBytes, totalBytes)
	// during the initial copy. total is -1 when the remote size cannot be
	// probed; callers must degrade to an indeterminate indicator. Invoked from
	// the copy loop — implementations must be cheap and non-blocking.
	Progress func(stage string, done, total int64) `json:"-"`
	// StallTimeout bounds a SILENT initial download: if not a single byte
	// arrives for this long the copy is aborted instead of hanging forever on a
	// half-open/black-holed connection (TCP alone gives no read deadline here).
	// Progress resets the timer, so a slow-but-alive transfer of a huge log is
	// never cut off. Zero selects streamStallTimeout.
	StallTimeout time.Duration `json:"-"`
}

// shellQuote safely embeds s as a single literal argument for a remote shell
// command (B5). It wraps s in single quotes and escapes any embedded single
// quote by ending the quote, inserting an escaped quote, and reopening — the
// POSIX-shell idiom: a quote is closed, an escaped quote inserted, then reopened. This makes `cat`/`tail` treat the path as a
// literal filename even when it contains spaces or shell metacharacters
// (e.g. ";rm -rf /"), so a crafted path can no longer break out of the command
// and inject arbitrary shell code. An empty path is rejected earlier by
// NewStreamerContext's guard, so the command never degenerates into a bare
// "cat" reading stdin.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// expandPath resolves a leading "~" in a local file path. Go's os.ReadFile does
// not understand "~", so a config like "~/.ssh/id_rsa" would fail with
// "no such file". Only the local KeyPath needs expansion — RemoteFilePath is
// interpreted by the remote shell, which expands "~" server-side.
func expandPath(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}

		return p
	}

	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}

		return p
	}

	if strings.HasPrefix(p, "~") {
		// ~user/... form.
		rest := p[1:]

		name, tail, _ := strings.Cut(rest, "/")
		if u, err := user.Lookup(name); err == nil {
			if tail == "" {
				return u.HomeDir
			}

			return filepath.Join(u.HomeDir, tail)
		}

		return p
	}

	return p
}

// validateRemotePath checks the path used for SSH navigation/streaming. Both
// absolute paths (starting with "/") and relative paths are accepted — the
// remote login shell resolves a relative path against the user's home directory,
// so we must not require a leading "/". The only hard constraint is that the
// path contains no NUL or ASCII control characters, which can never be part of a
// real file name and must never reach the remote shell. An empty path is now
// VALID here: it means "no file chosen yet" and is what drives the interactive
// remote directory browser (the actual stream open still requires a concrete
// file via NewStreamerContext's guard below, so cat never degenerates into a
// bare "cat" reading stdin).
func validateRemotePath(p string) error {
	for i := range p {
		b := p[i]
		if b == 0 || b < 0x20 || b == 0x7f {
			return errors.New("validation failed: remote_file_path contains invalid control characters")
		}
	}

	return nil
}

// Validate centralizes all input validation for an SSH connection config. It is
// called at the top of NewStreamer so both the OpenSSHFile and AcceptSSHHost
// flows are covered. Returning early here prevents downstream hangs (e.g. an
// empty RemoteFilePath makes `cat` read stdin and block forever) and surfaces
// clear, structured errors instead of opaque library failures.
func (c Config) Validate() error {
	if c.Host == "" {
		return errors.New("validation failed: host is required")
	}

	if p := c.Port; p != 0 && (p < 1 || p > 65535) {
		return fmt.Errorf("validation failed: port must be between 1 and 65535, got %d", p)
	}

	if c.Username == "" {
		return errors.New("validation failed: username is required")
	}

	if err := validateRemotePath(c.RemoteFilePath); err != nil {
		return err
	}

	switch c.AuthType {
	case "password":
		if c.Password == "" {
			return errors.New("validation failed: password is required")
		}
	case "key":
		if c.KeyPath == "" {
			return errors.New("validation failed: key_path is required")
		}
	default:
		return fmt.Errorf("validation failed: unsupported auth_type %q (expected 'password' or 'key')", c.AuthType)
	}

	return nil
}

const (
	// streamProbeInterval: how often the supervisor probes remote size to
	// detect rotation (local truncation hands off to the poller's rebuild).
	streamProbeInterval = 30 * time.Second
	// streamCloseTimeout bounds Close's wait for the supervisor so a parked
	// open on a dead connection can never freeze the whole app.
	streamCloseTimeout = 8 * time.Second
	reconnectBaseDelay = 1 * time.Second
	reconnectMaxDelay  = 30 * time.Second
	// streamOpenContainmentTimeout bounds the parked-open containment goroutine
	// so a session that hangs on a dead connection (never resolving resCh/errCh)
	// cannot leak its goroutine forever once the open is abandoned.
	streamOpenContainmentTimeout = 30 * time.Second
	// streamStallTimeout is the default Config.StallTimeout: how long the
	// initial copy may deliver zero bytes before it is aborted. Detection
	// happens between one and two intervals after the last byte (progress is
	// sampled on a ticker), which is fine for a watchdog whose only job is to
	// stop an open from hanging forever.
	streamStallTimeout = 60 * time.Second
	// streamProbeTimeout bounds the tiny `wc -c` size probe. It is a
	// best-effort progress denominator, so a parked probe must degrade to
	// "unknown total" instead of delaying (or hanging) the open.
	streamProbeTimeout = 15 * time.Second
	// streamHandshakeTimeout bounds the SSH handshake+auth. ClientConfig.Timeout
	// only covers the TCP dial: a host that accepts the connection and then goes
	// silent would otherwise park the open forever inside NewClientConn.
	streamHandshakeTimeout = 30 * time.Second
	// streamStderrGrace bounds the wait for the remote stderr drain when the
	// remote command exited non-zero: the tail turns "exited with status 1" into
	// a named cause ("Permission denied"), but must never delay the failure.
	streamStderrGrace = 2 * time.Second
	// streamAbortGrace is how long an aborted transfer is given to unwind after
	// its session was closed. Closing the session normally unsticks the copy;
	// when the transport itself is black-holed the channel-close never round
	// trips, so the client connection is closed to fail the pending read.
	streamAbortGrace = 5 * time.Second
	// parallelSyncThreshold is the file size above which parallel range downloads
	// are used for the initial sync (100 MB).
	parallelSyncThreshold = 100 * 1024 * 1024
	// parallelSyncWorkers is the number of concurrent range downloads.
	parallelSyncWorkers = 4
)

// Streamer copies a remote log file over SSH into a local temp file and keeps it appended as the remote file grows, so the UI can index and tail it like a regular local file.
type Streamer struct {
	cfg     Config
	client  *ssh.Client
	tmpFile *os.File
	stopCh  chan struct{}
	closed  bool
	mu      sync.Mutex
	// openCtx is the caller's context for the OPEN PHASE only (NewStreamerContext):
	// dial, handshake, size probe, remote session open and the initial full-file
	// copy. It is cleared before the tail supervisor starts, because callers
	// legitimately scope the open with a context they cancel as soon as the open
	// returns (see the desktop's openSSHResources) — keeping it bound afterwards
	// would tear the live tail down the instant a successful open finished. Once
	// the streamer exists, its background work is governed by Close/stopCh.
	// Always read it through openContext(), which is nil-safe.
	openCtx context.Context //nolint:containedctx // openCtx is scoped to the open phase and cleared before background work; passing it through the supervisor would tear down the live tail
	// downloaded counts bytes copied into the temp file (initial `cat` plus
	// supervisor tail appends) for progress reporting.
	downloaded atomic.Int64
	// remoteTotal is the probed remote size at open time (-1 when unknown);
	// the denominator for download-progress display.
	remoteTotal int64
	// abortStream tears down the currently published tail session (killing the
	// remote `tail` process and unsticking a stalled copy). Nil between
	// streams. Published/read under s.mu; also invoked directly by Close.
	abortStream func()
	// supDone closes when the supervisor goroutine has fully exited, letting
	// Close bound its wait instead of hanging on a dead connection.
	supDone chan struct{}
	// Tunables (overridden in tests): how often the supervisor probes the
	// remote size for rotation, how long Close waits for the supervisor, and
	// the reconnect backoff base.
	probeInterval time.Duration
	closeTimeout  time.Duration
	reconnectBase time.Duration
	reconnectMax  time.Duration
	// Open-path watchdog tunables (overridden in tests). Each falls back to its
	// stream* default when zero, so a Streamer built field-by-field (tests) is
	// never left with a zero timeout that would fire immediately.
	stallTimeout time.Duration
	probeTimeout time.Duration
	abortGrace   time.Duration
	// newRemoteStream is the seam between the supervisor and the SSH transport:
	// real impl wraps client.NewSession; tests inject scripted streams.
	newRemoteStream func(cmd string) (stdout, stderr io.Reader, wait func() error, abort func(), err error)
	// sf deduplicates concurrent dials to the same address. Streamer previously
	// cached *ssh.Client per-address, but ssh.Client holds mutable transport
	// state and was raced by concurrent users (tail goroutine, searches) — the
	// root cause of intermittent connection corruption. singleflight gives the
	// dedup benefit (one dial serves all in-flight callers) without sharing a
	// single mutable client across goroutines.
	sf        *singleflight.Group
	localPath string
	// keepTempFile, when true, makes Close leave the local temp file on disk
	// (e.g. the UI needs it for a "Save As" export). By default Close removes
	// it, so a crash/panic between Close and an explicit CleanupTempFile can
	// never leak a temp file.
	keepTempFile bool
	// fetchFailures counts consecutive failed stream attempts for the warn
	// throttle. Touched only by the supervisor goroutine.
	fetchFailures int
}

// NewStreamer validates the config, dials the SSH server, performs the initial
// full-file sync into a local temp file, and starts a background goroutine that
// tails the remote file for appended bytes. The streamed local path is exposed
// via LocalPath and can be handed to a FileIndexer.
//
// Returns a *HostKeyUnknownError if the server's host key is not yet trusted;
// the caller should call WriteKnownHost with the error's Line and retry.
// Any other error means the connection or sync failed.
//
// It is NewStreamerContext with a background context: nothing can interrupt the
// open except its own timeouts. Long-lived callers (the desktop app) should pass
// their lifecycle context so a shutdown aborts an in-flight open.
func NewStreamer(cfg Config) (*Streamer, error) {
	return NewStreamerContext(context.Background(), cfg)
}

// NewStreamerContext is NewStreamer bound to ctx for the duration of the OPEN:
// cancelling ctx aborts the dial, the handshake, the size probe, the remote
// session open and the initial full-file copy, and the partially built streamer
// is torn down before the error is returned. Without this, an open of a remote
// log was uninterruptible — the dial has a timeout, but the handshake, the
// `NewSession` round-trip and the `cat` copy could park indefinitely on a
// half-open connection, so the caller (a Wails binding, or a CLI waiting on
// Ctrl+C) had no way to give up, and app shutdown could not reclaim the session,
// the fd or the temp file.
//
// ctx does NOT govern the streamer after the open returns: callers scope the open
// with a context they cancel as soon as it completes, so the live tail is driven
// by Close alone. Cancel the tail by calling Close.
func NewStreamerContext(ctx context.Context, cfg Config) (*Streamer, error) { //nolint:gocyclo // streamer open-phase setup is inherently branchy
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ssh: open cancelled before connecting: %w", err)
	}

	// Apply the implicit port default now that validation has passed.
	if cfg.Port == 0 {
		cfg.Port = 22
	}

	// An empty RemoteFilePath is valid for the config (it drives the interactive
	// directory browser), but streaming a file REQUIRES a concrete path. Return a
	// clear error before the `cat` so an empty value never degenerates into a
	// bare "cat" reading stdin and hanging forever — the old regression.
	if cfg.RemoteFilePath == "" {
		return nil, errors.New("validation failed: remote_file_path is required to stream a remote log")
	}

	var authMethods []ssh.AuthMethod

	// Build auth methods directly from the (already-validated) config. A shared
	// *ssh.Client must NOT be cached and reused across goroutines: ssh.Client
	// owns mutable transport/handshake state, and concurrent use (the tail
	// goroutine racing a reconnect or a search) corrupts that state. So each
	// dial yields a private client owned solely by this Streamer's tail
	// goroutine; concurrent dials to the same address are collapsed via
	// singleflight (getOrCreateClient) so we never open redundant connections
	// without ever sharing a single mutable client.
	switch cfg.AuthType { //nolint:dupl // symmetric auth-type branching
	case "password":
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	case "key":
		keyBytes, err := os.ReadFile(expandPath(cfg.KeyPath))
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}

		var signer ssh.Signer
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		authMethods = append(authMethods, ssh.PublicKeys(signer))
	default:
		// Unreachable after Config.Validate(), kept as a defensive guard with
		// the same vocabulary so both layers read identically.
		return nil, fmt.Errorf("unsupported auth_type %q", cfg.AuthType)
	}

	hostKeyCallback, err := newHostKeyCallback(cfg.KnownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SSH host key verification: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if cfg.Progress != nil {
		cfg.Progress("connecting", 0, 0)
	}

	s := &Streamer{
		cfg:         cfg,
		sf:          &singleflight.Group{},
		remoteTotal: -1,
		openCtx:     ctx,
	}

	client, err := s.getOrCreateClient(ctx, address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect via SSH to %s: %w", address, err)
	}

	slog.Info("ssh: established connection", "address", address)

	tmpFile, err := os.CreateTemp("", "glogg-ssh-*.log")
	if err != nil {
		if cerr := client.Close(); cerr != nil {
			slog.Warn("ssh: failed to close client after temp-file failure", "err", cerr)
		}

		return nil, fmt.Errorf("failed to create temp file for SSH stream: %w", err)
	}

	s.client = client
	s.tmpFile = tmpFile
	s.localPath = tmpFile.Name()
	s.stopCh = make(chan struct{})
	s.supDone = make(chan struct{})
	s.probeInterval = streamProbeInterval
	s.closeTimeout = streamCloseTimeout
	s.reconnectBase = reconnectBaseDelay
	s.reconnectMax = reconnectMaxDelay

	s.stallTimeout = cfg.StallTimeout
	if s.stallTimeout <= 0 {
		s.stallTimeout = streamStallTimeout
	}

	s.probeTimeout = streamProbeTimeout
	s.abortGrace = streamAbortGrace
	s.newRemoteStream = s.openRemoteStream

	// Best-effort total for determinate progress; a failed probe only degrades
	// the UI to an indeterminate spinner.
	if sz, ok := s.probeRemoteSize(); ok {
		s.remoteTotal = sz
	}

	if err := s.initialSync(ctx); err != nil {
		if cerr := s.Close(); cerr != nil {
			slog.Warn("ssh: failed to close streamer after initial sync failure", "err", cerr)
		}

		return nil, err
	}

	// The open is done: unbind ctx before the tail starts. Callers derive the open
	// context from their lifecycle and cancel it as soon as the open returns (see
	// the desktop's openSSHResources), so a streamer that stayed bound to it would
	// lose its live tail the moment it was successfully opened.
	s.endOpenPhase()

	go s.startSupervisor()

	return s, nil
}

// getOrCreateClient dials the SSH server for this streamer's address. It uses a
// singleflight.Group (s.sf) so concurrent callers — e.g. the initial connect
// racing with an early reconnect, or several in-flight searches — collapse into
// a single dial instead of opening a storm of parallel connections to the same
// host. The resulting *ssh.Client is private to this Streamer and must not be
// shared: ssh.Client owns mutable transport state. Searches read the already
// streamed local temp file rather than the live client, so a single owner (the
// tail goroutine) is sufficient.
func (s *Streamer) getOrCreateClient(ctx context.Context, address string, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	res, err, _ := s.sf.Do(address, func() (any, error) {
		return dialContext(ctx, address, sshConfig)
	})
	if err != nil {
		return nil, err
	}

	client, ok := res.(*ssh.Client)
	if !ok {
		return nil, fmt.Errorf("unexpected client type %T", res)
	}
	return client, nil //nolint:wsl_v5 // final return after typed assertion
}

// dialContext is ssh.Dial with two bounds it does not provide: ctx cancellation
// and a handshake deadline. ssh.Dial applies ClientConfig.Timeout to the TCP
// dial ONLY, so a host that completes the TCP connect and then stops responding
// (a silently dropped route, a wedged sshd) parks the caller inside the key
// exchange forever. Here the dial follows ctx, the handshake runs under a socket
// deadline, and a cancellation during the handshake closes the connection so the
// in-flight read fails immediately.
func dialContext(ctx context.Context, address string, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	d := net.Dialer{Timeout: sshConfig.Timeout}

	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(streamHandshakeTimeout)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to set handshake deadline: %w", err)
	}

	// Unblock a handshake parked on a silent peer as soon as ctx is cancelled.
	handshakeDone := make(chan struct{})
	defer close(handshakeDone)

	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-handshakeDone:
		}
	}()

	c, chans, reqs, err := ssh.NewClientConn(conn, address, sshConfig)
	if err != nil {
		_ = conn.Close()

		if ctxErr := ctx.Err(); ctxErr != nil {
			// A cancellation closes the socket, so the library error is just the
			// symptom ("use of closed network connection"): report the cause —
			// unless the handshake already produced a typed host-key error
			// (HostKeyUnknownError drives the UI trust prompt, HostKeyMismatchError
			// flags a possible MITM). Those must reach OpenSSHFile intact; wrapping
			// them as "handshake cancelled" would swallow the prompt under a
			// cancel-during-handshake race.
			var (
				hkUnknown  *HostKeyUnknownError
				hkMismatch *HostKeyMismatchError
			)

			if !errors.As(err, &hkUnknown) && !errors.As(err, &hkMismatch) {
				return nil, fmt.Errorf("handshake cancelled: %w", ctxErr)
			}
		}

		return nil, err
	}

	// Clear the one-shot handshake deadline: the tail stream must not inherit it,
	// or a quiet log would kill the connection after streamHandshakeTimeout.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("failed to clear handshake deadline: %w", err)
	}

	return ssh.NewClient(c, chans, reqs), nil
}

// newHostKeyCallback builds an ssh.HostKeyCallback backed by OpenSSH
// known_hosts. Unknown hosts return a *HostKeyUnknownError (carrying the line
// to append on user acceptance); a mismatch with a known host is treated as a
// potential MITM and returns a hard error. A missing known_hosts file is not an
// error: it simply means every host starts out unknown.
func newHostKeyCallback(path string) (ssh.HostKeyCallback, error) {
	base, err := knownhosts.New(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No known_hosts yet — start from an empty database so unknown hosts
			// are detected (and surfaced) rather than silently accepted.
			base = func(_ string, _ net.Addr, _ ssh.PublicKey) error {
				return &knownhosts.KeyError{}
			}
		} else {
			return nil, fmt.Errorf("failed to read known_hosts: %w", err)
		}
	}

	return func(host string, addr net.Addr, key ssh.PublicKey) error {
		if err := base(host, addr, key); err != nil {
			var ke *knownhosts.KeyError
			if errors.As(err, &ke) && len(ke.Want) == 0 {
				// Expected first-contact flow: the returned HostKeyUnknownError
				// drives the UI conversation, so only a debug trace is logged
				// here (the fact is handled exactly once — by the caller).
				slog.Debug("ssh: unknown host key, prompting user to verify",
					"host", host, "fingerprint", ssh.FingerprintSHA256(key))

				return &HostKeyUnknownError{
					Host:        host,
					Fingerprint: ssh.FingerprintSHA256(key),
					Line:        knownhosts.Line([]string{host}, key),
				}
			}

			slog.Error("ssh: HOST KEY MISMATCH — possible MITM, rejecting connection",
				"host", host, "fingerprint", ssh.FingerprintSHA256(key))

			return &HostKeyMismatchError{Host: host, Err: err}
		}

		return nil
	}, nil
}

// WriteKnownHost appends a known_hosts line (as produced by HostKeyUnknownError.Line)
// to the file at path, creating the file and its directory if necessary.
func WriteKnownHost(path, line string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create known_hosts dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // user-provided known_hosts path is intended
	if err != nil {
		return fmt.Errorf("failed to open known_hosts: %w", err)
	}

	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("ssh: failed to close known_hosts file", "err", cerr)
		}
	}()

	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("failed to write known_hosts: %w", err)
	}

	return nil
}

// LocalPath returns the path of the local temp file holding the streamed remote
// log. Pass it to indexer.NewFileIndexer to view/tail the remote file locally.
func (s *Streamer) LocalPath() string {
	return s.localPath
}

// Downloaded returns the number of bytes copied into the local temp file so far
// (initial copy + tail appends).
func (s *Streamer) Downloaded() int64 { return s.downloaded.Load() }

// RemoteTotal returns the probed remote size at open time, or -1 when unknown.
func (s *Streamer) RemoteTotal() int64 { return s.remoteTotal }

// progressThrottle bounds Progress-callback frequency during the bulk copy so
// the UI event channel is not flooded by 32 KB write granularity.
const progressThrottle = 200 * time.Millisecond

// countingWriter counts bytes flowing into the temp file and reports throttled
// download progress through cfg.Progress.
type countingWriter struct {
	w        io.Writer
	s        *Streamer
	lastEmit time.Time
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if n > 0 {
		done := cw.s.downloaded.Add(int64(n))
		cw.maybeEmit(done)
	}

	return n, err
}

func (cw *countingWriter) maybeEmit(done int64) {
	if cw.s.cfg.Progress == nil {
		return
	}

	now := time.Now()
	if now.Sub(cw.lastEmit) < progressThrottle {
		return
	}

	cw.lastEmit = now
	cw.s.cfg.Progress("downloading", done, cw.s.remoteTotal)
}

// initialSync performs the one-time full copy of the remote file into the local
// temp file via "cat" so the indexer has the complete contents before tailing
// begins. A failed remote command (missing file, permission denied) must fail
// the sync: discarding the exit status would leave an empty temp file that
// indexes as a valid-looking zero-line log.
//
// The copy is guarded (watchInitialCopy): it aborts when ctx is cancelled, when
// Close fires, or when the transfer stalls silently. It also goes through the
// newRemoteStream seam, which gives it the same parked-open containment as the
// tail supervisor — an unabortable io.Copy over an SSH pipe is exactly how an
// open used to hang forever with no way to give up.
//
// For files larger than parallelSyncThreshold (100 MB), uses parallel range
// downloads via multiple SSH sessions to saturate bandwidth.
func (s *Streamer) initialSync(ctx context.Context) error {
	// Use parallel sync for large files when we know the total size
	if s.remoteTotal >= parallelSyncThreshold {
		return s.initialSyncParallel(ctx)
	}

	return s.initialSyncSequential(ctx)
}

// initialSyncSequential is the original single-stream initial sync using "cat".
func (s *Streamer) initialSyncSequential(ctx context.Context) error {
	cmd := "cat -- " + shellQuote(s.cfg.RemoteFilePath)

	stdout, stderr, wait, abort, err := s.newRemoteStream(cmd)
	if err != nil {
		return err
	}
	// Best-effort teardown after the data is already transferred; a close
	// failure here carries no signal the caller could act on.
	defer abort()

	// Drain remote stderr so its tail can name the real cause ("cat: /var/log/x:
	// Permission denied") instead of leaving the caller with a bare non-zero exit
	// status. Shares drainStderr with the tail supervisor.
	stderrCh := drainStderr(stderr)

	// Counting wrapper drives the download-progress indicator; the first emit
	// happens immediately (0/N) so the UI switches from "connecting" at once.
	cw := &countingWriter{w: s.tmpFile, s: s}
	if s.cfg.Progress != nil {
		s.cfg.Progress("downloading", 0, s.remoteTotal)
	}

	copyDone := make(chan struct{})

	abortErr := make(chan error, 1)
	go s.watchInitialCopy(ctx, abort, copyDone, abortErr)

	_, cerr := io.Copy(cw, stdout)

	close(copyDone)

	if s.cfg.Progress != nil {
		s.cfg.Progress("downloading", s.downloaded.Load(), s.remoteTotal)
	}

	// A watchdog abort surfaces as an ordinary read/exit failure ("session
	// closed"), so its reason wins: it is the cause, not the symptom.
	select {
	case werr := <-abortErr:
		return werr
	default:
	}

	if cerr != nil && !errors.Is(cerr, io.EOF) {
		return fmt.Errorf("failed to read remote file stream: %w", cerr)
	}

	if err := wait(); err != nil {
		if tail := stderrTailOf(stderrCh, streamStderrGrace); tail != "" {
			return fmt.Errorf("remote cat %s failed: %w (remote stderr: %s)",
				s.cfg.RemoteFilePath, err, tail)
		}

		return fmt.Errorf("remote cat %s failed: %w", s.cfg.RemoteFilePath, err)
	}

	return nil
}

// initialSyncParallel performs the initial sync using concurrent range downloads
// via multiple SSH sessions. Each range is downloaded independently and written
// to the correct offset in the pre-allocated temp file.
func (s *Streamer) initialSyncParallel(ctx context.Context) error {
	if s.remoteTotal <= 0 {
		// Unknown size, fall back to sequential
		return s.initialSyncSequential(ctx)
	}

	// Pre-allocate the temp file to the full remote size
	if err := s.tmpFile.Truncate(s.remoteTotal); err != nil {
		slog.Warn("ssh: failed to pre-allocate temp file, falling back to sequential",
			"remote", s.cfg.RemoteFilePath, "err", err)
		return s.initialSyncSequential(ctx)
	}

	// Ensure we start at the beginning
	if _, err := s.tmpFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek temp file: %w", err)
	}

	// Calculate range boundaries
	rangeSize := s.remoteTotal / parallelSyncWorkers
	if rangeSize == 0 {
		rangeSize = s.remoteTotal
	}

	type rangeResult struct {
		index int
		start int64
		end   int64
		n     int64
		err   error
	}

	results := make(chan rangeResult, parallelSyncWorkers)
	var wg sync.WaitGroup

	// Progress tracking
	var totalDownloaded atomic.Int64
	progressDone := make(chan struct{})
	if s.cfg.Progress != nil {
		s.cfg.Progress("downloading", 0, s.remoteTotal)
		go func() {
			ticker := time.NewTicker(progressThrottle)
			defer ticker.Stop()
			for {
				select {
				case <-progressDone:
					return
				case <-ticker.C:
					done := totalDownloaded.Load()
					s.cfg.Progress("downloading", done, s.remoteTotal)
				}
			}
		}()
	}

	// Launch workers for each range
	for i := 0; i < parallelSyncWorkers; i++ {
		start := int64(i) * rangeSize
		end := start + rangeSize
		if i == parallelSyncWorkers-1 {
			end = s.remoteTotal // Last worker gets the remainder
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(idx int, rangeStart, rangeEnd int64) {
			defer wg.Done()

			n, err := s.downloadRange(ctx, rangeStart, rangeEnd)
			results <- rangeResult{index: idx, start: rangeStart, end: rangeEnd, n: n, err: err}
		}(i, start, end)
	}

	// Wait for all workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var totalBytes int64
	var firstErr error
	for r := range results {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		totalBytes += r.n
		totalDownloaded.Add(r.n)
	}

	close(progressDone)

	if s.cfg.Progress != nil {
		s.cfg.Progress("downloading", totalDownloaded.Load(), s.remoteTotal)
	}

	if firstErr != nil {
		return firstErr
	}

	// Verify total bytes match expected
	if totalBytes != s.remoteTotal {
		slog.Warn("ssh: parallel sync byte count mismatch",
			"expected", s.remoteTotal, "got", totalBytes,
			"remote", s.cfg.RemoteFilePath)
		// Not a hard error - the file might be slightly different
	}

	return nil
}

// downloadRange downloads a single byte range [start, end) of the remote file
// using `dd` and writes it to the correct offset in the temp file.
func (s *Streamer) downloadRange(ctx context.Context, start, end int64) (int64, error) {
	// Use `dd` for precise byte-range copying: dd if=/path bs=1 skip=start count=length
	// Note: `dd` with bs=1 is slow; we use a larger block size and calculate skip/count accordingly.
	blockSize := int64(64 * 1024) // 64KB blocks
	skipBlocks := start / blockSize
	skipBytes := start % blockSize
	countBytes := end - start
	countBlocks := (countBytes + blockSize - 1) / blockSize // ceiling

	// Adjust for partial first block
	cmd := fmt.Sprintf("dd if=%s bs=%d skip=%d count=%d 2>/dev/null",
		shellQuote(s.cfg.RemoteFilePath), blockSize, skipBlocks, countBlocks)

	stdout, stderr, wait, abort, err := s.newRemoteStream(cmd)
	if err != nil {
		return 0, err
	}
	defer abort()

	// Drain stderr to avoid blocking the remote command
	_ = drainStderr(stderr)

	// We need to handle the skipBytes offset in the first block
	// The dd command reads from skipBlocks*blockSize, so we need to skip the first skipBytes
	var reader io.Reader = stdout
	if skipBytes > 0 {
		// Wrap to skip the first skipBytes
		reader = &offsetReader{r: stdout, skip: skipBytes}
	}

	// Limit to exactly countBytes
	reader = io.LimitReader(reader, countBytes)

	// Write to temp file at the correct offset
	if _, err := s.tmpFile.Seek(start, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek temp file for range %d-%d: %w", start, end, err)
	}

	cw := &countingWriter{w: s.tmpFile, s: s}
	n, cerr := io.Copy(cw, reader)

	// Wait for the command to complete
	if err := wait(); err != nil {
		if cerr == nil {
			cerr = err
		}
	}

	if cerr != nil && !errors.Is(cerr, io.EOF) {
		return n, fmt.Errorf("failed to download range %d-%d: %w", start, end, cerr)
	}

	return n, nil
}

// offsetReader wraps an io.Reader to skip the first n bytes.
type offsetReader struct {
	r    io.Reader
	skip int64
}

func (or *offsetReader) Read(p []byte) (int, error) {
	if or.skip > 0 {
		skipBuf := make([]byte, min(int(or.skip), len(p)))
		n, err := or.r.Read(skipBuf)
		or.skip -= int64(n)
		if err != nil {
			return 0, err
		}
		if or.skip > 0 {
			// Need more data to skip
			return or.Read(p)
		}
		return or.r.Read(p)
	}
	return or.r.Read(p)
}

// drainStderr copies the remote command's stderr (bounded to 4 KiB) into a
// buffered channel and returns it. A nil stderr closes the channel immediately so
// callers can select on it unconditionally. Shared by initialSync and the tail
// supervisor so both surface remote failure causes identically.
func drainStderr(stderr io.Reader) <-chan string {
	ch := make(chan string, 1)

	if stderr != nil {
		go func() {
			b, _ := io.ReadAll(io.LimitReader(stderr, 4096))
			ch <- strings.TrimSpace(string(b))
		}()
	} else {
		close(ch)
	}

	return ch
}

// stderrTailOf returns the remote stderr tail (last 200 bytes), waiting up to
// grace for the drain to finish. It is consulted only after the remote command
// has already exited, so its stderr is at EOF and the drain is about to deliver;
// the bound merely stops a pathological stream from delaying the error report.
func stderrTailOf(ch <-chan string, grace time.Duration) string {
	select {
	case t := <-ch:
		if len(t) > 200 {
			t = t[len(t)-200:]
		}

		return t
	case <-time.After(grace):
		return ""
	}
}

// watchInitialCopy aborts the initial copy when the caller cancels ctx, when
// Close fires, or when the download stalls (no byte progress for stallTimeout).
// It reports the reason on abortErr and returns as soon as copyDone closes, so
// the normal path costs one goroutine for the duration of the transfer.
func (s *Streamer) watchInitialCopy(ctx context.Context, abort func(), copyDone <-chan struct{}, abortErr chan<- error) {
	stall := s.stallTimeout
	if stall <= 0 {
		stall = streamStallTimeout
	}

	ticker := time.NewTicker(stall)
	defer ticker.Stop()

	last := s.downloaded.Load()

	for {
		select {
		case <-copyDone:
			return
		case <-ctx.Done():
			abortErr <- fmt.Errorf("initial sync of %s cancelled after %d bytes: %w",
				s.cfg.RemoteFilePath, s.downloaded.Load(), ctx.Err())

			s.abortAndUnstick(abort, copyDone)

			return
		case <-s.stopCh:
			abortErr <- fmt.Errorf("streamer closed during initial sync of %s after %d bytes",
				s.cfg.RemoteFilePath, s.downloaded.Load())

			s.abortAndUnstick(abort, copyDone)

			return
		case <-ticker.C:
			now := s.downloaded.Load()
			if now != last {
				last = now
				continue
			}

			slog.Warn("ssh: initial sync stalled; aborting open",
				"remote", s.cfg.RemoteFilePath, "bytes_copied", now, "stall_timeout", stall)

			abortErr <- fmt.Errorf("initial sync of %s stalled: no data for %s (%d bytes copied)",
				s.cfg.RemoteFilePath, stall, now)

			s.abortAndUnstick(abort, copyDone)

			return
		}
	}
}

// abortAndUnstick closes the remote session and, if the copy is still parked a
// grace period later, closes the client transport too. Closing the session is
// enough for a live connection (the channel close ends the read), but a
// black-holed transport never round-trips that close, and the pending read would
// keep the open — and the caller — hanging. Only used on paths that already
// failed the open, so tearing the connection down is safe.
func (s *Streamer) abortAndUnstick(abort func(), copyDone <-chan struct{}) {
	abort()

	grace := s.abortGrace
	if grace <= 0 {
		grace = streamAbortGrace
	}

	select {
	case <-copyDone:
		return
	case <-time.After(grace):
	}

	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	if client == nil {
		return
	}

	slog.Warn("ssh: aborted copy still parked; closing connection to unstick it",
		"remote", s.cfg.RemoteFilePath)

	if err := client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		slog.Warn("ssh: failed to close client while unsticking a parked copy", "err", err)
	}
}

// startSupervisor owns the persistent remote tail: ONE
// `tail -c +<offset+1> -f` session streams appended bytes into the local temp
// file continuously. On stream death it reconnects with exponential backoff,
// resuming from the local end (never duplicating or gapping). Every
// probeInterval a tiny `wc -c` session detects remote ROTATION (size < our
// offset): the local file is truncated to zero and the stream restarts from
// byte 1 — the desktop poller observes the shrink and rebuilds its index.
// Exits promptly on Close. The deferred recover converts a panic in the
// untrusted-input streaming path into a logged terminal failure instead of
// crashing the whole GUI.
func (s *Streamer) startSupervisor() { //nolint:gocyclo,funlen // streamer lifecycle is inherently branchy
	defer close(s.supDone)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("ssh: tail supervisor panicked; tailing stopped for this stream",
				"remote", s.cfg.RemoteFilePath, "panic", r, "stack", string(debug.Stack()))
		}
	}()

	// Seed the offset from the temp file: initialSync already mirrored the
	// whole remote file before the supervisor starts. Starting at 0 would
	// re-download (and duplicate) the entire file.
	info, serr := s.tmpFile.Stat()
	if serr != nil {
		slog.Error("ssh: cannot stat temp file; live tail disabled",
			"remote", s.cfg.RemoteFilePath, "err", serr)

		return
	}

	offset := info.Size()
	fails := 0

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		cmd := fmt.Sprintf("tail -c +%d -f -- %s", offset+1, shellQuote(s.cfg.RemoteFilePath))

		stdout, stderr, wait, abort, err := s.newRemoteStream(cmd)
		if err != nil {
			fails++

			s.warnThrottled("ssh: failed to open tail stream", err, offset)

			if !s.backoffSleep(&fails) {
				return
			}

			continue
		}

		s.mu.Lock()

		if s.closed {
			abort()
			s.mu.Unlock()

			return
		}

		s.abortStream = abort
		s.mu.Unlock()

		pumpCh := make(chan struct {
			n   int64
			err error
		}, 1)

		go func() {
			n, perr := s.pump(stdout)
			pumpCh <- struct {
				n   int64
				err error
			}{n, perr}
		}()

		// Drain remote stderr so its tail becomes part of the failure diagnostic
		// (remote tail errors land here, not on stdout). Same path as initialSync.
		stderrCh := drainStderr(stderr)
		stderrTail := func() string {
			return stderrTailOf(stderrCh, streamStderrGrace)
		}

		probeTimer := time.NewTimer(s.probeInterval)

		var (
			perr     error
			n        int64
			rotating bool
			done     bool
			closed   bool
		)

	waitLoop:
		for {
			select {
			case r := <-pumpCh:
				n, perr = r.n, r.err
				done = true

				break waitLoop
			case <-probeTimer.C:
				if rsize, ok := s.probeRemoteSize(); ok && rsize < offset+n {
					rotating = true
					break waitLoop
				}

				probeTimer.Reset(s.probeInterval)
			case <-s.stopCh:
				closed = true
				break waitLoop
			}
		}

		probeTimer.Stop()

		// Unpublish, terminate the remote tail, collect the pump outcome.
		s.mu.Lock()
		// Unconditional clear is safe: only this goroutine publishes streams,
		// and Close's abort() on a dead session is an idempotent no-op.
		s.abortStream = nil
		closed = closed || s.closed
		s.mu.Unlock()
		abort()

		werr := wait()

		if !done {
			r := <-pumpCh
			n += r.n
			perr = r.err
		}

		offset += n

		switch {
		case closed:
			return
		case rotating:
			// Remote was rotated/truncated: restart the mirror from byte 1 of
			// the new file. The poller sees the local shrink and rebuilds.
			if terr := s.tmpFile.Truncate(0); terr != nil {
				s.warnThrottled("ssh: failed to truncate temp file after rotation", terr, 0)

				if !s.backoffSleep(&fails) {
					return
				}

				continue
			}

			if _, serr := s.tmpFile.Seek(0, io.SeekStart); serr != nil {
				s.warnThrottled("ssh: failed to rewind temp file after rotation", serr, 0)
			}

			offset = 0
			fails = 0

			continue
		case perr != nil && !errors.Is(perr, io.EOF):
			fails++

			s.warnAlways("ssh: tail stream read failed; reconnecting from local end", perr, offset, stderrTail())

			if !s.backoffSleep(&fails) {
				return
			}
		case werr != nil:
			fails++

			s.warnAlways("ssh: tail stream exited abnormally (remote stderr follows)", werr, offset, stderrTail())

			if !s.backoffSleep(&fails) {
				return
			}
		default:
			// Clean EOF on a `-f` stream means the connection or remote process
			// died silently. Reconnect after backoff.
			fails++

			s.warnAlways("ssh: tail stream ended unexpectedly; reconnecting", nil, offset, stderrTail())

			if !s.backoffSleep(&fails) {
				return
			}
		}
	}
}

// probeRemoteSize asks the remote for the file's current size. Uses wc -c
// (POSIX) rather than stat -c so the probe also works on BSD/macOS remotes.
// The read is bounded: the probe only feeds the progress denominator and the
// supervisor's rotation check, so a session parked on a dead connection must
// degrade to "unknown" instead of blocking the open (or the supervisor loop) on
// an io.ReadAll that never returns.
func (s *Streamer) probeRemoteSize() (int64, bool) {
	stdout, stderr, wait, abort, err := s.newRemoteStream(
		"wc -c < " + shellQuote(s.cfg.RemoteFilePath))
	_ = stderr

	if err != nil {
		return 0, false
	}

	defer abort()

	// Buffered so the reader can always finish and exit, even after a timeout.
	out := make(chan string, 1)

	go func() {
		// A size is a handful of digits; the limit keeps a chatty or hostile
		// remote from turning the probe into an unbounded read.
		data, _ := io.ReadAll(io.LimitReader(stdout, 64))
		_ = wait()

		out <- strings.TrimSpace(string(data))
	}()

	timeout := s.probeTimeout
	if timeout <= 0 {
		timeout = streamProbeTimeout
	}

	select {
	case text := <-out:
		sz, perr := strconv.ParseInt(text, 10, 64)
		if perr != nil {
			return 0, false
		}

		return sz, true
	case <-time.After(timeout):
		// The deferred abort tears the parked session down, which unblocks the
		// reader goroutine so it cannot outlive this streamer.
		slog.Warn("ssh: remote size probe timed out", "remote", s.cfg.RemoteFilePath, "timeout", timeout)
		return 0, false
	case <-s.stopCh:
		return 0, false
	case <-s.openDone():
		// The probe also runs on the open path (it feeds the progress
		// denominator), where a cancelled open must not have to wait out the
		// probe timeout — tens of seconds against an unresponsive host — before
		// the caller gives up. After the open this channel is nil.
		return 0, false
	}
}

// backoffSleep sleeps an exponentially increasing delay between reconnect
// attempts (1s..30s cap). Returns false if Close fired during the sleep.
func (s *Streamer) backoffSleep(fails *int) bool {
	delay := s.reconnectBase << min(*fails-1, 4)
	if delay > s.reconnectMax || delay <= 0 {
		delay = s.reconnectMax
	}

	select {
	case <-time.After(delay):
		return true
	case <-s.stopCh:
		return false
	}
}

// warnAlways logs every stream failure WITH the remote stderr tail: the first
// diagnostic after a connect regression must name the cause, not a counter.
// Later repeats fall back to the throttled policy so a dead link cannot flood
// the sink.
func (s *Streamer) warnAlways(msg string, err error, n int64, stderrTail string) {
	s.fetchFailures++
	if s.fetchFailures == 1 || s.fetchFailures%120 == 0 {
		args := []any{
			"remote", s.cfg.RemoteFilePath, "bytes_copied", n,
			"consecutive_failures", s.fetchFailures, "err", err,
		}
		if stderrTail != "" {
			args = append(args, "remote_stderr", stderrTail)
		}

		slog.Warn(msg, args...)
	}
}

// pump appends everything stdout delivers to the end of the temp file. Called
// from a supervisor-owned goroutine per stream; tmpFile writes are serialized
// by the supervisor lifecycle (one live stream at a time). The copy itself is not
// directly interruptible — it is unblocked by tearing its session down, which the
// supervisor does when Close fires.
func (s *Streamer) pump(stdout io.Reader) (int64, error) {
	if _, err := s.tmpFile.Seek(0, io.SeekEnd); err != nil {
		return 0, fmt.Errorf("failed to seek temp file: %w", err)
	}

	n, err := io.Copy(s.tmpFile, stdout)

	return n, err
}

// openRemoteStream is the real newRemoteStream: one SSH exec session running
// cmd, with stdout piped. The returned abort kills the session (terminating
// the remote process). Opening runs in a helper goroutine so Close, or a
// cancelled open context, can abandon a NewSession parked on a black-holed
// connection instead of hanging forever.
func (s *Streamer) openRemoteStream(cmd string) (io.Reader, io.Reader, func() error, func(), error) { //nolint:gocyclo // streamer lifecycle is inherently branchy
	type opened struct {
		stdout io.Reader
		stderr io.Reader
		wait   func() error
		abort  func()
	}

	resCh := make(chan opened, 1)
	errCh := make(chan error, 1)

	go func() {
		sess, err := s.client.NewSession()
		if err != nil {
			errCh <- fmt.Errorf("failed to open SSH session: %w", err)
			return
		}

		stdout, err := sess.StdoutPipe()
		if err != nil {
			_ = sess.Close()

			errCh <- fmt.Errorf("failed to open stdout pipe: %w", err)

			return
		}

		stderr, _ := sess.StderrPipe()
		if err := sess.Start(cmd); err != nil {
			_ = sess.Close()

			errCh <- fmt.Errorf("failed to start remote command: %w", err)

			return
		}

		resCh <- opened{
			stdout: stdout,
			stderr: stderr,
			wait:   sess.Wait,
			abort:  func() { _ = sess.Close() },
		}
	}()
	// Snapshot the open-phase context once: this call either belongs to the open
	// (context live) or to the supervisor (context already cleared, nil channel).
	openCtx := s.openContext()

	var openDone <-chan struct{}
	if openCtx != nil {
		openDone = openCtx.Done()
	}
	// Parked-open containment: drain whichever result lands eventually and tear
	// it down, so nothing outlives the abandoned open except the dead connection
	// itself. The goroutine is bounded by the open context, the stop channel,
	// and a hard timeout so a session parked on a dead connection (that never
	// sends a result to resCh/errCh) cannot leak the goroutine forever.
	abandon := func() {
		go func() {
			// Prefer tearing down a completed session that landed just as we bailed,
			// so it is never skipped by a ready cancel/stop case below.
			select {
			case o := <-resCh:
				o.abort()
				return
			default:
			}

			select {
			case o := <-resCh:
				o.abort()
			case <-errCh:
			case <-openDone:
			case <-s.stopCh:
			case <-time.After(streamOpenContainmentTimeout):
			}
		}()
	}

	select {
	case o := <-resCh:
		return o.stdout, o.stderr, o.wait, o.abort, nil
	case e := <-errCh:
		return nil, nil, nil, nil, e
	case <-s.stopCh:
		abandon()
		return nil, nil, nil, nil, errors.New("streamer closed while opening remote stream")
	case <-openDone:
		// The open itself is a round-trip that can park on a silent peer, and it
		// runs BEFORE the copy watchdog exists (initialSync opens the `cat`
		// session first), so without this case a cancelled context could not
		// interrupt an open at all — the very hang the context was added for.
		abandon()
		return nil, nil, nil, nil, fmt.Errorf("ssh: open of remote stream cancelled: %w", openCtx.Err())
	}
}

// openContext returns the open-phase context, or nil once the open has finished
// (endOpenPhase) or when the streamer was built field-by-field in tests. Guarded
// by s.mu because the supervisor goroutine reads it too.
func (s *Streamer) openContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.openCtx
}

// openDone returns the open-phase context's Done channel, or nil outside the open
// phase. A nil channel blocks forever in a select, so callers can list the case
// unconditionally and get "no cancellation source" for free.
func (s *Streamer) openDone() <-chan struct{} {
	ctx := s.openContext()
	if ctx == nil {
		return nil
	}

	return ctx.Done()
}

// endOpenPhase unbinds the open context, handing the streamer over to Close-driven
// teardown. It runs after the initial copy succeeds and before the supervisor
// starts: callers scope the open with a context they cancel as soon as the open
// returns, so leaving it bound would kill the live tail immediately.
func (s *Streamer) endOpenPhase() {
	s.mu.Lock()
	s.openCtx = nil
	s.mu.Unlock()
}

// warnThrottled logs at most once per 120 consecutive failed attempts so a
// persistently dead connection cannot flood the log sink (~720 lines/day/tab
// vs ~43k unthrottled). Only the supervisor goroutine calls this, so
// fetchFailures needs no lock of its own.
func (s *Streamer) warnThrottled(msg string, err error, n int64) {
	s.fetchFailures++
	if s.fetchFailures != 1 && s.fetchFailures%120 != 0 {
		return
	}

	slog.Warn(msg, "remote", s.cfg.RemoteFilePath,
		"bytes_copied", n, "consecutive_failures", s.fetchFailures, "err", err)
}

// KeepTempFile opts a Streamer out of the automatic temp-file cleanup performed
// by Close. Call this when the UI still needs the local temp file after the
// Streamer is closed (e.g. for a "Save As" export of the streamed log).
func (s *Streamer) KeepTempFile() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.keepTempFile = true
}

// Close stops the tailing goroutine, closes the SSH client, closes the local
// temp file handle, and — unless KeepTempFile was called — removes the local
// temp file from disk. Safe to call multiple times (idempotent).
func (s *Streamer) Close() error { //nolint:gocyclo // streamer lifecycle is inherently branchy
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()
		return nil
	}

	s.closed = true
	close(s.stopCh)

	// Abort the published tail session: closing its channel terminates the
	// remote tail process and unsticks a stalled copy, letting the supervisor
	// observe stopCh and exit.
	if s.abortStream != nil {
		s.abortStream()
		s.abortStream = nil
	}

	// Wait is BOUNDED: a dead connection could otherwise park an open/pump
	// indefinitely and freeze every Wails binding that needs s.mu behind
	// CloseTab/Shutdown (the io-F1 hang). On timeout we proceed anyway,
	// logging loudly — worst case one leaked goroutine/connection instead of
	// a frozen app.
	s.mu.Unlock()

	select {
	case <-s.supDone:
	case <-time.After(s.closeTimeout):
		slog.Error("ssh: timed out waiting for tail supervisor; proceeding with teardown",
			"remote", s.cfg.RemoteFilePath)
	}

	s.mu.Lock()

	var closeErr error

	if s.tmpFile != nil {
		if err := s.tmpFile.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("temp file: %w", err))
		}
	}

	if s.client != nil {
		// net.ErrClosed is expected when a failed open already tore the
		// transport down to unstick a parked copy (abortAndUnstick).
		if err := s.client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			closeErr = errors.Join(closeErr, fmt.Errorf("client: %w", err))
		}
	}

	if !s.keepTempFile && s.localPath != "" {
		if err := os.Remove(s.localPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			closeErr = errors.Join(closeErr, fmt.Errorf("temp file remove: %w", err))
		}
	}

	s.mu.Unlock()

	return closeErr
}

// CleanupTempFile removes the local temp file created by NewStreamer. It is a
// no-op once Close has already removed the file (the normal case); callers that
// previously relied on a two-step Close+Cleanup flow continue to work, and
// calling it after Close is harmless. Unlike Close, this always removes the
// file — KeepTempFile only defers deletion away from Close so the UI can still
// export the streamed log before calling CleanupTempFile explicitly.
func (s *Streamer) CleanupTempFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.localPath == "" {
		return nil
	}

	if err := os.Remove(s.localPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove temp file: %w", err)
	}

	return nil
}

// --- Lister: browse-only remote directory navigation ----------------------

// DirEntry is one entry of a remote directory listing. It aliases
// remote.DirEntry so every remote backend (ssh, ftp) hands the application layer
// the exact same type and the browser UI stays transport-agnostic.
type DirEntry = remote.DirEntry

// Lister is a browse-only SSH connection. It dials, authenticates and verifies
// the host key exactly like NewStreamerContext, but never runs cat/tail, never
// creates a temp file and never builds an index: it only executes bounded,
// one-shot listing commands so the user can pick a remote log interactively when
// no path was supplied. The single *ssh.Client is private to this Lister (an
// ssh.Client owns mutable transport state and must not be shared) and is reused
// across ListDir/HomeDir, so navigating directories costs no extra dial.
type Lister struct {
	cfg    Config
	client *ssh.Client
	// stopCh closes on Close, unsticking a listing parked on a dead connection.
	stopCh chan struct{}
	closed bool
	mu     sync.Mutex
	// newRemoteCommand is the seam between a listing and the SSH transport: the
	// real impl opens one exec session on l.client (openRemoteCommand); tests
	// inject scripted output. Mirrors Streamer.newRemoteStream, but for one-shot
	// commands whose whole stdout is read and parsed.
	newRemoteCommand func(cmd string) (stdout, stderr io.Reader, wait func() error, abort func(), err error)
}

// Lister satisfies the transport-agnostic browse contract.
var _ remote.Lister = (*Lister)(nil)

// NewLister validates cfg, dials the SSH server and verifies its host key,
// returning a connection ready for ListDir/HomeDir. An empty
// Config.RemoteFilePath is expected here — browsing is exactly how the user
// picks that path — and nothing is downloaded: there is no initial sync.
//
// Like NewStreamer it returns a *HostKeyUnknownError when the server's host key
// is not yet trusted, so the UI's trust flow (WriteKnownHost + retry) is
// identical for browsing and for opening a stream.
func NewLister(cfg Config) (*Lister, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.Port == 0 {
		cfg.Port = 22
	}

	var authMethods []ssh.AuthMethod

	switch cfg.AuthType { //nolint:dupl // symmetric auth-type branching
	case "password":
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	case "key":
		keyBytes, err := os.ReadFile(expandPath(cfg.KeyPath))
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}

		var signer ssh.Signer
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		authMethods = append(authMethods, ssh.PublicKeys(signer))
	default:
		// Unreachable after Config.Validate(); kept as a defensive guard with
		// the same vocabulary NewStreamerContext uses.
		return nil, fmt.Errorf("unsupported auth_type %q", cfg.AuthType)
	}

	hostKeyCallback, err := newHostKeyCallback(cfg.KnownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SSH host key verification: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	// dialContext bounds the handshake and surfaces the typed host-key errors
	// intact (*HostKeyUnknownError drives the UI trust prompt).
	client, err := dialContext(context.Background(), address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect via SSH to %s: %w", address, err)
	}

	l := &Lister{cfg: cfg, client: client, stopCh: make(chan struct{})}
	l.newRemoteCommand = l.openRemoteCommand

	return l, nil
}

// openRemoteCommand is the real newRemoteCommand: one SSH exec session running
// cmd with stdout/stderr piped. The returned abort closes the session, which
// terminates the remote process. It stays deliberately simpler than
// Streamer.openRemoteStream: a listing is a short, bounded command, and both
// cancellation and Close are handled one level up in runCommand.
func (l *Lister) openRemoteCommand(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
	l.mu.Lock()
	client := l.client
	l.mu.Unlock()

	if client == nil {
		return nil, nil, nil, nil, errors.New("ssh: lister is closed")
	}

	sess, err := client.NewSession()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to open SSH session: %w", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	stderr, _ := sess.StderrPipe()
	if err := sess.Start(cmd); err != nil {
		_ = sess.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to start remote command: %w", err)
	}

	return stdout, stderr, sess.Wait, func() { _ = sess.Close() }, nil
}

// HomeDir resolves the remote user's home directory by letting the remote login
// shell expand "~". It is the browser's start directory and the target of an
// empty path.
func (l *Lister) HomeDir(ctx context.Context) (string, error) {
	out, err := l.runCommand(ctx, "echo ~")
	if err != nil {
		return "", fmt.Errorf("failed to resolve remote home dir: %w", err)
	}

	home := strings.TrimSpace(string(out))
	if home == "" {
		return "", errors.New("failed to resolve remote home dir: empty response")
	}

	return home, nil
}

// ListDir lists the remote directory at path, one level deep. An empty path or
// "~" resolves to the remote home directory first. Entries are sorted with
// directories first, then symlinks, then files, each group alphabetically; "."
// and ".." are never returned (walking up is the caller's job, e.g. a ".." row).
// path reaches the remote shell ONLY through shellQuote, and control characters
// are rejected outright, so a crafted path can neither escape the quoted
// argument nor smuggle control bytes into the command.
func (l *Lister) ListDir(ctx context.Context, path string) ([]remote.DirEntry, error) {
	if path == "" || path == "~" {
		home, err := l.HomeDir(ctx)
		if err != nil {
			return nil, err
		}

		path = home
	}

	if err := validateRemotePath(path); err != nil {
		return nil, err
	}

	// GNU coreutils: print every entry one level deep as a NUL-terminated
	// "type<TAB>fullpath<TAB>size<TAB>mtime" record. %y is d/f/l. NUL+tab keep
	// names containing spaces or newlines intact. BSD/macOS find lacks -printf,
	// so a GNU (Linux) remote is a documented requirement.
	cmd := fmt.Sprintf("find %s -maxdepth 1 -mindepth 1 -printf '%%y\\t%%p\\t%%s\\t%%T@\\0'", shellQuote(path))

	out, err := l.runCommand(ctx, cmd)
	if err != nil {
		if isBusyBoxFindError(err) {
			return nil, fmt.Errorf("failed to list remote directory %s: the remote shell uses BusyBox (e.g. Alpine Linux) whose `find` does not support `-printf`; install the full GNU findutils package on the server (e.g. `apk add findutils` on Alpine) so the SSH browser can list directories: %w", path, err)
		}

		return nil, fmt.Errorf("failed to list remote directory %s: %w", path, err)
	}

	entries := parseFindOutput(out)
	sortRemoteEntries(entries)

	return entries, nil
}

// Close stops any in-flight listing and closes the SSH client. Safe to call
// multiple times (idempotent).
func (l *Lister) Close() error {
	l.mu.Lock()

	if l.closed {
		l.mu.Unlock()
		return nil
	}

	l.closed = true
	if l.stopCh != nil {
		close(l.stopCh)
	}

	client := l.client
	l.client = nil
	l.mu.Unlock()

	if client == nil {
		return nil
	}
	// net.ErrClosed is expected when the transport was already torn down (e.g.
	// a dead connection dropped by the caller before Close).
	if err := client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("failed to close SSH lister client: %w", err)
	}

	return nil
}

// runCommand executes one bounded remote command and returns its stdout. The
// command runs in a helper goroutine so a cancelled ctx or a concurrent Close
// abandons a session parked on a black-holed connection instead of hanging the
// caller; the abandoned session is torn down as soon as it materializes.
func (l *Lister) runCommand(ctx context.Context, cmd string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ssh: remote command cancelled before start: %w", err)
	}

	l.mu.Lock()
	closed, stopCh := l.closed, l.stopCh
	l.mu.Unlock()

	if closed {
		return nil, errors.New("ssh: lister is closed")
	}

	type result struct {
		out []byte
		err error
	}
	// Both channels are buffered so an abandoned goroutine always completes and
	// exits instead of blocking forever on an unread send.
	resCh := make(chan result, 1)
	abortCh := make(chan func(), 1)

	go func() {
		stdout, stderr, wait, abort, err := l.newRemoteCommand(cmd)
		if err != nil {
			resCh <- result{err: err}
			return
		}

		abortCh <- abort

		out, derr := drainRemoteCommand(stdout, stderr, wait)

		abort()

		resCh <- result{out: out, err: derr}
	}()

	select {
	case r := <-resCh:
		return r.out, r.err
	case <-ctx.Done():
		slog.Debug("ssh: abandoning remote listing (context cancelled)",
			"host", l.cfg.Host, "err", ctx.Err())
		abandonRemoteCommand(abortCh)

		return nil, fmt.Errorf("ssh: remote command cancelled: %w", ctx.Err())
	case <-stopCh:
		slog.Debug("ssh: abandoning remote listing (lister closed)", "host", l.cfg.Host)
		abandonRemoteCommand(abortCh)

		return nil, errors.New("ssh: lister closed while running a remote command")
	}
}

// abandonRemoteCommand tears down a remote command whose caller already gave up:
// aborting the session unsticks the parked read so the worker goroutine cannot
// outlive the listing. The wait is bounded because a session parked inside
// NewSession on a dead connection may never publish its abort at all.
func abandonRemoteCommand(abortCh <-chan func()) {
	go func() {
		select {
		case abort := <-abortCh:
			abort()
		case <-time.After(streamOpenContainmentTimeout):
		}
	}()
}

// drainRemoteCommand reads a one-shot command's stdout to EOF and folds the
// remote stderr tail into the error when the command exits non-zero, so a
// listing failure names its cause ("No such file or directory") instead of a
// bare exit status.
func drainRemoteCommand(stdout, stderr io.Reader, wait func() error) ([]byte, error) {
	stderrCh := drainStderr(stderr)

	out, rerr := io.ReadAll(stdout)
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return nil, rerr
	}

	if werr := wait(); werr != nil {
		if tail := stderrTailOf(stderrCh, streamStderrGrace); tail != "" {
			return nil, fmt.Errorf("%w (remote stderr: %s)", werr, tail)
		}

		return nil, werr
	}

	return out, nil
}

// parseFindOutput decodes the NUL-terminated, tab-separated records produced by
// the GNU `find -printf` listing. Each record is type<TAB>path<TAB>size<TAB>mtime;
// a path containing literal tabs is reconstructed by rejoining the middle fields,
// because size and mtime are always the trailing two numeric tokens. Malformed
// records are skipped: a partially readable directory is still worth showing.
func parseFindOutput(out []byte) []remote.DirEntry {
	var entries []remote.DirEntry

	for rec := range bytes.SplitSeq(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		// Split on every tab: a name may contain a literal tab, which the find
		// command emits as a tab separator, so the path can be spread across
		// several fields. The last two fields are always size and mtime;
		// everything between the kind (field 0) and those is the (possibly
		// tab-split) path, which must be rejoined to keep Path intact. Decode
		// on bytes — only the needed fields are converted to string, never a
		// full string(rec) copy (LOW alloc reduction).
		parts := bytes.Split(rec, []byte("\t"))
		if len(parts) < 4 {
			continue
		}

		kind := string(parts[0])
		sizeStr := string(parts[len(parts)-2])
		mtimeStr := string(parts[len(parts)-1])
		// Rejoin the middle fields into the full path, preserving literal tabs.
		var pathBuf []byte

		for i := 1; i < len(parts)-2; i++ {
			if i > 1 {
				pathBuf = append(pathBuf, '\t')
			}

			pathBuf = append(pathBuf, parts[i]...)
		}

		fullPath := string(pathBuf)

		var rkind string

		switch kind {
		case "d":
			rkind = "dir"
		case "l":
			rkind = "symlink"
		default:
			// Unknown type char (socket, fifo, ...) is treated conservatively as
			// a plain file: selectable, never navigable.
			rkind = "file"
		}

		size, _ := strconv.ParseInt(sizeStr, 10, 64)
		// %T@ is "seconds.fraction"; the browser only needs whole seconds.
		mtime, _ := strconv.ParseFloat(mtimeStr, 64)

		entries = append(entries, remote.DirEntry{
			Name:    filepath.Base(fullPath),
			Path:    fullPath,
			Kind:    rkind,
			Size:    size,
			ModTime: int64(mtime),
		})
	}

	return entries
}

// sortRemoteEntries orders directories first, then symlinks, then files (each
// group alphabetical, case-insensitive), so the browser shows a stable listing
// with the navigable rows on top.
func sortRemoteEntries(entries []remote.DirEntry) {
	rank := func(kind string) int {
		switch kind {
		case "dir":
			return 0
		case "symlink":
			return 1
		default:
			return 2
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if ri, rj := rank(entries[i].Kind), rank(entries[j].Kind); ri != rj {
			return ri < rj
		}

		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

// isBusyBoxFindError reports whether a listing failed because the remote `find`
// is BusyBox's (e.g. on Alpine Linux), which does not implement the GNU -printf
// flag the browser relies on. The caller turns this into an actionable hint
// instead of a raw BusyBox usage dump.
func isBusyBoxFindError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return (strings.Contains(msg, "busybox") || strings.Contains(msg, "unrecognized: -printf")) &&
		strings.Contains(msg, "-printf")
}
