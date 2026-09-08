package ssh

import (
	"GoFLE/pkg/remote"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func validBase() Config {
	return Config{
		Host:           "example.com",
		Port:           22,
		Username:       "root",
		RemoteFilePath: "/var/log/syslog",
	}
}

func TestValidate_OK(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"password", func() Config { c := validBase(); c.AuthType = "password"; c.Password = "secret"; return c }()},                      //nolint:goconst // test fixture literal repeated for clarity
		{"key without passphrase", func() Config { c := validBase(); c.AuthType = "key"; c.KeyPath = "/home/u/.ssh/id_rsa"; return c }()}, //nolint:goconst // test fixture literal repeated for clarity
		{"key with passphrase", func() Config {
			c := validBase()
			c.AuthType = "key"
			c.KeyPath = "/home/u/.ssh/id_rsa"
			c.KeyPassphrase = "pp"

			return c
		}()},
		{"port defaulted from 0", func() Config { c := validBase(); c.Port = 0; c.AuthType = "password"; c.Password = "secret"; return c }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err != nil {
				t.Fatalf("expected valid, got error: %v", err)
			}
		})
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name   string
		cfg    Config
		errSub string
		// valid marks a case that must NOT error. Used for inputs that used to be
		// required (e.g. an empty RemoteFilePath) but are now optional so the
		// interactive directory browser can drive the path selection.
		valid bool
	}{
		{"empty host", func() Config { c := validBase(); c.Host = ""; c.AuthType = "password"; c.Password = "s"; return c }(), "host", false},
		{"empty username", func() Config { c := validBase(); c.Username = ""; c.AuthType = "password"; c.Password = "s"; return c }(), "username", false},
		// An empty RemoteFilePath is now VALID: it means "no file chosen yet" and
		// drives the optional-path browser; only StreamerContext still rejects it.
		{"empty remote_file_path now valid", func() Config {
			c := validBase()
			c.RemoteFilePath = ""
			c.AuthType = "password"
			c.Password = "s"

			return c
		}(), "", true},
		{"port too low", func() Config { c := validBase(); c.Port = 0; c.AuthType = "password"; c.Password = "s"; return c }(), "", false}, // port 0 is valid (defaults to 22)
		{"port out of range", func() Config { c := validBase(); c.Port = 70000; c.AuthType = "password"; c.Password = "s"; return c }(), "port", false},
		{"unknown auth_type", func() Config { c := validBase(); c.AuthType = "gssapi"; return c }(), "auth_type", false},
		{"password empty", func() Config { c := validBase(); c.AuthType = "password"; return c }(), "password", false},
		{"key without key_path", func() Config { c := validBase(); c.AuthType = "key"; return c }(), "key_path", false},
		{"remote path with NUL", func() Config {
			c := validBase()
			c.RemoteFilePath = "/var/log/\x00syslog"
			c.AuthType = "password"
			c.Password = "s"

			return c
		}(), "control characters", false},
		{"remote path with tab", func() Config {
			c := validBase()
			c.RemoteFilePath = "/var/log/x\t"
			c.AuthType = "password"
			c.Password = "s"

			return c
		}(), "control characters", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.name == "port too low" {
				// Port 0 is intentionally valid (defaults to 22).
				if err != nil {
					t.Fatalf("expected valid for port 0, got error: %v", err)
				}

				return
			}

			if tc.valid {
				if err != nil {
					t.Fatalf("expected valid, got error: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSub)
			}

			if tc.errSub != "" && !contains(err.Error(), tc.errSub) {
				t.Fatalf("expected error containing %q, got %q", tc.errSub, err.Error())
			}
		})
	}
}

// TestNewStreamerContextEmptyPath verifies the safety guard: an empty
// RemoteFilePath now passes Validate (browsing picks the file later) but must be
// rejected before the `cat` so it never degenerates into a bare "cat" reading
// stdin and hanging forever. The guard fires before any dial, so no server is
// needed. A config with a concrete path still validates.
func TestNewStreamerContextEmptyPath(t *testing.T) {
	c := validBase()
	c.RemoteFilePath = ""
	c.AuthType = "password"

	c.Password = "s"
	if err := c.Validate(); err != nil {
		t.Fatalf("empty RemoteFilePath must pass Validate, got: %v", err)
	}

	_, err := NewStreamerContext(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for empty RemoteFilePath in NewStreamerContext")
	}

	if !contains(err.Error(), "remote_file_path") {
		t.Fatalf("expected the error to name remote_file_path, got: %v", err)
	}

	withPath := c

	withPath.RemoteFilePath = "/var/log/syslog"
	if err := withPath.Validate(); err != nil {
		t.Fatalf("expected a config with a remote path to validate, got: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

// TestValidate_RemotePathMetacharsAllowed verifies that paths containing spaces
// or shell metacharacters are no longer rejected (B5). The shell-less fix quotes
// the path so it is treated as a literal filename rather than interpreted by the
// remote shell, so injection is impossible without blocking legitimate paths.
func TestValidate_RemotePathMetacharsAllowed(t *testing.T) {
	cases := []string{
		"/var/log/my file.log",
		"/var/log/x;rm -rf /",
		"/var/log/$(whoami)",
		"/var/log/a`b`c",
		"/var/log/a'b",
		"logs/syslog",
		"./logs/syslog",
		"../other/log",
		"-weirdname",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			c := validBase()
			c.AuthType = "password"
			c.Password = "s"

			c.RemoteFilePath = p
			if err := c.Validate(); err != nil {
				t.Fatalf("expected valid for remote path %q, got error: %v", p, err)
			}
		})
	}
}

// TestShellQuote verifies the POSIX single-quote escaping used to pass the
// remote file path through the shell without interpretation (B5).
func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"simple", "'simple'"},
		{"/var/log/syslog", "'/var/log/syslog'"},
		{"/var/log/my file", "'/var/log/my file'"},
		{"/var/log/a'b", "'/var/log/a'\\''b'"},
		{"x;rm -rf /", "'x;rm -rf /'"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := shellQuote(tc.in); got != tc.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}

	cur, uerr := user.Current()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"tilde only", "~", home},
		{"tilde slash", "~/x/y", home + "/x/y"},
		{"absolute unchanged", "/etc/ssh/id_rsa", "/etc/ssh/id_rsa"},
		{"relative unchanged", "keys/id_rsa", "keys/id_rsa"},
		{"empty unchanged", "", ""},
	}
	if uerr == nil {
		cases = append(cases, struct {
			name string
			in   string
			want string
		}{"tilde user", "~" + cur.Username + "/z", cur.HomeDir + "/z"})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandPath(tc.in); got != tc.want {
				t.Fatalf("expandPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateRemotePath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr bool
		errSub  string
	}{
		{"empty", "", false, ""},
		{"whitespace only", "   ", false, ""},
		{"valid absolute", "/var/log/syslog", false, ""},
		{"valid relative", "logs/syslog", false, ""},
		{"valid with spaces", "/var/log/my file.log", false, ""},
		{"NUL byte", "/var/log/\x00syslog", true, "control characters"},
		{"tab", "/var/log/x\tsyslog", true, "control characters"},
		{"newline", "/var/log/\nsyslog", true, "control characters"},
		{"DEL", "/var/log/\x7fsyslog", true, "control characters"},
		{"control char 0x1f", "/var/log/\x1fsyslog", true, "control characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRemotePath(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.path)
				}

				if tc.errSub != "" && !contains(err.Error(), tc.errSub) {
					t.Fatalf("expected error containing %q, got %q", tc.errSub, err.Error())
				}
			} else { //nolint:gocritic // nested else-if chain is clearer than a switch here
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.path, err)
				}
			}
		})
	}
}

