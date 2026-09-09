package tui

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/ftp"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/search"
	"GoFLE/pkg/ssh"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// entryKindDir and entryKindSymlink are the remote.DirEntry kinds that the
// browser navigates into rather than opening as a file.
const (
	entryKindDir     = "dir"
	entryKindSymlink = "symlink"
)

// authTypePassword is the config spelling for password-based authentication.
const authTypePassword = "password"

// fillRemoteFormFromRecent writes a saved SSH/FTP summary into the connect form
// fields so the user can browse or reopen without retyping credentials.
func (m *model) fillRemoteFormFromRecent(sum *config.SSHConfigSummary) {
	if sum == nil {
		return
	}
	proto := sum.Protocol
	if proto == "" {
		proto = remote.ProtocolSSH
	}
	port := sum.Port
	if port == 0 {
		port = remoteDefaultPort(proto)
	}
	auth := sum.AuthType
	if auth == "" {
		auth = authTypePassword
	}
	vals := []string{
		proto,
		sum.Host,
		strconv.Itoa(port),
		sum.Username,
		auth,
		sum.Password,
		sum.KeyPath,
		sum.RemoteFilePath,
	}
	for i := range m.remoteForm {
		if i < len(vals) {
			m.remoteForm[i].SetValue(vals[i])
		}
	}
}

// startPathFromRemoteSummary returns the directory of the last opened remote
// file, or "~" when the path is empty.
func startPathFromRemoteSummary(sum *config.SSHConfigSummary) string {
	if sum == nil {
		return "~"
	}
	p := strings.TrimSpace(sum.RemoteFilePath)
	if p == "" || p == "~" {
		return "~"
	}
	return parentDir(p)
}

// browseFromRecent opens the remote directory browser using credentials from a
// saved recent entry, starting in the directory of that entry's file.
func (m *model) browseFromRecent(item config.RecentFileItem) tea.Cmd {
	if item.SSH == nil {
		m.status = "not a remote entry"
		return nil
	}
	m.fillRemoteFormFromRecent(item.SSH)
	m.remoteMode = false
	m.remoteBrowseMode = true
	m.remoteCurrentPath = startPathFromRemoteSummary(item.SSH)
	m.remoteCursor = 0
	m.remoteLoading = true
	m.remoteBrowseErr = ""
	m.remoteAfterTrustOpen = false
	m.remoteProto = item.SSH.Protocol
	if m.remoteProto == "" {
		m.remoteProto = remote.ProtocolSSH
	}
	// Clear remote path field: user is choosing a new file.
	if len(m.remoteForm) > 7 {
		m.remoteForm[7].SetValue("")
	}
	return m.remoteListCmd(m.remoteCurrentPath, m.remoteFormConfig())
}

// browseSameServer opens the remote directory browser on the host of the
// currently open remote file (or the last connect form). Used by the in-viewer
// "b" key so the CLI TUI matches the desktop "other file on this server" action
// for both SSH and FTP.
func (m *model) browseSameServer() tea.Cmd {
	if m.lastRemoteCfg != nil && m.lastRemoteCfg.Host != "" {
		sum := summaryFromRemoteConfig(m.lastRemoteCfg)
		m.fillRemoteFormFromRecent(sum)
		m.remoteMode = false
		m.remoteBrowseMode = true
		start := parentDir(m.lastRemoteCfg.RemoteFilePath)
		if start == "" || start == "." {
			start = "~"
		}
		m.remoteCurrentPath = start
		m.remoteCursor = 0
		m.remoteLoading = true
		m.remoteBrowseErr = ""
		m.remoteAfterTrustOpen = false
		m.remoteProto = m.lastRemoteCfg.Protocol
		if m.remoteProto == "" {
			m.remoteProto = remote.ProtocolSSH
		}
		if len(m.remoteForm) > 7 {
			m.remoteForm[7].SetValue("")
		}
		return m.remoteListCmd(m.remoteCurrentPath, m.remoteFormConfig())
	}
	// Fallback: form already has a host from a previous connect attempt.
	if m.remoteFormConfig().Host != "" {
		cfg := m.remoteFormConfig()
		m.remoteMode = false
		m.remoteBrowseMode = true
		start := parentDir(cfg.RemoteFilePath)
		if start == "" || start == "." {
			start = "~"
		}
		m.remoteCurrentPath = start
		m.remoteCursor = 0
		m.remoteLoading = true
		m.remoteBrowseErr = ""
		m.remoteAfterTrustOpen = false
		m.remoteProto = cfg.Protocol
		if m.remoteProto == "" {
			m.remoteProto = remote.ProtocolSSH
		}
		if len(m.remoteForm) > 7 {
			m.remoteForm[7].SetValue("")
		}
		return m.remoteListCmd(m.remoteCurrentPath, cfg)
	}
	m.status = "no remote connection — open an SSH/FTP file first (or press S)"
	return nil
}

