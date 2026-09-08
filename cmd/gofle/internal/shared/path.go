package shared

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// ExpandPath expands a leading "~", "~/x" or "~user/x" prefix and normalizes
// relative paths against the current working directory. It mirrors the local
// expansion done inside pkg/ssh for key paths, but lives here (exported) so the
// CLI can resolve export targets without touching the ssh package's unexported
// helper.
func ExpandPath(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}

		return p
	}

	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}

		return p
	}

	if strings.HasPrefix(p, "~") {
		// ~user/... form.
		rest := p[1:]

		name, tail, _ := strings.Cut(rest, "/")
		if u, err := user.Lookup(name); err == nil {
			if tail == "" {
				return u.HomeDir
			}

			return filepath.Join(u.HomeDir, tail)
		}

		return p
	}

	if !filepath.IsAbs(p) {
		if cwd, err := os.Getwd(); err == nil {
			return filepath.Join(cwd, p)
		}
	}

	return p
}

// PrepareExportTarget resolves a raw export path into an absolute path and
// reports whether the parent directory must be created first. An empty path is
// rejected. The caller is expected to prompt the user when createDir is true and,
// on confirmation, create the directory with os.MkdirAll before writing.
func PrepareExportTarget(path string) (absPath string, createDir bool, err error) {
	raw := strings.TrimSpace(path)
	if raw == "" {
		return "", false, os.ErrInvalid
	}

	abs := ExpandPath(raw)

	dir := filepath.Dir(abs)
	if fi, statErr := os.Stat(dir); statErr != nil {
		if os.IsNotExist(statErr) {
			statErr = nil
		}

		return abs, true, statErr
	} else if !fi.IsDir() {
		return abs, true, nil
	}

	return abs, false, nil
}
