package ftp

import (
	"fmt"
)

func Example_validConfig() {
	cfg := Config{
		Host:           "ftp.example.com",
		Port:           21,
		Username:       "user",
		Password:       "secret",
		RemoteFilePath: "/logs/app.log",
		TLSMode:        "explicit",
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
		Password: "secret",
	}

	if err := cfg.Validate(); err != nil {
		fmt.Printf("Invalid: %v\n", err)
	} else {
		fmt.Println("Valid config")
	}
	// Output: Invalid: validation failed: host is required
}

func ExampleCertUnknownError() {
	err := &CertUnknownError{
		Host:        "ftp.example.com:21",
		Fingerprint: "a1b2c3d4e5f6...",
		Line:        "ftp.example.com:21 a1b2c3d4e5f6...",
	}
	fmt.Printf("Host: %s\n", err.Host)
	fmt.Printf("Fingerprint: %s\n", err.Fingerprint)
	fmt.Printf("Error: %s\n", err.Error())
	// Output:
	// Host: ftp.example.com:21
	// Fingerprint: a1b2c3d4e5f6...
	// Error: unknown FTP server certificate for ftp.example.com:21 (fingerprint a1b2c3d4e5f6...)
}

func ExampleCertMismatchError() {
	err := &CertMismatchError{
		Host:        "ftp.example.com:21",
		Fingerprint: "a1b2c3d4e5f6...",
		Expected:    "f1e2d3c4b5a6...",
	}
	fmt.Printf("Host: %s\n", err.Host)
	fmt.Printf("Error: %s\n", err.Error())
	// Output:
	// Host: ftp.example.com:21
	// Error: FTP server certificate mismatch for ftp.example.com:21: possible MITM
}

func ExampleWriteCertTrust() {
	// This demonstrates the format of a cert trust store line
	line := "ftp.example.com:21 a1b2c3d4e5f67890abcdef1234567890abcdef1234567890abcdef1234567890"
	fmt.Println("Trust store line:", line)
	// Output: Trust store line: ftp.example.com:21 a1b2c3d4e5f67890abcdef1234567890abcdef1234567890abcdef1234567890
}

func ExampleConfig() {
	cfg := Config{
		Host:           "ftps.example.com",
		Port:           990,
		Username:       "deploy",
		Password:       "secret",
		RemoteFilePath: "/var/log/app.log",
		TLSMode:        "implicit",
	}
	fmt.Printf("Host: %s:%d\n", cfg.Host, cfg.Port)
	fmt.Printf("User: %s, TLS: %s\n", cfg.Username, cfg.TLSMode)
	// Output:
	// Host: ftps.example.com:990
	// User: deploy, TLS: implicit
}

func Example_tlsModes() {
	modes := []string{"", "none", "explicit", "implicit"}
	for _, m := range modes {
		display := m
		if display == "" {
			display = "none (plain FTP)"
		}
		fmt.Printf("  %s\n", display)
	}
	// Output:
	//   none (plain FTP)
	//   none
	//   explicit
	//   implicit
}
