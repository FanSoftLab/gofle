package indexer

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

	return magic[0] == 0x1f && magic[1] == 0x8b, nil
}

// isLikelyTarByExtension reports whether the path suggests a tar archive.
func isLikelyTarByExtension(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tar") ||
		strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz")
}

// tarHeaderEntry is one regular-file member seen during the tar header walk.
type tarHeaderEntry struct {
	name string
	size int64
}

// selectTarLog applies the log-file preference order over walked headers:
// the first .log member, else the largest member, else the first member.
// Returns nil when there are no regular-file members.
func selectTarLog(entries []tarHeaderEntry) *tarHeaderEntry {
	if len(entries) == 0 {
		return nil
	}

	for i := range entries {
		if strings.HasSuffix(strings.ToLower(entries[i].name), ".log") {
			return &entries[i]
		}
	}

	best := &entries[0]
	for i := 1; i < len(entries); i++ {
		if entries[i].size > best.size {
			best = &entries[i]
		}
	}

	return best
}

// extractTarFile writes the current tar member (already positioned at its
// payload) into destDir under its sanitized name and returns the written path.
func extractTarFile(tr *tar.Reader, hdr *tar.Header, destDir string) (string, error) {
	safeName := filepath.Clean(hdr.Name)
	if strings.HasPrefix(safeName, "..") || filepath.IsAbs(safeName) {
		return "", fmt.Errorf("refusing to extract unsafe tar member %q", hdr.Name)
	}

	targetPath := filepath.Join(destDir, safeName)

	// Ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory for %s: %w", targetPath, err)
	}

	// Create the file
	outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", targetPath, err)
	}

	// Limit individual file size to 10GB
	const maxFileSize = 10 * 1024 * 1024 * 1024

	limitedReader := io.LimitReader(tr, maxFileSize)

	buf := make([]byte, 1024*1024)

	if _, err := io.CopyBuffer(outFile, limitedReader, buf); err != nil {
		_ = outFile.Close()
		return "", fmt.Errorf("failed to extract %s: %w", hdr.Name, err)
	}

	if err := outFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close extracted file %s: %w", targetPath, err)
	}

	return targetPath, nil
}

// EnsureDecompressed returns a path the byte-offset indexer can read.
//
// The index is a table of pread offsets, which is meaningless over a
// compressed stream — so a gzip file is transparently decompressed into a temp
// file and THAT path is returned. If the gzip file contains a tar archive,
// the archive is extracted and the path to the primary log file is returned.
// Non-gzip files pass through unchanged with a no-op cleanup. The caller owns
// cleanup and must invoke it when the indexed view is closed; it removes the
// temp file only for decompressed inputs.
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

	// Open the gzip file
	src, err := os.Open(path) //nolint:gosec // user-provided path is intended
	if err != nil {
		return "", nil, fmt.Errorf("failed to open %s: %w", path, err)
	}

	defer func() { _ = src.Close() }()

	zr, err := gzip.NewReader(src)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read gzip header of %s: %w", path, err)
	}

	// Check if this is a tar.gz by extension or by trying to read as tar
	isTar := isLikelyTarByExtension(path)

	if isTar {
		// Extract the primary log file to a temp directory
		tmpDir, err := os.MkdirTemp("", "gofle-tar-*")
		if err != nil {
			return "", nil, fmt.Errorf("failed to create temp directory for tar extraction: %w", err)
		}

		logFile, err := extractPrimaryTarLog(zr, path, tmpDir)
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", nil, err
		}

		return logFile, func() { _ = os.RemoveAll(tmpDir) }, nil
	}

	// Regular gzip file (not tar) - decompress to a single temp file
	tmp, err := os.CreateTemp("", "gofle-gz-*.log")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file for decompression: %w", err)
	}

	// Limit decompression to 10GB to prevent zip bombs
	const maxDecompressedSize = 10 * 1024 * 1024 * 1024

	limitedReader := io.LimitReader(zr, maxDecompressedSize)

	buf := make([]byte, 1024*1024)

	if _, err := io.CopyBuffer(tmp, limitedReader, buf); err != nil {
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

// extractPrimaryTarLog picks and extracts the ONE log file to index from a
// tar.gz stream (G1). The old shape extracted EVERY regular member to disk and
// only then picked the best candidate — paying disk writes for members it
// would never read.
//
// The walk is single-inflate when it can be: under the same preference rule
// as before (first .log, else largest, else first), the first .log member is
// an unconditional winner, so it is extracted on the spot and the rest of the
// archive is never inflated. Only when no .log exists does the largest rule
// need all header sizes: the walk collects them (payloads discarded), and the
// chosen member is extracted from a fresh pass over the archive file — one
// extra inflate, still exactly one member written to disk.
func extractPrimaryTarLog(zr *gzip.Reader, srcPath, destDir string) (string, error) {
	tr := tar.NewReader(zr)

	var entries []tarHeaderEntry

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Only regular files
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		safeName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(safeName, "..") || filepath.IsAbs(safeName) {
			continue // Skip potentially dangerous paths (payload is skipped)
		}

		if strings.HasSuffix(strings.ToLower(safeName), ".log") {
			return extractTarFile(tr, hdr, destDir)
		}

		entries = append(entries, tarHeaderEntry{name: safeName, size: hdr.Size})
	}

	best := selectTarLog(entries)
	if best == nil {
		return "", errors.New("no regular files found in tar archive")
	}

	// Re-open and re-inflate the archive; extract only the chosen member.
	src, err := os.Open(srcPath) //nolint:gosec // user-provided path is intended
	if err != nil {
		return "", fmt.Errorf("failed to reopen %s for extraction: %w", srcPath, err)
	}
	defer func() { _ = src.Close() }()

	zr2, err := gzip.NewReader(src)
	if err != nil {
		return "", fmt.Errorf("failed to re-read gzip header of %s: %w", srcPath, err)
	}

	tr2 := tar.NewReader(zr2)

	for {
		hdr, err := tr2.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("tar member %q vanished between passes", best.name)
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar entry: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		safeName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(safeName, "..") || filepath.IsAbs(safeName) {
			continue
		}

		if safeName == best.name {
			return extractTarFile(tr2, hdr, destDir)
		}
	}
}
