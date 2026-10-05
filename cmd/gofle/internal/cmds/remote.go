package cmds

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"GoFLE/pkg/ftp"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/ssh"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// exportChunkSize bounds how many lines are buffered per write when streaming a
// file to stdout (--dump).
const exportChunkSize = 4096

// authTypePassword is the config spelling for password-based authentication,
// shared by the ssh and ftp paths.
const authTypePassword = "password"

// entryKindDir and entryKindSymlink are the remote.DirEntry kinds that are
// navigable in the interactive browser (as opposed to selectable files).
const (
	entryKindDir     = "dir"
	entryKindSymlink = "symlink"
)

// remotePaths bundles the two trust-store locations the remote backends read
// and write: the OpenSSH known_hosts file (SSH) and the FTP certificate trust
// store (ftp://). Both default to the app's known-hosts path when empty.
type remotePaths struct {
	KnownHostsPath string
	FTPCerts       string
}

// buildRemoteURL reconstructs the canonical ssh:// or ftp:// target used for
// recent-list keys and viewer opens: "scheme://host:port/remote/path". The
// username is deliberately omitted — credentials live in the encrypted recent
// summary, and the same spelling is what the GUI records, so entries from both
// tools dedupe onto one key.
func buildRemoteURL(proto, username, host string, port int, remotePath string) string {
	_ = username

	var b strings.Builder
	if proto == remote.ProtocolFTP {
		b.WriteString("ftp://")
	} else {
		b.WriteString("ssh://")
	}

	b.WriteString(host)
	fmt.Fprintf(&b, ":%d", port)

	if remotePath == "" {
		remotePath = "/"
	}

	b.WriteString(remotePath)

	return b.String()
}

// resolveRemoteHost picks the target host from the positional argument or the
// --host flag (exactly one source), splitting an embedded "[user@]" prefix into
// the username when one is present. An explicit --user always wins over the
// embedded prefix. A missing or duplicated host is a usage error. It is
// protocol-agnostic: ssh and ftp share the same host-resolution rules.
func resolveRemoteHost(positional, hostFlag, username string) (host, user string, err error) {
	host = positional
	if hostFlag != "" {
		if host != "" {
			return "", "", NewUsageError(errors.New("pass the host either as <host> or via --host, not both"))
		}

		host = hostFlag
	}

	if host == "" {
		return "", "", NewUsageError(errors.New(`missing host: pass <host> or use --host/-H, e.g. gofle ssh user@host -p 22 --remote /var/log/x.log`))
	}

	user = username

	if at := strings.LastIndex(host, "@"); at >= 0 {
		prefix, bare := host[:at], host[at+1:]
		if prefix == "" || bare == "" {
			return "", "", NewUsageError(fmt.Errorf("invalid host %q: expected [user@]host form", host))
		}

		host = bare

		if user == "" {
			user = prefix
		}
	}

	return host, user, nil
}

