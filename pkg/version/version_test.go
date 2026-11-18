// Package version provides build-time version information for GoFLE.
package version

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		input      string
		major      int
		minor      int
		patch      int
		prerelease string
		build      string
		ok         bool
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
		{"v2.5.7-rc.1+build.123", 2, 5, 7, "rc.1", "build.123", true},
		{"0.0.1", 0, 0, 1, "", "", true},
		{"1.0.0-", 1, 0, 0, "", "", true},
		{"1.0.0+", 1, 0, 0, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Helper()
			major, minor, patch, prerelease, build, ok := ParseSemVer(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseSemVer(%q): ok=%v, want %v", tt.input, ok, tt.ok)
				return
			}
			if !ok {
				return
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
		})
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

func TestInfoStringDevEmptyCommit(t *testing.T) {
	info := Info{
		Version:     "dev",
		BuildTime:   "2026-09-27T15:30:00Z",
		BuildCommit: "",
		BuildSource: "local",
	}
	s := info.String()
	expected := "dev (unknown, 2026-09-27T15:30:00Z)"
	if s != expected {
		t.Errorf("Info.String() = %q, want %q", s, expected)
	}
}

func TestInfoDetailedString(t *testing.T) {
	info := Info{
		Version:     "2.0.0",
		BuildTime:   "2026-09-27T15:30:00Z",
		BuildCommit: "abc1234",
		BuildSource: "ci",
		GoVersion:   "go1.23.0",
		Platform:    "linux/amd64",
	}
	s := info.DetailedString()
	expectedLines := []string{
		"Version:     2.0.0",
		"Build Time:  2026-09-27T15:30:00Z",
		"Build Commit: abc1234",
		"Build Source: ci",
		"Go Version:   go1.23.0",
		"Platform:     linux/amd64",
	}
	for _, line := range expectedLines {
		if !strings.Contains(s, line) {
			t.Errorf("DetailedString() missing %q, got %q", line, s)
		}
	}
}

func TestGet(t *testing.T) {
	t.Helper()
	info := Get()

	if info.Version == "" {
		t.Error("Get().Version should not be empty")
	}
	if info.GoVersion == "" {
		t.Error("Get().GoVersion should not be empty")
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Errorf("Get().GoVersion should start with 'go', got %q", info.GoVersion)
	}
	if info.Platform == "" {
		t.Error("Get().Platform should not be empty")
	}
	expectedPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if info.Platform != expectedPlatform {
		t.Errorf("Get().Platform = %q, want %q", info.Platform, expectedPlatform)
	}
	if info.BuildSource == "" {
		t.Error("Get().BuildSource should not be empty")
	}

	info2 := Get()
	if info2.Version != info.Version || info2.GoVersion != info.GoVersion || info2.Platform != info.Platform {
		t.Error("Get() should return consistent values")
	}
}

func TestFormatBuildTime(t *testing.T) {
	t.Helper()
	before := time.Now().UTC().Add(-time.Second)
	result := FormatBuildTime()
	after := time.Now().UTC().Add(time.Second)

	_, err := time.Parse(time.RFC3339, result)
	if err != nil {
		t.Errorf("FormatBuildTime() returned invalid RFC3339: %v", err)
	}

	parsed, _ := time.Parse(time.RFC3339, result)
	if parsed.Before(before) || parsed.After(after) {
		t.Errorf("FormatBuildTime() returned time outside expected range: %v", parsed)
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		input    []string
		sep      string
		expected string
	}{
		{[]string{}, ",", ""},
		{[]string{"a"}, ",", "a"},
		{[]string{"a", "b"}, ",", "a,b"},
		{[]string{"a", "b", "c"}, ",", "a,b,c"},
		{[]string{"hello", "world"}, " ", "hello world"},
		{[]string{"a", "b"}, "", "ab"},
		{[]string{"a", "b", "c", "d"}, "-", "a-b-c-d"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
			t.Helper()
			result := join(tt.input, tt.sep)
			if result != tt.expected {
				t.Errorf("join(%v, %q) = %q, want %q", tt.input, tt.sep, result, tt.expected)
			}
		})
	}
}

func TestInfoStringEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		info     Info
		expected string
	}{
		{
			name: "release version with empty commit",
			info: Info{
				Version:     "1.0.0",
				BuildTime:   "2026-01-01T00:00:00Z",
				BuildCommit: "",
				BuildSource: "release",
			},
			expected: "1.0.0 (, 2026-01-01T00:00:00Z)",
		},
		{
			name: "version with special chars",
			info: Info{
				Version:     "1.0.0-alpha.1",
				BuildTime:   "2026-01-01T00:00:00Z",
				BuildCommit: "abc-def",
				BuildSource: "local",
			},
			expected: "1.0.0-alpha.1 (abc-def, 2026-01-01T00:00:00Z)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			result := tt.info.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseSemVerVersionPrefix(t *testing.T) {
	tests := []struct {
		input string
		major int
		minor int
		patch int
		ok    bool
	}{
		{"v1.2.3", 1, 2, 3, true}, // lowercase v is stripped
		{"V1.2.3", 0, 2, 3, true}, // uppercase V is NOT stripped, atoi("V1")=0
		{"1.2.3", 1, 2, 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Helper()
			major, minor, patch, _, _, ok := ParseSemVer(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseSemVer(%q): ok=%v, want %v", tt.input, ok, tt.ok)
				return
			}
			if !ok {
				return
			}
			if major != tt.major || minor != tt.minor || patch != tt.patch {
				t.Errorf("ParseSemVer(%q) = %d.%d.%d, want %d.%d.%d", tt.input, major, minor, patch, tt.major, tt.minor, tt.patch)
			}
		})
	}
}

