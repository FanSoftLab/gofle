package shared

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"GoFLE/pkg/config"
	"GoFLE/pkg/ftp"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/search"
	"GoFLE/pkg/ssh"
)

// SSHScheme is the URL scheme used to address a remote log over SSH.
const SSHScheme = "ssh://"

// FTPScheme is the URL scheme used to address a remote log over FTP/FTPS.
const FTPScheme = "ftp://"

// ErrNoStoredCredentials is returned when an ssh:// target has no matching
// recent entry to take connection details from. Callers branch on it via
// errors.Is instead of string-matching the message.
var ErrNoStoredCredentials = errors.New("no stored credentials")

// ResolveTarget opens a log target and returns an indexer, a search engine bound
// to it, and a cleanup function. The target is either a local file path or an
// ssh://[user@]host[:port]/remote/path URL. When the target is remote, the
// resolved credentials come from the matching recent entry; if none is found, a
// clear error instructs the user to use `gofle ssh` for interactive credentials.
//
// ctx bounds the remote open: cancelling it (Ctrl+C, a terminating signal) aborts
// the dial, the handshake and the initial full-file transfer instead of leaving
// the command parked on a black-holed connection, and stays bound to the streamer
// so the background tail stops with it. It is ignored for local targets, which
// cannot block.
//
// The returned cleanup must be called once the caller is done, regardless of
// whether the target is local or remote.
func ResolveTarget(ctx context.Context, target string, c *config.AppConfig) (idx *indexer.FileIndexer, se *search.SearchEngine, cleanup func(), err error) {
	if strings.HasPrefix(target, FTPScheme) {
		return resolveFTP(ctx, target, c)
	}

	if strings.HasPrefix(target, SSHScheme) {
		return resolveSSH(ctx, target, c)
	}

	indexPath, gzCleanup, gerr := indexer.EnsureDecompressed(target)
	if gerr != nil {
		return nil, nil, nil, fmt.Errorf("failed to open local file %q: %w", target, gerr)
	}

	idx, err = indexer.NewFileIndexer(indexPath)
	if err != nil {
		gzCleanup()
		return nil, nil, nil, fmt.Errorf("failed to open local file %q: %w", target, err)
	}

	cleanup = func() {
		_ = idx.Close()

		gzCleanup()
	}
	se = search.NewSearchEngine(idx)

	return idx, se, cleanup, nil
}

// resolveSSH streams a remote log over SSH into a local temp file and indexes
// that. Credentials are taken from the matching recent entry when present. ctx is
// the caller's cancellation scope for the open and the resulting live tail.
func resolveSSH(ctx context.Context, target string, c *config.AppConfig) (idx *indexer.FileIndexer, se *search.SearchEngine, cleanup func(), err error) {
	host, port, username, remotePath, err := parseSSHURL(target)
	if err != nil {
		return nil, nil, nil, err
	}

	authType, password, keyPath, keyPass := "", "", "", ""
	if rec := findRecentRemote(c, target); rec != nil {
		// findRecentSSH resolves through config.GetRecentFiles, which already
		// returns the SSH summary DECRYPTED. Re-decrypting here would run
		// DecryptSecret on plaintext and blank every credential (see config.go).
		authType = rec.AuthType
		if username == "" {
			username = rec.Username
		}

		password = rec.Password
		keyPath = rec.KeyPath
		keyPass = rec.KeyPassphrase
	}

	if authType == "" {
		return nil, nil, nil, fmt.Errorf("%w for %q; use `gofle ssh` to connect interactively and cache them", ErrNoStoredCredentials, target)
	}

	cfg := ssh.Config{
		Host:           host,
		Port:           port,
		Username:       username,
		AuthType:       authType,
		Password:       password,
		KeyPath:        keyPath,
		KeyPassphrase:  keyPass,
		RemoteFilePath: remotePath,
		KnownHostsPath: c.KnownHostsPath(),
	}

	s, err := ssh.NewStreamerContext(ctx, cfg)
	if err != nil {
		if _, ok := errors.AsType[*ssh.HostKeyUnknownError](err); ok {
			return nil, nil, nil, err
		}

		return nil, nil, nil, fmt.Errorf("failed to open ssh target %q: %w", target, err)
	}

	idx, err = indexer.NewFileIndexer(s.LocalPath())
	if err != nil {
		_ = s.Close()
		return nil, nil, nil, fmt.Errorf("failed to index streamed file for %q: %w", target, err)
	}

	cleanup = func() { _ = s.Close() }
	se = search.NewSearchEngine(idx)

	return idx, se, cleanup, nil
}

