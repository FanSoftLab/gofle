// Package version provides build-time version information for GoFLE.
package version

import (
	"testing"
)

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		input    string
		major    int
		minor    int
		patch    int
		prerelease string
		build    string
		ok       bool
	}{
		{"1.0.0", 1, 0, 0, "", "", true},
		{"v1.0.0", 1, 0, 0, "", "", true},
		{"2.1.0", 2, 1, 0, "", "", true},
		{"10.20.30", 10, 20, 30, "", "", true},
		{"1.0.0-alpha", 1, 0, 0, "alpha", "", true},
		{"1.0.0-beta.1", 1, 0, 0, "beta.1", "", true},
		{"1.0.0+build.1", 1, 0, 0, "", "build.1", true},
		{"1.0.0-alpha+build.1", 1, 0, 0, "alpha", "build.1", true},
		{"invalid", 0, 0, 0, "", "", false},
		{"1.0", 0, 0, 0, "", "", false},
		{"1.0.0.0", 0, 0, 0, "", "", false},
		{"", 0, 0, 0, "", "", false},
	}

	for _, tt := range tests {
		major, minor, patch, prerelease, build, ok := ParseSemVer(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseSemVer(%q): ok=%v, want %v", tt.input, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if major != tt.major || minor != tt.minor || patch != tt.patch {
			t.Errorf("ParseSemVer(%q): %d.%d.%d, want %d.%d.%d", tt.input, major, minor, patch, tt.major, tt.minor, tt.patch)
		}
		if prerelease != tt.prerelease {
			t.Errorf("ParseSemVer(%q): prerelease=%q, want %q", tt.input, prerelease, tt.prerelease)
		}
		if build != tt.build {
			t.Errorf("ParseSemVer(%q): build=%q, want %q", tt.input, build, tt.build)
		}
	}
}

func TestInfoString(t *testing.T) {
	info := Info{
		Version:     "2.0.0",
		BuildTime:   "2026-09-27T15:30:00Z",
		BuildCommit: "abc1234",
		BuildSource: "local",
	}
	s := info.String()
	expected := "2.0.0 (abc1234, 2026-09-27T15:30:00Z)"
	if s != expected {
		t.Errorf("Info.String() = %q, want %q", s, expected)
	}
}

func TestInfoStringDev(t *testing.T) {
	info := Info{
		Version:     "dev-abc1234",
		BuildTime:   "2026-09-27T15:30:00Z",
		BuildCommit: "abc1234",
		BuildSource: "local",
	}
	s := info.String()
	expected := "dev-abc1234 (abc1234, 2026-09-27T15:30:00Z)"
	if s != expected {
		t.Errorf("Info.String() = %q, want %q", s, expected)
	}
}