func TestParseSemVerLargeNumbers(t *testing.T) {
	major, minor, patch, _, _, ok := ParseSemVer("999.888.777")
	if !ok {
		t.Fatal("ParseSemVer should succeed for large numbers")
	}
	if major != 999 || minor != 888 || patch != 777 {
		t.Errorf("ParseSemVer = %d.%d.%d, want 999.888.777", major, minor, patch)
	}
}

func TestParseSemVerLeadingZeros(t *testing.T) {
	major, minor, patch, _, _, ok := ParseSemVer("01.02.03")
	if !ok {
		t.Fatal("ParseSemVer should succeed for leading zeros")
	}
	if major != 1 || minor != 2 || patch != 3 {
		t.Errorf("ParseSemVer = %d.%d.%d, want 1.2.3", major, minor, patch)
	}
}

func TestParseSemVerWithBuildOnly(t *testing.T) {
	major, minor, patch, prerelease, build, ok := ParseSemVer("1.0.0+build123")
	if !ok {
		t.Fatal("ParseSemVer should succeed")
	}
	if major != 1 || minor != 0 || patch != 0 {
		t.Errorf("version = %d.%d.%d", major, minor, patch)
	}
	if prerelease != "" {
		t.Errorf("prerelease = %q, want empty", prerelease)
	}
	if build != "build123" {
		t.Errorf("build = %q, want build123", build)
	}
}

func TestParseSemVerWithPrereleaseOnly(t *testing.T) {
	major, minor, patch, prerelease, build, ok := ParseSemVer("1.0.0-alpha")
	if !ok {
		t.Fatal("ParseSemVer should succeed")
	}
	if major != 1 || minor != 0 || patch != 0 {
		t.Errorf("version = %d.%d.%d", major, minor, patch)
	}
	if prerelease != "alpha" {
		t.Errorf("prerelease = %q, want alpha", prerelease)
	}
	if build != "" {
		t.Errorf("build = %q, want empty", build)
	}
}

func TestParseSemVerInvalidCases(t *testing.T) {
	// These cases should fail (ok=false) because they don't have exactly 3 dot-separated parts
	// or the leading "-" is treated as prerelease separator
	invalidCases := []string{
		"not-a-version",
		"1.2",
		"1.2.3.4",
		"",
		"-1.0.0", // leading "-" splits into prerelease
		"1.-1.0", // "-" in middle splits prerelease
		"+",      // split by "+" gives ["", ""], coreAndPre="", split("") by "-" gives []
		"+build", // same, coreAndPre=""
	}

	for _, tc := range invalidCases {
		t.Run(tc, func(t *testing.T) {
			t.Helper()
			_, _, _, _, _, ok := ParseSemVer(tc)
			if ok {
				t.Errorf("ParseSemVer(%q) should fail", tc)
			}
		})
	}

	// These cases parse but with atoi returning 0 for non-numeric parts
	// (atoi returns 0 on non-digits, so these technically "succeed" with 0 values)
	atoiZeroCases := []struct {
		input string
		major int
		minor int
		patch int
	}{
		{"1.2.x", 1, 2, 0},
		{"a.b.c", 0, 0, 0},
		{"1.0.-1", 1, 0, 0}, // "-1" after prerelease split, atoi("-1")=0
	}

	for _, tc := range atoiZeroCases {
		t.Run(tc.input, func(t *testing.T) {
			t.Helper()
			major, minor, patch, _, _, ok := ParseSemVer(tc.input)
			if !ok {
				t.Errorf("ParseSemVer(%q) should succeed (atoi returns 0)", tc.input)
				return
			}
			if major != tc.major || minor != tc.minor || patch != tc.patch {
				t.Errorf("ParseSemVer(%q) = %d.%d.%d, want %d.%d.%d", tc.input, major, minor, patch, tc.major, tc.minor, tc.patch)
			}
		})
	}
}

func TestInfoStringRegex(t *testing.T) {
	info := Info{
		Version:     "1.2.3",
		BuildTime:   "2026-01-01T00:00:00Z",
		BuildCommit: "abc123",
		BuildSource: "test",
	}
	result := info.String()
	pattern := regexp.MustCompile(`^\d+\.\d+\.\d+ \(.+, .+\)$`)
	if !pattern.MatchString(result) {
		t.Errorf("String() = %q does not match expected pattern", result)
	}
}

func TestIndexByte(t *testing.T) {
	tests := []struct {
		input    string
		byte     byte
		expected int
	}{
		{"hello", 'e', 1},
		{"hello", 'l', 2}, // first occurrence
		{"hello", 'z', -1},
		{"", 'a', -1},
		{"a", 'a', 0},
		{"abc", 'a', 0},
		{"abc", 'c', 2},
		{"aaa", 'a', 0},
		{"test", 't', 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q/%c", tt.input, tt.byte), func(t *testing.T) {
			t.Helper()
			result := indexByte(tt.input, tt.byte)
			if result != tt.expected {
				t.Errorf("indexByte(%q, %c) = %d, want %d", tt.input, tt.byte, result, tt.expected)
			}
		})
	}
}
