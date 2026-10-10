package cmds

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// SessionCmd shows or restores the shared tab session.
var SessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Show or restore the saved tab session (shared with the desktop app)",
	Args:  UsageArgs(cobra.MaximumNArgs(0)),
	RunE:  runSessionShow,
}

var sessionShowCmd = &cobra.Command{
	Use:   "show",
	Short: "List tabs in the saved session",
	Args:  UsageArgs(cobra.MaximumNArgs(0)),
	RunE:  runSessionShow,
}

var sessionRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Open the first saved tab in the viewer (then press Y to load the rest)",
	Long: `Opens the previously active tab from the shared saved session and automatically
loads the remaining tabs (and groups) into the interactive viewer.

Requires "save tabs on exit" (Settings → Save tabs on exit, or desktop equivalent).
You can still press Y later to re-sync any tabs that failed to open.`,
	Args: UsageArgs(cobra.MaximumNArgs(0)),
	RunE: runSessionRestore,
}

var sessionClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the saved session",
	Args:  UsageArgs(cobra.MaximumNArgs(0)),
	RunE:  runSessionClear,
}

func init() {
	SessionCmd.AddCommand(sessionShowCmd, sessionRestoreCmd, sessionClearCmd)
}

func runSessionShow(cmd *cobra.Command, _ []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}
	sess := cfg.GetSavedSession()
	if len(sess.Tabs) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No saved session.")
		return nil
	}
	groupTitle := map[string]string{}
	for _, g := range sess.Groups {
		groupTitle[g.ID] = g.Title
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Saved session: %d tab(s)", len(sess.Tabs))
	if len(sess.Groups) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), ", %d group(s)", len(sess.Groups))
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	for i, t := range sess.Tabs {
		label := t.Title
		if label == "" {
			label = filepath.Base(t.Path)
		}
		mark := " "
		if t.ID == sess.ActiveTabID {
			mark = "*"
		}
		g := ""
		if t.GroupID != "" {
			if title, ok := groupTitle[t.GroupID]; ok && title != "" {
				g = " [" + title + "]"
			} else {
				g = " [group]"
			}
		}
		path := displayTabPath(t)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s%2d  %s%s\n    %s\n", mark, i+1, label, g, path)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\nRestore: gofle session restore   (loads all tabs automatically)")
	return nil
}

func runSessionRestore(cmd *cobra.Command, _ []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}
	sess := cfg.GetSavedSession()
	if len(sess.Tabs) == 0 {
		return fmt.Errorf("no saved session to restore")
	}

	tab := sess.Tabs[0]
	for _, t := range sess.Tabs {
		if t.ID == sess.ActiveTabID {
			tab = t
			break
		}
	}
	target := targetFromSavedTabItem(tab)
	if target == "" {
		return fmt.Errorf("saved tab has empty path")
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Restoring %s …\n", target)
	if len(sess.Tabs) > 1 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Tip: press Y in the viewer to open the remaining tabs.")
	}
	return OpenViewer(cmd, target)
}

func runSessionClear(cmd *cobra.Command, _ []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}
	if err := cfg.ClearSavedSession(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Saved session cleared.")
	return nil
}

func displayTabPath(t config.SavedTabItem) string {
	if t.IsSSH && t.SSH != nil {
		proto := t.SSH.Protocol
		if proto == "" {
			proto = "ssh"
		}
		host := t.SSH.Host
		if t.SSH.Port != 0 {
			host = fmt.Sprintf("%s:%d", host, t.SSH.Port)
		}
		p := t.SSH.RemoteFilePath
		if p == "" {
			p = t.Path
		}
		return proto + "://" + host + p
	}
	return t.Path
}

func targetFromSavedTabItem(t config.SavedTabItem) string {
	if t.IsSSH && t.SSH != nil {
		proto := t.SSH.Protocol
		if proto == "" {
			proto = "ssh"
		}
		var b strings.Builder
		if proto == "ftp" {
			b.WriteString("ftp://")
		} else {
			b.WriteString("ssh://")
		}
		if t.SSH.Username != "" {
			b.WriteString(t.SSH.Username)
			b.WriteString("@")
		}
		b.WriteString(t.SSH.Host)
		if t.SSH.Port != 0 {
			fmt.Fprintf(&b, ":%d", t.SSH.Port)
		}
		p := t.SSH.RemoteFilePath
		if p == "" {
			p = t.Path
		}
		if p == "" {
			p = "/"
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		b.WriteString(p)
		return b.String()
	}
	return t.Path
}
