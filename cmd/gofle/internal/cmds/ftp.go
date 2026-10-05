// Package cmds implements the gofle CLI subcommands (ssh, ftp, recent) and the
// shared helpers they use to resolve targets, connect to remote hosts and open
// the interactive viewer.
package cmds

import (
	"github.com/spf13/cobra"
)

func init() {
	FTPCmd.Flags().StringP("host", "H", "", "remote host ([user@]host); positional <host> also works")
	FTPCmd.Flags().StringP("user", "u", "", "remote username")
	FTPCmd.Flags().IntP("port", "p", 21, "remote port")
	FTPCmd.Flags().String("remote", "", "remote file path (optional; omit to browse interactively)")
	FTPCmd.Flags().StringP("key", "k", "", "ignored for ftp (key auth unsupported)")
	FTPCmd.Flags().Bool("view", false, "open the streamed file in the interactive terminal viewer (default)")
	FTPCmd.Flags().Bool("dump", false, "print the streamed file to stdout instead of opening the viewer")
	FTPCmd.Flags().Bool("insecure-accept-host-key", false, "trust an unknown FTPS certificate instead of aborting")
	FTPCmd.Flags().String("tls", "", "FTP transport security: \"\" (plain), \"explicit\" or \"implicit\"")
}

// FTPCmd connects to a remote host over FTP/FTPS, streams a log file locally, and
// opens it in the interactive terminal viewer by default (or dumps to stdout
// with --dump). It mirrors SSHCmd but addresses the ftp:// scheme. --remote is
// OPTIONAL: when omitted an interactive directory browser is launched.
var FTPCmd = &cobra.Command{
	Use:   "ftp [flags] <host>",
	Short: "Connect to a remote host and stream a log file over FTP/FTPS",
	Long: `Connect to a remote host over FTP or FTPS (explicit/implicit) and stream a
remote log file into a local temp file.

By default the file is opened in the interactive terminal viewer (--view).
Pass --dump to print it to stdout instead (useful for piping).

The host is taken from the positional <host> argument or from --host/-H
(exactly one of the two). Both accept a "[user@]host" form; a username
embedded there is used unless --user is given explicitly.

--remote is optional. When omitted, an interactive browser lists the remote
directories so you can pick a file.

FTP authenticates with a password only. The password is always read
interactively from the terminal (no echo; never accepted as a flag, so it
does not appear in shell history). An unknown FTPS certificate
(explicit/implicit) aborts unless --insecure-accept-host-key is given.`,
	Args:          UsageArgs(cobra.MaximumNArgs(1)),
	RunE:          runFTP,
	SilenceUsage:  true,
	SilenceErrors: true,
	Example: `  # open in interactive terminal viewer (default; password prompted)
  gofle ftp user@host --remote /var/log/sys.log

  # dump to stdout (for piping)
  gofle ftp user@host --remote /var/log/sys.log --dump

  # FTPS explicit + browse directory
  gofle ftp --host host -u user --tls explicit

  # FTPS implicit + dump
  gofle ftp user@host --tls implicit --remote /var/log/x.log --dump`,
}

func runFTP(cmd *cobra.Command, args []string) error {
	return runRemote(cmd, "ftp", args)
}
