package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"GoFLE/pkg/ftp"
)

func main() {
	var (
		host   = flag.String("host", envOr("FTP_HOST", "127.0.0.1"), "FTP server host")
		port   = flag.Int("port", atoiEnv("FTP_PORT", 21), "FTP server port")
		user   = flag.String("user", envOr("FTP_USER", "anonymous"), "FTP username")
		pass   = flag.String("password", envOr("FTP_PASSWORD", "anonymous@"), "FTP password")
		path   = flag.String("remote", envOr("FTP_REMOTE", "/var/log/test.log"), "remote file path to stream")
		tlsm   = flag.String("tls", envOr("FTP_TLS", ""), "tls mode: ''|explicit|implicit")
		trust  = flag.Bool("trust", false, "write an unknown FTPS cert to the trust store and exit")
		trustp = flag.String("trust-store", envOr("FTP_CERTS", ""), "cert trust store path (defaults to ~/.config/GoFLE/ftp_certs)")
	)
	flag.Parse()

	cfg := ftp.Config{
		Host:           *host,
		Port:           *port,
		Username:       *user,
		AuthType:       "password",
		Password:       *pass,
		RemoteFilePath: *path,
		TLSMode:        *tlsm,
		KnownHostsPath: *trustp,
	}
	if err := cfg.Validate(); err != nil {
		fmt.Println("VALIDATE_FAIL:", err)
		os.Exit(1)
	}

	st, err := ftp.NewStreamer(context.Background(), cfg)
	if err != nil {
		if ce, ok := errors.AsType[*ftp.CertUnknownError](err); ok {
			fmt.Println("CERT_UNKNOWN (expected on first FTPS contact):")
			fmt.Println("  host:", ce.Host)
			fmt.Println("  fingerprint:", ce.Fingerprint)
			if *trust {
				storePath := *trustp
				if storePath == "" {
					storePath = defaultCertStore()
				}
				if werr := ftp.WriteCertTrust(storePath, ce.Line); werr != nil {
					fmt.Println("WRITE_TRUST_FAIL:", werr)
					os.Exit(1)
				}
				fmt.Println("WROTE_TRUST:", ce.Line)
			} else {
				fmt.Println("  re-run with -trust to write it, or set -trust-store")
			}
			os.Exit(1)
		}
		fmt.Println("OPEN_FAIL:", err)
		os.Exit(1)
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
		f, _ := os.OpenFile(*path, os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString("world-growth\n")
		f.Close()
		time.Sleep(1500 * time.Millisecond)
		b2, _ := os.ReadFile(st.LocalPath())
		if strings.Contains(string(b2), "world-growth") {
			fmt.Println("PASS: live growth observed over real FTP")
			return
		}
	}
	b3, _ := os.ReadFile(st.LocalPath())
	fmt.Println("FAIL_NO_GROWTH: local=", strings.TrimSpace(string(b3)))
	os.Exit(1)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoiEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func defaultCertStore() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "GoFLE", "ftp_certs")
	}
	return filepath.Join(os.TempDir(), "GoFLE", "ftp_certs")
}