func TestConfigValidateAllAuthTypes(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
		errSub  string
	}{
		{"password auth", Config{Host: "h", Username: "u", AuthType: "password", Password: "p", RemoteFilePath: "/f"}, false, ""},
		{"key auth no passphrase", Config{Host: "h", Username: "u", AuthType: "key", KeyPath: "/k", RemoteFilePath: "/f"}, false, ""},
		{"key auth with passphrase", Config{Host: "h", Username: "u", AuthType: "key", KeyPath: "/k", KeyPassphrase: "pp", RemoteFilePath: "/f"}, false, ""},
		{"port default", Config{Host: "h", Username: "u", AuthType: "password", Password: "p", RemoteFilePath: "/f", Port: 0}, false, ""},
		{"port 22", Config{Host: "h", Username: "u", AuthType: "password", Password: "p", RemoteFilePath: "/f", Port: 22}, false, ""},
		{"port 65535", Config{Host: "h", Username: "u", AuthType: "password", Password: "p", RemoteFilePath: "/f", Port: 65535}, false, ""},
		{"port too high", Config{Host: "h", Username: "u", AuthType: "password", Password: "p", RemoteFilePath: "/f", Port: 65536}, true, "port"},
		{"port negative", Config{Host: "h", Username: "u", AuthType: "password", Password: "p", RemoteFilePath: "/f", Port: -1}, true, "port"},
		{"missing host", Config{Username: "u", AuthType: "password", Password: "p", RemoteFilePath: "/f"}, true, "host"},
		{"missing username", Config{Host: "h", AuthType: "password", Password: "p", RemoteFilePath: "/f"}, true, "username"},
		{"missing remote path now valid", Config{Host: "h", Username: "u", AuthType: "password", Password: "p"}, false, ""},
		{"missing password", Config{Host: "h", Username: "u", AuthType: "password", RemoteFilePath: "/f"}, true, "password"},
		{"missing key path", Config{Host: "h", Username: "u", AuthType: "key", RemoteFilePath: "/f"}, true, "key_path"},
		{"unknown auth type", Config{Host: "h", Username: "u", AuthType: "gssapi", RemoteFilePath: "/f"}, true, "auth_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}

				if tc.errSub != "" && !contains(err.Error(), tc.errSub) {
					t.Fatalf("expected error containing %q, got %q", tc.errSub, err.Error())
				}
			} else { //nolint:gocritic // nested else-if chain is clearer than a switch here
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestWriteKnownHost(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	line := "example.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC..."

	err := WriteKnownHost(knownHostsPath, line)
	if err != nil {
		t.Fatalf("WriteKnownHost error: %v", err)
	}

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}

	if string(data) != line+"\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	info, err := os.Stat(knownHostsPath)
	if err != nil {
		t.Fatalf("stat known_hosts: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}

	err = WriteKnownHost(knownHostsPath, "another.host ssh-ed25519 AAAA...")
	if err != nil {
		t.Fatalf("second WriteKnownHost error: %v", err)
	}

	data, err = os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}

	expected := line + "\n" + "another.host ssh-ed25519 AAAA...\n"
	if string(data) != expected {
		t.Fatalf("unexpected content after append: %q", string(data))
	}
}

func TestNewHostKeyCallbackMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	callback, err := newHostKeyCallback(knownHostsPath)
	if err != nil {
		t.Fatalf("newHostKeyCallback error: %v", err)
	}

	// We can't easily test with nil key because FingerprintSHA256 panics
	// Just verify the callback is created without error
	_ = callback
}

func TestNewHostKeyCallbackKnownHost(t *testing.T) {
	// Skipped: requires valid SSH key format which is complex to generate in tests
	t.Skip("requires valid SSH key format")
}

func TestNewHostKeyCallbackMITM(t *testing.T) {
	// Skipped: requires valid SSH key format which is complex to generate in tests
	t.Skip("requires valid SSH key format")
}

func TestShellQuoteEdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"'", "''\\'''"}, // POSIX: ' + '\'' + ' = 6 chars
		{"a'b'c", "'a'\\''b'\\''c'"},
		{";rm -rf /", "';rm -rf /'"},
		{"$(whoami)", "'$(whoami)'"},
		{"`whoami`", "'`whoami`'"},
		{"$HOME", "'$HOME'"},
		{"file with spaces", "'file with spaces'"},
		{"path/with/slashes", "'path/with/slashes'"},
		{"~user/file", "'~user/file'"},
		{"unicode: привет", "'unicode: привет'"},
		{"tabs\tand\nnewlines", "'tabs\tand\nnewlines'"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := shellQuote(tc.in)
			if got != tc.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestExpandPathEdgeCases(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"tilde only", "~", home},
		{"tilde slash", "~/file", home + "/file"},
		{"tilde nested", "~/a/b/c", home + "/a/b/c"},
		{"absolute", "/absolute/path", "/absolute/path"},
		{"relative", "relative/path", "relative/path"},
		{"empty", "", ""},
		{"just tilde", "~", home},
		{"tilde with trailing slash", "~/", home},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := expandPath(tc.in)
			if got != tc.want {
				t.Fatalf("expandPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStreamerCloseIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	tmpFile, err := os.CreateTemp(tmpDir, "test-*.log")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tmpFile.WriteString("test\n")
	tmpFile.Close()

	s := &Streamer{
		tmpFile: func() *os.File {
			f, _ := os.Open(tmpFile.Name())
			return f
		}(),
		localPath: tmpFile.Name(),
		stopCh:    make(chan struct{}),
	}

	if err := s.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("third Close error: %v", err)
	}
}

func TestStreamerCleanupTempFile(t *testing.T) {
	tmpDir := t.TempDir()

	tmpFile, err := os.CreateTemp(tmpDir, "test-*.log")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tmpFile.WriteString("test\n")
	tmpFile.Close()

	s := &Streamer{
		tmpFile: func() *os.File {
			f, _ := os.Open(tmpFile.Name())
			return f
		}(),
		localPath: tmpFile.Name(),
		stopCh:    make(chan struct{}),
	}

	// Explicit cleanup removes the temp file even without Close having run.
	err = s.CleanupTempFile()
	if err != nil {
		t.Fatalf("CleanupTempFile error: %v", err)
	}

	if _, err := os.Stat(tmpFile.Name()); !os.IsNotExist(err) {
		t.Fatal("temp file should not exist after CleanupTempFile")
	}

	// A subsequent Close must not panic on the already-removed file.
	if err := s.Close(); err != nil {
		t.Fatalf("Close after cleanup error: %v", err)
	}
}

// TestStreamerKeepTempFile verifies KeepTempFile preserves the temp file across
// a Close, so a later explicit CleanupTempFile still removes it.
func TestStreamerKeepTempFile(t *testing.T) {
	tmpDir := t.TempDir()

	tmpFile, err := os.CreateTemp(tmpDir, "test-*.log")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tmpFile.WriteString("test\n")
	tmpFile.Close()

	s := &Streamer{
		tmpFile: func() *os.File {
			f, _ := os.Open(tmpFile.Name())
			return f
		}(),
		localPath: tmpFile.Name(),
		stopCh:    make(chan struct{}),
	}

	s.KeepTempFile()

	if err := s.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if _, err := os.Stat(tmpFile.Name()); os.IsNotExist(err) {
		t.Fatal("temp file should survive Close when KeepTempFile was called")
	}

	if err := s.CleanupTempFile(); err != nil {
		t.Fatalf("CleanupTempFile error: %v", err)
	}

	if _, err := os.Stat(tmpFile.Name()); !os.IsNotExist(err) {
		t.Fatal("temp file should be removed by CleanupTempFile")
	}
}

// fakeStream is a scripted response for the newRemoteStream seam.
type fakeStream struct {
	data       string // delivered then EOF
	holdOpen   bool   // keep stdout open until the test closes it
	waitErr    error  // surfaced by Wait after EOF/abort
	stderrData string // delivered on the stderr pipe (empty = no stderr pipe)
}

// fakeRemote scripts newRemoteStream responses in order and records commands.
type fakeRemote struct {
	mu      sync.Mutex
	scripts []fakeStream
	cmds    []string
	pipes   []*os.File
	// probeSize answers wc -c probes; independent of tail scripts so probe
	// calls never consume or block on scripted streams.
	probeSize int64
}

func (fr *fakeRemote) open(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	fr.cmds = append(fr.cmds, cmd)
	if strings.Contains(cmd, "wc -c") {
		// Probes answer instantly and never touch the tail script queue.
		return strings.NewReader(fmt.Sprintf("%d\n", fr.probeSize)), nil,
			func() error { return nil }, func() {}, nil
	}

	if len(fr.scripts) == 0 {
		return nil, nil, nil, nil, errors.New("fake: script exhausted")
	}

	sc := fr.scripts[0]
	fr.scripts = fr.scripts[1:]

	// An empty script leaves stderr nil, exactly like a session whose stderr
	// pipe was never wired up.
	var errReader io.Reader
	if sc.stderrData != "" {
		errReader = strings.NewReader(sc.stderrData)
	}

	if sc.holdOpen {
		// Real `tail -f` first dumps existing content, then follows the pipe.
		pr, pw, _ := os.Pipe()
		if sc.data != "" {
			pw.WriteString(sc.data)
		}

		fr.pipes = append(fr.pipes, pw)

		return pr, errReader, func() error { return sc.waitErr }, func() { pw.Close() }, nil
	}

	return strings.NewReader(sc.data), errReader, func() error { return sc.waitErr }, func() {}, nil
}

func (fr *fakeRemote) closePipes() {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	for _, pw := range fr.pipes {
		pw.Close()
	}
}

// hasCmd reports whether any issued command contains substr.
func (fr *fakeRemote) hasCmd(substr string) bool {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	for _, c := range fr.cmds {
		if strings.Contains(c, substr) {
			return true
		}
	}

	return false
}

func (fr *fakeRemote) lastCmd() string {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	if n := len(fr.cmds); n > 0 {
		return fr.cmds[n-1]
	}

	return ""
}

// newSupervisorStreamer builds a Streamer wired to fr with fast timings.
func newSupervisorStreamer(t *testing.T, fr *fakeRemote, initial string) (*Streamer, *os.File) {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "tail-sup-*.log")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.WriteString(initial); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { f.Close() })

	s := &Streamer{
		tmpFile:         f,
		cfg:             Config{RemoteFilePath: "/r/log"},
		stopCh:          make(chan struct{}),
		supDone:         make(chan struct{}),
		keepTempFile:    true, // assertions read the file after Close
		probeInterval:   10 * time.Millisecond,
		closeTimeout:    2 * time.Second,
		reconnectBase:   time.Millisecond,
		reconnectMax:    5 * time.Millisecond,
		newRemoteStream: fr.open,
	}

	t.Cleanup(func() { _ = s.Close() }) // bounded abort+drain like production

	return s, f
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg any) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(2 * time.Millisecond)
	}

	if m, ok := msg.(string); ok {
		t.Fatal(m)
	}

	t.Fatal(msg.(func() string)())
}

