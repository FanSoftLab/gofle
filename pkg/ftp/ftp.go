// Package ftp streams remote log files over FTP/FTPS into a local temp file
// and tails them incrementally, mirroring the shape of pkg/ssh.
//
// Streamer opens an FTP connection, copies the remote file to a local temp file
// (which the indexer then treats as a regular local file), and polls the remote
// for appended bytes every streamProbeInterval. FTPS (explicit or implicit) pins
// the server certificate against a local trust store (ftp_certs); an unknown
// certificate surfaces as a CertUnknownError for the UI to confirm, exactly like
// ssh's HostKeyUnknownError against known_hosts.
package ftp

import (
	"GoFLE/pkg/remote"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jlaffaye/ftp"
)

// CertUnknownError is returned by NewStreamer/NewLister when the server's FTPS
// certificate is not present in the local trust store. The UI surfaces Host +
// Fingerprint to the user; on acceptance the caller writes Line via
// WriteCertTrust and retries the connection.
type CertUnknownError struct {
	Host        string
	Fingerprint string
	Line        string
}

// Error implements the error interface.
func (e *CertUnknownError) Error() string {
	return fmt.Sprintf("unknown FTP server certificate for %s (fingerprint %s)", e.Host, e.Fingerprint)
}

// CertMismatchError is returned when the server presents a certificate that
// differs from the one recorded in the trust store — a potential man-in-the-middle
// attack.
type CertMismatchError struct {
	Host        string
	Fingerprint string
	Expected    string
}

// Error implements the error interface.
func (e *CertMismatchError) Error() string {
	return fmt.Sprintf("FTP server certificate mismatch for %s: possible MITM", e.Host)
}

// Config holds the parameters for opening an FTP stream or browse connection.
// It mirrors ssh.Config but adds TLSMode and reuses KnownHostsPath as the FTPS
// certificate trust store path.
type Config struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	AuthType       string `json:"auth_type"` // "password" (only password auth is supported over FTP)
	Password       string `json:"password,omitempty"`
	KeyPath        string `json:"key_path,omitempty"`
	KeyPassphrase  string `json:"key_passphrase,omitempty"`
	RemoteFilePath string `json:"remote_file_path"`
// TLSMode selects the FTP transport security: "" (default) = "explicit" FTPS,
// "explicit" = FTPS with AUTH TLS on the control connection, "implicit" =
// TLS from the first byte of the control connection, "none" = plain FTP
// (credentials sent in cleartext; not recommended).
	TLSMode string `json:"tls_mode,omitempty"`
	// KnownHostsPath points at the FTPS certificate trust store (one
	// "host:port <sha256-fingerprint>" line per host). Defaults to
	// ~/.config/gofle/ftp_certs when empty.
	KnownHostsPath string `json:"known_hosts_path,omitempty"`
	// Progress, when set, receives open-stage updates: ("connecting",0,0) before
	// the dial and throttled ("downloading", doneBytes, totalBytes) during the
	// initial copy. total is -1 when the remote size cannot be probed.
	Progress func(stage string, done, total int64) `json:"-"`
	// StallTimeout bounds a silent initial download. Zero selects
	// streamStallTimeout.
	StallTimeout time.Duration `json:"-"`
	// ConnectTimeout bounds the FTP dial (and TLS handshake when applicable).
	// Zero selects a 15s default.
	ConnectTimeout time.Duration `json:"-"`
}

// validateRemotePath checks the path used for FTP navigation/streaming. Both
// absolute and relative paths are accepted; the only hard constraint is that
// the path contains no NUL or ASCII control characters, which can never be part
// of a real file name and must never reach the server.
func validateRemotePath(p string) error {
	for i := range p {
		b := p[i]
		if b == 0 || b < 0x20 || b == 0x7f {
			return errors.New("validation failed: remote_file_path contains invalid control characters")
		}
	}

	return nil
}

// Validate centralizes all input validation for an FTP connection config. An
// empty RemoteFilePath is VALID here (it drives the interactive directory
// browser); only NewStreamer additionally requires a concrete file path.
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

	if c.Password == "" {
		return errors.New("validation failed: password is required")
	}

	switch c.TLSMode {
	case "", "none", "explicit", "implicit":
	default:
		return fmt.Errorf("validation failed: unsupported tls_mode %q (expected '', 'none', 'explicit' or 'implicit')", c.TLSMode)
	}

	return nil
}

