package indexer

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

// isGzipMagic reports whether the file starts with the gzip magic bytes
// (0x1f 0x8b).
func isGzipMagic(path string) (bool, error) {
	// Validate path to prevent path traversal via symlinks
	if err := validateFilePath(path); err != nil {
		return false, err
	}

	f, err := os.Open(path) //nolint:gosec // user-provided path is intended
	if err != nil {
		return false, err
	}

	defer func() { _ = f.Close() }()

	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false, nil // empty or 1-byte file: definitely not gzip
	}

	return magic[0] == 0x1f && magic[1] == 0x8b, nil //nolint:gosec // io.ReadFull guarantees a full 2-byte read
}

// EnsureDecompressed returns a path the byte-offset indexer can read.
//
// The index is a table of pread offsets, which is meaningless over a
// compressed stream — so a gzip file is transparently decompressed into a temp
// file and THAT path is returned. Non-gzip files pass through unchanged with a
// no-op cleanup. The caller owns cleanup and must invoke it when the indexed
// view is closed; it removes the temp file only for decompressed inputs.
func EnsureDecompressed(path string) (indexedPath string, cleanup func(), err error) {
	// Validate path to prevent path traversal via symlinks
	if err := validateFilePath(path); err != nil {
		return "", nil, err
	}

	gz, err := isGzipMagic(path)
	if err != nil {
		return "", nil, fmt.Errorf("failed to probe %s: %w", path, err)
	}

	if !gz {
		return path, func() {}, nil
	}

	src, err := os.Open(path) //nolint:gosec // user-provided path is intended
	if err != nil {
		return "", nil, fmt.Errorf("failed to open %s: %w", path, err)
	}

	defer func() { _ = src.Close() }()

	zr, err := gzip.NewReader(src)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read gzip header of %s: %w", path, err)
	}

	tmp, err := os.CreateTemp("", "gofle-gz-*.log")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file for decompression: %w", err)
	}

	// Limit decompression to 10GB to prevent zip bombs
	const maxDecompressedSize = 10 * 1024 * 1024 * 1024
	limitedReader := io.LimitReader(zr, maxDecompressedSize)

	buf := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(tmp, limitedReader, buf); err != nil { //nolint:gosec // decompression is bounded by the caller on trusted log files
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return "", nil, fmt.Errorf("failed to decompress %s: %w", path, err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("failed to finalize decompressed file: %w", err)
	}

	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}
