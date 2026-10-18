package main

import (
	"GoFLE/pkg/config"
	"testing"
)

func TestSaveAndGetSavedSession(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{cfg: cfg}

	session := config.SavedSession{
		ActiveTabID: "tab1",
		TopLevel: []config.SavedTopEntry{
			{Type: "tab", ID: "tab1"},
			{Type: "group", ID: "group1"},
		},
		Tabs: []config.SavedTabItem{
			{
				ID:      "tab1",
				Title:   "server.log",
				Path:    "/var/log/server.log",
				IsSSH:   false,
				GroupID: "group1",
			},
		},
		Groups: []config.SavedGroupItem{
			{
				ID:        "group1",
				Title:     "Servers",
				Color:     "#cba6f7",
				Collapsed: false,
				MemberIDs: []string{"tab1"},
			},
		},
	}

	if err := app.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := app.GetSavedSession()
	if err != nil {
		t.Fatalf("GetSavedSession: %v", err)
	}

	if got.ActiveTabID != "tab1" {
		t.Fatalf("ActiveTabID=%q, want tab1", got.ActiveTabID)
	}
	if len(got.TopLevel) != 2 {
		t.Fatalf("TopLevel len=%d, want 2", len(got.TopLevel))
	}
	if len(got.Tabs) != 1 {
		t.Fatalf("Tabs len=%d, want 1", len(got.Tabs))
	}
	if got.Tabs[0].Title != "server.log" {
		t.Fatalf("Tab title=%q, want server.log", got.Tabs[0].Title)
	}
	if len(got.Groups) != 1 {
		t.Fatalf("Groups len=%d, want 1", len(got.Groups))
	}
	if got.Groups[0].Color != "#cba6f7" {
		t.Fatalf("Group color=%q, want #cba6f7", got.Groups[0].Color)
	}
}

func TestClearSavedSession(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{cfg: cfg}

	session := config.SavedSession{
		ActiveTabID: "tab1",
		TopLevel:    []config.SavedTopEntry{{Type: "tab", ID: "tab1"}},
		Tabs: []config.SavedTabItem{
			{ID: "tab1", Title: "log.txt", Path: "/var/log/log.txt", IsSSH: false},
		},
	}

	if err := app.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if err := app.ClearSavedSession(); err != nil {
		t.Fatalf("ClearSavedSession: %v", err)
	}

	got, err := app.GetSavedSession()
	if err != nil {
		t.Fatalf("GetSavedSession after clear: %v", err)
	}

	if got.ActiveTabID != "" {
		t.Fatalf("ActiveTabID=%q, want empty after clear", got.ActiveTabID)
	}
	if len(got.TopLevel) != 0 {
		t.Fatalf("TopLevel len=%d, want 0 after clear", len(got.TopLevel))
	}
	if len(got.Tabs) != 0 {
		t.Fatalf("Tabs len=%d, want 0 after clear", len(got.Tabs))
	}
	if len(got.Groups) != 0 {
		t.Fatalf("Groups len=%d, want 0 after clear", len(got.Groups))
	}
}

func TestGetRecentFilesWithSSHCreds(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{cfg: cfg}

	// Add a recent SSH file with encrypted credentials.
	summary := &config.SSHConfigSummary{
		Host:           "example.com",
		Port:           22,
		Username:       "alice",
		Password:       "secret-password",
		KeyPath:        "/home/alice/.ssh/id_ed25519",
		KeyPassphrase:  "key-passphrase",
		RemoteFilePath: "/var/log/app.log",
		Protocol:       "ssh",
	}

	if err := cfg.AddRecentFile("ssh://example.com:22/var/log/app.log", 1024, true, summary); err != nil {
		t.Fatalf("AddRecentFile: %v", err)
	}

	recent, err := app.GetRecentFiles()
	if err != nil {
		t.Fatalf("GetRecentFiles: %v", err)
	}

	if len(recent) != 1 {
		t.Fatalf("recent len=%d, want 1", len(recent))
	}

	item := recent[0]
	if item.Path != "ssh://example.com:22/var/log/app.log" {
		t.Fatalf("Path=%q, want ssh://example.com:22/var/log/app.log", item.Path)
	}
	if !item.IsSSH {
		t.Fatal("IsSSH should be true")
	}
	if item.SSH == nil {
		t.Fatal("SSH should not be nil")
	}
	// Credentials should be decrypted.
	if item.SSH.Username != "alice" {
		t.Fatalf("Username=%q, want alice", item.SSH.Username)
	}
	if item.SSH.Password != "secret-password" {
		t.Fatalf("Password=%q, want secret-password", item.SSH.Password)
	}
	if item.SSH.KeyPassphrase != "key-passphrase" {
		t.Fatalf("KeyPassphrase=%q, want key-passphrase", item.SSH.KeyPassphrase)
	}
}

func TestGetRecentFilesLocalNotSSH(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{cfg: cfg}

	if err := cfg.AddRecentFile("/var/log/local.log", 512, false, nil); err != nil {
		t.Fatalf("AddRecentFile: %v", err)
	}

	recent, err := app.GetRecentFiles()
	if err != nil {
		t.Fatalf("GetRecentFiles: %v", err)
	}

	if len(recent) != 1 {
		t.Fatalf("recent len=%d, want 1", len(recent))
	}

	item := recent[0]
	if item.Path != "/var/log/local.log" {
		t.Fatalf("Path=%q, want /var/log/local.log", item.Path)
	}
	if item.IsSSH {
		t.Fatal("IsSSH should be false for local file")
	}
	if item.SSH != nil {
		t.Fatal("SSH should be nil for local file")
	}
}

func TestGetRecentFilesEmpty(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{cfg: cfg}

	recent, err := app.GetRecentFiles()
	if err != nil {
		t.Fatalf("GetRecentFiles: %v", err)
	}

	if len(recent) != 0 {
		t.Fatalf("recent len=%d, want 0", len(recent))
	}
}
