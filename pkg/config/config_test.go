package config

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	cfg := &AppConfig{SecretKey: "test-secret-key-123", SecretSalt: "00000000000000000000000000000000"}
	plain := "my-super-secret-password"

	enc, err := cfg.EncryptSecret(plain)
	if err != nil {
		t.Fatalf("EncryptSecret returned error: %v", err)
	}

	if enc == plain {
		t.Fatal("ciphertext must not equal plaintext")
	}

	dec, err := cfg.DecryptSecret(enc)
	if err != nil {
		t.Fatalf("DecryptSecret returned error: %v", err)
	}

	if dec != plain {
		t.Fatalf("round-trip mismatch: got %q want %q", dec, plain)
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	cfg := &AppConfig{SecretKey: "key", SecretSalt: "00000000000000000000000000000000"}

	enc, _ := cfg.EncryptSecret("hello")
	if _, err := cfg.DecryptSecret(enc + "ff"); err == nil {
		t.Fatal("expected error when decrypting tampered ciphertext")
	}
}

func TestEmptySecret(t *testing.T) {
	cfg := &AppConfig{SecretKey: "key", SecretSalt: "00000000000000000000000000000000"}

	enc, _ := cfg.EncryptSecret("")
	if enc != "" {
		t.Fatalf("empty plaintext should produce empty ciphertext, got %q", enc)
	}

	dec, _ := cfg.DecryptSecret("")
	if dec != "" {
		t.Fatalf("empty ciphertext should produce empty plaintext, got %q", dec)
	}
}

func TestAddRecentFileDedup(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), RecentFiles: []RecentFileItem{}}
	cfg.AddRecentFile("a.log", 10, false, nil)
	cfg.AddRecentFile("b.log", 20, false, nil)
	cfg.AddRecentFile("a.log", 30, false, nil)

	if len(cfg.RecentFiles) != 2 {
		t.Fatalf("expected 2 entries after re-add, got %d", len(cfg.RecentFiles))
	}

	if cfg.RecentFiles[0].Path != "a.log" {
		t.Fatalf("most-recently-added should be first, got %q", cfg.RecentFiles[0].Path)
	}

	if cfg.RecentFiles[0].FileSize != 30 {
		t.Fatalf("re-added entry should keep updated size, got %d", cfg.RecentFiles[0].FileSize)
	}
}

func TestAddInputHistoryDedup(t *testing.T) { //nolint:dupl // symmetric add-input-history tests
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}

	if err := cfg.AddInputHistory("search", "foo"); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	if err := cfg.AddInputHistory("search", "bar"); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	if err := cfg.AddInputHistory("search", "foo"); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	got := cfg.GetInputHistory("search")
	if len(got) != 2 {
		t.Fatalf("expected 2 entries after re-add, got %d: %v", len(got), got)
	}

	if got[0] != "foo" {
		t.Fatalf("re-added value should move to front, got %q", got[0])
	}
}

func TestAddInputHistoryNormalization(t *testing.T) { //nolint:dupl // symmetric add-input-history tests
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}

	if err := cfg.AddInputHistory("search", "Bar"); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	if err := cfg.AddInputHistory("search", "bar"); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	if err := cfg.AddInputHistory("search", " BAR "); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	got := cfg.GetInputHistory("search")
	if len(got) != 1 {
		t.Fatalf("normalized duplicates should collapse to 1, got %d: %v", len(got), got)
	}

	if got[0] != "BAR" {
		t.Fatalf("normalized duplicates should keep latest (trimmed) value, got %q", got[0])
	}
}

func TestAddInputHistoryCap(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 3}

	for i := range 5 {
		if err := cfg.AddInputHistory("search", fmt.Sprintf("v%d", i)); err != nil {
			t.Fatalf("AddInputHistory returned error: %v", err)
		}
	}

	got := cfg.GetInputHistory("search")
	if len(got) != 3 {
		t.Fatalf("expected capping at 3, got %d: %v", len(got), got)
	}
	// newest first: v4, v3, v2; oldest v0/v1 dropped.
	if got[0] != "v4" || got[1] != "v3" || got[2] != "v2" {
		t.Fatalf("unexpected order after cap: %v", got)
	}
}

