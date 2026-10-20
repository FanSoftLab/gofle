package indexer

import (
	"archive/tar"
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

func writeTarGzFixture(t *testing.T, files map[string]string, filename string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), filename)

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
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

func TestEnsureDecompressedTarGz(t *testing.T) {
	files := map[string]string{
		"app.log":   "line one\nline two with ERROR\nline three\n",
		"other.txt": "some other content\n",
	}
	path := writeTarGzFixture(t, files, "app.tar.gz")

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	// Should return the .log file
	if !strings.HasSuffix(indexed, "app.log") {
		t.Fatalf("expected .log file, got %q", indexed)
	}

	got, err := os.ReadFile(indexed)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}

	if !strings.Contains(string(got), "line two") {
		t.Fatalf("decompressed content mismatch: %q", got)
	}

	cleanup()

	// Temp directory should be cleaned up
	if _, err := os.Stat(filepath.Dir(indexed)); !os.IsNotExist(err) {
		t.Fatal("cleanup did not remove the temp directory")
	}
}

func TestEnsureDecompressedTgz(t *testing.T) {
	files := map[string]string{
		"logs/app.log":   "line one\nline two with ERROR\n",
		"logs/debug.log": "debug info\n",
	}
	path := writeTarGzFixture(t, files, "app.tgz")

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	// Should return a .log file (order not guaranteed with map iteration)
	if !strings.HasSuffix(indexed, ".log") {
		t.Fatalf("expected .log file, got %q", indexed)
	}

	got, err := os.ReadFile(indexed)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}

	// Either file should work - verify content is one of the expected files
	content := string(got)
	if !strings.Contains(content, "line") && !strings.Contains(content, "debug") {
		t.Fatalf("decompressed content mismatch: %q", got)
	}

	cleanup()

	if _, err := os.Stat(filepath.Dir(filepath.Dir(indexed))); !os.IsNotExist(err) {
		t.Fatal("cleanup did not remove the temp directory")
	}
}

func TestEnsureDecompressedTarGzNoLogExtension(t *testing.T) {
	files := map[string]string{
		"access.txt": "line one\nline two with ERROR\n",
		"error.txt":  "another error\n",
	}
	path := writeTarGzFixture(t, files, "app.tar.gz")

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	// Should return the largest file (access.txt has more content)
	got, err := os.ReadFile(indexed)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}

	if !strings.Contains(string(got), "line two") {
		t.Fatalf("decompressed content mismatch: %q", got)
	}

	cleanup()
}

func TestEnsureDecompressedTarGzSingleFile(t *testing.T) {
	files := map[string]string{
		"single.log": "only file\nwith ERROR\n",
	}
	path := writeTarGzFixture(t, files, "app.tar.gz")

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	got, err := os.ReadFile(indexed)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}

	if !strings.Contains(string(got), "ERROR") {
		t.Fatalf("decompressed content mismatch: %q", got)
	}

	cleanup()
}

func TestEnsureDecompressedTarGzNestedPaths(t *testing.T) {
	files := map[string]string{
		"var/log/app.log": "nested log\nERROR here\n",
		"var/log/syslog":  "system log\n",
	}
	path := writeTarGzFixture(t, files, "app.tar.gz")

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	got, err := os.ReadFile(indexed)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}

	if !strings.Contains(string(got), "ERROR here") {
		t.Fatalf("decompressed content mismatch: %q", got)
	}

	cleanup()
}

func TestEnsureDecompressedTarGzEmptyArchive(t *testing.T) {
	path := writeTarGzFixture(t, map[string]string{}, "app.tar.gz")

	_, _, err := EnsureDecompressed(path)
	if err == nil {
		t.Fatal("expected error for empty tar archive")
	}
	if !strings.Contains(err.Error(), "no regular files found") {
		t.Fatalf("expected 'no regular files found' error, got: %v", err)
	}
}

func TestEnsureDecompressedTarGzPathTraversal(t *testing.T) {
	files := map[string]string{
		"../etc/passwd": "malicious\n",
		"app.log":       "legitimate\n",
	}
	path := writeTarGzFixture(t, files, "app.tar.gz")

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	// Should skip the malicious file and return app.log
	if !strings.HasSuffix(indexed, "app.log") {
		t.Fatalf("expected app.log, got %q", indexed)
	}

	cleanup()
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

func TestEnsureDecompressedTarGzWithDirs(t *testing.T) {
	files := map[string]string{
		"logs/":        "", // directory entry
		"logs/app.log": "log content\nERROR\n",
		"README.md":    "readme\n",
	}
	path := writeTarGzFixture(t, files, "app.tar.gz")

	indexed, cleanup, err := EnsureDecompressed(path)
	if err != nil {
		t.Fatalf("EnsureDecompressed: %v", err)
	}

	got, err := os.ReadFile(indexed)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}

	if !strings.Contains(string(got), "ERROR") {
		t.Fatalf("decompressed content mismatch: %q", got)
	}

	cleanup()
}
