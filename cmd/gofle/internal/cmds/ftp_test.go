package cmds

import (
	"GoFLE/pkg/ftp"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// TestFTPCmdAcceptsHostFlag mirrors TestSSHCmdAcceptsHostFlag but for the ftp
// command: --remote is optional, so a host-only invocation must NOT fail on the
// old required-flag "--remote" check. It proceeds into the connect/browse flow
// and fails later (no stored credentials / no password on a non-interactive run).
func TestFTPCmdAcceptsHostFlag(t *testing.T) {
	assertHostFlagAcceptedWithoutRemote(t, FTPCmd)
}

// TestFTPCmdRejectsMissingHost verifies that invoking ftp with neither a
// positional host nor --host yields the guidance usage error (exit-2 class).
func TestFTPCmdRejectsMissingHost(t *testing.T) {
	t.Cleanup(func() {
		_ = FTPCmd.Flags().Set("host", "")
		_ = FTPCmd.Flags().Set("remote", "")
	})
	FTPCmd.SetArgs([]string{"-p", "21"})

	_, err := FTPCmd.ExecuteC()
	if err == nil || !strings.Contains(err.Error(), "missing host") {
		t.Fatalf("expected 'missing host' usage error, got: %v", err)
	}
}

// TestFTPCmdRejectsKey verifies that ftp rejects the --key flag, since FTP
// authenticates with a password only.
func TestFTPCmdRejectsKey(t *testing.T) {
	t.Cleanup(func() {
		_ = FTPCmd.Flags().Set("host", "")
		_ = FTPCmd.Flags().Set("key", "")
		_ = FTPCmd.Flags().Set("remote", "")
	})
	FTPCmd.SetArgs([]string{"--host", "h", "--key", "/tmp/id_rsa"})

	_, err := FTPCmd.ExecuteC()
	if err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("expected ftp to reject --key, got: %v", err)
	}
}

// TestConnectFTPHonorsCancelledContext pins the FTP half of the cancellable-open
// work: connectFTP must not dial once its context is cancelled. A valid config is
// needed so NewStreamer reaches the ctx check (an invalid config would fail
// validation before the cancellation is observed).
func TestConnectFTPHonorsCancelledContext(t *testing.T) {
	cfg := ftp.Config{
		Host:           "192.0.2.1",
		Port:           21,
		Username:       "u",
		AuthType:       "password",
		Password:       "secret",
		RemoteFilePath: "/var/log/x.log",
		KnownHostsPath: filepath.Join(t.TempDir(), "ftp_certs"),
	}

	assertConnectHonorsCancelledContext(t, "connectFTP", func(ctx context.Context) (io.Closer, error) {
		s, err := connectFTP(ctx, cfg, false, cfg.KnownHostsPath)
		if s == nil {
			return nil, err
		}

		return s, err
	})
}

// TestFTPBuildRemoteURLSchemeDispatch verifies the canonical URL builder emits
// the correct scheme per protocol, matching what the recent list keys on.
func TestFTPBuildRemoteURLSchemeDispatch(t *testing.T) {
	if got := buildRemoteURL("ftp", "", "h", 21, "/x.log"); got != "ftp://h:21/x.log" {
		t.Fatalf("ftp url = %q, want ftp://h:21/x.log", got)
	}

	if got := buildRemoteURL("ssh", "", "h", 22, "/x.log"); got != "ssh://h:22/x.log" {
		t.Fatalf("ssh url = %q, want ssh://h:22/x.log", got)
	}
}