// resolveFTP streams a remote log over FTP/FTPS into a local temp file and
// indexes it, mirroring resolveSSH but for the ftp:// scheme. Credentials come
// from the matching recent entry (keyed ftps://…). FTP authenticates with a
// password only; any cached key material is ignored, and the TLS mode (plain /
// explicit / implicit) is taken from the cached summary so a reconnect uses the
// same transport security. An unknown FTPS certificate surfaces as a
// *ftp.CertUnknownError for the caller to confirm, exactly like SSH's
// HostKeyUnknownError.
func resolveFTP(ctx context.Context, target string, c *config.AppConfig) (idx *indexer.FileIndexer, se *search.SearchEngine, cleanup func(), err error) {
	proto, host, port, username, remotePath, perr := parseRemoteURL(target)
	if perr != nil {
		return nil, nil, nil, perr
	}

	_ = proto

	authType, password, tlsMode := "", "", ""
	if rec := findRecentRemote(c, target); rec != nil {
		authType = rec.AuthType
		if username == "" {
			username = rec.Username
		}

		password = rec.Password
		tlsMode = rec.TLSMode
	}

	if authType == "" {
		return nil, nil, nil, fmt.Errorf("%w for %q; use `gofle ftp` to connect interactively and cache them", ErrNoStoredCredentials, target)
	}

	// FTP supports password auth only; key material is discarded.
	cfg := ftp.Config{
		Host:           host,
		Port:           port,
		Username:       username,
		AuthType:       "password",
		Password:       password,
		RemoteFilePath: remotePath,
		TLSMode:        tlsMode,
		KnownHostsPath: c.KnownHostsPath(),
	}

	s, err := ftp.NewStreamer(ctx, cfg)
	if err != nil {
		var ce *ftp.CertUnknownError
		if errors.As(err, &ce) {
			return nil, nil, nil, err
		}

		return nil, nil, nil, fmt.Errorf("failed to open ftp target %q: %w", target, err)
	}

	idx, err = indexer.NewFileIndexer(s.LocalPath())
	if err != nil {
		_ = s.Close()
		return nil, nil, nil, fmt.Errorf("failed to index streamed file for %q: %w", target, err)
	}

	cleanup = func() { _ = s.Close() }
	se = search.NewSearchEngine(idx)

	return idx, se, cleanup, nil
}

// parseRemoteURL parses [scheme://][user@]host[:port]/remote/path for either
// the ssh:// or ftp:// scheme into its components, returning the protocol
// string ("ssh"/"ftp"). remotePath retains its leading slash (an absolute
// remote path).
func parseRemoteURL(target string) (proto, host string, port int, username string, remotePath string, err error) {
	scheme := SSHScheme
	proto = remote.ProtocolSSH

	switch {
	case strings.HasPrefix(target, FTPScheme):
		scheme = FTPScheme
		proto = remote.ProtocolFTP
	case strings.HasPrefix(target, SSHScheme):
		// default ssh:// already set
	default:
		return "", "", 0, "", "", fmt.Errorf("unsupported remote target %q: expected ssh:// or ftp://", target)
	}

	raw := strings.TrimPrefix(target, scheme)

	slash := strings.Index(raw, "/")
	if slash < 0 {
		return "", "", 0, "", "", fmt.Errorf("invalid remote target %q: missing remote path", target)
	}

	authority := raw[:slash]
	remotePath = raw[slash:] // includes leading "/"

	if u, a, ok := strings.Cut(authority, "@"); ok {
		username, authority = u, a
	}

	host = authority
	port = defaultPort(proto)

	if colon := strings.LastIndex(authority, ":"); colon >= 0 {
		host = authority[:colon]

		p, convErr := strconv.Atoi(authority[colon+1:])
		if convErr != nil {
			return "", "", 0, "", "", fmt.Errorf("invalid port in remote target %q: %w", target, convErr)
		}

		port = p
	}

	if host == "" {
		return "", "", 0, "", "", fmt.Errorf("invalid remote target %q: missing host", target)
	}

	return proto, host, port, username, remotePath, nil
}