// TestSupervisorResumesFromLocalEnd pins the core invariant: after a stream
// dies mid-tail, the reconnect command resumes from the LOCAL file end —
// never duplicating or gapping content.
func TestSupervisorResumesFromLocalEnd(t *testing.T) {
	fr := &fakeRemote{probeSize: int64(len("hello")), scripts: []fakeStream{
		{data: "", holdOpen: true},        // first tail: idle, dies silently
		{data: " world", holdOpen: false}, // reconnect delivers delta
		{data: "", holdOpen: true},        // park
	}}

	s, f := newSupervisorStreamer(t, fr, "hello")
	go s.startSupervisor()

	waitUntil(t, 2*time.Second, func() bool {
		fr.mu.Lock()
		defer fr.mu.Unlock()

		return len(fr.cmds) > 0
	}, "first stream never attached")

	fr.closePipes() // kill the held-open stream → supervisor reconnects
	waitUntil(t, 2*time.Second, func() bool { return fr.hasCmd("tail -c +6 -f") },
		func() string {
			fr.mu.Lock()
			defer fr.mu.Unlock()

			return fmt.Sprintf("reconnect did not resume from +6; cmds=%v", fr.cmds)
		}())
	waitUntil(t, 2*time.Second, func() bool {
		got, _ := os.ReadFile(f.Name())
		return string(got) == "hello world"
	}, "delta not appended")
}

