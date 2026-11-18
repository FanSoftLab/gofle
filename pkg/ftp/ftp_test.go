package ftp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	base := Config{Host: "h", Username: "u", Password: "p"}

	if err := (Config{}).Validate(); err == nil {
		t.Fatal("expected error for empty config")
	}

	if err := func() error { c := base; c.Host = ""; return c.Validate() }(); err == nil {
		t.Fatal("expected error for empty host")
	}

	if err := func() error { c := base; c.Username = ""; return c.Validate() }(); err == nil {
		t.Fatal("expected error for empty username")
	}

	if err := func() error { c := base; c.Password = ""; return c.Validate() }(); err == nil {
		t.Fatal("expected error for empty password")
	}

	if err := func() error { c := base; c.Port = 70000; return c.Validate() }(); err == nil {
		t.Fatal("expected error for out-of-range port")
	}

	if err := func() error { c := base; c.TLSMode = "bogus"; return c.Validate() }(); err == nil {
		t.Fatal("expected error for bad tls_mode")
	}

	if err := func() error { c := base; c.RemoteFilePath = "ok/\x00path"; return c.Validate() }(); err == nil {
		t.Fatal("expected error for control char in path")
	}

	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	for _, mode := range []string{"", "none", "explicit", "implicit"} {
		c := base

		c.TLSMode = mode
		if err := c.Validate(); err != nil {
			t.Fatalf("tls_mode %q rejected: %v", mode, err)
		}
	}
}

func generateTestCert(t *testing.T) (rawDER []byte, fingerprint string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		Subject:               pkix.Name{CommonName: "test-ftp"},
		Issuer:                pkix.Name{CommonName: "test-ftp"},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	// Re-encode as PEM just to exercise a realistic parse path parity, but we
	// only need the DER for rawCerts.
	_ = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	fp := sha256.Sum256(der)

	return der, hex.EncodeToString(fp[:])
}

func TestCertVerifier(t *testing.T) {
	der, fp := generateTestCert(t)
	host := "ftp.example.com:21"

	wantCases := []struct {
		name      string
		storeBody string
		wantErr   error // nil, *CertUnknownError or *CertMismatchError
	}{
		{"known match", host + " " + fp + "\n", nil},
		{"unknown host", "# empty store\n", &CertUnknownError{}},
		{"mismatch", host + " deadbeef\n", &CertMismatchError{}},
	}

	for _, tc := range wantCases {
		t.Run(tc.name, func(t *testing.T) {
			store := filepath.Join(t.TempDir(), "ftp_certs")
			if err := os.WriteFile(store, []byte(tc.storeBody), 0o600); err != nil {
				t.Fatal(err)
			}

			verify := newCertVerifier(store, host)
			err := verify([][]byte{der}, nil)

			var unknownTarget *CertUnknownError
			var mismatchTarget *CertMismatchError //nolint:wsl_v5 // test assertion switch is clearer as-is

			switch {
			case tc.wantErr == nil:
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
			case errors.As(tc.wantErr, &unknownTarget):
				var ce *CertUnknownError
				if !errors.As(err, &ce) {
					t.Fatalf("expected *CertUnknownError, got %T: %v", err, err)
				}
				if ce.Host != host || ce.Fingerprint != fp { //nolint:wsl_v5 // test assertion switch is clearer as-is
					t.Fatalf("unexpected CertUnknownError fields: %+v", ce)
				}
				if !strings.HasPrefix(ce.Line, host+" ") || !strings.Contains(ce.Line, fp) { //nolint:wsl_v5 // test assertion switch is clearer as-is
					t.Fatalf("unexpected Line: %q", ce.Line)
				}
			case errors.As(tc.wantErr, &mismatchTarget):
				if _, ok := errors.AsType[*CertMismatchError](err); !ok {
					t.Fatalf("expected *CertMismatchError, got %T: %v", err, err)
				}
			}
		})
	}
}

func TestWriteCertTrust(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "nested", "ftp_certs")

	if err := WriteCertTrust(store, "ftp.example.com:21 abcd"); err != nil {
		t.Fatalf("WriteCertTrust: %v", err)
	}

	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "ftp.example.com:21 abcd\n" {
		t.Fatalf("unexpected store content: %q", string(data))
	}

	// Append a second line.
	if err := WriteCertTrust(store, "ftp2.example.com:21 ef01"); err != nil {
		t.Fatalf("WriteCertTrust append: %v", err)
	}

	data, _ = os.ReadFile(store)
	if !strings.Contains(string(data), "ef01") {
		t.Fatalf("append missing: %q", string(data))
	}
}

func TestReadCertStore(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "ftp_certs")

	body := "# comment\n\nhost:21 aaaa\nother:990 bbbb\n"
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := readCertStore(store)
	if err != nil {
		t.Fatal(err)
	}

	if m["host:21"] != "aaaa" || m["other:990"] != "bbb"+"b" {
		t.Fatalf("unexpected map: %v", m)
	}

	missing, err := readCertStore(filepath.Join(dir, "does-not-exist"))
	if err != nil {
		t.Fatal(err)
	}

	if len(missing) != 0 {
		t.Fatalf("expected empty map for missing file, got %v", missing)
	}
}