// runRemote is the protocol-agnostic entry point shared by `gofle ssh` and
// `gofle ftp`. proto is "ssh" or "ftp" (remote.ProtocolSSH / remote.ProtocolFTP).
// The --remote flag is OPTIONAL: when omitted, an interactive directory browser
// is launched so the user can pick a file; the chosen path then drives the usual
// view/dump flow. Password auth always prompts interactively on the terminal
// (no echo, no --password flag) so the secret never appears in shell history.
func runRemote(cmd *cobra.Command, proto string, args []string) error {
	flags := cmd.Flags()

	cfg, err := shared.Config()
	if err != nil {
		return err
	}

	positional := ""
	if len(args) > 0 {
		positional = args[0]
	}

	username, _ := flags.GetString("user")
	hostFlag, _ := flags.GetString("host")
	port, _ := flags.GetInt("port")

	host, username, err := resolveRemoteHost(positional, hostFlag, username)
	if err != nil {
		return err
	}

	remotePath, _ := flags.GetString("remote")
	keyPath, _ := flags.GetString("key")
	dump, _ := flags.GetBool("dump")
	// --view is the default; --dump opts into printing to stdout instead.
	view := !dump
	insecure, _ := flags.GetBool("insecure-accept-host-key")
	tlsMode, _ := flags.GetString("tls")

	authType, err := remoteAuthType(proto, keyPath)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	paths := remotePaths{
		KnownHostsPath: cfg.KnownHostsPath(),
		FTPCerts:       cfg.KnownHostsPath(),
	}

	rcfg := remote.Config{
		Protocol:       proto,
		Host:           host,
		Port:           port,
		Username:       username,
		AuthType:       authType,
		Password:       "",
		KeyPath:        keyPath,
		KeyPassphrase:  "",
		RemoteFilePath: remotePath,
		TLSMode:        tlsMode,
		KnownHostsPath: cfg.KnownHostsPath(),
	}
	if proto == remote.ProtocolFTP {
		// FTP ignores key material.
		rcfg.KeyPath = ""
		rcfg.KeyPassphrase = ""
	}

	// Password auth: always prompt interactively on the terminal (no echo).
	// There is intentionally no --password flag so the secret never appears
	// in shell history or process listings.
	if rcfg.AuthType == authTypePassword {
		pw, err := readPasswordTTY("Password: ")
		if err != nil {
			return err
		}
		rcfg.Password = pw
	}

	// Optional interactive directory browser: pick a remote file when --remote
	// is not given. The browser reuses the already-resolved credentials
	// (including any password we just prompted for), so no second prompt.
	if rcfg.RemoteFilePath == "" {
		chosen, perr := pickRemotePath(ctx, cmd, &rcfg, insecure, paths)
		if perr != nil {
			return perr
		}

		rcfg.RemoteFilePath = chosen
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to %s:%d …\n", host, port)

	streamer, err := connectRemote(ctx, rcfg, insecure, paths)
	if err != nil {
		return err
	}

	summary := &config.SSHConfigSummary{
		Host:           host,
		Port:           port,
		Username:       username,
		RemoteFilePath: rcfg.RemoteFilePath,
		AuthType:       authType,
		Password:       rcfg.Password,
		KeyPath:        keyPath,
		Protocol:       proto,
		TLSMode:        tlsMode,
	}

	targetURL := buildRemoteURL(proto, username, host, port, rcfg.RemoteFilePath)
	if err := shared.RecordRecent(cfg, targetURL, true, 0, summary); err != nil {
		slog.Warn("gofle remote: failed to persist recent entry", "target", targetURL, "err", err)
	}

	if view {
		// OpenViewer re-resolves the recent entry (handling its own trust
		// prompt); close the throwaway streamer first so nothing leaks.
		_ = streamer.Close()

		return OpenViewer(cmd, targetURL)
	}

	return dumpRemote(ctx, cmd, streamer)
}

// remoteAuthType decides which authentication the connection uses. FTP
// authenticates with a password only, so a provided --key is rejected rather
// than silently ignored.
func remoteAuthType(proto, keyPath string) (string, error) {
	if proto == remote.ProtocolFTP {
		if keyPath != "" {
			return "", NewUsageError(errors.New("ftp does not support key authentication; remove --key and use --password"))
		}

		return authTypePassword, nil
	}

	if keyPath == "" {
		return authTypePassword, nil
	}

	return "key", nil
}

// pickRemotePath runs the interactive directory browser and returns the selected
// remote file path. Password (if needed) has already been prompted for and
// stored in rcfg by the caller, so the browse connection reuses it.
func pickRemotePath(ctx context.Context, cmd *cobra.Command, rcfg *remote.Config, insecure bool, paths remotePaths) (string, error) {
	listCfg := *rcfg
	listCfg.RemoteFilePath = ""

	lister, lerr := connectLister(ctx, listCfg, insecure, paths)
	if lerr != nil {
		return "", lerr
	}

	chosen, berr := runRemoteBrowser(ctx, cmd, lister, "/")
	if cerr := lister.Close(); cerr != nil {
		slog.Warn("gofle remote: failed to close directory lister", "err", cerr)
	}

	if berr != nil {
		return "", berr
	}

	if chosen == "" {
		return "", NewUsageError(errors.New("no remote file selected"))
	}

	return chosen, nil
}

// dumpRemote indexes the streamed file and writes it to the command's stdout in
// bounded chunks. It takes ownership of streamer and closes it, together with
// the indexer, before returning.
func dumpRemote(ctx context.Context, cmd *cobra.Command, streamer remote.Streamer) (retErr error) {
	idx, ierr := indexer.NewFileIndexer(streamer.LocalPath())
	if ierr != nil {
		_ = streamer.Close()

		return fmt.Errorf("failed to index streamed file: %w", ierr)
	}

	w := bufio.NewWriter(cmd.OutOrStdout())

	defer func() {
		if ferr := w.Flush(); ferr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed to flush dump output: %w", ferr))
		}

		if cerr := idx.Close(); cerr != nil {
			slog.Warn("gofle remote: failed to close indexer after dump", "err", cerr)
		}

		if serr := streamer.Close(); serr != nil {
			slog.Warn("gofle remote: failed to close streamer after dump", "err", serr)
		}
	}()

	total := idx.TotalLines()

	var start int64
	for start < total {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}

		count := int64(exportChunkSize)
		if start+count > total {
			count = total - start
		}

		gerr := idx.ScanLinesRetry(start, count, func(_ int64, line []byte) bool {
			if _, werr := w.Write(line); werr != nil {
				return false
			}

			return w.WriteByte('\n') == nil
		})
		if gerr != nil {
			return gerr
		}

		start += count
	}

	return nil
}

