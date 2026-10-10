package cmds

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/cmd/gofle/internal/tui"
	"GoFLE/pkg/config"
	"GoFLE/pkg/ssh"
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
)

// OpenViewer opens the interactive TUI viewer for a local file or an ssh://
// target. It performs the work formerly done by the `view` subcommand: load the
// shared config, resolve the target (streaming + indexing for ssh://),
// interactively trust an unknown SSH host key on first contact, record the open
// in the recent list, and finally run the Bubbletea viewer.
//
// It is the single entry point used by `gofle <target>` (root RunE),
// `gofle recent open <n>` and `gofle ssh --view`.
func OpenViewer(cmd *cobra.Command, target string) error {
	return openViewer(cmd, target, false)
}

// OpenViewerRestore is like OpenViewer but also restores remaining tabs from the
// shared saved session once the primary target is open (`gofle session restore`).
func OpenViewerRestore(cmd *cobra.Command, target string) error {
	return openViewer(cmd, target, true)
}

func openViewer(cmd *cobra.Command, target string, autoRestore bool) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("no target specified")
	}

	cfg, err := shared.LoadCLIConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// The command's context carries the signal cancellation installed by main:
	// Ctrl+C during a remote open aborts the dial/transfer instead of leaving the
	// terminal parked on a connection that will never answer. A command built
	// outside Execute (tests) has no context.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if after, ok := strings.CutPrefix(target, shared.SSHScheme); ok {
		// The resolve below dials + streams silently; give the terminal user a
		// minimal wait cue (full progress lives in the desktop overlay).
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connecting and streaming %s …\n", after)
	}

	idx, se, cleanup, err := shared.ResolveTarget(ctx, target, cfg)
	if err != nil {
		hk, ok := errors.AsType[*ssh.HostKeyUnknownError](err)
		if !ok {
			return err
		}

		trusted, terr := confirmHostKey(cmd, cfg, hk)
		if terr != nil {
			return terr
		}

		if !trusted {
			return fmt.Errorf("host key not trusted for %q: %w", target, err)
		}

		idx, se, cleanup, err = shared.ResolveTarget(ctx, target, cfg)
		if err != nil {
			return fmt.Errorf("failed to open %q after trusting host key: %w", target, err)
		}
	}

	isSSH := strings.HasPrefix(target, shared.SSHScheme)
	// The viewer is about to open; a failed recent-list write is logged so the
	// session is not aborted over bookkeeping.
	if rerr := shared.RecordRecent(cfg, target, isSSH, idx.FileSize(), nil); rerr != nil {
		slog.Warn("failed to persist recent entry", "target", target, "err", rerr)
	}

	return tui.RunView(ctx, idx, se, cleanup, cfg, target, autoRestore)
}

// confirmHostKey surfaces an unknown SSH host key's fingerprint, asks the user
// whether to trust it, and records it in known_hosts when the answer is "y". It
// reports whether the key was accepted; a declined key is not an error, while a
// failure to persist an accepted key is.
func confirmHostKey(cmd *cobra.Command, cfg *config.AppConfig, hk *ssh.HostKeyUnknownError) (bool, error) {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "The authenticity of host %q can't be established.\n%s\nTrust this host? (y/N) ",
		hk.Host, hk.Fingerprint)

	reader := bufio.NewReader(cmd.InOrStdin())

	line, readErr := reader.ReadString('\n')
	// A read failure (closed or EOF stdin) means the user cannot answer, which
	// is a decline rather than a failure of its own: only an explicit "y" trusts
	// the host.
	trusted := readErr == nil && strings.TrimSpace(strings.ToLower(line)) == "y"

	if !trusted {
		return false, nil
	}

	if werr := ssh.WriteKnownHost(cfg.KnownHostsPath(), hk.Line); werr != nil {
		return false, fmt.Errorf("failed to write known_hosts: %w", werr)
	}

	return true, nil
}
