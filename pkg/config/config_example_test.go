package config

import (
	"fmt"
	"testing"
)

func ExampleAppConfig_AddRecentFile() {
	t := &testing.T{}
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SecretKey: "test-key"}

	summary := &SSHConfigSummary{
		Host:           "server.example.com",
		Port:           22,
		Username:       "deploy",
		RemoteFilePath: "/var/log/app.log",
		AuthType:       "password",
		Password:       "secret123",
	}

	cfg.AddRecentFile("ssh://server.example.com/var/log/app.log", 1024, true, summary)

	recent := cfg.GetRecentFiles()
	fmt.Printf("Recent files: %d\n", len(recent))
	fmt.Printf("First: %s (SSH: %v)\n", recent[0].Path, recent[0].IsSSH)
	// Output:
	// Recent files: 1
	// First: ssh://server.example.com/var/log/app.log (SSH: true)
}

func ExampleAppConfig_GetRecentFiles() {
	t := &testing.T{}
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SecretKey: "test-key"}

	cfg.AddRecentFile("/var/log/syslog", 2048, false, nil)
	cfg.AddRecentFile("/var/log/auth.log", 4096, false, nil)

	recent := cfg.GetRecentFiles()
	for i, f := range recent {
		fmt.Printf("%d: %s (%d bytes)\n", i, f.Path, f.FileSize)
	}
	// Output:
	// 0: /var/log/auth.log (4096 bytes)
	// 1: /var/log/syslog (2048 bytes)
}

func Example_inputHistory() {
	t := &testing.T{}
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, MaxHistorySize: 100}

	cfg.AddInputHistory("search", "error")
	cfg.AddInputHistory("search", "timeout")
	cfg.AddInputHistory("search", "connection refused")

	history := cfg.GetInputHistory("search")
	for i, v := range history {
		fmt.Printf("%d: %s\n", i, v)
	}
	// Output:
	// 0: connection refused
	// 1: timeout
	// 2: error
}

func ExampleAppConfig_SaveSession() {
	t := &testing.T{}
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SaveTabsExit: true, SecretKey: "test-key"}

	session := SavedSession{
		ActiveTabID: "tab_2",
		TopLevel: []SavedTopEntry{
			{Type: "tab", ID: "tab_1"},
			{Type: "group", ID: "group_a"},
		},
		Tabs: []SavedTabItem{
			{ID: "tab_1", Title: "syslog", Path: "/var/log/syslog"},
			{ID: "tab_2", Path: "/var/log/auth.log", GroupID: "group_a"},
		},
		Groups: []SavedGroupItem{
			{ID: "group_a", Title: "Auth", Color: "#10b981", Collapsed: false, MemberIDs: []string{"tab_2"}},
		},
	}

	cfg.SaveSession(session)

	// Reload and verify
	reloaded := &AppConfig{configDir: dir, SaveTabsExit: true}
	reloaded.loadSavedSession()
	got := reloaded.GetSavedSession()

	fmt.Printf("Active: %s, Tabs: %d, Groups: %d\n", got.ActiveTabID, len(got.Tabs), len(got.Groups))
	// Output:
	// Active: tab_2, Tabs: 2, Groups: 1
}

func Example_encryptDecryptSecret() {
	cfg := &AppConfig{SecretKey: "my-secret-key-32-bytes-long!!", SecretSalt: "00000000000000000000000000000000"}
	plain := "super-secret-password"

	enc, _ := cfg.EncryptSecret(plain)
	dec, _ := cfg.DecryptSecret(enc)

	fmt.Printf("Encrypted: [varies]...\n")
	fmt.Printf("Decrypted: %s\n", dec)
	// Output:
	// Encrypted: [varies]...
	// Decrypted: super-secret-password
}

func Example_newTestConfig() {
	t := &testing.T{}
	cfg := NewTestConfig(t)
	fmt.Println("Test config created")
	fmt.Printf("Secret key len: %d\n", len(cfg.SecretKey))
	// Output:
	// Test config created
	// Secret key len: 64
}