// connectRemote opens a streamer for the given protocol, transparently trusting
// and retrying once on an unknown host key / certificate when insecure mode is
// requested. ctx bounds the attempt; a dial that never completes is abandoned
// when the user interrupts the command.
func connectRemote(ctx context.Context, rcfg remote.Config, insecure bool, paths remotePaths) (remote.Streamer, error) {
	if rcfg.Protocol == remote.ProtocolSSH {
		s, err := connectSSH(ctx, sshConfigFrom(rcfg, paths.KnownHostsPath), insecure, paths.KnownHostsPath)
		if err != nil {
			return nil, err
		}

		return s, nil
	}

	return connectFTP(ctx, ftpConfigFrom(rcfg, paths.FTPCerts), insecure, paths.FTPCerts)
}

// connectLister opens a browse-only connection for the given protocol, handling
// the same trust flow as connectRemote.
func connectLister(ctx context.Context, rcfg remote.Config, insecure bool, paths remotePaths) (remote.Lister, error) {
	if rcfg.Protocol == remote.ProtocolSSH {
		l, err := connectSSHLister(ctx, sshConfigFrom(rcfg, paths.KnownHostsPath), insecure, paths.KnownHostsPath)
		if err != nil {
			return nil, err
		}

		return l, nil
	}

	return connectFTPLister(ctx, ftpConfigFrom(rcfg, paths.FTPCerts), insecure, paths.FTPCerts)
}

// sshConfigFrom maps the unified remote.Config onto an ssh.Config.
func sshConfigFrom(rcfg remote.Config, knownHosts string) ssh.Config {
	return ssh.Config{
		Host:           rcfg.Host,
		Port:           rcfg.Port,
		Username:       rcfg.Username,
		AuthType:       rcfg.AuthType,
		Password:       rcfg.Password,
		KeyPath:        rcfg.KeyPath,
		KeyPassphrase:  rcfg.KeyPassphrase,
		RemoteFilePath: rcfg.RemoteFilePath,
		KnownHostsPath: knownHosts,
	}
}

// ftpConfigFrom maps the unified remote.Config onto an ftp.Config. The cert
// trust store is taken from certsStore when set; otherwise the config's own
// path (which itself defaults to ~/.config/GoFLE/ftp_certs).
func ftpConfigFrom(rcfg remote.Config, certsStore string) ftp.Config {
	kp := certsStore
	if kp == "" {
		kp = rcfg.KnownHostsPath
	}

	return ftp.Config{
		Host:           rcfg.Host,
		Port:           rcfg.Port,
		Username:       rcfg.Username,
		AuthType:       rcfg.AuthType,
		Password:       rcfg.Password,
		RemoteFilePath: rcfg.RemoteFilePath,
		TLSMode:        rcfg.TLSMode,
		KnownHostsPath: kp,
	}
}