// summaryFromRemoteConfig builds a Recent-style summary from a live Config so
// fillRemoteFormFromRecent can populate the form fields.
func summaryFromRemoteConfig(cfg *remote.Config) *config.SSHConfigSummary {
	if cfg == nil {
		return nil
	}
	return &config.SSHConfigSummary{
		Host:           cfg.Host,
		Port:           cfg.Port,
		Username:       cfg.Username,
		RemoteFilePath: cfg.RemoteFilePath,
		AuthType:       cfg.AuthType,
		Password:       cfg.Password,
		KeyPath:        cfg.KeyPath,
		KeyPassphrase:  cfg.KeyPassphrase,
		Protocol:       cfg.Protocol,
		TLSMode:        cfg.TLSMode,
	}
}

// remoteConfigFromSummary is the inverse of summaryFromRemoteConfig.
func remoteConfigFromSummary(sum *config.SSHConfigSummary) remote.Config {
	if sum == nil {
		return remote.Config{}
	}
	proto := sum.Protocol
	if proto == "" {
		proto = remote.ProtocolSSH
	}
	port := sum.Port
	if port == 0 {
		port = remoteDefaultPort(proto)
	}
	auth := sum.AuthType
	if auth == "" {
		auth = authTypePassword
	}
	return remote.Config{
		Protocol:       proto,
		Host:           sum.Host,
		Port:           port,
		Username:       sum.Username,
		AuthType:       auth,
		Password:       sum.Password,
		KeyPath:        sum.KeyPath,
		KeyPassphrase:  sum.KeyPassphrase,
		RemoteFilePath: sum.RemoteFilePath,
		TLSMode:        sum.TLSMode,
	}
}

// findRecentSummaryForTarget returns the decrypted SSH/FTP summary for a
// recent entry whose reconstructed target matches target (ssh:// or ftp://).
func findRecentSummaryForTarget(cfg *config.AppConfig, target string) *config.SSHConfigSummary {
	if cfg == nil || target == "" {
		return nil
	}
	for _, it := range cfg.GetRecentFiles() {
		if !it.IsSSH || it.SSH == nil {
			continue
		}
		if remoteTargetFromRecent(it) == target {
			return it.SSH
		}
		// Also match by host+path when schemes differ only by default port.
		if it.SSH.Host != "" && strings.Contains(target, it.SSH.Host) &&
			it.SSH.RemoteFilePath != "" && strings.Contains(target, it.SSH.RemoteFilePath) {
			return it.SSH
		}
	}
	return nil
}


// enterRemoteMode opens the protocol-agnostic remote connect form, focusing the
// first field. The form carries a protocol selector (ssh|ftp) so the same overlay
// drives both transports.
func (m *model) enterRemoteMode() {
	m.remoteMode = true
	m.remoteBrowseMode = false
	m.remoteBrowseErr = ""
	m.remoteHostPrompt = nil
	m.remoteAfterTrustOpen = false

	m.remoteFormFocus = 0
	for i := range m.remoteForm {
		m.remoteForm[i].Blur()
	}

	m.remoteForm[0].Focus()
}

// remoteDefaultPort returns the well-known default port for a protocol.
func remoteDefaultPort(proto string) int {
	if proto == remote.ProtocolFTP {
		return 21
	}

	return 22
}

