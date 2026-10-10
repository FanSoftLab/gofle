// Package main implements the gofle command-line entry point: it wires the
// cobra command tree, installs interrupt-scoped cancellation and maps command
// failures onto process exit codes.
package main

import (
	"GoFLE/cmd/gofle/internal/cmds"
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"GoFLE/pkg/logging"
	"GoFLE/pkg/version"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:          "version",
	Short:        "Print the gofle version",
	Args:         cmds.UsageArgs(cobra.NoArgs),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		info := version.Get()
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "gofle %s\n", info.String())
		return nil
	},
}

var rootCmd = &cobra.Command{
	Use:   "gofle [target]",
	Short: "GoFLE terminal companion — open logs locally or over SSH/FTP in an interactive viewer",
	// Long/Example are filled in init() so section headers and samples can use
	// the same palette as the interactive TUI help when stdout is a terminal.
	Args:         cmds.UsageArgs(cobra.MaximumNArgs(1)),
	SilenceUsage: true,
	// Errors are printed exactly once — by main() below. Cobra's own printer
	// is silenced everywhere so a returned error is never logged twice.
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return cmds.OpenViewer(cmd, args[0])
	},
}

// help palette (Catppuccin Mocha) — matches cmd/gofle/internal/tui help styles.
const (
	ansiReset   = "\033[0m"
	ansiTitle   = "\033[1;38;2;203;166;247m" // #cba6f7 bold
	ansiSection = "\033[1;38;2;249;226;175m" // #f9e2af bold
	ansiExample = "\033[38;2;166;227;161m"   // #a6e3a1
	ansiMuted   = "\033[38;2;166;173;200m"   // #a6adc8
)

func rootHelpLong(color bool) string {
	h := func(s string) string {
		if !color {
			return s
		}
		return ansiSection + s + ansiReset
	}
	title := "GoFLE terminal viewer — open local or remote logs interactively."
	if color {
		title = ansiTitle + title + ansiReset
	}
	note := "  Same backend as the desktop app. Config is shared (~/.config/GoFLE/)."
	if color {
		note = ansiMuted + note + ansiReset
	}
	return title + "\n\n" + note + "\n\n" +
		h("USAGE") + `
  gofle <file>                 local log
  gofle ssh://user@host/path   remote SSH
  gofle ftp://user@host/path   remote FTP/FTPS

` + h("COMMANDS") + `
  ssh / ftp     stream a remote file (viewer by default; --dump for stdout)
  recent        interactive recent-files picker
  presets       list / import / export search presets
  session       show / restore / clear saved tabs
  version       print version

` + h("VIEWER") + `  (press ? inside the viewer)
  Navigation    ↑↓ jk · PgUp/PgDn · g · Home/End
  Search        /  \  A  u  P · n p · x
  View          m  s  r  f
  Tabs          T  Ctrl+Tab  1–9  Ctrl+W  G  Y
  Other         F2 settings · S remote · q quit

` + h("VIEW MODES") + `  (settings.json → view_mode)
  split      log | results   (default)
  results    matching lines only
  normal     full log`
}

func rootHelpExample(color bool) string {
	ex := func(s string) string {
		if !color {
			return s
		}
		return ansiExample + s + ansiReset
	}
	cmt := func(s string) string {
		if !color {
			return s
		}
		return ansiMuted + s + ansiReset
	}
	return cmt("  # local") + "\n" +
		ex("  gofle /var/log/app.log") + "\n\n" +
		cmt("  # remote URLs (always open the interactive viewer)") + "\n" +
		ex("  gofle ssh://user@host/var/log/sys.log") + "\n" +
		ex("  gofle ftp://user@host/var/log/sys.log") + "\n\n" +
		cmt("  # ssh/ftp subcommands: default opens the viewer; add --dump for stdout") + "\n" +
		ex("  gofle ssh user@host --remote /var/log/x.log") + "\n" +
		ex("  gofle ssh user@host --remote /var/log/x.log --dump") + "\n" +
		ex("  gofle ftp user@host --tls explicit --remote /var/log/x.log") + "\n\n" +
		cmt("  # history") + "\n" +
		ex("  gofle recent") + "\n\n" +
		cmt("  # presets & session (shared with desktop)") + "\n" +
		ex("  gofle presets list") + "\n" +
		ex("  gofle presets import ./presets-export.json") + "\n" +
		ex("  gofle session show") + "\n" +
		ex("  gofle session restore")
}

func init() {
	color := shared.IsTTY() && os.Getenv("NO_COLOR") == ""
	rootCmd.Long = rootHelpLong(color)
	rootCmd.Example = rootHelpExample(color)

	// Flag parse failures (unknown flag, bad value) are usage errors: main()
	// maps them to exit code 2. Children inherit the root's flag error func.
	rootCmd.SetFlagErrorFunc(cmds.FlagErrorFunc())
	rootCmd.AddCommand(
		versionCmd,
		cmds.SSHCmd,
		cmds.FTPCmd,
		cmds.RecentCmd,
		cmds.PresetsCmd,
		cmds.SessionCmd,
	)

	// Add custom version flags
	rootCmd.Flags().Bool("version", false, "Print version and exit")
	rootCmd.Flags().BoolP("version-short", "v", false, "Print version and exit (shorthand)")

	// Check for --version/-v before command execution
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
			info := version.Get()
			_, _ = fmt.Fprintf(os.Stdout, "gofle %s\n", info.String())

			os.Exit(0)
		}

		if versionShort, _ := cmd.Flags().GetBool("version-short"); versionShort {
			info := version.Get()
			_, _ = fmt.Fprintf(os.Stdout, "gofle %s\n", info.String())

			os.Exit(0)
		}

		return nil
	}
}

func main() {
	// The whole body lives in run() so the deferred logging teardown below
	// actually runs: os.Exit skips deferred calls, so the exit code has to
	// travel back out of the function that owns the defers.
	os.Exit(run())
}

// run executes the root command and returns the process exit code.
func run() int {
	// Structured logging to stderr + gofle.log beside the shared config dir.
	defer logging.Setup(config.DefaultDir())()

	if _, err := shared.LoadCLIConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "gofle: failed to load config: %v\n", err)

		return 1
	}

	// Interrupt-scoped context for every command: an SSH open streams a whole
	// remote file before the viewer appears, and on a black-holed connection that
	// wait is bounded only by watchdog timeouts measured in tens of seconds. A
	// first Ctrl+C now cancels that open instead of being ignored.
	//
	// ONLY SIGINT is trapped. Trapping SIGTERM too would swallow the first
	// termination request on every path that does not observe the context (a local
	// index build, a gzip decompression), so `kill`, systemd and container
	// shutdowns would appear to do nothing until the SIGKILL timeout — worse
	// behavior than before. SIGTERM keeps its default disposition and still
	// terminates immediately.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		<-ctx.Done()
		// Restore the default disposition after the first interrupt: a second
		// Ctrl+C must kill the process outright, never be absorbed by a handler
		// that has already asked everything to wind down.
		stop()
		fmt.Fprintln(os.Stderr, "\ngofle: interrupted, winding down (press Ctrl+C again to force quit)")
	}()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		// Every command returns its errors here (SilenceErrors everywhere), so
		// this is the single print point — a failure can never exit silently.
		fmt.Fprintf(os.Stderr, "gofle: %v\n", err)
		// Unix convention: 2 = usage error (bad flags/args), 1 = runtime error.
		if _, ok := errors.AsType[*cmds.UsageError](err); ok {
			return 2
		}

		return 1
	}

	return 0
}