// defaultPort returns the well-known default port for a protocol.
func defaultPort(proto string) int {
	if proto == remote.ProtocolFTP {
		return 21
	}

	return 22
}

// parseSSHURL parses ssh://[user@]host[:port]/remote/path into its components.
// remotePath retains its leading slash (an absolute remote path).
func parseSSHURL(target string) (host string, port int, username string, remotePath string, err error) {
	proto, host, port, username, remotePath, err := parseRemoteURL(target)
	if err != nil {
		return "", 0, "", "", err
	}

	if proto != remote.ProtocolSSH {
		return "", 0, "", "", fmt.Errorf("invalid ssh target %q", target)
	}

	return host, port, username, remotePath, nil
}

// parseFTPURL parses ftp://[user@]host[:port]/remote/path into its components.
func parseFTPURL(target string) (host string, port int, username string, remotePath string, err error) {
	proto, host, port, username, remotePath, err := parseRemoteURL(target)
	if err != nil {
		return "", 0, "", "", err
	}

	if proto != remote.ProtocolFTP {
		return "", 0, "", "", fmt.Errorf("invalid ftp target %q", target)
	}

	return host, port, username, remotePath, nil
}

// findRecentRemote returns the recent entry (ssh:// or ftp://) whose connection
// components match target, or nil. Entries are matched by parsed host/port/path
// rather than raw string equality: the GUI and the CLI have historically
// serialized different URL spellings, and credentials recorded by one side must
// resolve for the other. The scheme of target selects which recent entries are
// considered (a legacy SSH entry with an empty Protocol is treated as "ssh").
func findRecentRemote(c *config.AppConfig, target string) *config.SSHConfigSummary {
	proto, tHost, tPort, tUser, tPath, err := parseRemoteURL(target)
	if err != nil {
		return nil
	}

	for _, item := range c.GetRecentFiles() {
		if !item.IsSSH || item.SSH == nil {
			continue
		}
		// Legacy SSH entries have an empty Protocol => "ssh". Only filter when a
		// concrete protocol is set and differs from the target's scheme, so an
		// ssh:// target never matches an ftp:// entry and vice versa.
		if item.SSH.Protocol != "" && item.SSH.Protocol != proto {
			continue
		}

		if remoteEntryMatches(item.Path, tHost, tPort, tUser, tPath) {
			return item.SSH
		}
	}

	return nil
}

// remoteEntryMatches reports whether a recent entry's parsed host/port/path
// (and, when both sides specify one, username) match the target's components.
// The raw URL strings are not compared: the GUI and CLI have historically
// serialized different spellings, and credentials recorded by one side must
// resolve for the other.
func remoteEntryMatches(recentPath, tHost string, tPort int, tUser, tPath string) bool {
	_, iHost, iPort, iUser, iPath, perr := parseRemoteURL(recentPath)
	if perr != nil {
		return false
	}

	if iHost != tHost || iPort != tPort || iPath != tPath {
		return false
	}

	if tUser != "" && iUser != "" && tUser != iUser {
		return false
	}

	return true
}

// HumanSize formats a byte count in a human-readable form (B, KB, MB, ...).
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for d := n / unit; d >= unit; d /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
