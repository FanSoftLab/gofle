package main

import (
	"GoFLE/pkg/config"
	"os"
	"testing"
	"time"
)

func TestNewSSHSummary(t *testing.T) {
	cfg := SSHConfig{
		Host:           "example.com",
		Port:           2222,
		Username:       "alice",
		AuthType:       "password",
		Password:       "secret",
		KeyPath:        "/home/alice/.ssh/id_ed25519",
		KeyPassphrase:  "passphrase",
		RemoteFilePath: "/var/log/app.log",
	}

	sum := newSSHSummary(cfg)

	if sum.Host != "example.com" {
		t.Fatalf("Host=%q, want example.com", sum.Host)
	}
	if sum.Port != 2222 {
		t.Fatalf("Port=%d, want 2222", sum.Port)
	}
	if sum.Username != "alice" {
		t.Fatalf("Username=%q, want alice", sum.Username)
	}
	if sum.AuthType != "password" {
		t.Fatalf("AuthType=%q, want password", sum.AuthType)
	}
	if sum.RemoteFilePath != "/var/log/app.log" {
		t.Fatalf("RemoteFilePath=%q, want /var/log/app.log", sum.RemoteFilePath)
	}
	if sum.Protocol != "" {
		t.Fatalf("Protocol=%q, want empty (ssh default)", sum.Protocol)
	}
}

func TestNewSSHSummaryZeroValues(t *testing.T) {
	cfg := SSHConfig{}
	sum := newSSHSummary(cfg)

	if sum.Host != "" {
		t.Fatalf("Host=%q, want empty", sum.Host)
	}
	if sum.Port != 0 {
		t.Fatalf("Port=%d, want 0", sum.Port)
	}
	if sum.Username != "" {
		t.Fatalf("Username=%q, want empty", sum.Username)
	}
	if sum.RemoteFilePath != "" {
		t.Fatalf("RemoteFilePath=%q, want empty", sum.RemoteFilePath)
	}
}

func TestAckSSHHostNoPending(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{
		cfg:              cfg,
		pendingSSHHost:   make(map[string]string),
		pendingSSHConfig: make(map[string]SSHConfig),
	}

	err := app.AckSSHHost("nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error for non-pending host, got nil")
	}

	if err.Error() != "no pending SSH host to accept: nonexistent.example.com" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAckSSHHostWritesKnownHost(t *testing.T) {
	cfg := config.NewTestConfig(t)
	knownHostsPath := cfg.KnownHostsPath()

	app := &App{
		cfg: cfg,
		pendingSSHHost: map[string]string{
			"example.com": "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKklabg",
		},
		pendingSSHConfig: map[string]SSHConfig{
			"example.com": {
				Host:           "example.com",
				Port:           22,
				Username:       "alice",
				RemoteFilePath: "/var/log/app.log",
			},
		},
	}

	err := app.AckSSHHost("example.com")
	if err != nil {
		t.Fatalf("AckSSHHost: %v", err)
	}

	// Verify the key was written.
	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("known_hosts not written: %v", err)
	}
	expected := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKklabg\n"
	if string(data) != expected {
		t.Fatalf("known_hosts content = %q, expected %q", string(data), expected)
	}

	// Pending maps should be cleared.
	if _, ok := app.pendingSSHHost["example.com"]; ok {
		t.Fatal("pendingSSHHost should be cleared after accept")
	}
	if _, ok := app.pendingSSHConfig["example.com"]; ok {
		t.Fatal("pendingSSHConfig should be cleared after accept")
	}
}

func TestAckSSHHostPartialPending(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{
		cfg: cfg,
		pendingSSHHost:   map[string]string{"example.com": "some-line"},
		pendingSSHConfig: map[string]SSHConfig{}, // no config for this host
	}

	err := app.AckSSHHost("example.com")
	if err == nil {
		t.Fatal("expected error when only host line pending but no config")
	}
}

func TestCheckConnRateLimitNotLimited(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{
		cfg:          cfg,
		connAttempts: make(map[string]*connAttemptInfo),
	}

	err := app.checkConnRateLimit("example.com", 22)
	if err != nil {
		t.Fatalf("checkConnRateLimit: %v, want nil (no prior attempts)", err)
	}
}

func TestCheckConnRateLimitLockedOut(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{
		cfg: cfg,
		connAttempts: map[string]*connAttemptInfo{
			"example.com:22": {
				count:        5,
				lockedUntil:  time.Now().Add(5 * time.Minute),
				lastAttempt:  time.Now(),
			},
		},
	}

	err := app.checkConnRateLimit("example.com", 22)
	if err == nil {
		t.Fatal("expected rate limit error when locked out, got nil")
	}
}

func TestRecordConnFailureThenSuccess(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{
		cfg:          cfg,
		connAttempts: make(map[string]*connAttemptInfo),
	}

	// No prior attempts — check should pass.
	if err := app.checkConnRateLimit("example.com", 22); err != nil {
		t.Fatalf("initial checkConnRateLimit: %v", err)
	}

	// Record failures up to the limit.
	for i := 0; i < 5; i++ {
		app.recordConnFailure("example.com", 22)
	}

	// After 5 failures, the host should be locked out.
	err := app.checkConnRateLimit("example.com", 22)
	if err == nil {
		t.Fatal("expected lockout after 5 failures, got nil")
	}

	// A success resets the counter.
	app.recordConnSuccess("example.com", 22)

	// After success, check should pass again.
	if err := app.checkConnRateLimit("example.com", 22); err != nil {
		t.Fatalf("checkConnRateLimit after success: %v, want nil", err)
	}
}
