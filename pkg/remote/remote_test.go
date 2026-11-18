package remote

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDirEntryKind(t *testing.T) {
	e := DirEntry{
		Name:    "app.log",
		Path:    "/var/log/app.log",
		Kind:    "file",
		Size:    1024,
		ModTime: 1700000000,
	}
	if e.Kind != "file" {
		t.Fatalf("expected Kind=file, got %q", e.Kind)
	}

	if e.Size != 1024 {
		t.Fatalf("expected Size=1024, got %d", e.Size)
	}
}

func TestDirEntryJSONMarshal(t *testing.T) {
	e := DirEntry{
		Name:    "test.txt",
		Path:    "/home/user/test.txt",
		Kind:    "file",
		Size:    2048,
		ModTime: 1700000000,
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded DirEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Name != e.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, e.Name)
	}
	if decoded.Path != e.Path {
		t.Errorf("Path: got %q, want %q", decoded.Path, e.Path)
	}
	if decoded.Kind != e.Kind {
		t.Errorf("Kind: got %q, want %q", decoded.Kind, e.Kind)
	}
	if decoded.Size != e.Size {
		t.Errorf("Size: got %d, want %d", decoded.Size, e.Size)
	}
	if decoded.ModTime != e.ModTime {
		t.Errorf("ModTime: got %d, want %d", decoded.ModTime, e.ModTime)
	}
}

func TestDirEntryJSONUnmarshal(t *testing.T) {
	jsonData := `{"name":"doc.pdf","path":"/docs/doc.pdf","kind":"file","size":4096,"mod_time":1710000000}`

	var e DirEntry
	if err := json.Unmarshal([]byte(jsonData), &e); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if e.Name != "doc.pdf" {
		t.Errorf("Name = %q, want doc.pdf", e.Name)
	}
	if e.Path != "/docs/doc.pdf" {
		t.Errorf("Path = %q, want /docs/doc.pdf", e.Path)
	}
	if e.Kind != "file" {
		t.Errorf("Kind = %q, want file", e.Kind)
	}
	if e.Size != 4096 {
		t.Errorf("Size = %d, want 4096", e.Size)
	}
	if e.ModTime != 1710000000 {
		t.Errorf("ModTime = %d, want 1710000000", e.ModTime)
	}
}

func TestDirEntryJSONDirKind(t *testing.T) {
	jsonData := `{"name":"folder","path":"/home/folder","kind":"dir","size":0,"mod_time":1720000000}`

	var e DirEntry
	if err := json.Unmarshal([]byte(jsonData), &e); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if e.Kind != "dir" {
		t.Errorf("Kind = %q, want dir", e.Kind)
	}
	if e.Size != 0 {
		t.Errorf("Size = %d, want 0 for directory", e.Size)
	}
}

func TestDirEntryJSONSymlinkKind(t *testing.T) {
	jsonData := `{"name":"link","path":"/home/link","kind":"symlink","size":12,"mod_time":1730000000}`

	var e DirEntry
	if err := json.Unmarshal([]byte(jsonData), &e); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if e.Kind != "symlink" {
		t.Errorf("Kind = %q, want symlink", e.Kind)
	}
}

func TestDirEntryJSONEmptyFields(t *testing.T) {
	jsonData := `{}`

	var e DirEntry
	if err := json.Unmarshal([]byte(jsonData), &e); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if e.Name != "" {
		t.Errorf("Name = %q, want empty", e.Name)
	}
	if e.Path != "" {
		t.Errorf("Path = %q, want empty", e.Path)
	}
	if e.Kind != "" {
		t.Errorf("Kind = %q, want empty", e.Kind)
	}
	if e.Size != 0 {
		t.Errorf("Size = %d, want 0", e.Size)
	}
	if e.ModTime != 0 {
		t.Errorf("ModTime = %d, want 0", e.ModTime)
	}
}

func TestProtocolConstants(t *testing.T) {
	if ProtocolSSH != "ssh" {
		t.Errorf("ProtocolSSH = %q, want ssh", ProtocolSSH)
	}
	if ProtocolFTP != "ftp" {
		t.Errorf("ProtocolFTP = %q, want ftp", ProtocolFTP)
	}
}