// remoteFormConfig assembles a remote.Config from the connect-form fields. The
// auth type is "key" when the auth field says so (ssh only); otherwise
// "password". The port defaults per protocol when left blank. The known_hosts /
// cert trust store comes from the app config, so the browser trusts the same
// store as the desktop app.
func (m *model) remoteFormConfig() remote.Config {
	proto := m.remoteForm[0].Value()
	if proto == "" {
		proto = remote.ProtocolSSH
	}

	host := m.remoteForm[1].Value()
	portRaw := m.remoteForm[2].Value()
	user := m.remoteForm[3].Value()

	authType := m.remoteForm[4].Value()
	if authType == "" {
		authType = authTypePassword
	}

	password := m.remoteForm[5].Value()
	keyPath := m.remoteForm[6].Value()
	path := m.remoteForm[7].Value()

	port := remoteDefaultPort(proto)

	if portRaw != "" {
		if p, err := strconv.Atoi(portRaw); err == nil {
			port = p
		}
	}

	return remote.Config{
		Protocol:       proto,
		Host:           host,
		Port:           port,
		Username:       user,
		AuthType:       authType,
		Password:       password,
		KeyPath:        keyPath,
		RemoteFilePath: path,
	}
}

// remoteFormProto returns the protocol currently selected in the form.
func (m *model) remoteFormProto() string {
	p := m.remoteForm[0].Value()
	if p == "" {
		return remote.ProtocolSSH
	}

	return p
}

// remoteFormAuth returns the auth type currently selected in the form.
func (m *model) remoteFormAuth() string {
	a := m.remoteForm[4].Value()
	if a == "" {
		return authTypePassword
	}

	return a
}

// remoteFieldFocusable reports whether form field i should be reachable by
// Tab/arrows. The key-path field is skipped for ftp (ftp uses password auth
// only) and when ssh auth is "password"; the password field is skipped for ssh
// key auth. This realizes the auth-type selector hiding password vs key fields.
func (m *model) remoteFieldFocusable(i int) bool {
	switch i {
	case 6: // key path
		return m.remoteFormProto() != remote.ProtocolFTP && m.remoteFormAuth() == "key"
	case 5: // password
		return m.remoteFormProto() != remote.ProtocolSSH || m.remoteFormAuth() != "key"
	}

	return true
}

// remoteFormKey handles key input while the connect form is open.
func (m *model) remoteFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A pending unknown-host trust prompt takes precedence over form editing.
	if m.remoteHostPrompt != nil {
		return m.remoteHostTrustKey(msg)
	}
	// While the remote stream is opening off-thread, swallow input so the user
	// cannot re-trigger the open or mutate form state mid-flight.
	if m.remoteOpening {
		return *m, nil
	}

	switch msg.String() {
	case "tab", keyEnter:
		return m.remoteFormAdvance()
	case "shift+tab", "up", "k":
		m.remoteFormFocusPrev()

		return *m, nil
	case keyDown, "j":
		m.remoteFormFocusNext()

		return *m, nil
	case keyEsc:
		m.closeRemoteForm()

		return *m, nil
	}

	var cmd tea.Cmd

	m.remoteForm[m.remoteFormFocus], cmd = m.remoteForm[m.remoteFormFocus].Update(msg)

	return *m, cmd
}

// remoteFormAdvance moves focus to the next focusable field, or submits the form
// when the focused field was the last one.
func (m *model) remoteFormAdvance() (tea.Model, tea.Cmd) {
	m.remoteForm[m.remoteFormFocus].Blur()

	if m.remoteFormFocus < len(m.remoteForm)-1 {
		// advance to the next focusable field
		if next := m.nextFocusableField(m.remoteFormFocus + 1); next >= 0 {
			m.remoteFormFocus = next
			m.remoteForm[m.remoteFormFocus].Focus()

			return *m, nil
		}
	}

	return m.remoteSubmit()
}

// remoteFormFocusPrev moves focus to the previous focusable field, staying on the
// current one when there is none.
func (m *model) remoteFormFocusPrev() {
	m.remoteForm[m.remoteFormFocus].Blur()

	if prev := m.prevFocusableField(m.remoteFormFocus - 1); prev >= 0 {
		m.remoteFormFocus = prev
	}

	m.remoteForm[m.remoteFormFocus].Focus()
}

