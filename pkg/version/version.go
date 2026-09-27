// Package version provides build-time version information for GoFLE.
// Version is injected at build time via -ldflags (see build.sh and Dockerfile).
package version

import (
	"fmt"
	"runtime"
	"time"
)

// Build information injected at build time via -ldflags.
// These variables are set by the linker during build.
var (
	// Version is the semantic version (MAJOR.MINOR.PATCH) or a dev identifier.
	Version = "dev"

	// BuildTime is the UTC build timestamp in RFC3339 format.
	BuildTime = ""

	// BuildCommit is the git commit hash (short).
	BuildCommit = ""

	// BuildSource indicates the build source: "local", "ci", "release", etc.
	BuildSource = "local"
)

// Info holds all version information.
type Info struct {
	Version     string
	BuildTime   string
	BuildCommit string
	BuildSource string
	GoVersion   string
	Platform    string
}

// Get returns the current version info.
func Get() Info {
	return Info{
		Version:     Version,
		BuildTime:   BuildTime,
		BuildCommit: BuildCommit,
		BuildSource: BuildSource,
		GoVersion:   runtime.Version(),
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
	}
}

// String returns a human-readable version string.
func (i Info) String() string {
	if i.Version == "dev" {
		commit := i.BuildCommit
		if commit == "" {
			commit = "unknown"
		}
		return fmt.Sprintf("%s (%s, %s)", i.Version, commit, i.BuildTime)
	}
	return fmt.Sprintf("%s (%s, %s)", i.Version, i.BuildCommit, i.BuildTime)
}

// DetailedString returns a detailed version string for diagnostics.
func (i Info) DetailedString() string {
	var b []string
	b = append(b, fmt.Sprintf("Version:     %s", i.Version))
	b = append(b, fmt.Sprintf("Build Time:  %s", i.BuildTime))
	b = append(b, fmt.Sprintf("Build Commit: %s", i.BuildCommit))
	b = append(b, fmt.Sprintf("Build Source: %s", i.BuildSource))
	b = append(b, fmt.Sprintf("Go Version:   %s", i.GoVersion))
	b = append(b, fmt.Sprintf("Platform:     %s", i.Platform))
	return join(b, "\n")
}

func join(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	if len(ss) == 1 {
		return ss[0]
	}
	n := len(sep) * (len(ss) - 1)
	for _, s := range ss {
		n += len(s)
	}
	b := make([]byte, n)
	bp := copy(b, ss[0])
	for _, s := range ss[1:] {
		bp += copy(b[bp:], sep)
		bp += copy(b[bp:], s)
	}
	return string(b)
}

// ParseSemVer attempts to parse a semantic version string.
// Returns major, minor, patch, and any prerelease/build metadata.
func ParseSemVer(v string) (major, minor, patch int, prerelease, build string, ok bool) {
	// Strip leading 'v' if present
	if len(v) > 0 && v[0] == 'v' {
		v = v[1:]
	}
	if v == "" {
		return 0, 0, 0, "", "", false
	}
	// Simple semver parsing (not full spec compliant, but sufficient for our use)
	// MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]
	
	// First split by '+' to separate build metadata
	buildParts := split(v, "+")
	coreAndPre := buildParts[0]
	if len(buildParts) > 1 {
		build = buildParts[1]
	}
	
	// Then split by '-' to separate prerelease
	parts := split(coreAndPre, "-")
	if len(parts) == 0 {
		return 0, 0, 0, "", "", false
	}
	core := parts[0]
	if len(parts) > 1 {
		prerelease = parts[1]
	}
	
	coreParts := split(core, ".")
	if len(coreParts) != 3 {
		return 0, 0, 0, "", "", false
	}
	major = atoi(coreParts[0])
	minor = atoi(coreParts[1])
	patch = atoi(coreParts[2])
	return major, minor, patch, prerelease, build, true
}

func split(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// FormatBuildTime returns the current UTC time in RFC3339 format for build injection.
func FormatBuildTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}