const (
	streamProbeInterval = 30 * time.Second
	streamCloseTimeout  = 8 * time.Second
	streamStallTimeout  = 60 * time.Second
	// parallelSyncThreshold is the file size above which parallel range downloads
	// are used for the initial sync (100 MB).
	parallelSyncThreshold = 100 * 1024 * 1024
	// parallelSyncWorkers is the number of concurrent range downloads.
	parallelSyncWorkers = 4
)

// resolveCertStore returns the trust store path to use: cfg.KnownHostsPath when
// set, otherwise ~/.config/gofle/ftp_certs (architecture-agnostic app dir).
func resolveCertStore(cfg Config) string {
	if cfg.KnownHostsPath != "" {
		return cfg.KnownHostsPath
	}

	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "gofle", "ftp_certs")
	}

	return filepath.Join(os.TempDir(), "gofle", "ftp_certs")
}

// certStoreKey returns the store line key ("host:port") for an address.
func certStoreKey(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

// readCertStore parses the trust store file into a host->fingerprint map. A
// missing file yields an empty map (every host starts out unknown). Comment and
// blank lines are ignored.
func readCertStore(path string) (map[string]string, error) {
	m := map[string]string{}

	data, err := os.ReadFile(path) //nolint:gosec // user-provided path is intended
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m, nil
		}

		return nil, fmt.Errorf("failed to read cert trust store: %w", err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		m[fields[0]] = fields[1]
	}

	return m, nil
}

// newCertVerifier returns a tls.Config.VerifyPeerCertificate that pins the
// server leaf certificate against the trust store at storePath for the given
// host. With InsecureSkipVerify=true the standard chain verification is skipped,
// but VerifyPeerCertificate still runs: a matching fingerprint passes, an absent
// host returns *CertUnknownError, and a differing fingerprint returns
// *CertMismatchError (potential MITM).
func newCertVerifier(storePath, host string) func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("ftp: server presented no certificate")
		}

		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("ftp: failed to parse server certificate: %w", err)
		}

		fp := sha256.Sum256(leaf.Raw)
		hexFP := hex.EncodeToString(fp[:])

		store, err := readCertStore(storePath)
		if err != nil {
			return err
		}

		want, ok := store[host]
		if !ok {
			return &CertUnknownError{
				Host:        host,
				Fingerprint: hexFP,
				Line:        fmt.Sprintf("%s %s", host, hexFP),
			}
		}

		if !strings.EqualFold(want, hexFP) {
			return &CertMismatchError{Host: host, Fingerprint: hexFP, Expected: want}
		}

		return nil
	}
}

// WriteCertTrust appends a trust store line (as produced by
// CertUnknownError.Line) to the file at path, creating the file and its
// directory if necessary.
func WriteCertTrust(path, line string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create cert trust store dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // user-provided path is intended
	if err != nil {
		return fmt.Errorf("failed to open cert trust store: %w", err)
	}

	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("ftp: failed to close cert trust store", "err", cerr)
		}
	}()

	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("failed to write cert trust store: %w", err)
	}

	return nil
}