// connectFTP opens an FTP streamer, transparently accepting and retrying once on
// an unknown server certificate when insecure mode is requested.
func connectFTP(ctx context.Context, ftpCfg ftp.Config, insecure bool, certsStore string) (*ftp.Streamer, error) {
	if certsStore != "" {
		ftpCfg.KnownHostsPath = certsStore
	}

	s, err := ftp.NewStreamer(ctx, ftpCfg)
	if err == nil {
		return s, nil
	}

	var ce *ftp.CertUnknownError
	if errors.As(err, &ce) && insecure {
		if werr := ftp.WriteCertTrust(certsStore, ce.Line); werr != nil {
			return nil, fmt.Errorf("failed to record FTP certificate: %w", werr)
		}

		return ftp.NewStreamer(ctx, ftpCfg)
	}

	if errors.As(err, &ce) {
		return nil, fmt.Errorf("unknown FTP server certificate for %s (fingerprint %s); re-run with --insecure-accept-host-key to trust it",
			ce.Host, ce.Fingerprint)
	}

	return nil, err
}

// connectFTPLister opens an FTP browse-only connection, handling the cert-trust
// flow like connectFTP. The context is accepted for signature symmetry with the
// SSH variant and with connectLister; ftp.NewLister has no context-aware form
// yet, so it is deliberately unused here.
func connectFTPLister(_ context.Context, ftpCfg ftp.Config, insecure bool, certsStore string) (*ftp.Lister, error) {
	if certsStore != "" {
		ftpCfg.KnownHostsPath = certsStore
	}

	l, err := ftp.NewLister(ftpCfg)
	if err == nil {
		return l, nil
	}

	var ce *ftp.CertUnknownError
	if errors.As(err, &ce) && insecure {
		if werr := ftp.WriteCertTrust(certsStore, ce.Line); werr != nil {
			return nil, fmt.Errorf("failed to record FTP certificate: %w", werr)
		}

		return ftp.NewLister(ftpCfg)
	}

	if errors.As(err, &ce) {
		return nil, fmt.Errorf("unknown FTP server certificate for %s (fingerprint %s); re-run with --insecure-accept-host-key to trust it",
			ce.Host, ce.Fingerprint)
	}

	return nil, err
}

// connectSSHLister opens an SSH browse-only connection, transparently trusting
// and retrying once on an unknown host key when insecure mode is requested. The
// context is accepted for signature symmetry with connectLister; ssh.NewLister
// has no context-aware form yet, so it is deliberately unused here.
func connectSSHLister(_ context.Context, sshCfg ssh.Config, insecure bool, knownHosts string) (*ssh.Lister, error) {
	l, err := ssh.NewLister(sshCfg)
	if err == nil {
		return l, nil
	}

	var hkErr *ssh.HostKeyUnknownError
	if errors.As(err, &hkErr) && insecure {
		if werr := ssh.WriteKnownHost(knownHosts, hkErr.Line); werr != nil {
			return nil, fmt.Errorf("failed to record host key: %w", werr)
		}

		return ssh.NewLister(sshCfg)
	}

	if errors.As(err, &hkErr) {
		return nil, fmt.Errorf("unknown SSH host key for %s (fingerprint %s); re-run with --insecure-accept-host-key to trust it",
			hkErr.Host, hkErr.Fingerprint)
	}

	return nil, err
}

// runRemoteBrowser is a self-contained, dependency-free directory browser (it
// does not import the TUI viewer package). It lists the current path, prints a
// numbered menu (directories first, then files, with "0" to go up), and reads a
// choice from stdin. A directory descends; ".." ascends; a file is selected and
// its path returned. ctx cancellation and Lister.Close are honored.
func runRemoteBrowser(ctx context.Context, cmd *cobra.Command, lister remote.Lister, startPath string) (string, error) {
	cur := startPath
	if cur == "" {
		cur = "/"
	}

	reader := bufio.NewReader(cmd.InOrStdin())

	for {
		if cerr := ctx.Err(); cerr != nil {
			return "", cerr
		}

		entries, err := lister.ListDir(ctx, cur)
		if err != nil {
			return "", fmt.Errorf("failed to list %s: %w", cur, err)
		}

		sortRemoteEntries(entries)

		out := cmd.OutOrStdout()
		printRemoteMenu(out, cur, entries)

		line, rerr := reader.ReadString('\n')
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return "", rerr
		}

		choice := strings.TrimSpace(line)
		if choice == "" {
			if errors.Is(rerr, io.EOF) {
				return "", nil
			}

			continue
		}

		next, picked := resolveMenuChoice(out, cur, choice, entries)
		if picked != "" {
			return picked, nil
		}

		cur = next
	}
}

