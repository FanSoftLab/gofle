package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"GoFLE/pkg/ssh"
)

func main() {
	// Remote-side fixture (same container = "remote" filesystem).
	os.WriteFile("/var/log/test.log", []byte("hello\n"), 0o644)

	kh := "/tmp/kilo-known-hosts"
	_ = os.WriteFile(kh, nil, 0o600)
	cfg := ssh.Config{
		Host: "127.0.0.1", Port: 22, Username: "root",
		RemoteFilePath: "/var/log/test.log",
		AuthType:       "password", Password: "testpw",
		KnownHostsPath: kh,
	}
	if err := cfg.Validate(); err != nil {
		fmt.Println("VALIDATE_FAIL:", err)
		os.Exit(1)
	}

	st, err := ssh.NewStreamer(cfg)
	if err != nil {
		var hk *ssh.HostKeyUnknownError
		if !errors.As(err, &hk) {
			fmt.Println("OPEN_FAIL:", err)
			os.Exit(1)
		}
		fmt.Println("HOSTKEY_UNKNOWN (expected on first contact) -> trusting")
		if werr := ssh.WriteKnownHost(cfg.KnownHostsPath, hk.Line); werr != nil {
			fmt.Println("WRITE_KH_FAIL:", werr)
			os.Exit(1)
		}
		st, err = ssh.NewStreamer(cfg)
		if err != nil {
			fmt.Println("RETRY_OPEN_FAIL:", err)
			os.Exit(1)
		}
	}
	defer st.Close()

	b, _ := os.ReadFile(st.LocalPath())
	if !strings.Contains(string(b), "hello") {
		fmt.Println("INITIAL_SYNC_MISSING:", string(b))
		os.Exit(1)
	}
	fmt.Println("INITIAL_OK")

	// Simulate remote growth while the persistent stream follows.
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		f, _ := os.OpenFile("/var/log/test.log", os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString("world-growth\n")
		f.Close()
		time.Sleep(1500 * time.Millisecond)
		b2, _ := os.ReadFile(st.LocalPath())
		if strings.Contains(string(b2), "world-growth") {
			fmt.Println("PASS: live growth observed over real SSH")
			return
		}
	}
	b3, _ := os.ReadFile(st.LocalPath())
	fmt.Println("FAIL_NO_GROWTH: local=", strings.TrimSpace(string(b3)))
	os.Exit(1)
}