// remoteFormFocusNext moves focus to the next focusable field, staying on the
// current one when there is none.
func (m *model) remoteFormFocusNext() {
	m.remoteForm[m.remoteFormFocus].Blur()

	if next := m.nextFocusableField(m.remoteFormFocus + 1); next >= 0 {
		m.remoteFormFocus = next
	}

	m.remoteForm[m.remoteFormFocus].Focus()
}

// nextFocusableField returns the first focusable field index at or after from,
// or -1 when there is none.
func (m *model) nextFocusableField(from int) int {
	for i := from; i < len(m.remoteForm); i++ {
		if m.remoteFieldFocusable(i) {
			return i
		}
	}

	return -1
}

// prevFocusableField returns the first focusable field index at or before from,
// or -1 when there is none.
func (m *model) prevFocusableField(from int) int {
	for i := from; i >= 0; i-- {
		if m.remoteFieldFocusable(i) {
			return i
		}
	}

	return -1
}

// closeRemoteForm dismisses the connect form, blurring every field.
func (m *model) closeRemoteForm() {
	m.remoteMode = false
	for i := range m.remoteForm {
		m.remoteForm[i].Blur()
	}
}

// remoteSubmit validates the form and either opens the stream (concrete path) or
// opens the remote directory browser (empty path).
func (m *model) remoteSubmit() (tea.Model, tea.Cmd) {
	cfg := m.remoteFormConfig()
	if cfg.Host == "" {
		m.remoteBrowseErr = "host is required"
		return *m, nil
	}

	if cfg.Username == "" {
		m.remoteBrowseErr = "username is required"
		return *m, nil
	}

	if cfg.AuthType == "key" {
		if cfg.Protocol != remote.ProtocolSSH {
			m.remoteBrowseErr = "key auth is ssh-only"
			return *m, nil
		}

		if cfg.KeyPath == "" {
			m.remoteBrowseErr = "key path is required for key auth"
			return *m, nil
		}
	} else if cfg.Password == "" {
		m.remoteBrowseErr = "password is required"
		return *m, nil
	}

	if cfg.RemoteFilePath == "" {
		// No path → browse the remote home directory for a log file.
		m.remoteMode = false
		m.remoteBrowseMode = true
		m.remoteCurrentPath = "~"
		m.remoteCursor = 0
		m.remoteLoading = true
		m.remoteBrowseErr = ""
		m.remoteAfterTrustOpen = false
		m.remoteProto = cfg.Protocol

		return *m, m.remoteListCmd("~", cfg)
	}
	// Open the stream off the UI thread so a large remote log does not freeze
	// the terminal (H1). The result arrives as a remoteOpenedMsg and swaps the
	// viewer state in Update.
	m.remoteOpening = true

	return *m, m.openRemoteCmd(cfg)
}

// remoteBrowseKey handles key input while the remote directory browser is open.
func (m *model) remoteBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.remoteHostPrompt != nil {
		return m.remoteHostTrustKey(msg)
	}

	if m.remoteLoading {
		return *m, nil
	}

	if m.remoteOpening {
		return *m, nil
	}

	switch msg.String() {
	case "up", "k":
		if len(m.remoteEntries) > 0 {
			m.remoteCursor = (m.remoteCursor + len(m.remoteEntries) - 1) % len(m.remoteEntries)
		}
	case keyDown, "j":
		if len(m.remoteEntries) > 0 {
			m.remoteCursor = (m.remoteCursor + 1) % len(m.remoteEntries)
		}
	case keyEnter:
		return m.remoteBrowseOpen()
	case "backspace", "u", "h":
		return m.remoteBrowseUp()
	case keyEsc:
		m.remoteBrowseBack()
	}

	return *m, nil
}

// remoteBrowseOpen navigates into the highlighted directory, or opens the
// highlighted file's stream off the UI thread.
func (m *model) remoteBrowseOpen() (tea.Model, tea.Cmd) {
	if len(m.remoteEntries) == 0 {
		return *m, nil
	}

	e := m.remoteEntries[m.remoteCursor]
	if e.Kind == entryKindDir || e.Kind == entryKindSymlink {
		m.remoteLoading = true

		return *m, m.remoteListCmd(e.Path, m.remoteFormConfig())
	}
	// A file: fill the path and open the stream directly.
	cfg := m.remoteFormConfig()
	cfg.RemoteFilePath = e.Path
	m.remoteOpening = true

	return *m, m.openRemoteCmd(cfg)
}