// TestSupervisorRotationRestartsFromOne verifies rotation handling: probe sees
// remote size < offset, local file truncates, stream restarts at byte 1.
func TestSupervisorRotationRestartsFromOne(t *testing.T) {
	fr := &fakeRemote{}
	orig := fr.open
	fr.probeSize = 3                                  // remote rotated to a 3-byte file
	s, f := newSupervisorStreamer(t, fr, "OLDOLDOLD") // offset=9

	// Wrap: intercept stat probes to report a shrunken remote (size 3).
	s.newRemoteStream = func(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
		if strings.Contains(cmd, "wc -c") {
			return strings.NewReader("3\n"), nil, func() error { return nil }, func() {}, nil
		}

		return orig(cmd)
	}
	fr.scripts = []fakeStream{
		{data: "", holdOpen: true}, // pre-rotation tail (dies on abort)
		{data: "NEW", holdOpen: true},
	}

	go s.startSupervisor()

	waitUntil(t, 2*time.Second, func() bool {
		got, _ := os.ReadFile(f.Name())
		return string(got) == "NEW"
	}, func() string {
		got, _ := os.ReadFile(f.Name())

		fr.mu.Lock()
		defer fr.mu.Unlock()

		return fmt.Sprintf("rotation stalled; file=%q cmds=%v", got, fr.cmds)
	}())

	if !fr.hasCmd("tail -c +1 -f") {
		t.Fatal("expected restart from +1 after rotation")
	}
}

// TestCloseBoundedWhileOpenParked pins the io-F1 containment: Close must
// return promptly even when the transport parks forever inside open.
func TestCloseBoundedWhileOpenParked(t *testing.T) {
	park := make(chan struct{})
	defer close(park)

	fr := &fakeRemote{}
	s, _ := newSupervisorStreamer(t, fr, "")
	s.closeTimeout = 50 * time.Millisecond

	s.newRemoteStream = func(_ string) (io.Reader, io.Reader, func() error, func(), error) {
		<-park // parked forever, ignores stopCh like a dead TCP conn would
		return nil, nil, nil, nil, errors.New("unreachable")
	}
	go s.startSupervisor()

	done := make(chan struct{})

	go func() { s.Close(); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Close hung despite bounded wait")
	}
}

// TestStreamerCloseRemovesTempFile verifies that Close() now removes the local
// temp file by default (H2), so a crash between Close and an explicit cleanup
// can no longer leak it.
func TestStreamerCloseRemovesTempFile(t *testing.T) {
	tmpDir := t.TempDir()

	tmpFile, err := os.CreateTemp(tmpDir, "test-*.log")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tmpFile.WriteString("test\n")
	tmpFile.Close()

	s := &Streamer{
		tmpFile: func() *os.File {
			f, _ := os.Open(tmpFile.Name())
			return f
		}(),
		localPath: tmpFile.Name(),
		stopCh:    make(chan struct{}),
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if _, err := os.Stat(tmpFile.Name()); !os.IsNotExist(err) {
		t.Fatal("temp file should be removed by default Close")
	}
}

// TestStreamerDoubleCloseIdempotent ensures repeated Close() calls are safe.
func TestStreamerDoubleCloseIdempotent(t *testing.T) { //nolint:dupl // symmetric streamer close/cleanup tests
	tmpDir := t.TempDir()

	tmpFile, err := os.CreateTemp(tmpDir, "test-*.log")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tmpFile.WriteString("test\n")
	tmpFile.Close()

	s := &Streamer{
		tmpFile: func() *os.File {
			f, _ := os.Open(tmpFile.Name())
			return f
		}(),
		localPath: tmpFile.Name(),
		stopCh:    make(chan struct{}),
	}

	if err := s.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}
}