// dialClient connects to the FTP server per cfg: plain, explicit FTPS, or
// implicit FTPS. For FTPS the control connection uses InsecureSkipVerify with a
// VerifyPeerCertificate that pins the server certificate (newCertVerifier).
// Empty TLSMode defaults to "explicit" (FTPS) for security; use "none" to
// explicitly opt in to plaintext FTP.
func dialClient(cfg Config) (*ftp.ServerConn, error) {
	port := cfg.Port
	if port == 0 {
		port = 21
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	tlsMode := cfg.TLSMode
	if tlsMode == "" {
		tlsMode = "explicit" // default to FTPS for security
	}

	switch tlsMode {
	case "none":
		slog.Warn("ftp: using plaintext FTP (no TLS); credentials sent in cleartext",
			"host", cfg.Host, "port", port)
		return ftp.Dial(addr, ftp.DialWithTimeout(timeout))
	case "explicit", "implicit":
		store := resolveCertStore(cfg)
		host := certStoreKey(cfg.Host, port)

		tlsCfg := &tls.Config{
			InsecureSkipVerify:    true, //nolint:gosec // TLS verification is delegated to VerifyPeerCertificate cert pinning
			VerifyPeerCertificate: newCertVerifier(store, host),
			ServerName:            cfg.Host,
		}
		if cfg.TLSMode == "explicit" {
			return ftp.Dial(addr, ftp.DialWithExplicitTLS(tlsCfg), ftp.DialWithTimeout(timeout))
		}

		return ftp.Dial(addr, ftp.DialWithTLS(tlsCfg), ftp.DialWithTimeout(timeout))
	default:
		return nil, fmt.Errorf("unsupported tls_mode %q", cfg.TLSMode)
	}
}

// withCancel runs fn in a goroutine and races its result against ctx cancellation
// and stopCh. jlaffaye/ftp does not take a context, so this is the best-effort
// cancellation seam that mirrors ssh's openRemoteStream containment.
func withCancel[T any](ctx context.Context, stopCh <-chan struct{}, fn func() (T, error)) (T, error) {
	var zero T

	resCh := make(chan struct {
		v  T
		ok bool
		e  error
	}, 1)

	go func() {
		v, e := fn()
		resCh <- struct {
			v  T
			ok bool
			e  error
		}{v, e == nil, e}
	}()
	// drain consumes the in-flight result (if any) so the goroutine can exit and
	// releases a leaked *ftp.Response on the cancellation path below.
	drain := func() {
		r := <-resCh
		if c, ok := any(r.v).(io.Closer); ok && c != nil {
			_ = c.Close()
		}
	}

	select {
	case r := <-resCh:
		if r.ok {
			return r.v, nil
		}

		return zero, r.e
	case <-ctx.Done():
		drain()
		return zero, fmt.Errorf("ftp: operation cancelled: %w", ctx.Err())
	case <-stopCh:
		drain()
		return zero, errors.New("ftp: streamer closed before operation completed")
	}
}

// Streamer copies a remote log file over FTP into a local temp file and keeps it
// appended as the remote file grows, so the UI can index and tail it like a
// regular local file.
type Streamer struct {
	cfg       Config
	client    *ftp.ServerConn
	tmpFile   *os.File
	localPath string
	stopCh    chan struct{}
	closed    bool
	mu        sync.Mutex
	// activeResp is the in-flight data connection, guarded by mu so Close can
	// unblock a parked tail read by setting a deadline.
	activeResp    *ftp.Response
	downloaded    atomic.Int64
	remoteTotal   int64
	offset        uint64
	keepTempFile  bool
	probeInterval time.Duration
	closeTimeout  time.Duration
	stallTimeout  time.Duration
	supDone       chan struct{}
	// probeCh/probeRes implement a reusable, long-lived probe worker so repeated
	// size probes (every probeInterval) do not allocate a fresh goroutine,
	// channel, and result struct each time. probeSize and the supervisor both
	// route FileSize calls through probe.
	probeCh  chan func() (int64, error)
	probeRes chan probeResult
}

type probeResult struct {
	sz  int64
	err error
}

var _ remote.Streamer = (*Streamer)(nil)

// NewStreamer validates the config, connects to the FTP server, performs the
// initial full-file sync into a local temp file, and starts a background
// goroutine that tails the remote file for appended bytes. The streamed local
// path is exposed via LocalPath and can be handed to a FileIndexer.
//
// Parameters:
//   - ctx: Context for the open phase. Cancelled to abort dial, login, and initial sync.
//   - cfg: Config containing host, port, username, password, TLS mode, and remote file path.
//
// Returns a *Streamer and nil error on success, or a *CertUnknownError for
// explicit/implicit FTPS when the server certificate is not yet trusted (caller
// should call WriteCertTrust with the error's Line and retry). Any other error
// means the connection or sync failed.
//
// Play: https://go.dev/play/p/FTP_NEWSTREAMER
func NewStreamer(ctx context.Context, cfg Config) (*Streamer, error) { //nolint:gocyclo // streamer lifecycle is inherently branchy
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ftp: open cancelled before connecting: %w", err)
	}

	if cfg.Port == 0 {
		cfg.Port = 21
	}

	if cfg.RemoteFilePath == "" {
		return nil, errors.New("validation failed: remote_file_path required to stream a remote log")
	}

	client, err := dialClient(cfg)
	if err != nil {
		var ce *CertUnknownError
		if errors.As(err, &ce) {
			return nil, ce
		}

		return nil, fmt.Errorf("failed to connect to %s:%d: %w", cfg.Host, cfg.Port, err)
	}

	if err := client.Login(cfg.Username, cfg.Password); err != nil {
		var ce *CertUnknownError
		if errors.As(err, &ce) {
			_ = client.Quit()
			return nil, ce
		}

		_ = client.Quit()

		return nil, fmt.Errorf("ftp: login failed: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "glogg-ftp-*.log")
	if err != nil {
		_ = client.Quit()
		return nil, fmt.Errorf("failed to create temp file for FTP stream: %w", err)
	}

	stall := cfg.StallTimeout
	if stall <= 0 {
		stall = streamStallTimeout
	}

	s := &Streamer{
		cfg:           cfg,
		client:        client,
		tmpFile:       tmpFile,
		localPath:     tmpFile.Name(),
		stopCh:        make(chan struct{}),
		supDone:       make(chan struct{}),
		remoteTotal:   -1,
		probeInterval: streamProbeInterval,
		closeTimeout:  streamCloseTimeout,
		stallTimeout:  stall,
		probeCh:       make(chan func() (int64, error), 1),
		probeRes:      make(chan probeResult, 1),
	}
	go s.probeWorker()

	if s.cfg.Progress != nil {
		s.cfg.Progress("connecting", 0, 0)
	}

	if total, ok := s.probeSize(ctx); ok {
		s.remoteTotal = total
	}

	if err := s.initialSync(ctx); err != nil {
		if cerr := s.Close(); cerr != nil {
			slog.Warn("ftp: failed to close streamer after initial sync failure", "err", cerr)
		}

		return nil, err
	}

	go s.startSupervisor()

	return s, nil
}

// probeWorker is the long-lived goroutine that services FileSize probes through
// a reused channel/struct, avoiding a per-poll goroutine + channel allocation.
func (s *Streamer) probeWorker() {
	for fn := range s.probeCh {
		sz, err := fn()
		s.probeRes <- probeResult{sz: sz, err: err}
	}
}

// probe runs fn on the probe worker, racing its result against ctx/stopCh. The
// result channel is buffered so a cancelled caller that never reads still lets
// the worker deliver and accept the next probe (no goroutine/value leak).
func (s *Streamer) probe(ctx context.Context, fn func() (int64, error)) (int64, error) {
	select {
	case s.probeCh <- fn:
	case <-ctx.Done():
		return 0, fmt.Errorf("ftp: probe cancelled: %w", ctx.Err())
	case <-s.stopCh:
		return 0, errors.New("ftp: streamer closed before probe completed")
	}

	select {
	case r := <-s.probeRes:
		return r.sz, r.err
	case <-ctx.Done():
		return 0, fmt.Errorf("ftp: probe cancelled: %w", ctx.Err())
	case <-s.stopCh:
		return 0, errors.New("ftp: streamer closed before probe completed")
	}
}

// probeSize reads the remote file size (best-effort progress denominator).
func (s *Streamer) probeSize(ctx context.Context) (int64, bool) {
	sz, err := s.probe(ctx, func() (int64, error) {
		return s.client.FileSize(s.cfg.RemoteFilePath)
	})
	if err != nil {
		return 0, false
	}

	return sz, true
}

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
// temp file via Retr so the indexer has the complete contents before tailing.
//
// For files larger than parallelSyncThreshold (100 MB), uses parallel range
// downloads via multiple FTP connections (REST + RETR) to saturate bandwidth.
func (s *Streamer) initialSync(ctx context.Context) error {
	// Use parallel sync for large files when we know the total size
	if s.remoteTotal >= parallelSyncThreshold {
		return s.initialSyncParallel(ctx)
	}

	return s.initialSyncSequential(ctx)
}

// initialSyncSequential is the original single-stream initial sync using RETR.
func (s *Streamer) initialSyncSequential(ctx context.Context) error {
	r, err := withCancel(ctx, s.stopCh, func() (*ftp.Response, error) {
		return s.client.Retr(s.cfg.RemoteFilePath)
	})
	if err != nil {
		return fmt.Errorf("failed to retrieve remote file %s: %w", s.cfg.RemoteFilePath, err)
	}

	defer func() { _ = r.Close() }()

	if _, err := s.tmpFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek temp file: %w", err)
	}

	if s.cfg.Progress != nil {
		s.cfg.Progress("downloading", 0, s.remoteTotal)
	}

	cw := &countingWriter{w: s.tmpFile, s: s}
	if _, err := io.Copy(cw, r); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to copy remote file %s: %w", s.cfg.RemoteFilePath, err)
	}

	if s.cfg.Progress != nil {
		s.cfg.Progress("downloading", s.downloaded.Load(), s.remoteTotal)
	}

	return nil
}

