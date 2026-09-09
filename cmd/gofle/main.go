// Package main implements the gofle command-line entry point: it wires the
// cobra command tree, installs interrupt-scoped cancellation and maps command
// failures onto process exit codes.
package main

import (
	"GoFLE/cmd/gofle/internal/cmds"
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"GoFLE/pkg/logging"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

// version is injected at build time via
// -ldflags "-X main.version=<ver>" (see Dockerfile/build.sh); "dev" is the
// fallback for plain `go build`. Note: the linker matches the main package as
// "main" here — the full import path does not inject.
var version = "dev"

var versionCmd = &cobra.Command{
	Use:          "version",
	Short:        "Print the gofle version",
	Args:         cmds.UsageArgs(cobra.NoArgs),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "gofle %s\n", version)
		return nil
	},
}

var rootCmd = &cobra.Command{
	Use:   "gofle [target]",
	Short: "GoFLE terminal companion — open logs locally or over SSH in an interactive viewer",
	Long: `GoFLE terminal viewer — open local or remote logs interactively.

  Same backend as the desktop app. Config is shared
  (~/.config/gofle/settings.json): theme, language, pane layout, view mode.

USAGE
  gofle <file>                 open local log
  gofle ssh://user@host/path   open over SSH
  gofle ftp://user@host/path   open over FTP/FTPS
  gofle                        this help

COMMANDS
  ssh     connect via SSH and stream a remote file
  ftp     connect via FTP/FTPS and stream a remote file
  recent  open recent files (interactive picker)
  version print version

VIEWER KEYS  (press ? inside the viewer for the full list)
  ↑↓ j k     scroll line          PgUp/PgDn  page
  /          search               \          AND-filter in results
  n p        next / prev match    [ ]        first / last match
  m          cycle view mode      s          split   r  results-only
  f          follow / tail        g          go to line
  e          export               o          recent files
  S          remote connect       q          quit

VIEW MODES  (saved in settings.json → view_mode)
  split      log | results   (default)
  results    matching lines only
  normal     full log`,
	Example: `  # local
  gofle /var/log/app.log

  # remote URLs
  gofle ssh://user@host/var/log/sys.log
  gofle ftp://user@host/var/log/sys.log

  # interactive remote session
  gofle ssh host -u user -p 22 --remote /var/log/x.log --view
  gofle ftp host -u user --tls explicit --remote /var/log/x.log --view

  # history
  gofle recent`,
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

func init() {
	// Flag parse failures (unknown flag, bad value) are usage errors: main()
	// maps them to exit code 2. Children inherit the root's flag error func.
	rootCmd.SetFlagErrorFunc(cmds.FlagErrorFunc())
	rootCmd.AddCommand(
		versionCmd,
		cmds.SSHCmd,
		cmds.FTPCmd,
		cmds.RecentCmd,
	)

	// Add custom version flags
	rootCmd.Flags().Bool("version", false, "Print version and exit")
	rootCmd.Flags().BoolP("version-short", "v", false, "Print version and exit (shorthand)")

	// Check for --version/-v before command execution
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
			_, _ = fmt.Fprintf(os.Stdout, "gofle %s\n", version)

			os.Exit(0)
		}

		if versionShort, _ := cmd.Flags().GetBool("version-short"); versionShort {
			_, _ = fmt.Fprintf(os.Stdout, "gofle %s\n", version)

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
