package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	if home, err := os.UserHomeDir(); err == nil {
		if got := ExpandPath("~"); got != home {
			t.Errorf("ExpandPath(~) = %q, want %q", got, home)
		}

		want := filepath.Join(home, "logs", "x.log")
		if got := ExpandPath("~/logs/x.log"); got != want {
			t.Errorf("ExpandPath(~/logs/x.log) = %q, want %q", got, want)
		}
	}

	// absolute paths are returned unchanged
	abs := filepath.Join(string(filepath.Separator), "tmp", "a.log")
	if got := ExpandPath(abs); got != abs {
		t.Errorf("ExpandPath(absolute) = %q, want %q", got, abs)
	}

	// relative paths are normalized to an absolute path against the cwd
	rel := ExpandPath("rel/path.log")
	if !filepath.IsAbs(rel) {
		t.Errorf("ExpandPath(relative) = %q, want absolute", rel)
	}
}

func TestPrepareExportTarget(t *testing.T) {
	dir := t.TempDir()

	// existing directory -> no creation needed
	abs, need, err := PrepareExportTarget(filepath.Join(dir, "out.log"))
	if err != nil {
		t.Fatalf("PrepareExportTarget existing dir: %v", err)
	}

	if need {
		t.Error("PrepareExportTarget reported needCreate for an existing directory")
	}

	if !filepath.IsAbs(abs) {
		t.Errorf("PrepareExportTarget did not return an absolute path: %q", abs)
	}

	// missing directory -> caller must create it
	missing := filepath.Join(dir, "nope", "deep", "out.log")

	abs, need, err = PrepareExportTarget(missing)
	if err != nil {
		t.Fatalf("PrepareExportTarget missing dir: %v", err)
	}

	if !need {
		t.Error("PrepareExportTarget should report needCreate for a missing directory")
	}

	if abs == "" {
		t.Error("PrepareExportTarget returned an empty path")
	}

	// empty path -> error
	if _, _, err := PrepareExportTarget(""); err == nil {
		t.Error("PrepareExportTarget should error on an empty path")
	}
}