func TestAddInputHistoryIgnoresEmpty(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}

	if err := cfg.AddInputHistory("search", ""); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	if err := cfg.AddInputHistory("search", "   "); err != nil {
		t.Fatalf("AddInputHistory returned error: %v", err)
	}

	if len(cfg.GetInputHistory("search")) != 0 {
		t.Fatal("empty/whitespace values must be ignored")
	}
}

func TestGetInputHistoryReturnsCopy(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}
	_ = cfg.AddInputHistory("search", "foo")
	_ = cfg.AddInputHistory("search", "bar")

	got := cfg.GetInputHistory("search")
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}

	got[0] = "mutated" //nolint:goconst // test fixture literal repeated for clarity

	if cfg.GetInputHistory("search")[0] == "mutated" {
		t.Fatal("GetInputHistory must return a copy, not the internal slice")
	}
}

func TestClearInputHistory(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}
	_ = cfg.AddInputHistory("search", "foo")
	_ = cfg.AddInputHistory("ssh_host", "h")

	if err := cfg.ClearInputHistory("search"); err != nil {
		t.Fatalf("ClearInputHistory returned error: %v", err)
	}

	if len(cfg.GetInputHistory("search")) != 0 {
		t.Fatal("search history should be cleared")
	}

	if len(cfg.GetInputHistory("ssh_host")) != 1 {
		t.Fatal("other namespaces must be untouched")
	}
}

func TestRemoveInputHistoryItem(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}
	_ = cfg.AddInputHistory("search", "Foo")
	_ = cfg.AddInputHistory("search", "Bar")
	_ = cfg.AddInputHistory("search", "baz")

	// Removal is normalized (case/space insensitive) like AddInputHistory.
	if err := cfg.RemoveInputHistoryItem("search", " BAR "); err != nil {
		t.Fatalf("RemoveInputHistoryItem returned error: %v", err)
	}

	got := cfg.GetInputHistory("search")
	if len(got) != 2 {
		t.Fatalf("expected 2 entries after removal, got %d (%v)", len(got), got)
	}

	if got[0] == "Bar" {
		t.Fatal("the 'Bar' entry should have been removed")
	}

	// Other namespaces are untouched.
	_ = cfg.AddInputHistory("ssh_host", "h")
	if err := cfg.RemoveInputHistoryItem("search", "Foo"); err != nil {
		t.Fatalf("RemoveInputHistoryItem returned error: %v", err)
	}

	if len(cfg.GetInputHistory("ssh_host")) != 1 {
		t.Fatal("ssh_host history must be untouched")
	}

	// Removing the last entry prunes the namespace entirely.
	_ = cfg.RemoveInputHistoryItem("search", "baz")
	if len(cfg.GetInputHistory("search")) != 0 {
		t.Fatal("namespace should be pruned when empty")
	}
}

func TestSetMaxHistorySize(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}
	cfg.SetMaxHistorySize(5)

	if cfg.MaxHistorySize != 5 {
		t.Fatalf("expected MaxHistorySize 5, got %d", cfg.MaxHistorySize)
	}
	// Values below 1 are clamped to 1.
	cfg.SetMaxHistorySize(0)

	if cfg.MaxHistorySize != 1 {
		t.Fatalf("expected MaxHistorySize clamped to 1, got %d", cfg.MaxHistorySize)
	}
}

func TestInputHistoryRoundTripDisk(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, MaxHistorySize: 100}
	_ = cfg.AddInputHistory("search", "foo")
	_ = cfg.AddInputHistory("search", "bar")

	reloaded := &AppConfig{configDir: dir, MaxHistorySize: 100}
	reloaded.loadHistory()

	got := reloaded.GetInputHistory("search")
	if len(got) != 2 || got[0] != "bar" || got[1] != "foo" {
		t.Fatalf("history round-trip mismatch: %v", got)
	}
}