// remoteBrowseUp lists the parent directory, unless the browser already sits on
// the filesystem root ("/"). Home ("~") is no longer a hard ceiling: after the
// first listing the path is promoted to the absolute remote home, so users can
// walk up to /var/log etc.
func (m *model) remoteBrowseUp() (tea.Model, tea.Cmd) {
	if m.remoteCurrentPath == "/" {
		return *m, nil
	}

	parent := parentDir(m.remoteCurrentPath)
	if parent == m.remoteCurrentPath {
		// Still unresolved "~"/"" — jump to filesystem root.
		parent = "/"
	}
	m.remoteLoading = true

	return *m, m.remoteListCmd(parent, m.remoteFormConfig())
}

// remoteBrowseBack returns to the connect form with its values preserved.
func (m *model) remoteBrowseBack() {
	m.remoteBrowseMode = false
	m.remoteMode = true
	m.remoteEntries = nil

	m.remoteFormFocus = 0
	for i := range m.remoteForm {
		m.remoteForm[i].Blur()
	}

	m.remoteForm[0].Focus()
}

// remoteHostTrustKey resolves the inline unknown-host trust prompt (y = trust and
// continue, n/esc = cancel). Trusting persists the key and then either re-opens
// the stream (form submit) or re-lists the directory (browser). FTP has no
// equivalent prompt.
func (m *model) remoteHostTrustKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	host := ""
	if m.remoteHostPrompt != nil {
		host = m.remoteHostPrompt.Host
	}

	switch msg.String() {
	case "y", "Y":
		line := m.remoteHostPrompt.Line

		m.remoteHostPrompt = nil
		if m.remoteAfterTrustOpen {
			// Re-run the stream open that tripped the host-key prompt, off the
			// UI thread (H1).
			m.remoteOpening = true
			return *m, m.openRemoteCmd(m.remoteFormConfig())
		}

		knownHosts := ""
		if m.cfg != nil {
			knownHosts = m.cfg.KnownHostsPath()
		}

		if err := ssh.WriteKnownHost(knownHosts, line); err != nil {
			m.remoteBrowseErr = err.Error()
			return *m, nil
		}

		return *m, m.remoteListCmd(m.remoteCurrentPath, m.remoteFormConfig())
	case "n", "N", keyEsc:
		m.remoteHostPrompt = nil
		m.remoteBrowseErr = fmt.Sprintf("host key for %s not trusted", host)

		return *m, nil
	}

	return *m, nil
}

// remoteListCmd is the async tea.Cmd that dials and lists one remote directory.
// It dispatches on the protocol to the matching backend lister.
func (m *model) remoteListCmd(path string, cfg remote.Config) tea.Cmd {
	return func() tea.Msg {
		knownHosts := ""
		if m.cfg != nil {
			knownHosts = m.cfg.KnownHostsPath()
		}

		var (
			lister remote.Lister
			lerr   error
		)

		switch cfg.Protocol {
		case remote.ProtocolFTP:
			lister, lerr = ftp.NewLister(ftp.Config{
				Host:           cfg.Host,
				Port:           cfg.Port,
				Username:       cfg.Username,
				AuthType:       authTypePassword,
				Password:       cfg.Password,
				RemoteFilePath: "",
				TLSMode:        cfg.TLSMode,
				KnownHostsPath: knownHosts,
			})
		default:
			lister, lerr = ssh.NewLister(ssh.Config{
				Host:           cfg.Host,
				Port:           cfg.Port,
				Username:       cfg.Username,
				AuthType:       cfg.AuthType,
				Password:       cfg.Password,
				KeyPath:        cfg.KeyPath,
				RemoteFilePath: "",
				KnownHostsPath: knownHosts,
			})
		}

		if lerr != nil {
			return remoteListedMsg{path: path, err: lerr}
		}

		// A browse-only connection has nothing useful to report on close, and
		// logging from here would corrupt the alternate screen, so the close
		// error is deliberately dropped.
		defer func() { _ = lister.Close() }()

		entries, lerr := lister.ListDir(m.ctx, path)

		resolved := path
		if lerr == nil && (path == "" || path == "~") {
			home := ""
			if h, herr := lister.HomeDir(m.ctx); herr == nil {
				home = h
			}
			resolved = resolveListedPath(path, entries, home)
		}

		return remoteListedMsg{path: resolved, entries: entries, err: lerr}
	}
}