// sortRemoteEntries orders a listing for display: directories first, then
// symlinks, then files, alphabetically within each group.
func sortRemoteEntries(entries []remote.DirEntry) {
	sort.Slice(entries, func(i, j int) bool {
		ri, rj := dirRank(entries[i].Kind), dirRank(entries[j].Kind)
		if ri != rj {
			return ri < rj
		}

		return entries[i].Name < entries[j].Name
	})
}

// printRemoteMenu renders the numbered directory menu for cur, with "0" as the
// entry that ascends to the parent.
func printRemoteMenu(out io.Writer, cur string, entries []remote.DirEntry) {
	_, _ = fmt.Fprintf(out, "\n%s\n", cur)
	_, _ = fmt.Fprintln(out, " 0. .. (up)")

	for i, e := range entries {
		_, _ = fmt.Fprintf(out, " %d. %s%s\n", i+1, entryMarker(e.Kind), e.Name)
	}

	_, _ = fmt.Fprint(out, "Select> ")
}

// resolveMenuChoice interprets one menu selection. It returns the directory to
// display next (cur itself when the input was unusable) and, when a file was
// picked, that file's path. Invalid input is reported on out.
func resolveMenuChoice(out io.Writer, cur, choice string, entries []remote.DirEntry) (next, picked string) {
	n, perr := strconv.Atoi(choice)
	if perr != nil {
		_, _ = fmt.Fprintln(out, "enter a number (0 to go up)")

		return cur, ""
	}

	if n == 0 {
		return parentDir(cur), ""
	}

	if n < 0 || n > len(entries) {
		_, _ = fmt.Fprintln(out, "choice out of range")

		return cur, ""
	}

	e := entries[n-1]
	if e.Kind == entryKindDir || e.Kind == entryKindSymlink {
		return e.Path, ""
	}

	return cur, e.Path
}

// dirRank orders directory listings: directories first, then symlinks, then
// files, so the menu shows navigable entries above selectable ones.
func dirRank(kind string) int {
	switch kind {
	case entryKindDir:
		return 0
	case entryKindSymlink:
		return 1
	default:
		return 2
	}
}

func entryMarker(kind string) string {
	switch kind {
	case entryKindDir:
		return "/ "
	case entryKindSymlink:
		return "@ "
	default:
		return "  "
	}
}

// parentDir returns the parent of an absolute remote path; "/" is its own
// parent.
func parentDir(p string) string {
	if p == "" || p == "/" {
		return "/"
	}

	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		if i == 0 {
			return "/"
		}

		return p[:i]
	}

	return "/"
}

// readPasswordTTY prompts for a password on /dev/tty so it is read directly from
// the terminal (not from a redirected stdin) and is never echoed to the screen or
// written to shell history. When no tty is available (e.g. a piped/automated
// run) it falls back to reading a single line from stdin without echo. The
// password is never logged.
func readPasswordTTY(prompt string) (string, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		defer func() { _ = f.Close() }()

		// Non-fatal: proceed to read even if the prompt could not be shown.
		_, _ = f.WriteString(prompt)

		if b, rerr := term.ReadPassword(int(f.Fd())); rerr == nil {
			_, _ = f.WriteString("\n")

			return string(b), nil
		}
		// tty present but ReadPassword failed: fall back to a line read.
		return readLineFrom(f)
	}
	// No controlling tty: read a line from stdin (no echo available).
	return readLineFrom(os.Stdin)
}

// readLineFrom reads a single trimmed line from r, treating EOF as an empty
// string rather than an error so callers can branch on the empty password.
func readLineFrom(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return strings.TrimRight(line, "\r\n"), nil
}