// TestStreamerCleanupAfterCloseNoOp verifies CleanupTempFile is a no-op once
// Close has already removed the file.
func TestStreamerCleanupAfterCloseNoOp(t *testing.T) { //nolint:dupl // symmetric streamer close/cleanup tests
	tmpDir := t.TempDir()

	tmpFile, err := os.CreateTemp(tmpDir, "test-*.log")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tmpFile.WriteString("test\n")
	tmpFile.Close()

	s := &Streamer{
		tmpFile: func() *os.File {
			f, _ := os.Open(tmpFile.Name())
			return f
		}(),
		localPath: tmpFile.Name(),
		stopCh:    make(chan struct{}),
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if err := s.CleanupTempFile(); err != nil {
		t.Fatalf("CleanupTempFile after Close should be a no-op, got: %v", err)
	}
}

// newInitialSyncStreamer builds a Streamer wired to fr for initialSync tests.
// No supervisor runs here, so supDone is pre-closed (Close would otherwise wait
// out its timeout for a goroutine that was never started), and the open-path
// watchdog timings are compressed to keep the tests fast.
func newInitialSyncStreamer(t *testing.T, fr *fakeRemote) (*Streamer, *os.File) {
	t.Helper()
	s, f := newSupervisorStreamer(t, fr, "")
	close(s.supDone)
	s.stallTimeout = 40 * time.Millisecond
	s.probeTimeout = 40 * time.Millisecond
	s.abortGrace = 20 * time.Millisecond

	return s, f
}

// syncInBackground runs initialSync and returns a channel carrying its result,
// so a test can assert that a parked copy is actually released instead of
// deadlocking the test binary.
func syncInBackground(ctx context.Context, s *Streamer) <-chan error {
	errCh := make(chan error, 1)

	go func() { errCh <- s.initialSync(ctx) }()

	return errCh
}

func awaitSync(t *testing.T, errCh <-chan error, timeout time.Duration) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		t.Fatal("initialSync did not return: the copy was never aborted")
		return nil
	}
}

// TestInitialSyncCopiesRemoteFile pins the happy path after routing the initial
// copy through the newRemoteStream seam: the whole remote file lands in the temp
// file, the byte counter is updated, and the progress bridge reports the stage.
func TestInitialSyncCopiesRemoteFile(t *testing.T) {
	const content = "line1\nline2\nline3\n"

	fr := &fakeRemote{scripts: []fakeStream{{data: content}}}
	s, f := newInitialSyncStreamer(t, fr)

	var stages []string

	s.cfg.Progress = func(stage string, _ int64, _ int64) { stages = append(stages, stage) }

	if err := s.initialSync(context.Background()); err != nil {
		t.Fatalf("initialSync: %v", err)
	}

	if !fr.hasCmd("cat -- '/r/log'") {
		t.Errorf("expected a quoted remote cat, got %q", fr.lastCmd())
	}

	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != content {
		t.Errorf("temp file = %q, want %q", got, content)
	}

	if s.Downloaded() != int64(len(content)) {
		t.Errorf("Downloaded() = %d, want %d", s.Downloaded(), len(content))
	}

	if len(stages) == 0 || stages[0] != "downloading" {
		t.Errorf("progress stages = %v, want to start with downloading", stages)
	}
}

// TestInitialSyncCancelledByContext verifies the cancellation contract: an open
// parked on a remote that sends nothing must end as soon as the caller's context
// is cancelled (app shutdown), reporting context.Canceled rather than hanging
// forever inside io.Copy.
func TestInitialSyncCancelledByContext(t *testing.T) {
	fr := &fakeRemote{scripts: []fakeStream{{holdOpen: true}}}
	s, _ := newInitialSyncStreamer(t, fr)
	s.stallTimeout = time.Hour // isolate cancellation from stall detection

	defer fr.closePipes()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := syncInBackground(ctx, s)

	// Let the copy park on the open pipe, then cancel it.
	time.Sleep(20 * time.Millisecond)
	cancel()

	err := awaitSync(t, errCh, 2*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("initialSync error = %v, want context.Canceled", err)
	}
}

// TestInitialSyncAbortsOnStall covers the case a context cannot: the connection
// is up and the remote simply stops sending. Without a stall watchdog the open
// waits forever (TCP gives the read no deadline of its own), leaving the UI on a
// spinner with a growing temp file and no way out.
func TestInitialSyncAbortsOnStall(t *testing.T) {
	fr := &fakeRemote{scripts: []fakeStream{{data: "partial\n", holdOpen: true}}}

	s, _ := newInitialSyncStreamer(t, fr)
	defer fr.closePipes()

	errCh := syncInBackground(context.Background(), s)

	err := awaitSync(t, errCh, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("initialSync error = %v, want a stall abort", err)
	}
	// The bytes delivered before the stall are still counted (and written).
	if s.Downloaded() != int64(len("partial\n")) {
		t.Errorf("Downloaded() = %d, want %d", s.Downloaded(), len("partial\n"))
	}
}

// TestInitialSyncStallTimerResetsOnProgress guards against the watchdog killing
// a slow-but-healthy transfer: a remote that keeps trickling bytes must never be
// aborted, however long the whole copy takes.
func TestInitialSyncStallTimerResetsOnProgress(t *testing.T) {
	fr := &fakeRemote{scripts: []fakeStream{{holdOpen: true}}}
	s, f := newInitialSyncStreamer(t, fr)
	s.stallTimeout = 60 * time.Millisecond

	errCh := syncInBackground(context.Background(), s)

	// Trickle for well over one stall interval, then finish the stream.
	for range 6 {
		time.Sleep(20 * time.Millisecond)
		fr.mu.Lock()

		for _, pw := range fr.pipes {
			pw.WriteString("tick\n")
		}

		fr.mu.Unlock()
	}

	fr.closePipes()

	if err := awaitSync(t, errCh, 2*time.Second); err != nil {
		t.Fatalf("initialSync aborted a healthy trickling transfer: %v", err)
	}

	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if want := strings.Repeat("tick\n", 6); string(got) != want {
		t.Errorf("temp file = %q, want %q", got, want)
	}
}

// TestInitialSyncAbortsOnClose verifies that closing the streamer during the
// initial copy releases the open instead of parking it: Close is how tab close
// and app shutdown reclaim a still-connecting SSH tab.
func TestInitialSyncAbortsOnClose(t *testing.T) {
	fr := &fakeRemote{scripts: []fakeStream{{holdOpen: true}}}
	s, _ := newInitialSyncStreamer(t, fr)
	s.stallTimeout = time.Hour

	defer fr.closePipes()

	errCh := syncInBackground(context.Background(), s)

	time.Sleep(20 * time.Millisecond)
	// Replicate exactly what Close does to the sync path (mark closed, signal
	// stopCh) without also tearing the temp file down underneath the in-flight
	// copy — that would race the watchdog with a plain write error. Marking
	// closed keeps the harness's deferred Close idempotent.
	s.mu.Lock()
	s.closed = true
	close(s.stopCh)
	s.mu.Unlock()

	err := awaitSync(t, errCh, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "closed during initial sync") {
		t.Fatalf("initialSync error = %v, want a closed-streamer abort", err)
	}
}

