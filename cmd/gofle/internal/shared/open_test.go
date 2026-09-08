package shared

import (
	"testing"
)

// TestParseRemoteURLSchemeDispatch verifies that parseRemoteURL recognizes both
// schemes and returns the matching protocol, rejecting anything else.
//
//nolint:gocyclo // several independent scheme assertions share one table-style test
func TestParseRemoteURLSchemeDispatch(t *testing.T) {
	proto, host, port, user, path, err := parseRemoteURL("ssh://bob@host.example:222/var/log/x.log")
	if err != nil {
		t.Fatalf("ssh parse error: %v", err)
	}

	if proto != "ssh" || host != "host.example" || port != 222 || user != "bob" || path != "/var/log/x.log" {
		t.Fatalf("ssh parse = (%q,%q,%d,%q,%q), want (ssh,host.example,222,bob,/var/log/x.log)", proto, host, port, user, path)
	}

	proto, host, port, user, path, err = parseRemoteURL("ftp://alice@ftp.example:2121/pub/x.log")
	if err != nil {
		t.Fatalf("ftp parse error: %v", err)
	}

	if proto != "ftp" || host != "ftp.example" || port != 2121 || user != "alice" || path != "/pub/x.log" {
		t.Fatalf("ftp parse = (%q,%q,%d,%q,%q), want (ftp,ftp.example,2121,alice,/pub/x.log)", proto, host, port, user, path)
	}

	if _, _, _, _, _, err = parseRemoteURL("http://host/x"); err == nil {
		t.Fatal("expected error for unsupported scheme http://")
	}
}

// TestParseFTPURLSchemeEnforcement verifies parseFTPURL rejects ssh:// targets
// and parseSSHURL rejects ftp:// targets, while each accepts its own scheme.
func TestParseFTPURLSchemeEnforcement(t *testing.T) {
	if _, _, _, _, err := parseFTPURL("ssh://h:22/x"); err == nil {
		t.Fatal("parseFTPURL should reject ssh://")
	}

	if _, _, _, _, err := parseSSHURL("ftp://h:21/x"); err == nil {
		t.Fatal("parseSSHURL should reject ftp://")
	}

	if _, _, _, _, err := parseFTPURL("ftp://h:21/x"); err != nil {
		t.Fatalf("parseFTPURL should accept ftp://: %v", err)
	}

	if _, _, _, _, err := parseSSHURL("ssh://h:22/x"); err != nil {
		t.Fatalf("parseSSHURL should accept ssh://: %v", err)
	}
}
