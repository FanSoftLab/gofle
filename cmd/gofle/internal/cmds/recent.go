package cmds

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/cmd/gofle/internal/tui"
	"GoFLE/pkg/config"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// RecentCmd lists recently opened files (shared with the GoFLE GUI).
var RecentCmd = &cobra.Command{
	Use:   "list",
	Short: "List recently opened files",
	Long: `Open a recently used log file (local or SSH), shared with the GoFLE
desktop app. Running 'gofle list' launches an interactive picker: use ↑/↓ (or
k/j) to choose an entry and enter to open it, esc/q to cancel. For non-interactive
use, 'gofle list open <n>' reopens entry <n> directly.`,
	Example: `  gofle list              # interactive recent-file picker
  gofle list open 1     # reopen recent entry 1 in the viewer
  gofle list clear      # clear the recent list`,
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
	// Interactive picker (up/down to choose, enter to open), identical in feel
	// to the in-viewer "open recent" key.
	target, err := tui.RunRecentPicker(items)
	if err != nil {
		return err
	}

	if target == "" {
		return nil // canceled
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
