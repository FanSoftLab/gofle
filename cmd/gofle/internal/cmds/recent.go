package cmds

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/cmd/gofle/internal/tui"
	"GoFLE/pkg/config"
	"GoFLE/pkg/remote"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// RecentCmd opens recently used files (shared with the GoFLE GUI).
// Invoked as `gofle recent` (Open recent).
var RecentCmd = &cobra.Command{
	Use:   "recent",
	Short: "Open recent files",
	Long: `Open a recently used log file (local or SSH/FTP), shared with the GoFLE
desktop app. Running 'gofle recent' launches an interactive picker: use ↑/↓ (or
k/j) to choose an entry, enter to open it, b to browse another file on the same SSH/FTP server, esc/q to cancel. For non-interactive
use, 'gofle recent open <n>' reopens entry <n> directly.`,
	Example: `  gofle recent              # interactive recent-file picker (Open recent)
  gofle recent open 1       # reopen recent entry 1 in the viewer
  gofle recent clear        # clear the recent list`,
	Args:         UsageArgs(cobra.MaximumNArgs(0)),
	RunE:         runRecentList,
	SilenceUsage: true,
	// Errors print exactly once via main() (root silences cobra's printer).
	SilenceErrors: true,
}

var recentOpenCmd = &cobra.Command{
	Use:   "open <n>",
	Short: "Reopen recent entry <n> in the viewer",
	Args:  UsageArgs(cobra.ExactArgs(1)),
	RunE:  runRecentOpen,
}

var recentClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the recent list",
	Args:  UsageArgs(cobra.MaximumNArgs(0)),
	RunE:  runRecentClear,
}

func init() {
	RecentCmd.AddCommand(recentOpenCmd, recentClearCmd)
}

func runRecentList(cmd *cobra.Command, _ []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}

	items := cfg.GetRecentFiles()
	if len(items) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No recent files.")
		return nil
	}
	// Interactive picker (up/down to choose, enter to open, b to browse the
	// same remote server), identical in feel to the in-viewer "open recent" key.
	pick, err := tui.RunRecentPicker(items)
	if err != nil {
		return err
	}

	if pick.Browse != nil {
		return browseRecentServer(cmd, cfg, *pick.Browse)
	}

	if pick.Target == "" {
		return nil // canceled
	}

	return OpenViewer(cmd, pick.Target)
}

// browseRecentServer opens the remote directory browser using credentials from
// a recent SSH/FTP entry, then opens the chosen file in the viewer. Mirrors the
// in-viewer "b browse other file on server" action for `gofle recent`.
func browseRecentServer(cmd *cobra.Command, cfg *config.AppConfig, item config.RecentFileItem) error {
	if item.SSH == nil {
		return fmt.Errorf("not a remote entry")
	}
	sum := item.SSH
	proto := sum.Protocol
	if proto == "" {
		proto = "ssh"
	}
	port := sum.Port
	if port == 0 {
		if proto == "ftp" {
			port = 21
		} else {
			port = 22
		}
	}
	auth := sum.AuthType
	if auth == "" {
		auth = authTypePassword
	}

	rcfg := remote.Config{
		Protocol:       proto,
		Host:           sum.Host,
		Port:           port,
		Username:       sum.Username,
		AuthType:       auth,
		Password:       sum.Password,
		KeyPath:        sum.KeyPath,
		KeyPassphrase:  sum.KeyPassphrase,
		RemoteFilePath: "",
		TLSMode:        sum.TLSMode,
		KnownHostsPath: cfg.KnownHostsPath(),
	}

	paths := remotePaths{
		KnownHostsPath: cfg.KnownHostsPath(),
		FTPCerts:       cfg.KnownHostsPath(),
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Start in the parent directory of the last opened remote file when known.
	start := sum.RemoteFilePath
	if start == "" || start == "~" {
		start = "/"
	} else {
		// parent of the file path
		if i := strings.LastIndex(start, "/"); i > 0 {
			start = start[:i]
		} else if i == 0 {
			start = "/"
		}
	}

	if rcfg.AuthType == authTypePassword && rcfg.Password == "" {
		pw, err := readPasswordTTY("Password: ")
		if err != nil {
			return err
		}
		rcfg.Password = pw
	}

	listCfg := rcfg
	listCfg.RemoteFilePath = ""
	lister, lerr := connectLister(ctx, listCfg, false, paths)
	if lerr != nil {
		return lerr
	}

	chosen, berr := runRemoteBrowser(ctx, cmd, lister, start)
	if cerr := lister.Close(); cerr != nil {
		// best-effort
		_ = cerr
	}
	if berr != nil {
		return berr
	}
	if chosen == "" {
		return nil // canceled in browser
	}

	rcfg.RemoteFilePath = chosen
	target := buildRemoteURL(proto, "", sum.Host, port, chosen)

	// Persist updated path for next time; keep credentials from the summary.
	summary := &config.SSHConfigSummary{
		Host:           sum.Host,
		Port:           port,
		Username:       sum.Username,
		RemoteFilePath: chosen,
		AuthType:       auth,
		Password:       rcfg.Password,
		KeyPath:        sum.KeyPath,
		KeyPassphrase:  sum.KeyPassphrase,
		Protocol:       proto,
		TLSMode:        sum.TLSMode,
	}
	if err := shared.RecordRecent(cfg, target, true, 0, summary); err != nil {
		// non-fatal
		_ = err
	}

	return OpenViewer(cmd, target)
}

func runRecentOpen(cmd *cobra.Command, args []string) error {
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 {
		return NewUsageError(fmt.Errorf("invalid recent index %q", args[0]))
	}

	cfg, err := shared.Config()
	if err != nil {
		return err
	}

	items := cfg.GetRecentFiles()
	if n > len(items) {
		return fmt.Errorf("no recent entry %d (have %d)", n, len(items))
	}

	item := items[n-1]

	target := item.Path
	if item.IsSSH && item.SSH != nil {
		// Canonical no-userinfo target: credentials come from the stored
		// summary during resolution, never from the URL. The scheme follows
		// the stored Protocol so an ftp:// recent entry reopens over FTP.
		proto := "ssh"
		if item.SSH.Protocol == "ftp" {
			proto = "ftp"
		}

		target = buildRemoteURL(proto, "", item.SSH.Host, item.SSH.Port, item.SSH.RemoteFilePath)
	}

	return OpenViewer(cmd, target)
}

func runRecentClear(cmd *cobra.Command, _ []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}
	// Propagate the persist failure instead of asserting a false "cleared".
	if err := cfg.ClearRecentFiles(); err != nil {
		return fmt.Errorf("failed to clear recent list: %w", err)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Recent list cleared.")

	return nil
}

// describeRecent renders a recent entry for CLI display. SSH entries are
// shown WITHOUT username or any credential material — just host:port/path.
func describeRecent(path string, isSSH bool, ssh *config.SSHConfigSummary) string {
	if isSSH && ssh != nil {
		return fmt.Sprintf("%s:%d%s", ssh.Host, ssh.Port, ssh.RemoteFilePath)
	}

	return path
}