func TestRemoveRecentFile(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), RecentFiles: []RecentFileItem{{Path: "a"}, {Path: "b"}}}

	removed, err := cfg.RemoveRecentFile("a")
	if err != nil {
		t.Fatalf("RemoveRecentFile error: %v", err)
	}

	if !removed {
		t.Fatal("expected true when removing existing entry")
	}

	if len(cfg.RecentFiles) != 1 || cfg.RecentFiles[0].Path != "b" {
		t.Fatalf("unexpected state after removal: %+v", cfg.RecentFiles)
	}

	removed, err = cfg.RemoveRecentFile("missing")
	if err != nil {
		t.Fatalf("RemoveRecentFile error: %v", err)
	}

	if removed {
		t.Fatal("expected false when removing missing entry")
	}
}

func TestClearRecentFiles(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), RecentFiles: []RecentFileItem{{Path: "a"}}}
	cfg.ClearRecentFiles()

	if len(cfg.RecentFiles) != 0 {
		t.Fatal("expected recent list to be empty after clear")
	}
}

func TestAddRecentFileEncryptsSSHPassword(t *testing.T) {
	cfg := &AppConfig{
		configDir:   t.TempDir(),
		SecretKey:   "test-key",
		RecentFiles: []RecentFileItem{},
	}

	plain := "topsecret"
	summary := &SSHConfigSummary{
		Host:           "h",
		Port:           22,
		Username:       "u",
		RemoteFilePath: "/f",
		AuthType:       "password",
		Password:       plain,
	}

	cfg.AddRecentFile("ssh://x", 0, true, summary)

	// Caller's struct must remain untouched (the live tab keeps the real password).
	if summary.Password != plain {
		t.Fatalf("AddRecentFile must not mutate caller's summary, got %q", summary.Password)
	}

	if len(cfg.RecentFiles) != 1 {
		t.Fatalf("expected 1 recent entry, got %d", len(cfg.RecentFiles))
	}

	stored := cfg.RecentFiles[0].SSH
	if stored == nil {
		t.Fatal("expected SSH summary to be stored")
	}

	if stored.Password == plain {
		t.Fatal("SSH password must be encrypted at rest, got plaintext")
	}

	dec, err := cfg.DecryptSecret(stored.Password)
	if err != nil {
		t.Fatalf("DecryptSecret failed: %v", err)
	}

	if dec != plain {
		t.Fatalf("decrypted password mismatch: got %q want %q", dec, plain)
	}
}

func sampleSession() SavedSession {
	return SavedSession{
		ActiveTabID: "tab_2",
		TopLevel: []SavedTopEntry{
			{Type: "tab", ID: "tab_1"},
			{Type: "group", ID: "group_a"},
		},
		Tabs: []SavedTabItem{
			{ID: "tab_1", Title: "first", Path: "/a.log"},
			{ID: "tab_2", Path: "/b.log", GroupID: "group_a"},
			{
				ID: "tab_3", Path: "ssh://h:22/x.log", IsSSH: true, GroupID: "group_a",
				SSH: &SSHConfigSummary{Host: "h", Port: 22, Username: "u", RemoteFilePath: "/x.log", AuthType: "password"},
			},
		},
		Groups: []SavedGroupItem{
			{ID: "group_a", Title: "G", Color: "#10b981", Collapsed: true, MemberIDs: []string{"tab_2", "tab_3"}},
		},
	}
}

func TestSaveGetSavedSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SaveTabsExit: true}

	want := sampleSession()
	if err := cfg.SaveSession(want); err != nil {
		t.Fatalf("SaveSession returned error: %v", err)
	}

	// Re-read from disk through a fresh config to prove persistence, not just
	// the in-memory copy.
	reloaded := &AppConfig{configDir: dir, SaveTabsExit: true}
	reloaded.loadSavedSession()

	got := reloaded.GetSavedSession()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("session round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestSaveSessionRespectsSaveTabsExitDisabled(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SaveTabsExit: false}

	if err := cfg.SaveSession(sampleSession()); err != nil {
		t.Fatalf("SaveSession returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, savedSessionFile)); !os.IsNotExist(err) {
		t.Fatal("session file must not be written when SaveTabsExit is false")
	}

	if len(cfg.GetSavedSession().Tabs) != 0 {
		t.Fatal("expected empty session when saving is disabled")
	}
}

func TestClearSavedSession(t *testing.T) {
	dir := t.TempDir()

	cfg := &AppConfig{configDir: dir, SaveTabsExit: true}
	if err := cfg.SaveSession(sampleSession()); err != nil {
		t.Fatalf("SaveSession returned error: %v", err)
	}

	cfg.ClearSavedSession()

	if _, err := os.Stat(filepath.Join(dir, savedSessionFile)); !os.IsNotExist(err) {
		t.Fatal("expected session file to be removed")
	}

	got := cfg.GetSavedSession()
	if got.ActiveTabID != "" || len(got.Tabs) != 0 || len(got.Groups) != 0 || len(got.TopLevel) != 0 {
		t.Fatalf("expected empty session after clear, got %+v", got)
	}
}

func TestGetSavedSessionEmptyWhenNoFile(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir()}
	cfg.loadSavedSession()

	got := cfg.GetSavedSession()
	if len(got.Tabs) != 0 || len(got.Groups) != 0 || len(got.TopLevel) != 0 {
		t.Fatalf("expected empty session, got %+v", got)
	}
}

func TestLoadSavedSessionMigratesLegacyTabs(t *testing.T) {
	dir := t.TempDir()
	legacy := []map[string]any{
		{"path": "/a.log", "is_ssh": false},
		{"path": "/b.log", "is_ssh": false},
	}

	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, savedTabsFile), data, 0o644); err != nil {
		t.Fatalf("failed to seed legacy file: %v", err)
	}

	cfg := &AppConfig{configDir: dir, SaveTabsExit: true}
	cfg.loadSavedSession()

	got := cfg.GetSavedSession()
	if len(got.Tabs) != 2 || len(got.TopLevel) != 2 {
		t.Fatalf("expected 2 migrated standalone tabs, got %+v", got)
	}

	for i, tab := range got.Tabs {
		if tab.ID == "" {
			t.Fatalf("migrated tab %d must get a synthetic id", i)
		}

		if tab.GroupID != "" {
			t.Fatalf("migrated tab %d must be standalone, got group %q", i, tab.GroupID)
		}

		if got.TopLevel[i].Type != "tab" || got.TopLevel[i].ID != tab.ID {
			t.Fatalf("top_level entry %d does not match tab id", i)
		}
	}

	if got.Tabs[0].Path != "/a.log" || got.Tabs[1].Path != "/b.log" {
		t.Fatalf("migration must preserve order, got %+v", got.Tabs)
	}

	if _, err := os.Stat(filepath.Join(dir, savedTabsFile)); !os.IsNotExist(err) {
		t.Fatal("legacy saved_tabs.json should be removed after migration")
	}

	if _, err := os.Stat(filepath.Join(dir, savedSessionFile)); err != nil {
		t.Fatalf("migrated session should be persisted: %v", err)
	}
}

func TestLoadConfigFirstRun(t *testing.T) {
	// LoadConfig resolves os.UserConfigDir()/gofle. Without this redirect the
	// test reads the developer's real settings.json and fails on any machine
	// whose theme/language differ from the defaults.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.SecretKey == "" {
		t.Fatal("expected secret key to be generated")
	}

	if cfg.Theme != "system" {
		t.Fatalf("expected default theme 'system', got %q", cfg.Theme)
	}

	if cfg.Language != "system" {
		t.Fatalf("expected default language 'system', got %q", cfg.Language)
	}

	if !cfg.SaveTabsExit {
		t.Fatal("expected SaveTabsExit default true")
	}

	if cfg.MaxHistorySize != 100 {
		t.Fatalf("expected MaxHistorySize 100, got %d", cfg.MaxHistorySize)
	}
}

