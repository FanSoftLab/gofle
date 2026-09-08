package cmds

import (
	"GoFLE/cmd/gofle/internal/shared"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// assertHostFlagAcceptedWithoutRemote runs cmd with --host only and asserts the
// invocation is NOT rejected over the (now optional) --remote flag: the run has
// to proceed into the connect/browse flow and fail there instead. Shared by the
// ssh and ftp variants of the same guarantee so both stay in lockstep.
func assertHostFlagAcceptedWithoutRemote(t *testing.T, cmd *cobra.Command) {
	t.Helper()

	// Cobra retains parsed flag values across ExecuteC calls on the same
	// command, so leftover flags would leak into sibling tests.
	t.Cleanup(func() {
		for _, name := range []string{"host", "remote", "password", "key"} {
			_ = cmd.Flags().Set(name, "")
		}

		_ = cmd.Flags().Set("view", "false")
	})
	cmd.SetArgs([]string{"--host", "somehost"})

	_, err := cmd.ExecuteC()
	if err == nil {
		t.Fatal("expected a failure (no stored credentials / no password)")
	}

	if strings.Contains(err.Error(), "--remote") {
		t.Fatalf("expected --remote to be optional; got failure on it: %v", err)
	}
}

// assertConnectHonorsCancelledContext pins the CLI half of the cancellable-open
// work: a connect helper must refuse to dial once its context is cancelled
// instead of sitting in a blocking open until a watchdog timeout fires. connect
// must return a nil closer when it fails, so a typed-nil never reaches Close.
func assertConnectHonorsCancelledContext(t *testing.T, name string, connect func(context.Context) (io.Closer, error)) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)

	go func() {
		c, err := connect(ctx)
		if c != nil {
			_ = c.Close()
		}

		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s error = %v, want context.Canceled", name, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s ignored the cancelled context and blocked on the dial", name)
	}
}

// TestOpenViewerRejectsUnknownLocalTarget checks that OpenViewer surfaces a
// clear error for a local path that does not exist (it must not try to launch
// the TUI, which would block on a TTY).
func TestOpenViewerRejectsUnknownLocalTarget(t *testing.T) {
	if err := OpenViewer(&cobra.Command{}, "/nonexistent/path/does-not-exist-98765.log"); err == nil {
		t.Fatal("expected error for a nonexistent local file")
	}
}

// TestOpenViewerSSHWithoutCredentials verifies that an ssh:// target with no
// cached credentials fails with the expected guidance error rather than hanging
// or launching the viewer.
func TestOpenViewerSSHWithoutCredentials(t *testing.T) {
	err := OpenViewer(&cobra.Command{}, "ssh://bob@host.example.com/var/log/x.log")
	if err == nil {
		t.Fatal("expected error for ssh target without stored credentials")
	}

	if !errors.Is(err, shared.ErrNoStoredCredentials) {
		t.Fatalf("expected ErrNoStoredCredentials, got %v", err)
	}
}

// TestRecentOpenUsesOpenViewer ensures that reopening a recent entry routes
// through OpenViewer (i.e. no dangling reference to the removed ViewCmd). The
// compile-time absence of ViewCmd is the primary guard; this exercises the
// recent-open path with an out-of-range index to confirm the wiring without a TTY.
func TestRecentOpenUsesOpenViewer(t *testing.T) {
	// Out-of-range index returns a clean error from runRecentOpen without
	// launching the viewer.
	err := runRecentOpen(nil, []string{"999"})
	if err == nil {
		t.Fatal("expected error for out-of-range recent index")
	}
}

// TestRecentCommandRejectsArg confirms that `gofle list` is interactive and
// no longer accepts a numeric argument: passing one is a usage error (the user
// is meant to pick with ↑/↓ and open with enter), so no TUI is launched.
func TestRecentCommandRejectsArg(t *testing.T) {
	var out strings.Builder
	RecentCmd.SetOut(&out)
	RecentCmd.SetArgs([]string{"1"})

	_, err := RecentCmd.ExecuteC()
	if err == nil {
		t.Fatal("expected `gofle list 1` to be rejected (interactive picker takes no args)")
	} else if !strings.Contains(err.Error(), "accepts at most 0 arg") {
		t.Fatalf("unexpected error: %v", err)
	}
}
