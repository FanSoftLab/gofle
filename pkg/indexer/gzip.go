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

// findLogFile selects the best log file candidate from a list of extracted files.
// Preference order: .log extension, largest file, first file.
func findLogFile(files []string) (string, error) {
	if len(files) == 0 {
		return "", errors.New("no regular files found in tar archive")
	}

	// First pass: look for .log files
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".log") {
			return f, nil
		}
	}

	// Second pass: find largest file
	var largestFile string
	var largestSize int64 = -1

	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.Size() > largestSize {
			largestSize = info.Size()
			largestFile = f
		}
	}

	if largestFile != "" {
		return largestFile, nil
	}

	// Fallback: return first file
	return files[0], nil
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
		// Extract tar.gz to a temp directory
		tmpDir, err := os.MkdirTemp("", "gofle-tar-*")
		if err != nil {
			return "", nil, fmt.Errorf("failed to create temp directory for tar extraction: %w", err)
		}

		// Decompress gzip and extract tar in one pass
		files, err := extractTarFromGzip(zr, tmpDir)
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", nil, fmt.Errorf("failed to extract tar.gz %s: %w", path, err)
		}

		logFile, err := findLogFile(files)
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

// extractTarFromGzip reads from a gzip reader and extracts the tar archive
// to destDir. Returns list of extracted regular file paths.
func extractTarFromGzip(zr *gzip.Reader, destDir string) ([]string, error) {
	tr := tar.NewReader(zr)

	var extractedFiles []string

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Only extract regular files
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// Sanitize the file name to prevent path traversal
		safeName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(safeName, "..") || filepath.IsAbs(safeName) {
			continue // Skip potentially dangerous paths
		}

		targetPath := filepath.Join(destDir, safeName)

		// Ensure target directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create directory for %s: %w", targetPath, err)
		}

		// Create the file
		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", targetPath, err)
		}

		// Limit individual file size to 10GB
		const maxFileSize = 10 * 1024 * 1024 * 1024

		limitedReader := io.LimitReader(tr, maxFileSize)

		buf := make([]byte, 1024*1024)

		if _, err := io.CopyBuffer(outFile, limitedReader, buf); err != nil {
			_ = outFile.Close()
			return nil, fmt.Errorf("failed to extract %s: %w", hdr.Name, err)
		}

		if err := outFile.Close(); err != nil {
			return nil, fmt.Errorf("failed to close extracted file %s: %w", targetPath, err)
		}

		extractedFiles = append(extractedFiles, targetPath)
	}

	return extractedFiles, nil
}