// initialSyncParallel performs the initial sync using concurrent range downloads
// via multiple FTP connections (REST + RETR). Each range is downloaded independently
// and written to the correct offset in the pre-allocated temp file.
func (s *Streamer) initialSyncParallel(ctx context.Context) error {
	if s.remoteTotal <= 0 {
		// Unknown size, fall back to sequential
		return s.initialSyncSequential(ctx)
	}

	// Pre-allocate the temp file to the full remote size
	if err := s.tmpFile.Truncate(s.remoteTotal); err != nil {
		slog.Warn("ftp: failed to pre-allocate temp file, falling back to sequential",
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
		slog.Warn("ftp: parallel sync byte count mismatch",
			"expected", s.remoteTotal, "got", totalBytes,
			"remote", s.cfg.RemoteFilePath)
		// Not a hard error - the file might be slightly different
	}

	return nil
}

// downloadRange downloads a single byte range [start, end) of the remote file
// using FTP RETR with offset (RetrFrom) and writes it to the correct offset in the temp file.
func (s *Streamer) downloadRange(ctx context.Context, start, end int64) (int64, error) {
	// Use RETR with offset for byte-range retrieval
	r, err := withCancel(ctx, s.stopCh, func() (*ftp.Response, error) {
		return s.client.RetrFrom(s.cfg.RemoteFilePath, uint64(start))
	})
	if err != nil {
		return 0, err
	}
	defer func() { _ = r.Close() }()

	// Limit to exactly countBytes
	countBytes := end - start
	reader := io.LimitReader(r, countBytes)

	// Write to temp file at the correct offset
	if _, err := s.tmpFile.Seek(start, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek temp file for range %d-%d: %w", start, end, err)
	}

	cw := &countingWriter{w: s.tmpFile, s: s}
	n, cerr := io.Copy(cw, reader)

	if cerr != nil && !errors.Is(cerr, io.EOF) {
		return n, fmt.Errorf("failed to download range %d-%d: %w", start, end, cerr)
	}

	return n, nil
}

// startSupervisor polls the remote size every probeInterval and appends newly
// written bytes (RetrFrom) or restarts from zero on rotation (Truncate). It
// exits promptly on Close.
func (s *Streamer) startSupervisor() { //nolint:gocyclo // streamer lifecycle is inherently branchy
	defer close(s.supDone)

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		path := s.cfg.RemoteFilePath

		sz, err := s.probe(context.Background(), func() (int64, error) {
			return s.client.FileSize(path)
		})
		if err != nil {
			slog.Warn("ftp: size probe failed; retrying", "remote", path, "err", err)

			if !s.pollSleep() {
				return
			}

			continue
		}

		cur := s.offset
		if sz == int64(cur) { //nolint:gosec // file offset fits in int64 for expected log sizes
			if !s.pollSleep() {
				return
			}

			continue
		}

		var (
			resp *ftp.Response
			rerr error
		)

		if sz < int64(cur) { //nolint:gosec // file offset fits in int64 for expected log sizes
			// Rotation: restart the mirror from byte 0 of the new file.
			if terr := s.tmpFile.Truncate(0); terr != nil {
				slog.Warn("ftp: failed to truncate temp file after rotation", "err", terr)
			}

			if _, serr := s.tmpFile.Seek(0, io.SeekStart); serr != nil {
				slog.Warn("ftp: failed to rewind temp file after rotation", "err", serr)
			}

			s.offset = 0
			cur = 0
			resp, rerr = withCancel(context.Background(), s.stopCh, func() (*ftp.Response, error) {
				return s.client.Retr(path)
			})
		} else {
			resp, rerr = withCancel(context.Background(), s.stopCh, func() (*ftp.Response, error) {
				return s.client.RetrFrom(path, cur)
			})
		}

		if rerr != nil {
			slog.Warn("ftp: retrieve failed; retrying", "remote", path, "err", rerr)

			if !s.pollSleep() {
				return
			}

			continue
		}

		func() {
			s.mu.Lock()
			s.activeResp = resp
			s.mu.Unlock()
			stopWatch := s.watchStall(s.offset)

			defer func() {
				stopWatch()
				s.mu.Lock()
				s.activeResp = nil
				s.mu.Unlock()

				_ = resp.Close()
			}()

			if _, err := s.tmpFile.Seek(0, io.SeekEnd); err != nil {
				slog.Warn("ftp: failed to seek temp file for append", "err", err)
			}

			cw := &countingWriter{w: s.tmpFile, s: s}

			n, cerr := io.Copy(cw, resp)
			if cerr != nil && !errors.Is(cerr, io.EOF) {
				slog.Warn("ftp: tail copy failed", "remote", path, "err", cerr)
			}

			s.offset += uint64(n) //nolint:gosec // bytes read from a stream is non-negative
		}()

		if !s.pollSleep() {
			return
		}
	}
}