func TestConfigFields(t *testing.T) {
	cfg := Config{
		Protocol:       ProtocolSSH,
		Host:           "example.com",
		Port:           22,
		Username:       "user",
		AuthType:       "key",
		Password:       "secret",
		KeyPath:        "/home/user/.ssh/id_rsa",
		KeyPassphrase:  "passphrase",
		RemoteFilePath: "/remote/file.txt",
		TLSMode:        "explicit",
		KnownHostsPath: "/home/user/.ssh/known_hosts",
		StallTimeout:   30 * time.Second,
	}

	if cfg.Protocol != ProtocolSSH {
		t.Errorf("Protocol = %q, want %q", cfg.Protocol, ProtocolSSH)
	}
	if cfg.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", cfg.Host)
	}
	if cfg.Port != 22 {
		t.Errorf("Port = %d, want 22", cfg.Port)
	}
	if cfg.Username != "user" {
		t.Errorf("Username = %q, want user", cfg.Username)
	}
	if cfg.AuthType != "key" {
		t.Errorf("AuthType = %q, want key", cfg.AuthType)
	}
	if cfg.Password != "secret" {
		t.Errorf("Password = %q, want secret", cfg.Password)
	}
	if cfg.KeyPath != "/home/user/.ssh/id_rsa" {
		t.Errorf("KeyPath = %q, want /home/user/.ssh/id_rsa", cfg.KeyPath)
	}
	if cfg.KeyPassphrase != "passphrase" {
		t.Errorf("KeyPassphrase = %q, want passphrase", cfg.KeyPassphrase)
	}
	if cfg.RemoteFilePath != "/remote/file.txt" {
		t.Errorf("RemoteFilePath = %q, want /remote/file.txt", cfg.RemoteFilePath)
	}
	if cfg.TLSMode != "explicit" {
		t.Errorf("TLSMode = %q, want explicit", cfg.TLSMode)
	}
	if cfg.KnownHostsPath != "/home/user/.ssh/known_hosts" {
		t.Errorf("KnownHostsPath = %q, want /home/user/.ssh/known_hosts", cfg.KnownHostsPath)
	}
	if cfg.StallTimeout != 30*time.Second {
		t.Errorf("StallTimeout = %v, want 30s", cfg.StallTimeout)
	}
}