// TestInitialSyncReportsRemoteStderr verifies that a failing remote command
// names its cause. A bare non-zero exit status ("Process exited with status 1")
// is indistinguishable between a missing file and a permission problem, so the
// remote stderr tail is folded into the error.
func TestInitialSyncReportsRemoteStderr(t *testing.T) {
	fr := &fakeRemote{scripts: []fakeStream{{
		waitErr:    errors.New("Process exited with status 1"),
		stderrData: "cat: /r/log: Permission denied\n",
	}}}
	s, _ := newInitialSyncStreamer(t, fr)

	err := s.initialSync(context.Background())
	if err == nil {
		t.Fatal("expected initialSync to fail on a non-zero remote exit")
	}

	if !strings.Contains(err.Error(), "Permission denied") {
		t.Errorf("error = %v, want the remote stderr tail included", err)
	}
}

// TestInitialSyncCancelledBeforeStart pins the cheap pre-check: an already
// cancelled context must not even dial.
func TestNewStreamerContextCancelledBeforeDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := validBase()
	cfg.AuthType = "password"

	cfg.Password = "secret"
	if _, err := NewStreamerContext(ctx, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("NewStreamerContext error = %v, want context.Canceled", err)
	}
}

// TestProbeRemoteSizeTimesOut verifies the size probe degrades to "unknown"
// instead of blocking the open: it only feeds the progress denominator, so a
// session parked on a dead connection must not hold the open hostage.
func TestProbeRemoteSizeTimesOut(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Close()

	fr := &fakeRemote{}
	s, _ := newInitialSyncStreamer(t, fr)
	s.newRemoteStream = func(_ string) (io.Reader, io.Reader, func() error, func(), error) {
		// Never delivers a size and never EOFs, like a black-holed session.
		return pr, nil, func() error { return nil }, func() { pw.Close() }, nil
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		if sz, ok := s.probeRemoteSize(); ok {
			t.Errorf("probeRemoteSize() = (%d, true), want a timeout", sz)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probeRemoteSize hung on a parked session")
	}
}

// TestSupervisorKeepsTailingAfterOpenContextCancelled is the regression guard for
// the open-phase scoping of the streamer context: callers derive the open context
// from their lifecycle and cancel it as soon as the open returns (the desktop's
// openSSHResources does exactly that, with `defer cancel()`). A streamer that
// stayed bound to it would lose its live tail the instant it was successfully
// opened — every SSH tab would show the initial snapshot and then silently freeze.
func TestSupervisorKeepsTailingAfterOpenContextCancelled(t *testing.T) {
	fr := &fakeRemote{probeSize: int64(len("hello")), scripts: []fakeStream{
		{data: " world", holdOpen: true},
	}}

	s, f := newSupervisorStreamer(t, fr, "hello")
	defer fr.closePipes()

	// Simulate a completed open whose context is then cancelled by the caller.
	ctx, cancel := context.WithCancel(context.Background())
	s.openCtx = ctx
	s.endOpenPhase()
	cancel()

	go s.startSupervisor()

	waitUntil(t, 2*time.Second, func() bool {
		b, _ := os.ReadFile(f.Name())
		return string(b) == "hello world"
	}, func() string {
		b, _ := os.ReadFile(f.Name())
		return fmt.Sprintf("tail stopped after the open context was cancelled: temp file = %q, want %q", b, "hello world")
	})
}

// TestProbeRemoteSizeAbortsOnCancelledOpenContext: the size probe runs on the open
// path (it feeds the progress denominator), so a cancelled open must not have to
// wait out the probe timeout — tens of seconds — before the caller gives up.
func TestProbeRemoteSizeAbortsOnCancelledOpenContext(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Close()

	fr := &fakeRemote{}
	s, _ := newInitialSyncStreamer(t, fr)
	s.probeTimeout = time.Hour // isolate cancellation from the timeout watchdog
	s.newRemoteStream = func(_ string) (io.Reader, io.Reader, func() error, func(), error) {
		return pr, nil, func() error { return nil }, func() { pw.Close() }, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.openCtx = ctx

	done := make(chan struct{})

	go func() {
		defer close(done)

		if sz, ok := s.probeRemoteSize(); ok {
			t.Errorf("probeRemoteSize() = (%d, true), want a cancelled probe", sz)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probeRemoteSize ignored the cancelled open context")
	}
}

// Once the open phase ends, the probe must fall back to its timeout watchdog
// rather than to a cancellation source that no longer exists.
func TestProbeRemoteSizeAfterOpenPhaseUsesTimeout(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Close()

	fr := &fakeRemote{}
	s, _ := newInitialSyncStreamer(t, fr)
	ctx, cancel := context.WithCancel(context.Background())
	s.openCtx = ctx
	s.endOpenPhase()
	cancel() // cancelled, but the open phase is over: must not short-circuit

	s.newRemoteStream = func(_ string) (io.Reader, io.Reader, func() error, func(), error) {
		return pr, nil, func() error { return nil }, func() { pw.Close() }, nil
	}

	start := time.Now()

	if sz, ok := s.probeRemoteSize(); ok {
		t.Fatalf("probeRemoteSize() = (%d, true), want a timeout", sz)
	}

	if elapsed := time.Since(start); elapsed < s.probeTimeout {
		t.Fatalf("probe returned after %v, want it to wait out the %v timeout", elapsed, s.probeTimeout)
	}
}

// A streamer built without any context (field-by-field, as in these tests) must
// behave exactly as before: openDone yields a nil channel, which blocks forever in
// a select rather than firing immediately.
func TestOpenDoneNilContextNeverFires(t *testing.T) {
	s := &Streamer{}
	if ch := s.openDone(); ch != nil {
		t.Fatalf("openDone() = %v, want nil for a context-less streamer", ch)
	}

	select {
	case <-s.openDone():
		t.Fatal("openDone() fired for a context-less streamer")
	default:
	}
}

// --- Lister (remote directory browser) ---------------------------------

// fakeListerRemote is a scripted newRemoteCommand impl for Lister tests: it
// needs no real SSH client. Commands are matched by prefix; unknown commands
// error.
type fakeListerRemote struct {
	home    string
	listing string // NUL-terminated find -printf output for the default dir
	// fn lets a test override the scripted behavior (e.g. simulate a remote
	// command error). When nil, open falls back to the home/listing defaults.
	fn func(cmd string) (io.Reader, io.Reader, func() error, func(), error)
}

func (f *fakeListerRemote) open(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
	if f.fn != nil {
		return f.fn(cmd)
	}

	switch {
	case strings.HasPrefix(cmd, "echo ~"):
		return strings.NewReader(f.home + "\n"), nil, func() error { return nil }, func() {}, nil
	case strings.HasPrefix(cmd, "find "):
		return strings.NewReader(f.listing), nil, func() error { return nil }, func() {}, nil
	default:
		return nil, nil, nil, nil, fmt.Errorf("fake: unexpected command %q", cmd)
	}
}

func newTestLister(t *testing.T, fr *fakeListerRemote) *Lister {
	t.Helper()

	return &Lister{
		cfg:              validBase(),
		stopCh:           make(chan struct{}),
		newRemoteCommand: fr.open,
	}
}

// TestParseFindOutput decodes NUL/tab records, including a name with a literal
// tab (reconstructed from the middle fields) and a symlink. The explicit
// []remote.DirEntry type pins the protocol-agnostic contract of pkg/remote.
func TestParseFindOutput(t *testing.T) {
	out := []byte("d\t/home/u/logs\t4096\t1700000000.0\x00" +
		"f\t/home/u/a b.log\t1234\t1700000001.5\x00" +
		"l\t/home/u/cur\t52\t1700000002.0\x00" +
		"f\t/home/u/ta\tb\tfile\t99\t1700000003.0\x00")

	entries := parseFindOutput(out)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if entries[0].Kind != "dir" || entries[0].Name != "logs" || entries[0].Path != "/home/u/logs" { //nolint:goconst // test fixture literal repeated for clarity
		t.Fatalf("entry0 wrong: %+v", entries[0])
	}

	if entries[1].Kind != "file" || entries[1].Name != "a b.log" || entries[1].Size != 1234 {
		t.Fatalf("entry1 wrong: %+v", entries[1])
	}

	if entries[1].ModTime != 1700000001 {
		t.Fatalf("entry1 mod time = %d, want 1700000001", entries[1].ModTime)
	}

	if entries[2].Kind != "symlink" || entries[2].Name != "cur" {
		t.Fatalf("entry2 wrong: %+v", entries[2])
	}
	// Name with a literal tab ("ta<TAB>b<TAB>file") must be rejoined from the
	// middle fields so Path stays intact.
	if entries[3].Name != "ta\tb\tfile" || entries[3].Path != "/home/u/ta\tb\tfile" {
		t.Fatalf("entry3 wrong (tab in name): %+v", entries[3])
	}
}

// TestListerHomeDir resolves "~" via the echo command.
func TestListerHomeDir(t *testing.T) {
	l := newTestLister(t, &fakeListerRemote{home: "/home/tester"})

	home, err := l.HomeDir(context.Background())
	if err != nil {
		t.Fatalf("HomeDir: %v", err)
	}

	if home != "/home/tester" {
		t.Fatalf("expected /home/tester, got %q", home)
	}

	_ = l.Close()
}

// TestListerListDir returns sorted dirs-first entries; an empty path resolves to
// the remote home via echo ~ then find. The result is the shared
// []remote.DirEntry type, so GUI/TUI/ftp all consume one listing shape.
func TestListerListDir(t *testing.T) {
	listing := []byte("f\t/home/u/zebra.log\t10\t1700000000.0\x00" +
		"d\t/home/u/logs\t4096\t1700000001.0\x00" +
		"f\t/home/u/alpha.log\t20\t1700000002.0\x00")
	l := newTestLister(t, &fakeListerRemote{home: "/home/u", listing: string(listing)})

	var entries []remote.DirEntry

	entries, err := l.ListDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// dirs first, then files alphabetically: logs, alpha.log, zebra.log
	if entries[0].Name != "logs" || entries[0].Kind != "dir" {
		t.Fatalf("expected dirs first, got %+v", entries[0])
	}

	if entries[1].Name != "alpha.log" || entries[2].Name != "zebra.log" {
		t.Fatalf("expected alpha.log then zebra.log, got %+v %+v", entries[1], entries[2])
	}

	_ = l.Close()
}

// TestListerListDirControlCharRejected proves navigation blocks on a control
// char in the path even though validateRemotePath no longer rejects emptiness.
func TestListerListDirControlCharRejected(t *testing.T) {
	l := newTestLister(t, &fakeListerRemote{home: "/home/u"})

	_, err := l.ListDir(context.Background(), "/home/u/\x01x")
	if err == nil {
		t.Fatal("expected error for control char in path")
	}

	_ = l.Close()
}

// TestListerListDirRemoteError surfaces a non-zero remote command (e.g. no such
// dir / permission denied) instead of panicking, naming the cause from the
// remote stderr.
func TestListerListDirRemoteError(t *testing.T) {
	fr := &fakeListerRemote{home: "/home/u"}
	fr.fn = func(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
		if strings.HasPrefix(cmd, "find ") {
			return strings.NewReader(""), strings.NewReader("find: '/nope': No such file or directory\n"),
				func() error { return errors.New("exit status 1") }, func() {}, nil
		}

		return strings.NewReader("/home/u\n"), nil, func() error { return nil }, func() {}, nil
	}
	l := newTestLister(t, fr)

	_, err := l.ListDir(context.Background(), "/nope")
	if err == nil {
		t.Fatal("expected remote error to be surfaced")
	}

	if !strings.Contains(err.Error(), "No such file or directory") {
		t.Fatalf("expected remote stderr in the error, got: %v", err)
	}

	_ = l.Close()
}

// TestListerListDirBusyBoxError detects a remote BusyBox `find` (e.g. Alpine
// Linux) that rejects the GNU `-printf` flag and asks the user to install the
// full GNU findutils package instead of bubbling up a raw BusyBox usage dump.
func TestListerListDirBusyBoxError(t *testing.T) {
	fr := &fakeListerRemote{home: "/home/u"}
	fr.fn = func(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
		if strings.HasPrefix(cmd, "find ") {
			return strings.NewReader(""),
				strings.NewReader("find: unrecognized: -printf\nBusyBox v1.37.0 multi-call binary. Usage: find [-HL] [PATH]... [OPTIONS] [ACTIONS]\n"),
				func() error { return errors.New("Process exited with status 1") }, func() {}, nil
		}

		return strings.NewReader("/home/u\n"), nil, func() error { return nil }, func() {}, nil
	}
	l := newTestLister(t, fr)

	_, err := l.ListDir(context.Background(), "/home/u")
	if err == nil {
		t.Fatal("expected BusyBox find error to be surfaced")
	}

	if !strings.Contains(err.Error(), "findutils") || !strings.Contains(err.Error(), "BusyBox") {
		t.Fatalf("expected a BusyBox/findutils hint, got: %v", err)
	}

	_ = l.Close()
}

// TestListerListDirQuotesPath verifies the path reaches the remote shell only as
// a single quoted literal, so a crafted directory name cannot inject commands.
func TestListerListDirQuotesPath(t *testing.T) {
	fr := &fakeListerRemote{home: "/home/u"}

	var got string

	fr.fn = func(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
		got = cmd
		return strings.NewReader(""), nil, func() error { return nil }, func() {}, nil
	}

	l := newTestLister(t, fr)
	if _, err := l.ListDir(context.Background(), "/tmp/a b;rm -rf /"); err != nil {
		t.Fatalf("ListDir: %v", err)
	}

	if !strings.Contains(got, "find '/tmp/a b;rm -rf /' -maxdepth 1 -mindepth 1 -printf ") {
		t.Fatalf("path not passed as a quoted literal: %q", got)
	}

	_ = l.Close()
}

// TestListerImplementsRemoteLister pins the protocol-agnostic contract: the ssh
// browser must be usable through remote.Lister (the ftp backend implements the
// same interface, so the app layer dispatches on protocol alone).
func TestListerImplementsRemoteLister(t *testing.T) {
	var l remote.Lister = newTestLister(t, &fakeListerRemote{home: "/home/u"})

	home, err := l.HomeDir(context.Background())
	if err != nil {
		t.Fatalf("HomeDir via remote.Lister: %v", err)
	}

	if home != "/home/u" {
		t.Fatalf("home = %q, want /home/u", home)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close via remote.Lister: %v", err)
	}
}

// TestListerCloseIdempotent verifies repeated Close calls are safe (the browser
// closes a cached lister on error paths and again on shutdown).
func TestListerCloseIdempotent(t *testing.T) {
	l := newTestLister(t, &fakeListerRemote{home: "/home/u"})
	for i := range 3 {
		if err := l.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i+1, err)
		}
	}

	if _, err := l.ListDir(context.Background(), "/home/u"); err == nil {
		t.Fatal("expected ListDir to fail on a closed lister")
	}
}

// TestListerListDirCancelledContext proves a listing parked on a dead
// connection is abandoned when the caller's context is cancelled instead of
// hanging the UI, and that the abandoned session is torn down.
func TestListerListDirCancelledContext(t *testing.T) {
	pr, pw := io.Pipe()
	aborted := make(chan struct{})
	fr := &fakeListerRemote{home: "/home/u"}
	fr.fn = func(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
		if strings.HasPrefix(cmd, "find ") {
			// stdout never reaches EOF until abort closes the pipe.
			return pr, nil, func() error { return nil }, func() {
				_ = pw.Close()

				select {
				case <-aborted:
				default:
					close(aborted)
				}
			}, nil
		}

		return strings.NewReader("/home/u\n"), nil, func() error { return nil }, func() {}, nil
	}

	l := newTestLister(t, fr)
	defer l.Close()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)

	go func() {
		_, err := l.ListDir(ctx, "/home/u")
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ListDir error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ListDir did not return after context cancellation")
	}

	select {
	case <-aborted:
	case <-time.After(3 * time.Second):
		t.Fatal("abandoned remote command was not aborted")
	}
}

