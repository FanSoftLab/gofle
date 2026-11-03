package indexer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// benchLogContent is the small .log payload of the bench archive (named once
// so the literal is not repeated across the package's test files).
const benchLogContent = "line one\nline two with ERROR\n"

// benchTarGz builds a tar.gz with nMembers payload members (each ~200 KB) plus
// one small app.log placed LAST, so the walk must inflate every member before
// reaching the winner — same work either way, only disk writes differ.
func benchTarGz(b *testing.B, dir string, nMembers int) string {
	b.Helper()

	path := filepath.Join(dir, "bench.tar.gz")

	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	payload := bytes.Repeat([]byte("2026-08-15T10:00:00.000Z INFO tar member filler line padding\n"), 3000)

	for i := range nMembers {
		name := fmt.Sprintf("data/metrics_%03d.txt", i)

		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(payload))}); err != nil {
			b.Fatal(err)
		}

		if _, err := tw.Write(payload); err != nil {
			b.Fatal(err)
		}
	}

	logContent := benchLogContent

	if err := tw.WriteHeader(&tar.Header{Name: "app.log", Mode: 0o644, Size: int64(len(logContent))}); err != nil {
		b.Fatal(err)
	}

	if _, err := tw.Write([]byte(logContent)); err != nil {
		b.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		b.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}

	return path
}

// BenchmarkEnsureDecompressedTarGz pins the G1 change on a 21-member archive:
//   - extract-all (pre-G1): every regular member written to disk, then one
//     picked by preference.
//   - select-one (G1): the walk inflates without writing; only the chosen
//     member is written.
//
// Inflate work is identical; the win is 1 disk write instead of 21 plus the
// per-member MkdirAll/Open/Close churn — and byte-for-byte less temp disk
// usage, which dominates when /tmp is slow or size-capped.
func BenchmarkEnsureDecompressedTarGz(b *testing.B) {
	dir := b.TempDir()
	path := benchTarGz(b, dir, 20)

	const nMembers = 21

	b.Run("extract-all-pre-G1", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			tmpDir := b.TempDir()

			extracted, err := extractAllMembers(path, tmpDir)
			if err != nil {
				b.Fatal(err)
			}

			if extracted != nMembers {
				b.Fatalf("extracted %d, want %d", extracted, nMembers)
			}
		}
	})

	b.Run("select-one-G1", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			indexed, cleanup, err := EnsureDecompressed(path)
			if err != nil {
				b.Fatal(err)
			}

			data, err := os.ReadFile(indexed)
			if err != nil {
				b.Fatal(err)
			}
			if len(data) == 0 {
				b.Fatal("empty extraction")
			}

			cleanup()
		}
	})
}

// extractAllMembers is the pre-G1 shape: extract EVERY regular member of the
// archive into destDir, returning the count. Kept as the documented baseline.
func extractAllMembers(archivePath, destDir string) (int, error) {
	src, err := os.Open(archivePath)
	if err != nil {
		return 0, err
	}
	defer src.Close()

	zr, err := gzip.NewReader(src)
	if err != nil {
		return 0, err
	}
	defer zr.Close()

	tr := tar.NewReader(zr)

	extracted := 0

	for {
		hdr, err := tr.Next()

		if errors.Is(err, io.EOF) {
			return extracted, nil
		}

		if err != nil {
			return 0, err
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		target := filepath.Join(destDir, filepath.Clean(hdr.Name))

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return 0, err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return 0, err
		}

		if _, err := io.Copy(out, tr); err != nil {
			out.Close()

			return 0, err
		}

		if err := out.Close(); err != nil {
			return 0, err
		}

		extracted++
	}
}
