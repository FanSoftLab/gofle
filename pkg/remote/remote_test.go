package remote

import "testing"

func TestDirEntryKind(t *testing.T) {
	e := DirEntry{
		Name:    "app.log",
		Path:    "/var/log/app.log",
		Kind:    "file",
		Size:    1024,
		ModTime: 1700000000,
	}
	if e.Kind != "file" {
		t.Fatalf("expected Kind=file, got %q", e.Kind)
	}

	if e.Size != 1024 {
		t.Fatalf("expected Size=1024, got %d", e.Size)
	}
}