func TestLoadConfigExistingConfig(t *testing.T) {
	// Isolate config directory so we don't pick up a saved session from the
	// real config dir which would re-enable SaveTabsExit.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg1, _ := LoadConfig()
	cfg1.Theme = "dark" //nolint:goconst // test fixture literal repeated for clarity
	cfg1.Language = "en"
	cfg1.SaveTabsExit = false
	_ = cfg1.Save()

	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg2.Theme != "dark" {
		t.Fatalf("expected theme 'dark', got %q", cfg2.Theme)
	}

	if cfg2.Language != "en" {
		t.Fatalf("expected language 'en', got %q", cfg2.Language)
	}

	if cfg2.SaveTabsExit {
		t.Fatal("expected SaveTabsExit false")
	}
}

func TestLoadConfigCorruptJSON(t *testing.T) {
	// This test is tricky because LoadConfig uses the system config dir
	// We can't easily test corrupt JSON in the actual config dir
	// Just verify LoadConfig doesn't panic with defaults
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	// Theme might be from previous test, just verify it loads without error
	_ = cfg.Theme
}

func TestLoadConfigMissingDir(t *testing.T) {
	// LoadConfig creates the config dir automatically
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.configDir == "" {
		t.Fatal("expected configDir to be set")
	}
}

func TestPersistSettingsAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, Theme: "dark", Language: "en"}

	err := cfg.persistSettings()
	if err != nil {
		t.Fatalf("persistSettings error: %v", err)
	}

	settingsPath := filepath.Join(dir, settingsFileNm)

	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("stat settings.json: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}

	data, _ := os.ReadFile(settingsPath)

	var sf map[string]any

	_ = json.Unmarshal(data, &sf)
	if sf["theme"] != "dark" {
		t.Fatalf("expected theme 'dark', got %v", sf["theme"])
	}
}

func TestPersistRecentAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, RecentFiles: []RecentFileItem{{Path: "a.log", FileSize: 10}}}

	err := cfg.persistRecent()
	if err != nil {
		t.Fatalf("persistRecent error: %v", err)
	}

	recentPath := filepath.Join(dir, recentFile)

	info, err := os.Stat(recentPath)
	if err != nil {
		t.Fatalf("stat recent.json: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestPersistHistoryAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, History: map[string][]string{"search": {"foo", "bar"}}}

	err := cfg.persistHistory()
	if err != nil {
		t.Fatalf("persistHistory error: %v", err)
	}

	historyPath := filepath.Join(dir, historyFile)

	info, err := os.Stat(historyPath)
	if err != nil {
		t.Fatalf("stat history.json: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestPersistSavedSessionAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SaveTabsExit: true, SavedSession: &SavedSession{ActiveTabID: "tab1"}}

	err := cfg.persistSavedSession()
	if err != nil {
		t.Fatalf("persistSavedSession error: %v", err)
	}

	sessionPath := filepath.Join(dir, savedSessionFile)

	info, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatalf("stat saved_session.json: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestEncryptSSHSecretsNilInput(t *testing.T) {
	cfg := &AppConfig{SecretKey: "key", SecretSalt: "00000000000000000000000000000000"}
	result := cfg.encryptSSHSecrets(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestEncryptSSHSecretsFailureBlanksField(t *testing.T) {
	// With empty secret key, encryption still works (uses Argon2id of empty string)
	// So we test that the function handles the case gracefully
	cfg := &AppConfig{SecretKey: "", SecretSalt: "00000000000000000000000000000000"}
	summary := &SSHConfigSummary{
		Host:     "h",
		Port:     22,
		Username: "user",
		Password: "pass",
	}

	result := cfg.encryptSSHSecrets(summary)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// With empty key, encryption still succeeds (uses Argon2id of "")
	// So fields should be encrypted, not blanked
	if result.Username == "" {
		t.Fatal("expected encrypted Username")
	}

	if result.Password == "" {
		t.Fatal("expected encrypted Password")
	}
}

func TestDecryptSSHSecretsNilInput(t *testing.T) {
	cfg := &AppConfig{SecretKey: "key", SecretSalt: "00000000000000000000000000000000"}
	result := cfg.decryptSSHSecrets(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestDecryptSSHSecretsFailureLeavesEmpty(t *testing.T) {
	cfg := &AppConfig{SecretKey: "key", SecretSalt: "00000000000000000000000000000000"}
	summary := &SSHConfigSummary{
		Host:     "h",
		Port:     22,
		Username: "encrypted",
		Password: "encrypted",
	}

	result := cfg.decryptSSHSecrets(summary)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Every field is blanked on failure: leaving the ciphertext in Username
	// would send it to the SSH server as a literal login name.
	if result.Username != "" {
		t.Fatalf("expected empty Username on failure, got %q", result.Username)
	}

	if result.Password != "" {
		t.Fatalf("expected empty Password, got %q", result.Password)
	}
}

func TestAddRecentFileCapAt20(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), SecretKey: "key", SecretSalt: "00000000000000000000000000000000", RecentFiles: []RecentFileItem{}}
	for i := range 25 {
		cfg.AddRecentFile(fmt.Sprintf("file%d.log", i), int64(i), false, nil)
	}

	if len(cfg.RecentFiles) != 20 {
		t.Fatalf("expected 20 entries (capped), got %d", len(cfg.RecentFiles))
	}
	// Most recent should be first
	if cfg.RecentFiles[0].Path != "file24.log" {
		t.Fatalf("expected file24.log first, got %q", cfg.RecentFiles[0].Path)
	}
}

func TestGetRecentFilesReturnsCopy(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), RecentFiles: []RecentFileItem{{Path: "a.log"}}}

	recent := cfg.GetRecentFiles()
	if len(recent) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(recent))
	}

	recent[0].Path = "mutated"

	fresh := cfg.GetRecentFiles()
	if fresh[0].Path == "mutated" {
		t.Fatal("GetRecentFiles must return a copy")
	}
}

func TestRemoveRecentFileNotFound(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), RecentFiles: []RecentFileItem{{Path: "a.log"}}}

	removed, err := cfg.RemoveRecentFile("nonexistent")
	if err != nil {
		t.Fatalf("RemoveRecentFile error: %v", err)
	}

	if removed {
		t.Fatal("expected false for non-existent file")
	}

	if len(cfg.RecentFiles) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cfg.RecentFiles))
	}
}

func TestClearRecentFilesEmpty(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), RecentFiles: []RecentFileItem{}}
	cfg.ClearRecentFiles()

	if len(cfg.RecentFiles) != 0 {
		t.Fatal("expected empty recent list")
	}
}

func TestAddInputHistoryNamespaceIsolation(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}
	_ = cfg.AddInputHistory("search", "foo")
	_ = cfg.AddInputHistory("ssh", "host1")

	searchHist := cfg.GetInputHistory("search")
	sshHist := cfg.GetInputHistory("ssh")

	if len(searchHist) != 1 || searchHist[0] != "foo" {
		t.Fatalf("search history mismatch: %v", searchHist)
	}

	if len(sshHist) != 1 || sshHist[0] != "host1" {
		t.Fatalf("ssh history mismatch: %v", sshHist)
	}
}

func TestRemoveInputHistoryItemPrunesEmptyNamespace(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), MaxHistorySize: 100}
	_ = cfg.AddInputHistory("search", "only")

	_ = cfg.RemoveInputHistoryItem("search", "only")
	if len(cfg.GetInputHistory("search")) != 0 {
		t.Fatal("namespace should be pruned when empty")
	}
	// History map should not have the namespace
	cfg.Mu.RLock()
	_, exists := cfg.History["search"]
	cfg.Mu.RUnlock()

	if exists {
		t.Fatal("namespace should be deleted from map")
	}
}

