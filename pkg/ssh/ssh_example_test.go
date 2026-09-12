package ssh

import (
	"fmt"
)

func Example_validConfig() {
	cfg := Config{
		Host:           "example.com",
		Port:           22,
		Username:       "user",
		AuthType:       "password",
		Password:       "secret",
		RemoteFilePath: "/var/log/syslog",
	}

	if err := cfg.Validate(); err != nil {
		fmt.Printf("Invalid: %v\n", err)
	} else {
		fmt.Println("Valid config")
	}
	// Output: Valid config
}

func Example_invalidConfig() {
	cfg := Config{
		Host:     "",
		Username: "user",
		AuthType: "password",
		Password: "secret",
	}

	if err := cfg.Validate(); err != nil {
		fmt.Printf("Invalid: %v\n", err)
	} else {
		fmt.Println("Valid config")
	}
	// Output: Invalid: validation failed: host is required
}

func ExampleHostKeyUnknownError() {
	err := &HostKeyUnknownError{
		Host:        "example.com",
		Fingerprint: "SHA256:abc123...",
		Line:        "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...",
	}
	fmt.Printf("Host: %s\n", err.Host)
	fmt.Printf("Fingerprint: %s\n", err.Fingerprint)
	fmt.Printf("Error: %s\n", err.Error())
	// Output:
	// Host: example.com
	// Fingerprint: SHA256:abc123...
	// Error: unknown host key for example.com (fingerprint SHA256:abc123...)
}

func ExampleHostKeyMismatchError() {
	err := &HostKeyMismatchError{
		Host: "example.com",
		Err:  fmt.Errorf("key mismatch"),
	}
	fmt.Printf("Host: %s\n", err.Host)
	fmt.Printf("Error: %s\n", err.Error())
	// Output:
	// Host: example.com
	// Error: host key mismatch for example.com: possible MITM
}

func ExampleWriteKnownHost() {
	// This demonstrates the format of a known_hosts line
	line := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI..."
	fmt.Println("Known hosts line:", line)
	// Output: Known hosts line: example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...
}

func ExampleConfig() {
	cfg := Config{
		Host:           "myserver",
		Port:           2222,
		Username:       "deploy",
		AuthType:       "key",
		KeyPath:        "~/.ssh/id_ed25519",
		KeyPassphrase:  "passphrase",
		RemoteFilePath: "/var/log/app.log",
	}
	fmt.Printf("Host: %s:%d\n", cfg.Host, cfg.Port)
	fmt.Printf("User: %s, Auth: %s\n", cfg.Username, cfg.AuthType)
	fmt.Printf("Key: %s\n", cfg.KeyPath)
	// Output:
	// Host: myserver:2222
	// User: deploy, Auth: key
	// Key: ~/.ssh/id_ed25519
}
