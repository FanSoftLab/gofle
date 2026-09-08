package cmds

import (
	"GoFLE/pkg/ssh"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveSSHHost verifies the host-resolution rules for `gofle ssh`: the
// positional argument and --host are mutually exclusive, exactly one is
// required, and an embedded "[user@]" prefix fills the username unless an
// explicit --user already provides one.
func TestResolveSSHHost(t *testing.T) {
	cases := []struct {
		name       string
		positional string
		hostFlag   string
		username   string
		wantHost   string
		wantUser   string
		wantErr    string
	}{
		{name: "positional only", positional: "host1", wantHost: "host1", wantUser: ""},
		{name: "flag only", hostFlag: "host2", wantHost: "host2", wantUser: ""},
		{name: "user@host flag, no -u", hostFlag: "bob@host3", wantHost: "host3", wantUser: "bob"},
		{name: "user@host positional, no -u", positional: "amy@host4", wantHost: "host4", wantUser: "amy"},
		{name: "explicit -u wins over prefix", hostFlag: "bob@host5", username: "carol", wantHost: "host5", wantUser: "carol"},
		{name: "-u kept with plain host", positional: "host6", username: "dave", wantHost: "host6", wantUser: "dave"},
		{name: "both sources rejected", positional: "h", hostFlag: "h", wantErr: "not both"},
		{name: "missing host rejected", wantErr: "missing host"},
		{name: "bare @ rejected", hostFlag: "@host7", wantErr: "[user@]host"},
		{name: "bare user@ rejected", hostFlag: "bob@", wantErr: "[user@]host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, user, err := resolveSSHHost(tc.positional, tc.hostFlag, tc.username)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}

				var ue *UsageError
				if !errors.As(err, &ue) {
					t.Fatalf("expected UsageError classification, got %T", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if host != tc.wantHost || user != tc.wantUser {
				t.Fatalf("got (%q,%q) want (%q,%q)", host, user, tc.wantHost, tc.wantUser)
			}
		})
	}
}

// TestSSHCmdAcceptsHostFlag runs the real command tree with --host instead of a
// positional host. Since --remote is now OPTIONAL (an omitted --remote launches
// the interactive directory browser), the run must NOT fail on the old
// required-flag "--remote" check. It proceeds into the connect/browse flow and
// fails later (here: no stored credentials and no tty to read a password), which
// is the expected post-refactor behavior proving --host alone satisfies target.
func TestSSHCmdAcceptsHostFlag(t *testing.T) {
	assertHostFlagAcceptedWithoutRemote(t, SSHCmd)
}

// TestSSHCmdRejectsMissingHost verifies that invoking ssh with neither a
// positional host nor --host yields the guidance usage error (exit-2 class).
func TestSSHCmdRejectsMissingHost(t *testing.T) {
	t.Cleanup(func() { _ = SSHCmd.Flags().Set("host", "") })
	SSHCmd.SetArgs([]string{"-p", "22"})

	_, err := SSHCmd.ExecuteC()
	if err == nil || !strings.Contains(err.Error(), "missing host") {
		t.Fatalf("expected 'missing host' usage error, got: %v", err)
	}
}

// TestConnectSSHHonorsCancelledContext pins the CLI half of the cancellable-open
// work (M5): `gofle ssh` used to dial with a background context, so an interrupt
// during the connect or the initial full-file transfer had nothing to cancel —
// the command sat in a blocking open until a watchdog timeout fired. The context
// now reaches the streamer, which refuses to dial once it is cancelled.
func TestConnectSSHHonorsCancelledContext(t *testing.T) {
	cfg := ssh.Config{
		// Unroutable by RFC 5737 (TEST-NET-1): if cancellation were ignored, the
		// test would visibly hang on the dial instead of quietly passing.
		Host:           "192.0.2.1",
		Port:           22,
		Username:       "u",
		AuthType:       "password",
		Password:       "secret",
		RemoteFilePath: "/var/log/x.log",
		KnownHostsPath: filepath.Join(t.TempDir(), "known_hosts"),
	}

	assertConnectHonorsCancelledContext(t, "connectSSH", func(ctx context.Context) (io.Closer, error) {
		s, err := connectSSH(ctx, cfg, false, cfg.KnownHostsPath)
		if s == nil {
			return nil, err
		}

		return s, err
	})
}