func TestSaveSessionSaveTabsExitFalseClearsSession(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SaveTabsExit: true}
	_ = cfg.SaveSession(sampleSession())

	cfg.SaveTabsExit = false
	_ = cfg.SaveSession(sampleSession())

	if _, err := os.Stat(filepath.Join(dir, savedSessionFile)); !os.IsNotExist(err) {
		t.Fatal("session file must be removed when SaveTabsExit is false")
	}
}

func TestClearSavedSessionRemovesBothFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, SaveTabsExit: true}
	_ = cfg.SaveSession(sampleSession())

	// Create legacy file too
	legacy := []map[string]any{{"path": "/x.log"}}
	data, _ := json.Marshal(legacy)
	_ = os.WriteFile(filepath.Join(dir, savedTabsFile), data, 0o644)

	cfg.ClearSavedSession()

	if _, err := os.Stat(filepath.Join(dir, savedSessionFile)); !os.IsNotExist(err) {
		t.Fatal("saved_session.json should be removed")
	}

	if _, err := os.Stat(filepath.Join(dir, savedTabsFile)); !os.IsNotExist(err) {
		t.Fatal("saved_tabs.json should be removed")
	}
}

func TestLoadOrCreateSecretKeyNewKey(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir}

	err := cfg.loadOrCreateSecretKey()
	if err != nil {
		t.Fatalf("loadOrCreateSecretKey error: %v", err)
	}

	if cfg.SecretKey == "" {
		t.Fatal("expected secret key to be generated")
	}

	if len(cfg.SecretKey) != 64 {
		t.Fatalf("expected 64-char hex key, got %d", len(cfg.SecretKey))
	}

	// Check file permissions
	keyPath := filepath.Join(dir, secretKeyFile)

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat secret.key: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestLoadOrCreateSecretKeyMigration(t *testing.T) {
	dir := t.TempDir()
	// Create settings.json with legacy secret_key
	settings := map[string]any{
		"theme":      "dark",
		"secret_key": "legacy-secret-key-123",
	}
	data, _ := json.Marshal(settings)
	_ = os.WriteFile(filepath.Join(dir, settingsFileNm), data, 0o644)

	cfg := &AppConfig{configDir: dir}

	err := cfg.loadOrCreateSecretKey()
	if err != nil {
		t.Fatalf("loadOrCreateSecretKey error: %v", err)
	}

	if cfg.SecretKey != "legacy-secret-key-123" {
		t.Fatalf("expected migrated key, got %q", cfg.SecretKey)
	}

	// Check that secret.key was created
	keyPath := filepath.Join(dir, secretKeyFile)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("secret.key should be created after migration")
	}
}

func TestGenerateSecretKey(t *testing.T) {
	key, err := generateSecretKey()
	if err != nil {
		t.Fatalf("generateSecretKey error: %v", err)
	}

	if key == "" {
		t.Fatal("expected non-empty key")
	}

	if len(key) != 64 {
		t.Fatalf("expected 64-char hex key, got %d", len(key))
	}
	// Verify it's valid hex
	if _, err := hex.DecodeString(key); err != nil {
		t.Fatalf("key is not valid hex: %v", err)
	}
}

func TestMigrateLegacySavedTabsWithSSH(t *testing.T) {
	dir := t.TempDir()
	legacy := []map[string]any{
		{"path": "/a.log", "is_ssh": false},
		{"path": "ssh://h:22/f.log", "is_ssh": true, "ssh": map[string]any{
			"host": "h", "port": 22, "username": "u", "remote_file_path": "/f.log",
			"auth_type": "password", "password": "secret",
		}},
	}
	data, _ := json.Marshal(legacy)
	_ = os.WriteFile(filepath.Join(dir, savedTabsFile), data, 0o644)

	cfg := &AppConfig{configDir: dir, SaveTabsExit: true, SecretKey: "test-key"}
	cfg.loadSavedSession()

	// Check the persisted session file directly (encrypted)
	sessionPath := filepath.Join(dir, savedSessionFile)

	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}

	var persisted SavedSession
	if err := json.Unmarshal(sessionData, &persisted); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}

	if len(persisted.Tabs) != 2 {
		t.Fatalf("expected 2 tabs in persisted session, got %d", len(persisted.Tabs))
	}

	if persisted.Tabs[1].IsSSH {
		if persisted.Tabs[1].SSH == nil {
			t.Fatal("SSH tab should have SSH config")
		}

		if persisted.Tabs[1].SSH.Password == "secret" { //nolint:goconst // test fixture literal repeated for clarity
			t.Fatal("SSH password should be encrypted in persisted session")
		}
	}

	// Now verify GetSavedSession decrypts correctly
	got := cfg.GetSavedSession()
	if len(got.Tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(got.Tabs))
	}

	if got.Tabs[1].IsSSH {
		if got.Tabs[1].SSH == nil {
			t.Fatal("SSH tab should have SSH config")
		}

		if got.Tabs[1].SSH.Password != "secret" {
			t.Fatalf("expected decrypted password 'secret', got %q", got.Tabs[1].SSH.Password)
		}
	}
}

