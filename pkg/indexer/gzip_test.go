package indexer

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGzFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.log.gz")

	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)
	if _, err := io.WriteString(zw, content); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestEnsureDecompressedGzip(t *testing.T) {
	content := "line one\nline two with ERROR\n"
	path := writeGzFixture(t, content)

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	if indexed == path {
		t.Fatal("expected a distinct decompressed temp path")
	}

	got, err := os.ReadFile(indexed)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}

	if !strings.Contains(string(got), "line two") {
		t.Fatalf("decompressed content mismatch: %q", got)
	}

	cleanup()

	if _, err := os.Stat(indexed); !os.IsNotExist(err) {
		t.Fatal("cleanup did not remove the temp file")
	}
}

func TestEnsureDecompressedPassthrough(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "plain.log")
	if err := os.WriteFile(path, []byte("plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}
	defer cleanup()

	if indexed != path {
		t.Fatalf("plain file must pass through, got %q", indexed)
	}
}