// remoteOpenedMsg carries the result of an asynchronous remote-file open: the
// dial + initial download + local index all run off the UI thread (see
// openRemoteCmd), so the terminal never freezes on a large remote log.
type remoteOpenedMsg struct {
	cfg     remote.Config
	idx     *indexer.FileIndexer
	se      *search.SearchEngine
	cleanup func()
	status  string
	// hkErr is set (instead of err) when the dial tripped an unknown-host trust
	// prompt; the handler then surfaces the prompt rather than a bare error.
	hkErr *ssh.HostKeyUnknownError
	err   error
}

// openRemoteCmd is the async tea.Cmd that streams, downloads, and indexes a
// concrete remote log off the Bubbletea goroutine. It returns a remoteOpenedMsg
// whose handler swaps the viewer state — this is what keeps the TUI responsive
// during the (potentially minutes-long) remote open (H1).
func (m *model) openRemoteCmd(cfg remote.Config) tea.Cmd {
	return func() tea.Msg {
		knownHosts := ""
		if m.cfg != nil {
			knownHosts = m.cfg.KnownHostsPath()
		}

		var (
			s   remote.Streamer
			err error
		)

		switch cfg.Protocol {
		case remote.ProtocolFTP:
			s, err = ftp.NewStreamer(m.ctx, ftp.Config{
				Host:           cfg.Host,
				Port:           cfg.Port,
				Username:       cfg.Username,
				AuthType:       authTypePassword,
				Password:       cfg.Password,
				RemoteFilePath: cfg.RemoteFilePath,
				TLSMode:        cfg.TLSMode,
				KnownHostsPath: knownHosts,
			})
		default:
			s, err = ssh.NewStreamerContext(m.ctx, ssh.Config{
				Host:           cfg.Host,
				Port:           cfg.Port,
				Username:       cfg.Username,
				AuthType:       cfg.AuthType,
				Password:       cfg.Password,
				KeyPath:        cfg.KeyPath,
				RemoteFilePath: cfg.RemoteFilePath,
				KnownHostsPath: knownHosts,
			})
		}

		if err != nil {
			var hk *ssh.HostKeyUnknownError
			if errors.As(err, &hk) {
				return remoteOpenedMsg{cfg: cfg, hkErr: hk}
			}

			return remoteOpenedMsg{cfg: cfg, err: err}
		}

		idx, ierr := indexer.NewFileIndexer(s.LocalPath())
		if ierr != nil {
			_ = s.Close()
			return remoteOpenedMsg{cfg: cfg, err: ierr}
		}

		return remoteOpenedMsg{
			cfg:     cfg,
			idx:     idx,
			se:      search.NewSearchEngine(idx),
			cleanup: func() { _ = s.Close() },
			status:  fmt.Sprintf("opened %s %s", cfg.Protocol, cfg.RemoteFilePath),
		}
	}
}

// parentDir returns the parent directory of p. "/" is its own root; "~" is
// treated as unresolved home and maps to "/" so the browser can leave home once
// the absolute path is known (see resolveListedPath).
func parentDir(p string) string {
	if p == "/" {
		return "/"
	}
	if p == "~" || p == "" {
		return "/"
	}

	p = strings.TrimRight(p, "/")

	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/"
	}

	return p[:idx]
}

// resolveListedPath turns a "~"/"" request into the absolute directory that was
// actually listed, using the first entry's parent or the lister's HomeDir.
func resolveListedPath(requested string, entries []remote.DirEntry, home string) string {
	if requested != "" && requested != "~" {
		return requested
	}
	if len(entries) > 0 {
		return parentDir(entries[0].Path)
	}
	if home != "" {
		return home
	}
	return requested
}