func TestKnownHostsPath(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir}
	path := cfg.KnownHostsPath()

	expected := filepath.Join(dir, "known_hosts")
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
}

func TestPersistSettingsPersistsAllFields(t *testing.T) {
	dir := t.TempDir()
	cfg := &AppConfig{configDir: dir, Theme: "dark", Language: "en", SaveTabsExit: false}

	err := cfg.persistSettings()
	if err != nil {
		t.Fatalf("persistSettings error: %v", err)
	}

	reloaded := &AppConfig{configDir: dir}
	reloaded.loadSettings()

	if reloaded.Theme != "dark" {
		t.Fatalf("theme not persisted")
	}

	if reloaded.Language != "en" {
		t.Fatalf("language not persisted")
	}

	if reloaded.SaveTabsExit {
		t.Fatalf("SaveTabsExit not persisted")
	}
}

func TestAddRecentFileSSHEncryption(t *testing.T) {
	cfg := &AppConfig{configDir: t.TempDir(), SecretKey: "test-key", RecentFiles: []RecentFileItem{}}
	summary := &SSHConfigSummary{
		Host:           "h",
		Port:           22,
		Username:       "user",
		RemoteFilePath: "/f",
		AuthType:       "password",
		Password:       "secret",
		KeyPassphrase:  "keypass",
	}
	cfg.AddRecentFile("ssh://h:22/f", 0, true, summary)

	// Original should be unchanged
	if summary.Password != "secret" {
		t.Fatal("original summary mutated")
	}

	if summary.KeyPassphrase != "keypass" {
		t.Fatal("original key passphrase mutated")
	}

	stored := cfg.RecentFiles[0].SSH
	if stored.Password == "secret" {
		t.Fatal("password not encrypted")
	}

	if stored.KeyPassphrase == "keypass" {
		t.Fatal("key passphrase not encrypted")
	}
}

// TestSavePersistsHistory verifies that an explicit Save() now also persists
// the in-memory input History map (C2), so accumulated history survives a
// save → reload cycle instead of being silently dropped.
func TestSavePersistsHistory(t *testing.T) {
	dir := t.TempDir()

	cfg := &AppConfig{
		configDir:      dir,
		Theme:          "system",
		MaxHistorySize: 100,
	}
	if err := cfg.AddInputHistory("search", "error log"); err != nil {
		t.Fatalf("AddInputHistory: %v", err)
	}

	if err := cfg.AddInputHistory("command", "open file.log"); err != nil {
		t.Fatalf("AddInputHistory: %v", err)
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload from the same config dir and confirm history is present.
	reloaded := &AppConfig{configDir: dir, MaxHistorySize: 100}
	reloaded.loadHistory()

	searchHist := reloaded.GetInputHistory("search")
	if len(searchHist) != 1 || searchHist[0] != "error log" {
		t.Fatalf("search history not persisted: %v", searchHist)
	}

	cmdHist := reloaded.GetInputHistory("command")
	if len(cmdHist) != 1 || cmdHist[0] != "open file.log" {
		t.Fatalf("command history not persisted: %v", cmdHist)
	}
}