// watchStall launches a goroutine that detects a silently-dead tail link: if no
// new bytes are appended (s.offset does not advance) for stallTimeout while a
// tail copy is in progress, it forces the active data connection's read deadline
// so io.Copy unblocks and the supervisor can exit/retry. It returns a stop
// function that must be called when the copy ends; it is also cancelled via
// stopCh, so it cannot leak.
func (s *Streamer) watchStall(startOffset uint64) func() {
	done := make(chan struct{})

	go func() {
		lastOffset := startOffset
		lastProgress := time.Now()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				cur := s.offset
				if cur > lastOffset {
					lastOffset = cur
					lastProgress = time.Now()

					continue
				}

				if time.Since(lastProgress) >= s.stallTimeout {
					s.mu.Lock()

					if s.activeResp != nil {
						_ = s.activeResp.SetDeadline(time.Now().Add(2 * time.Second))
					}

					s.mu.Unlock()
					slog.Error("ftp: tail stall detected; aborting stream",
						"remote", s.cfg.RemoteFilePath)

					return
				}
			}
		}
	}()

	return func() { close(done) }
}

// pollSleep waits probeInterval, or returns false if Close fired meanwhile.
func (s *Streamer) pollSleep() bool {
	t := time.NewTimer(s.probeInterval)
	defer t.Stop()

	select {
	case <-t.C:
		return true
	case <-s.stopCh:
		return false
	}
}