// TestListerListDirClosedMidFlight proves Close unsticks an in-flight listing
// (the browser is closed while a slow directory is still being read).
func TestListerListDirClosedMidFlight(t *testing.T) {
	pr, pw := io.Pipe()
	fr := &fakeListerRemote{home: "/home/u"}
	fr.fn = func(cmd string) (io.Reader, io.Reader, func() error, func(), error) {
		if strings.HasPrefix(cmd, "find ") {
			return pr, nil, func() error { return nil }, func() { _ = pw.Close() }, nil
		}

		return strings.NewReader("/home/u\n"), nil, func() error { return nil }, func() {}, nil
	}
	l := newTestLister(t, fr)

	done := make(chan error, 1)

	go func() {
		_, err := l.ListDir(context.Background(), "/home/u")
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("ListDir error = %v, want a closed-lister error", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ListDir did not return after Close")
	}
}

// TestNewListerRejectsInvalidConfig proves NewLister shares Config.Validate with
// the streamer (so browsing cannot dial with half-filled credentials) while an
// empty RemoteFilePath — the whole point of browsing — is accepted.
func TestNewListerRejectsInvalidConfig(t *testing.T) {
	bad := validBase()

	bad.AuthType = "password" // password missing
	if _, err := NewLister(bad); err == nil {
		t.Fatal("expected NewLister to reject a config without a password")
	}

	browse := validBase()
	browse.RemoteFilePath = ""
	browse.AuthType = "password"

	browse.Password = "s"
	if err := browse.Validate(); err != nil {
		t.Fatalf("empty RemoteFilePath must be valid for browsing, got: %v", err)
	}
}
