package cmds

import (
	"GoFLE/pkg/ssh"
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	SSHCmd.Flags().StringP("host", "H", "", "remote host ([user@]host); positional <host> also works")
	SSHCmd.Flags().StringP("user", "u", "", "remote username")
	SSHCmd.Flags().IntP("port", "p", 22, "remote port")
	SSHCmd.Flags().String("remote", "", "remote file path (optional; omit to browse interactively)")
	SSHCmd.Flags().StringP("key", "k", "", "path to private key for key auth")
	SSHCmd.Flags().String("password", "", "password for password auth")
	SSHCmd.Flags().Bool("view", false, "open the streamed file in the viewer")
	SSHCmd.Flags().Bool("dump", false, "print the streamed file to stdout (default)")
	SSHCmd.Flags().Bool("insecure-accept-host-key", false, "trust an unknown host key instead of aborting")
}

// SSHCmd connects to a remote host over SSH, streams a log file locally, and
// either dumps it or opens it in the viewer. The host is taken from the
// positional <host> argument or --host/-H (exactly one of the two). --remote is
// OPTIONAL: when omitted an interactive directory browser is launched.
var SSHCmd = &cobra.Command{
	Use:   "ssh [flags] <host>",
	Short: "Connect to a remote host and stream a log file over SSH",
	Long: `Connect to a remote host over SSH, stream a remote log file into a local
temp file, and either print it (--dump) or open it in the viewer (--view).

The host is taken from the positional <host> argument or from --host/-H
(exactly one of the two). Both accept a "[user@]host" form; a username
embedded there is used unless --user is given explicitly.

--remote is optional. When omitted, an interactive browser lists the remote
directories so you can pick a file. Connection details (user, port, key/
password, remote path) may be supplied via flags; unknown host keys abort
unless --insecure-accept-host-key is given.`,
	Args:          UsageArgs(cobra.MaximumNArgs(1)),
	RunE:          runSSH,
	SilenceUsage:  true,
	SilenceErrors: true,
	Example: `  gofle ssh user@host -p 22 --remote /var/log/sys.log --view
  gofle ssh --host host -u user -p 2222 --remote /var/log/x.log --dump
  gofle ssh user@host --remote /var/log            # browse the directory
  gofle ssh user@host --key ~/.ssh/id_ed25519 --remote /var/log/x.log --view`,
}

// resolveSSHHost is the SSH-specific alias of resolveRemoteHost: host resolution
// rules are identical for ssh and ftp.
func resolveSSHHost(positional, hostFlag, username string) (host, user string, err error) {
	return resolveRemoteHost(positional, hostFlag, username)
}

func runSSH(cmd *cobra.Command, args []string) error {
	return runRemote(cmd, "ssh", args)
}

// connectSSH opens an SSH streamer, transparently accepting and retrying once
// on an unknown host key when insecure mode is requested. ctx bounds both
// attempts: a dial, handshake or initial transfer that never completes is
// abandoned when the user interrupts the command.
func connectSSH(ctx context.Context, sshCfg ssh.Config, insecure bool, knownHosts string) (*ssh.Streamer, error) {
	s, err := ssh.NewStreamerContext(ctx, sshCfg)
	if err == nil {
		return s, nil
	}

	var hkErr *ssh.HostKeyUnknownError
	if errors.As(err, &hkErr) && insecure {
		if werr := ssh.WriteKnownHost(knownHosts, hkErr.Line); werr != nil {
			return nil, fmt.Errorf("failed to record host key: %w", werr)
		}

		return ssh.NewStreamerContext(ctx, sshCfg)
	}

	if errors.As(err, &hkErr) {
		return nil, fmt.Errorf("unknown SSH host key for %s (fingerprint %s); re-run with --insecure-accept-host-key to trust it",
			hkErr.Host, hkErr.Fingerprint)
	}

	return nil, err
}

// buildSSHURL reconstructs the canonical ssh:// target used for recent-list
// keys and viewer opens: "ssh://host:port/remote/path". The username is
// deliberately omitted — credentials live in the encrypted recent-entry
// summary, and the same spelling is what the GUI records, so entries from
// both tools dedupe onto one key.
func buildSSHURL(username, host string, port int, remotePath string) string {
	return buildRemoteURL("ssh", username, host, port, remotePath)
}