func TestConfigJSONMarshal(t *testing.T) {
	cfg := Config{
		Protocol:       ProtocolFTP,
		Host:           "ftp.example.com",
		Port:           21,
		Username:       "ftpuser",
		AuthType:       "password",
		Password:       "ftppass",
		RemoteFilePath: "/remote/data.csv",
		TLSMode:        "implicit",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Protocol != cfg.Protocol {
		t.Errorf("Protocol: got %q, want %q", decoded.Protocol, cfg.Protocol)
	}
	if decoded.Host != cfg.Host {
		t.Errorf("Host: got %q, want %q", decoded.Host, cfg.Host)
	}
	if decoded.Port != cfg.Port {
		t.Errorf("Port: got %d, want %d", decoded.Port, cfg.Port)
	}
	if decoded.Username != cfg.Username {
		t.Errorf("Username: got %q, want %q", decoded.Username, cfg.Username)
	}
	if decoded.AuthType != cfg.AuthType {
		t.Errorf("AuthType: got %q, want %q", decoded.AuthType, cfg.AuthType)
	}
	if decoded.Password != cfg.Password {
		t.Errorf("Password: got %q, want %q", decoded.Password, cfg.Password)
	}
	if decoded.RemoteFilePath != cfg.RemoteFilePath {
		t.Errorf("RemoteFilePath: got %q, want %q", decoded.RemoteFilePath, cfg.RemoteFilePath)
	}
	if decoded.TLSMode != cfg.TLSMode {
		t.Errorf("TLSMode: got %q, want %q", decoded.TLSMode, cfg.TLSMode)
	}
}

func TestConfigJSONUnmarshal(t *testing.T) {
	jsonData := `{
		"protocol": "ssh",
		"host": "server.com",
		"port": 2222,
		"username": "admin",
		"auth_type": "password",
		"password": "admin123",
		"remote_file_path": "/data/file.txt",
		"tls_mode": "",
		"known_hosts_path": "/etc/ssh/known_hosts"
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(jsonData), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Protocol != "ssh" {
		t.Errorf("Protocol = %q, want ssh", cfg.Protocol)
	}
	if cfg.Host != "server.com" {
		t.Errorf("Host = %q, want server.com", cfg.Host)
	}
	if cfg.Port != 2222 {
		t.Errorf("Port = %d, want 2222", cfg.Port)
	}
	if cfg.Username != "admin" {
		t.Errorf("Username = %q, want admin", cfg.Username)
	}
	if cfg.AuthType != "password" {
		t.Errorf("AuthType = %q, want password", cfg.AuthType)
	}
	if cfg.Password != "admin123" {
		t.Errorf("Password = %q, want admin123", cfg.Password)
	}
	if cfg.RemoteFilePath != "/data/file.txt" {
		t.Errorf("RemoteFilePath = %q, want /data/file.txt", cfg.RemoteFilePath)
	}
	if cfg.TLSMode != "" {
		t.Errorf("TLSMode = %q, want empty", cfg.TLSMode)
	}
	if cfg.KnownHostsPath != "/etc/ssh/known_hosts" {
		t.Errorf("KnownHostsPath = %q, want /etc/ssh/known_hosts", cfg.KnownHostsPath)
	}
}

func TestConfigOmitEmptyFields(t *testing.T) {
	cfg := Config{
		Protocol:       ProtocolSSH,
		Host:           "host",
		Port:           22,
		Username:       "user",
		AuthType:       "key",
		RemoteFilePath: "/path",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Password != "" {
		t.Errorf("Password should be omitted/empty, got %q", decoded.Password)
	}
	if decoded.KeyPath != "" {
		t.Errorf("KeyPath should be omitted/empty, got %q", decoded.KeyPath)
	}
	if decoded.KeyPassphrase != "" {
		t.Errorf("KeyPassphrase should be omitted/empty, got %q", decoded.KeyPassphrase)
	}
	if decoded.TLSMode != "" {
		t.Errorf("TLSMode should be omitted/empty, got %q", decoded.TLSMode)
	}
	if decoded.KnownHostsPath != "" {
		t.Errorf("KnownHostsPath should be omitted/empty, got %q", decoded.KnownHostsPath)
	}
}

type mockLister struct {
	listDirFunc func(ctx context.Context, path string) ([]DirEntry, error)
	homeDirFunc func(ctx context.Context) (string, error)
	closeFunc   func() error
}

func (m *mockLister) ListDir(ctx context.Context, path string) ([]DirEntry, error) {
	if m.listDirFunc != nil {
		return m.listDirFunc(ctx, path)
	}
	return nil, nil
}

func (m *mockLister) HomeDir(ctx context.Context) (string, error) {
	if m.homeDirFunc != nil {
		return m.homeDirFunc(ctx)
	}
	return "", nil
}

func (m *mockLister) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestListerInterface(t *testing.T) {
	ctx := context.Background()
	expectedEntries := []DirEntry{
		{Name: "file1.txt", Path: "/file1.txt", Kind: "file", Size: 100, ModTime: 1000},
		{Name: "dir1", Path: "/dir1", Kind: "dir", Size: 0, ModTime: 2000},
	}

	l := &mockLister{
		listDirFunc: func(ctx context.Context, path string) ([]DirEntry, error) {
			if path != "/" {
				t.Errorf("ListDir called with path %q, want /", path)
			}
			return expectedEntries, nil
		},
		homeDirFunc: func(ctx context.Context) (string, error) {
			return "/home/user", nil
		},
		closeFunc: func() error {
			return nil
		},
	}

	entries, err := l.ListDir(ctx, "/")
	if err != nil {
		t.Fatalf("ListDir failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "file1.txt" {
		t.Errorf("first entry name = %q, want file1.txt", entries[0].Name)
	}

	home, err := l.HomeDir(ctx)
	if err != nil {
		t.Fatalf("HomeDir failed: %v", err)
	}
	if home != "/home/user" {
		t.Errorf("HomeDir = %q, want /home/user", home)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestListerInterfaceError(t *testing.T) {
	ctx := context.Background()
	expectedErr := context.DeadlineExceeded

	l := &mockLister{
		listDirFunc: func(ctx context.Context, path string) ([]DirEntry, error) {
			return nil, expectedErr
		},
		homeDirFunc: func(ctx context.Context) (string, error) {
			return "", expectedErr
		},
		closeFunc: func() error {
			return expectedErr
		},
	}

	_, err := l.ListDir(ctx, "/")
	if !errors.Is(err, expectedErr) {
		t.Errorf("ListDir error = %v, want %v", err, expectedErr)
	}

	_, err = l.HomeDir(ctx)
	if !errors.Is(err, expectedErr) {
		t.Errorf("HomeDir error = %v, want %v", err, expectedErr)
	}

	err = l.Close()
	if !errors.Is(err, expectedErr) {
		t.Errorf("Close error = %v, want %v", err, expectedErr)
	}
}

type mockStreamer struct {
	localPathFunc    func() string
	downloadedFunc   func() int64
	remoteTotalFunc  func() int64
	keepTempFileFunc func()
	closeFunc        func() error
}

func (m *mockStreamer) LocalPath() string {
	if m.localPathFunc != nil {
		return m.localPathFunc()
	}
	return ""
}

func (m *mockStreamer) Downloaded() int64 {
	if m.downloadedFunc != nil {
		return m.downloadedFunc()
	}
	return 0
}

func (m *mockStreamer) RemoteTotal() int64 {
	if m.remoteTotalFunc != nil {
		return m.remoteTotalFunc()
	}
	return 0
}

func (m *mockStreamer) KeepTempFile() {
	if m.keepTempFileFunc != nil {
		m.keepTempFileFunc()
	}
}

func (m *mockStreamer) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestStreamerInterface(t *testing.T) {
	s := &mockStreamer{
		localPathFunc: func() string {
			return "/tmp/stream_123.tmp"
		},
		downloadedFunc: func() int64 {
			return 1024
		},
		remoteTotalFunc: func() int64 {
			return 2048
		},
		keepTempFileFunc: func() {
			// no-op
		},
		closeFunc: func() error {
			return nil
		},
	}

	path := s.LocalPath()
	if path != "/tmp/stream_123.tmp" {
		t.Errorf("LocalPath = %q, want /tmp/stream_123.tmp", path)
	}

	downloaded := s.Downloaded()
	if downloaded != 1024 {
		t.Errorf("Downloaded = %d, want 1024", downloaded)
	}

	total := s.RemoteTotal()
	if total != 2048 {
		t.Errorf("RemoteTotal = %d, want 2048", total)
	}

	s.KeepTempFile()

	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestStreamerInterfaceProgress(t *testing.T) {
	var downloaded int64
	var total int64

	s := &mockStreamer{
		localPathFunc: func() string {
			return "/tmp/progress.tmp"
		},
		downloadedFunc: func() int64 {
			return downloaded
		},
		remoteTotalFunc: func() int64 {
			return total
		},
		keepTempFileFunc: func() {},
		closeFunc: func() error {
			return nil
		},
	}

	downloaded = 512
	total = 1024
	if s.Downloaded() != 512 {
		t.Errorf("Downloaded = %d, want 512", s.Downloaded())
	}
	if s.RemoteTotal() != 1024 {
		t.Errorf("RemoteTotal = %d, want 1024", s.RemoteTotal())
	}

	downloaded = 1024
	if s.Downloaded() != 1024 {
		t.Errorf("Downloaded = %d, want 1024", s.Downloaded())
	}
}

func TestStreamerInterfaceError(t *testing.T) {
	expectedErr := context.DeadlineExceeded

	s := &mockStreamer{
		localPathFunc:    func() string { return "" },
		downloadedFunc:   func() int64 { return 0 },
		remoteTotalFunc:  func() int64 { return 0 },
		keepTempFileFunc: func() {},
		closeFunc: func() error {
			return expectedErr
		},
	}

	err := s.Close()
	if !errors.Is(err, expectedErr) {
		t.Errorf("Close error = %v, want %v", err, expectedErr)
	}
}