// LocalPath returns the path of the local temp file holding the streamed remote log.
//
// Returns the absolute path to the local temp file.
//
// Example:
//
//	streamer, _ := ftp.NewStreamer(ctx, cfg)
//	idx, _ := indexer.NewFileIndexer(streamer.LocalPath())
func (s *Streamer) LocalPath() string { return s.localPath }

// Downloaded returns the number of bytes copied into the local temp file so far.
//
// Returns the total bytes downloaded since the streamer was created.
func (s *Streamer) Downloaded() int64 { return s.downloaded.Load() }

// RemoteTotal returns the probed remote size at open time, or -1 when unknown.
//
// Returns the remote file size in bytes, or -1 if the size could not be probed.
func (s *Streamer) RemoteTotal() int64 { return s.remoteTotal }

// KeepTempFile opts a Streamer out of the automatic temp-file cleanup performed by Close.
func (s *Streamer) KeepTempFile() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.keepTempFile = true
}

// Close stops the tailing goroutine, closes the FTP client, closes the local temp
// file handle, and — unless KeepTempFile was called — removes the local temp file.
// Safe to call multiple times (idempotent). It does not block on the caller beyond
// a bounded wait for the supervisor.
//
// Returns an error if any cleanup step fails, or nil on success.
func (s *Streamer) Close() error { //nolint:gocyclo // streamer lifecycle is inherently branchy
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()
		return nil
	}

	s.closed = true
	close(s.stopCh)

	// Unblock a parked tail read by setting a deadline on the active data
	// connection so the supervisor can observe stopCh and exit promptly.
	if s.activeResp != nil {
		_ = s.activeResp.SetDeadline(time.Now().Add(2 * time.Second))
	}

	s.mu.Unlock()

	select {
	case <-s.supDone:
	case <-time.After(s.closeTimeout):
		slog.Error("ftp: timed out waiting for tail supervisor; proceeding with teardown",
			"remote", s.cfg.RemoteFilePath)
	}

	// The supervisor has exited, so no further s.probe call can race with this
	// close; stop the probe worker goroutine to avoid a leak.
	close(s.probeCh)

	s.mu.Lock()

	var closeErr error

	if s.tmpFile != nil {
		if err := s.tmpFile.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("temp file: %w", err))
		}
	}

	if s.client != nil {
		if err := s.client.Quit(); err != nil && !errors.Is(err, net.ErrClosed) {
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

// Lister is a browse-only FTP connection (no file streaming).
type Lister struct {
	cfg    Config
	client *ftp.ServerConn
	stopCh chan struct{}
}

var _ remote.Lister = (*Lister)(nil)

// NewLister connects to the FTP server for directory navigation only. It returns
// a *CertUnknownError for explicit/implicit FTPS when the server certificate is
// not yet trusted.
func NewLister(cfg Config) (*Lister, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.Port == 0 {
		cfg.Port = 21
	}

	client, err := dialClient(cfg)
	if err != nil {
		var ce *CertUnknownError
		if errors.As(err, &ce) {
			return nil, ce
		}

		return nil, fmt.Errorf("failed to connect to %s:%d: %w", cfg.Host, cfg.Port, err)
	}

	if err := client.Login(cfg.Username, cfg.Password); err != nil {
		var ce *CertUnknownError
		if errors.As(err, &ce) {
			_ = client.Quit()
			return nil, ce
		}

		_ = client.Quit()

		return nil, fmt.Errorf("ftp: login failed: %w", err)
	}

	return &Lister{cfg: cfg, client: client, stopCh: make(chan struct{})}, nil
}

// ListDir lists the entries of a remote directory. An empty or "~" path resolves
// to the user's home directory via HomeDir.
func (l *Lister) ListDir(ctx context.Context, path string) ([]remote.DirEntry, error) {
	if path == "" || path == "~" {
		home, err := l.HomeDir(ctx)
		if err != nil {
			return nil, err
		}

		path = home
	}

	entries, err := withCancel(ctx, l.stopCh, func() ([]*ftp.Entry, error) {
		return l.client.List(path)
	})
	if err != nil {
		return nil, fmt.Errorf("ftp: list %s: %w", path, err)
	}

	out := make([]remote.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}

		kind := "file"

		switch e.Type {
		case ftp.EntryTypeFolder:
			kind = "dir"
		case ftp.EntryTypeLink:
			kind = "symlink"
		case ftp.EntryTypeFile: // regular files keep the default kind "file"
		}

		full := filepath.Join(path, e.Name)
		out = append(out, remote.DirEntry{
			Name:    e.Name,
			Path:    full,
			Kind:    kind,
			Size:    int64(e.Size), //nolint:gosec // remote file size fits in int64
			ModTime: e.Time.Unix(),
		})
	}

	sortFTPDirEntries(out)

	return out, nil
}

// sortFTPDirEntries orders directories first, then symlinks, then files, each
// group alphabetical (case-insensitive), using sort.SliceStable. This mirrors the
// TUI-side sort that is being removed so both the TUI and GUI receive stable,
// navigable listings.
func sortFTPDirEntries(entries []remote.DirEntry) {
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

// HomeDir returns the server's current (login) directory.
func (l *Lister) HomeDir(ctx context.Context) (string, error) {
	return withCancel(ctx, l.stopCh, func() (string, error) {
		return l.client.CurrentDir()
	})
}

// Close quits the FTP client.
func (l *Lister) Close() error {
	l.muClose()

	var closeErr error

	if l.client != nil {
		if err := l.client.Quit(); err != nil && !errors.Is(err, net.ErrClosed) {
			closeErr = fmt.Errorf("client: %w", err)
		}
	}

	return closeErr
}

func (l *Lister) muClose() {
	// Idempotent close of stopCh without a mutex on Lister (single owner).
	select {
	case <-l.stopCh:
	default:
		close(l.stopCh)
	}
}
