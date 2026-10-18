package main

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/remote"
	"os"
	"path/filepath"
	"testing"
)

func TestRemotePathDisplay(t *testing.T) {
	cases := []struct {
		proto string
		host  string
		port  int
		path  string
		want  string
	}{
		{"ssh", "example.com", 22, "/var/log/app.log", "ssh://example.com:22/var/log/app.log"},
		{"ftp", "example.com", 21, "/var/log/app.log", "ftp://example.com:21/var/log/app.log"},
		{"ssh", "192.168.1.100", 2222, "/tmp/log.txt", "ssh://192.168.1.100:2222/tmp/log.txt"},
		{"ftp", "files.example.com", 990, "/logs/server.log", "ftp://files.example.com:990/logs/server.log"},
	}

	for _, tc := range cases {
		t.Run(tc.proto+"_"+tc.host, func(t *testing.T) {
			got := remotePathDisplay(tc.proto, tc.host, tc.port, tc.path)
			if got != tc.want {
				t.Fatalf("remotePathDisplay(%q, %q, %d, %q) = %q, want %q",
					tc.proto, tc.host, tc.port, tc.path, got, tc.want)
			}
		})
	}
}

func TestResolveFTPCertStoreWithKnownHostsPath(t *testing.T) {
	cfg := remote.Config{
		KnownHostsPath: "/custom/ftp_certs",
	}

	got := resolveFTPCertStore(cfg)
	if got != "/custom/ftp_certs" {
		t.Fatalf("resolveFTPCertStore = %q, want %q", got, "/custom/ftp_certs")
	}
}

func TestResolveFTPCertStoreDefault(t *testing.T) {
	cfg := remote.Config{}
	got := resolveFTPCertStore(cfg)

	// Should contain the app dir name and ftp_certs.
	expectedSuffix := filepath.Join("gofle", "ftp_certs")
	if len(got) < len(expectedSuffix) || got[len(got)-len(expectedSuffix):] != expectedSuffix {
		t.Fatalf("resolveFTPCertStore default = %q, want suffix %q", got, expectedSuffix)
	}
}

func TestAckFTPCertNoPending(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{
		cfg:              cfg,
		pendingFTPHost:   make(map[string]string),
		pendingFTPConfig: make(map[string]remote.Config),
	}

	err := app.AckFTPCert("nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error for non-pending host, got nil")
	}

	if err.Error() != "no pending FTP host to accept: nonexistent.example.com" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAckFTPCertWritesCertTrust(t *testing.T) {
	certStorePath := filepath.Join(t.TempDir(), "ftp_certs")
	cfg := config.NewTestConfig(t)

	app := &App{
		cfg: cfg,
		pendingFTPHost: map[string]string{
			"ftp.example.com": "ftp.example.com sha256:AB12CD34...",
		},
		pendingFTPConfig: map[string]remote.Config{
			"ftp.example.com": {
				Host:           "ftp.example.com",
				Port:           21,
				Username:       "alice",
				RemoteFilePath: "/var/log/app.log",
				TLSMode:        "explicit",
				KnownHostsPath: certStorePath,
			},
		},
	}

	err := app.AckFTPCert("ftp.example.com")
	if err != nil {
		t.Fatalf("AckFTPCert: %v", err)
	}

	// Verify the cert was written.
	data, err := os.ReadFile(certStorePath)
	if err != nil {
		t.Fatalf("ftp_certs not written: %v", err)
	}
	expected := "ftp.example.com sha256:AB12CD34...\n"
	if string(data) != expected {
		t.Fatalf("ftp_certs content = %q, expected %q", string(data), expected)
	}

	// Pending maps should be cleared.
	if _, ok := app.pendingFTPHost["ftp.example.com"]; ok {
		t.Fatal("pendingFTPHost should be cleared after accept")
	}
	if _, ok := app.pendingFTPConfig["ftp.example.com"]; ok {
		t.Fatal("pendingFTPConfig should be cleared after accept")
	}
}

func TestAckFTPCertPartialPending(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{
		cfg: cfg,
		pendingFTPHost:   map[string]string{"ftp.example.com": "some-line"},
		pendingFTPConfig: map[string]remote.Config{}, // no config for this host
	}

	err := app.AckFTPCert("ftp.example.com")
	if err == nil {
		t.Fatal("expected error when only host line pending but no config")
	}
}

