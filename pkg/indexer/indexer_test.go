package indexer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

// errReaderFailure is the sentinel a test reader fails with to exercise the
// index scan's error return path.
var errReaderFailure = errors.New("synthetic read failure")

func writeFile(t *testing.T, content string) (*FileIndexer, string) {
	t.Helper()
	dir := t.TempDir()

	p := filepath.Join(dir, "test.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	idx, err := NewFileIndexer(p)
	if err != nil {
		t.Fatalf("failed to create indexer: %v", err)
	}

	return idx, p
}

func TestIndexerBasic(t *testing.T) {
	idx, _ := writeFile(t, "line1\nline2\nline3\n")
	if idx.TotalLines() != 3 {
		t.Fatalf("expected 3 lines, got %d", idx.TotalLines())
	}

	lines, err := idx.GetLines(0, 3)
	if err != nil {
		t.Fatalf("GetLines error: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("expected 3 returned lines, got %d", len(lines))
	}

	if lines[0].Content != "line1" || lines[0].Index != 0 {
		t.Fatalf("unexpected first line: %+v", lines[0])
	}

	if lines[2].Content != "line3" || lines[2].Index != 2 {
		t.Fatalf("unexpected third line: %+v", lines[2])
	}
}

func TestIndexerCRLFTrimming(t *testing.T) {
	idx, _ := writeFile(t, "a\r\nb\r\nc\r\n")
	if idx.TotalLines() != 3 {
		t.Fatalf("expected 3 lines, got %d", idx.TotalLines())
	}

	lines, _ := idx.GetLines(1, 1)
	if lines[0].Content != "b" {
		t.Fatalf("expected CRLF trimmed to 'b', got %q", lines[0].Content)
	}
}

func TestIndexerUpdateAndAppend(t *testing.T) { //nolint:gocyclo // table-driven search test is inherently branchy
	idx, p := writeFile(t, "a\nb\n")

	added, err := idx.UpdateIndex()
	if err != nil {
		t.Fatalf("UpdateIndex error: %v", err)
	}

	if added != 0 {
		t.Fatalf("expected 0 added for unchanged file, got %d", added)
	}

	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}

	if _, err := f.WriteString("c\nd\ne\n"); err != nil {
		t.Fatalf("append: %v", err)
	}

	f.Close()

	added, err = idx.UpdateIndex()
	if err != nil {
		t.Fatalf("UpdateIndex error after append: %v", err)
	}

	if added != 3 {
		t.Fatalf("expected 3 added lines, got %d", added)
	}

	if idx.TotalLines() != 5 {
		t.Fatalf("expected 5 total lines, got %d", idx.TotalLines())
	}

	lines, err := idx.GetLines(2, 3)
	if err != nil {
		t.Fatalf("GetLines error: %v", err)
	}

	if len(lines) != 3 || lines[0].Content != "c" || lines[2].Content != "e" {
		t.Fatalf("unexpected appended lines: %+v", lines)
	}

	g, err := idx.GetLine(4)
	if err != nil {
		t.Fatalf("GetLine error: %v", err)
	}

	if g != "e" {
		t.Fatalf("expected GetLine(4)='e', got %q", g)
	}
}

func TestIndexerOutOfBounds(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")
	if _, err := idx.GetLine(99); err == nil {
		t.Fatal("expected error for out-of-bounds line")
	}

	lines, err := idx.GetLines(-1, 2)
	if err != nil {
		t.Fatalf("GetLines error: %v", err)
	}

	if len(lines) != 0 {
		t.Fatalf("expected empty result for negative start, got %d", len(lines))
	}
}

func TestIndexerEmptyFile(t *testing.T) {
	idx, _ := writeFile(t, "")
	if idx.TotalLines() != 0 {
		t.Fatalf("expected 0 lines for empty file, got %d", idx.TotalLines())
	}

	if idx.FileSize() != 0 {
		t.Fatalf("expected 0 file size, got %d", idx.FileSize())
	}

	lines, _ := idx.GetLines(0, 10)
	if len(lines) != 0 {
		t.Fatalf("expected empty lines for empty file, got %d", len(lines))
	}
}

func TestIndexerSingleLineNoTrailingNewline(t *testing.T) {
	idx, _ := writeFile(t, "single line without newline")
	// Current behavior: indexer only indexes lines ending with \n
	// So a file without trailing newline shows 0 lines
	if idx.TotalLines() != 0 {
		t.Fatalf("expected 0 lines for file without trailing newline, got %d", idx.TotalLines())
	}
	// But GetLine(0) should fail
	if _, err := idx.GetLine(0); err == nil {
		t.Fatal("expected error for GetLine(0) on file without trailing newline")
	}
}

func TestIndexerSingleLineWithNewline(t *testing.T) {
	idx, _ := writeFile(t, "single line\n")
	if idx.TotalLines() != 1 {
		t.Fatalf("expected 1 line, got %d", idx.TotalLines())
	}

	line, err := idx.GetLine(0)
	if err != nil {
		t.Fatalf("GetLine error: %v", err)
	}

	if line != "single line" {
		t.Fatalf("expected 'single line', got %q", line)
	}
}

func TestIndexerFileTruncation(t *testing.T) {
	idx, p := writeFile(t, "a\nb\nc\nd\ne\n")

	added, err := idx.UpdateIndex()
	if err != nil {
		t.Fatalf("UpdateIndex error: %v", err)
	}

	if added != 0 {
		t.Fatalf("expected 0 added for unchanged file, got %d", added)
	}

	f, err := os.OpenFile(p, os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for truncate: %v", err)
	}

	if _, err := f.WriteString("x\ny\n"); err != nil {
		t.Fatalf("write truncated: %v", err)
	}

	f.Close()

	// H1: a truncated/rotated file is detected by the size decrease and the
	// index is rebuilt from scratch, so the new content (not stale lines) is
	// reported and TotalLines reflects the rebuilt file.
	added, err = idx.UpdateIndex()
	if err != nil {
		t.Fatalf("UpdateIndex after truncation error: %v", err)
	}

	if added != 2 {
		t.Fatalf("expected 2 lines after rebuild, got %d", added)
	}

	if idx.TotalLines() != 2 {
		t.Fatalf("expected 2 total lines after rebuild, got %d", idx.TotalLines())
	}
}

func TestIndexerScanLines(t *testing.T) {
	idx, _ := writeFile(t, "line1\nline2\nline3\nline4\nline5\n")

	var results []struct {
		idx  int64
		line string
	}

	err := idx.ScanLines(0, 5, func(i int64, line []byte) bool {
		results = append(results, struct {
			idx  int64
			line string
		}{i, string(line)})

		return true
	})
	if err != nil {
		t.Fatalf("ScanLines error: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 scanned lines, got %d", len(results))
	}

	for i, r := range results {
		expected := fmt.Sprintf("line%d", i+1)
		if r.line != expected || r.idx != int64(i) {
			t.Fatalf("line %d: got %q idx %d, want %q idx %d", i, r.line, r.idx, expected, i)
		}
	}
}

func TestIndexerScanLinesEmptyRange(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")

	var count int

	err := idx.ScanLines(0, 0, func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if err != nil {
		t.Fatalf("ScanLines error: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 calls for empty range, got %d", count)
	}
}

func TestIndexerScanLinesEarlyTermination(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\nd\ne\n")

	var count int

	err := idx.ScanLines(0, 5, func(i int64, _ []byte) bool {
		count++
		return i < 2
	})
	if err != nil {
		t.Fatalf("ScanLines error: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 calls before early termination, got %d", count)
	}
}

func TestIndexerScanLinesOutOfBounds(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")

	var count int

	err := idx.ScanLines(10, 5, func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if err != nil {
		t.Fatalf("ScanLines error: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 calls for out-of-bounds range, got %d", count)
	}
}

func TestIndexerCloseIdempotent(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")
	if err := idx.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	if err := idx.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}

	if err := idx.Close(); err != nil {
		t.Fatalf("third Close error: %v", err)
	}
}

func TestIndexerOperationsAfterClose(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")
	_ = idx.Close()

	if _, err := idx.GetLine(0); err == nil {
		t.Fatal("expected error for GetLine after close")
	}

	if _, err := idx.GetLines(0, 1); err == nil {
		t.Fatal("expected error for GetLines after close")
	}

	if _, err := idx.UpdateIndex(); err == nil {
		t.Fatal("expected error for UpdateIndex after close")
	}
	// TotalLines and FileSize return cached values even after close
	if idx.TotalLines() != 3 {
		t.Fatalf("expected TotalLines=3 (cached) after close, got %d", idx.TotalLines())
	}

	if idx.FileSize() == 0 {
		t.Fatalf("expected FileSize>0 (cached) after close, got %d", idx.FileSize())
	}
}

func TestIndexerConcurrentAccess(t *testing.T) {
	idx, _ := writeFile(t, "line1\nline2\nline3\nline4\nline5\n")

	var wg sync.WaitGroup

	errChan := make(chan error, 100)

	for range 10 {
		wg.Go(func() {
			for j := range 100 {
				lineIdx := int64(j % 5)
				if _, err := idx.GetLine(lineIdx); err != nil {
					errChan <- fmt.Errorf("GetLine: %w", err)
					return
				}

				if _, err := idx.GetLines(lineIdx, 1); err != nil {
					errChan <- fmt.Errorf("GetLines: %w", err)
					return
				}
			}
		})
	}

	for range 5 {
		wg.Go(func() {
			for range 50 {
				if _, err := idx.UpdateIndex(); err != nil {
					errChan <- fmt.Errorf("UpdateIndex: %w", err)
					return
				}
			}
		})
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestIndexerLargeFilePreallocation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "large.log")

	lines := int64(200000)

	var buf strings.Builder
	for i := range lines {
		fmt.Fprintf(&buf, "line %06d with some content here\n", i)
	}

	content := buf.String()

	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	idx, err := NewFileIndexer(p)
	if err != nil {
		t.Fatalf("create indexer for large file: %v", err)
	}

	if idx.TotalLines() != lines {
		t.Fatalf("expected %d lines, got %d", lines, idx.TotalLines())
	}

	line, err := idx.GetLine(lines - 1)
	if err != nil {
		t.Fatalf("GetLine last: %v", err)
	}

	expected := fmt.Sprintf("line %06d with some content here", lines-1)
	if line != expected {
		t.Fatalf("last line mismatch: got %q, want %q", line, expected)
	}

	lines2, err := idx.GetLines(lines-10, 10)
	if err != nil {
		t.Fatalf("GetLines last 10: %v", err)
	}

	if len(lines2) != 10 {
		t.Fatalf("expected 10 lines, got %d", len(lines2))
	}
}

func TestIndexerPoolBehavior(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")

	for i := range 1000 {
		_, err := idx.GetLine(int64(i % 3))
		if err != nil {
			t.Fatalf("GetLine error: %v", err)
		}
	}

	_, err := idx.GetLines(0, 3)
	if err != nil {
		t.Fatalf("GetLines error: %v", err)
	}

	var count int

	err = idx.ScanLines(0, 3, func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if err != nil {
		t.Fatalf("ScanLines error: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 ScanLines calls, got %d", count)
	}
}

func TestIndexerGetLinesEdgeCases(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")

	lines, err := idx.GetLines(0, 0)
	if err != nil {
		t.Fatalf("GetLines(0,0) error: %v", err)
	}

	if len(lines) != 0 {
		t.Fatalf("expected 0 lines for count=0, got %d", len(lines))
	}

	lines, err = idx.GetLines(2, 10)
	if err != nil {
		t.Fatalf("GetLines(2,10) error: %v", err)
	}

	if len(lines) != 1 || lines[0].Content != "c" {
		t.Fatalf("expected 1 line for count > remaining, got %d: %+v", len(lines), lines)
	}

	lines, err = idx.GetLines(3, 5)
	if err != nil {
		t.Fatalf("GetLines(3,5) error: %v", err)
	}

	if len(lines) != 0 {
		t.Fatalf("expected 0 lines for start >= total, got %d", len(lines))
	}

	lines, err = idx.GetLines(-1, 5)
	if err != nil {
		t.Fatalf("GetLines(-1,5) error: %v", err)
	}

	if len(lines) != 0 {
		t.Fatalf("expected 0 lines for negative start, got %d", len(lines))
	}
}

// TestIndexerUpdateIndexRotationRebuild verifies H1: when the watched file is
// rotated/truncated (size decreases), UpdateIndex rebuilds the index from
// scratch so stale lines are dropped and the new (smaller) content is indexed.
func TestIndexerUpdateIndexRotationRebuild(t *testing.T) { //nolint:gocyclo // table-driven search test is inherently branchy
	idx, p := writeFile(t, "old-line-1\nold-line-2\nold-line-3\n")

	if added, err := idx.UpdateIndex(); err != nil || added != 0 {
		t.Fatalf("initial UpdateIndex: added=%d err=%v", added, err)
	}

	if idx.TotalLines() != 3 {
		t.Fatalf("expected 3 lines initially, got %d", idx.TotalLines())
	}

	// Simulate a logrotate with copytruncate: the same file is truncated and
	// brand-new content written in its place.
	f, err := os.OpenFile(p, os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for truncate: %v", err)
	}

	newContent := "new-line-A\nnew-line-B\n"
	if _, err := f.WriteString(newContent); err != nil {
		t.Fatalf("write rotated content: %v", err)
	}

	f.Close()

	added, err := idx.UpdateIndex()
	if err != nil {
		t.Fatalf("UpdateIndex after rotation error: %v", err)
	}

	if added != 2 {
		t.Fatalf("expected 2 rebuilt lines, got %d", added)
	}

	if idx.TotalLines() != 2 {
		t.Fatalf("expected 2 total lines after rebuild, got %d", idx.TotalLines())
	}

	// The stale "old-line-*" content must be gone.
	lines, err := idx.GetLines(0, 2)
	if err != nil {
		t.Fatalf("GetLines: %v", err)
	}

	for _, l := range lines {
		if strings.HasPrefix(l.Content, "old-line") {
			t.Fatalf("stale line survived rotation: %q", l.Content)
		}
	}

	if lines[0].Content != "new-line-A" || lines[1].Content != "new-line-B" {
		t.Fatalf("unexpected rebuilt content: %v", lines)
	}
}

// TestUpdateIndexRotationRebuildDoesNotBlockReaders pins the M3 contract: the
// rotation rebuild re-scans the file WITHOUT the write lock and only swaps the
// finished table in under a short lock. Rebuilding a large file under the write
// lock froze every reader (viewport paging, an in-flight search, an export) for
// the whole multi-second pass.
//
// The observable proof is the invalidated table: the rebuild publishes "no lines
// yet" before it starts scanning, so a reader polling concurrently must be able
// to see TotalLines()==0 while UpdateIndex is still running. With the scan under
// the write lock, a reader could only ever see the old count or the new one —
// never the intermediate state, because it could not run at all.
func TestUpdateIndexRotationRebuildDoesNotBlockReaders(t *testing.T) { //nolint:gocyclo // table-driven search test is inherently branchy
	// Big enough that re-scanning it takes long enough for the reader loop below
	// to sample the unlocked window.
	const nLines = 200_000

	idx, p := bigIndexer(t, nLines, 100)
	if idx.TotalLines() != nLines {
		t.Fatalf("setup: TotalLines=%d want %d", idx.TotalLines(), nLines)
	}

	// Shrink in place: the file stays large (so the rebuild has real work) but
	// UpdateIndex sees a truncation and must rebuild from scratch.
	if err := os.Truncate(p, idx.FileSize()-4096); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	updateDone := make(chan error, 1)
	readerStop := make(chan struct{})
	sawEmpty := make(chan struct{})

	var (
		readErr error
		once    sync.Once
	)

	go func() {
		for {
			select {
			case <-readerStop:
				return
			default:
			}

			if idx.TotalLines() == 0 {
				// The rebuild window must serve reads (empty), not fail them:
				// stale offsets are gone, so nothing can be read short.
				if lines, err := idx.GetLines(0, 10); err != nil {
					readErr = err
				} else if len(lines) != 0 {
					readErr = fmt.Errorf("GetLines during rebuild returned %d lines, want 0", len(lines))
				}

				once.Do(func() { close(sawEmpty) })
			}
		}
	}()

	go func() {
		_, err := idx.UpdateIndex()
		updateDone <- err
	}()

	select {
	case <-sawEmpty:
	case err := <-updateDone:
		close(readerStop)

		if err != nil {
			t.Fatalf("UpdateIndex: %v", err)
		}

		t.Fatal("rebuild completed without a reader ever observing the unlocked window")
	case <-time.After(30 * time.Second):
		close(readerStop)
		t.Fatal("timed out waiting for the rebuild")
	}

	if err := <-updateDone; err != nil {
		close(readerStop)
		t.Fatalf("UpdateIndex: %v", err)
	}

	close(readerStop)

	if readErr != nil {
		t.Fatalf("reader failed during the rebuild: %v", readErr)
	}

	// The swapped-in table must be complete and readable. The truncation cut
	// 4096 bytes off the tail, and a final line without a newline is not indexed.
	if got := idx.TotalLines(); got == 0 || got >= nLines {
		t.Fatalf("TotalLines after rebuild = %d, want 0 < n < %d", got, nLines)
	}

	first, err := idx.GetLine(0)
	if err != nil || !strings.HasPrefix(first, "00000000 ") {
		t.Fatalf("GetLine(0) after rebuild = %q err=%v", first, err)
	}

	last, err := idx.GetLine(idx.TotalLines() - 1)
	if err != nil {
		t.Fatalf("GetLine(last) after rebuild: %v", err)
	}

	if len(last) != len(first) {
		t.Fatalf("last rebuilt line is truncated: %q", last)
	}
}

// TestGenerationTracksRebuilds pins Generation(): it is bumped once per
// offset-table rebuild and left alone by growth and no-op updates. Callers that
// cache per-line derivations (matched lines, level stats) key on it to know when
// every line index they hold has started denoting different content.
func TestGenerationTracksRebuilds(t *testing.T) { //nolint:gocyclo // table-driven search test is inherently branchy
	idx, p := writeFile(t, "a\nb\n")
	t.Cleanup(func() { _ = idx.Close() })

	gen0 := idx.Generation()

	if _, err := idx.UpdateIndex(); err != nil {
		t.Fatalf("no-change UpdateIndex: %v", err)
	}

	if idx.Generation() != gen0 {
		t.Fatalf("Generation changed on a no-change update: %d -> %d", gen0, idx.Generation())
	}

	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}

	if _, err := f.WriteString("c\n"); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if added, err := idx.UpdateIndex(); err != nil || added != 1 {
		t.Fatalf("growth UpdateIndex: added=%d err=%v", added, err)
	}

	if idx.Generation() != gen0 {
		t.Fatalf("Generation changed on growth: %d -> %d", gen0, idx.Generation())
	}

	if err := os.WriteFile(p, []byte("z\n"), 0o644); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	if _, err := idx.UpdateIndex(); err != nil {
		t.Fatalf("rotation UpdateIndex: %v", err)
	}

	if idx.Generation() != gen0+1 {
		t.Fatalf("Generation after rotation = %d, want %d", idx.Generation(), gen0+1)
	}

	if line, err := idx.GetLine(0); err != nil || line != "z" {
		t.Fatalf("GetLine(0) after rotation = %q err=%v", line, err)
	}
}

// TestUpdateIndexRetriesAfterFailedRebuild covers the recovery path of the
// off-lock rebuild: if the scan fails after the table was invalidated, fileSize
// already matches the rotated file, so the size comparison alone would never come
// back here and the file would stay stuck at zero lines forever. rebuildPending
// forces the next update to retry.
func TestUpdateIndexRetriesAfterFailedRebuild(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")
	t.Cleanup(func() { _ = idx.Close() })

	// Simulate the state a rebuild whose scan failed leaves behind: invalidated
	// table, fileSize already equal to the file on disk, retry flag set.
	idx.mu.Lock()
	idx.offsets = nil
	idx.rebuildPending = true
	idx.mu.Unlock()

	if got := idx.TotalLines(); got != 0 {
		t.Fatalf("setup: TotalLines=%d, want 0 for an invalidated table", got)
	}

	added, err := idx.UpdateIndex()
	if err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	if added != 3 || idx.TotalLines() != 3 {
		t.Fatalf("retry rebuilt %d lines (total %d), want 3", added, idx.TotalLines())
	}

	idx.mu.RLock()
	pending := idx.rebuildPending
	idx.mu.RUnlock()

	if pending {
		t.Fatal("rebuildPending still set after a successful retry")
	}

	if line, err := idx.GetLine(2); err != nil || line != "c" {
		t.Fatalf("GetLine(2) after retry = %q err=%v", line, err)
	}
	// A subsequent no-change update must not rebuild again.
	if added, err := idx.UpdateIndex(); err != nil || added != 0 {
		t.Fatalf("no-change UpdateIndex after retry: added=%d err=%v", added, err)
	}
}

func TestIndexerGetLineBytes(t *testing.T) {
	idx, _ := writeFile(t, "alpha\r\nbeta\n")

	b, err := idx.GetLineBytes(0)
	if err != nil || string(b) != "alpha" {
		t.Fatalf("GetLineBytes(0)=%q err=%v", b, err)
	}

	b, err = idx.GetLineBytes(1)
	if err != nil || string(b) != "beta" {
		t.Fatalf("GetLineBytes(1)=%q err=%v", b, err)
	}

	// GetLineBytes must return a caller-owned copy: mutating it must not affect
	// a subsequent GetLine.
	b[0] = 'Z'

	line, err := idx.GetLine(0)
	if err != nil || line != "alpha" {
		t.Fatalf("GetLine(0)=%q err=%v (GetLineBytes result was not isolated)", line, err)
	}
}

func TestIndexerTakeRebuiltFlag(t *testing.T) {
	idx, p := writeFile(t, "a\nb\nc\n")

	if idx.TakeRebuiltFlag() {
		t.Fatal("expected false before any UpdateIndex")
	}

	if _, err := idx.UpdateIndex(); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	if idx.TakeRebuiltFlag() {
		t.Fatal("expected false after a no-change update")
	}

	// Shrink the file (rotation/truncation): UpdateIndex must rebuild and raise
	// the flag exactly once.
	if err := os.WriteFile(p, []byte("x\ny\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	if _, err := idx.UpdateIndex(); err != nil {
		t.Fatalf("UpdateIndex after truncate: %v", err)
	}

	if !idx.TakeRebuiltFlag() {
		t.Fatal("expected true after rotation/truncation")
	}
	// The flag is consumed by the read above.
	if idx.TakeRebuiltFlag() {
		t.Fatal("expected false after the flag was consumed")
	}
}

// bigIndexer writes a file with nLines lines of ~lineLen bytes each — large
// enough that a full scan is split into many maxScanReadSize spans — and returns
// an indexer over it plus the path.
func bigIndexer(t *testing.T, nLines, lineLen int) (*FileIndexer, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "big.log")

	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	filler := strings.Repeat("x", lineLen)

	w := make([]byte, 0, 64*1024)
	for i := range nLines {
		w = append(w, fmt.Sprintf("%08d %s\n", i, filler)...)
		if len(w) >= 32*1024 {
			if _, err := f.Write(w); err != nil {
				t.Fatalf("write: %v", err)
			}

			w = w[:0]
		}
	}

	if _, err := f.Write(w); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	idx, err := NewFileIndexer(p)
	if err != nil {
		t.Fatalf("indexer: %v", err)
	}

	t.Cleanup(func() { _ = idx.Close() })

	return idx, p
}

// TestScanLinesDoesNotBlockUpdateIndex verifies that a long full-file scan
// (search / export / level stats) no longer starves the tail poller: ScanLines
// releases fi.mu between read spans, so UpdateIndex — which needs the write lock
// — gets in while the scan is still running. Holding the read lock for the whole
// pass froze live tailing for the entire duration of a search.
func TestScanLinesDoesNotBlockUpdateIndex(t *testing.T) {
	const nLines = 60_000

	idx, p := bigIndexer(t, nLines, 100)
	total := idx.TotalLines()

	// Slow the scan down inside the span callback (i.e. while the read lock is
	// held) so the whole pass takes far longer than a single span.
	inScan := make(chan struct{})

	var once sync.Once

	scanErr := make(chan error, 1)

	go func() {
		scanErr <- idx.ScanLines(0, total, func(i int64, _ []byte) bool {
			once.Do(func() { close(inScan) })

			if i%5_000 == 0 {
				time.Sleep(15 * time.Millisecond)
			}

			return true
		})
	}()

	<-inScan // the scan is in flight and has taken the read lock at least once

	// Append new content so UpdateIndex has real work to do, then time it.
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("append open: %v", err)
	}

	if _, err := f.WriteString("appended-while-scanning\n"); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("append close: %v", err)
	}

	start := time.Now()
	added, err := idx.UpdateIndex()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("UpdateIndex during scan: %v", err)
	}

	if added != 1 {
		t.Errorf("UpdateIndex added = %d, want 1", added)
	}

	// The scan must still be running: that is what makes this a contention test
	// rather than a measurement of an already-finished scan. A scan that is
	// already done here is the old behaviour — UpdateIndex waited for the whole
	// pass, so it could only return once the scan had released the read lock.
	select {
	case err := <-scanErr:
		t.Fatalf("scan already finished when UpdateIndex returned (err=%v): the writer "+
			"waited for the whole scan instead of a single span", err)
	default:
	}
	// One span (<= maxScanReadSize) is the new worst-case wait. The budget is
	// deliberately loose for slow/loaded CI machines, but still far below the
	// multi-second whole-scan wait the old code imposed.
	if elapsed > 2*time.Second {
		t.Errorf("UpdateIndex blocked %v during a scan, want a per-span wait", elapsed)
	}

	if err := <-scanErr; err != nil {
		t.Fatalf("ScanLines: %v", err)
	}
}

// TestScanLinesReportsRotationMidScan verifies the flip side of releasing the
// lock between spans: if the file is rotated (truncated) while a scan is in
// flight, the offset table is rebuilt underneath it and every line index it is
// walking becomes meaningless. The scan must fail with ErrIndexRebuilt instead
// of quietly returning content from unrelated lines.
func TestScanLinesReportsRotationMidScan(t *testing.T) {
	const nLines = 60_000

	idx, p := bigIndexer(t, nLines, 100)
	total := idx.TotalLines()

	inScan := make(chan struct{})

	var once sync.Once

	scanErr := make(chan error, 1)

	go func() {
		scanErr <- idx.ScanLines(0, total, func(i int64, _ []byte) bool {
			once.Do(func() { close(inScan) })

			if i%5_000 == 0 {
				time.Sleep(15 * time.Millisecond)
			}

			return true
		})
	}()

	<-inScan

	// Rotate: shrink the file, then let the poller rebuild the index.
	if err := os.WriteFile(p, []byte("rotated\n"), 0o644); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)

	for {
		if _, err := idx.UpdateIndex(); err != nil {
			t.Fatalf("UpdateIndex after rotation: %v", err)
		}

		if idx.TotalLines() == 1 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("index was never rebuilt after rotation")
		}

		time.Sleep(5 * time.Millisecond)
	}

	err := <-scanErr
	if !errors.Is(err, ErrIndexRebuilt) {
		t.Fatalf("ScanLines error = %v, want ErrIndexRebuilt", err)
	}
}

// TestScanLinesAfterCloseReturnsErrClosed pins the closed-indexer contract: the
// per-span lock acquisition checks isClosed, so a scan on a closed indexer fails
// fast with os.ErrClosed instead of surfacing a raw file-descriptor error.
func TestScanLinesAfterCloseReturnsErrClosed(t *testing.T) {
	idx, _ := writeFile(t, "a\nb\nc\n")
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err := idx.ScanLines(0, 3, func(int64, []byte) bool { return true })
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("ScanLines after Close = %v, want os.ErrClosed", err)
	}
}

// The index scan pass takes its 64 KiB bufio.Reader from a package pool (C1).
// Back-to-back scans over different readers must produce identical tables —
// the second scan reuses the recycled buffer via Reset — and the reader must
// also survive the error return path, since the deferred Put covers every
// return.
func TestAppendLineOffsetsWithLevelsReusesPooledReader(t *testing.T) {
	const content = "ERROR one\nWARN two\nplain three\n"

	scan := func(r io.Reader) ([]int64, []LevelCount, error) {
		return appendLineOffsetsWithLevels(r, []int64{0}, []LevelCount{{LineIndex: 0}}, 0, int64(len(content)), "test.log")
	}

	offsets1, levelCounts1, err := scan(strings.NewReader(content))
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}

	// The first call returned its reader to the pool; the second one must Reset
	// it and still index the fresh stream identically.
	offsets2, levelCounts2, err := scan(strings.NewReader(content))
	if err != nil {
		t.Fatalf("second scan (recycled reader): %v", err)
	}

	if !slices.Equal(offsets1, offsets2) {
		t.Fatalf("offsets differ between scans: %v vs %v", offsets1, offsets2)
	}

	if !slices.Equal(levelCounts1, levelCounts2) {
		t.Fatalf("levelCounts differ between scans: %+v vs %+v", levelCounts1, levelCounts2)
	}

	if len(offsets1) != 4 {
		t.Fatalf("expected 4 offsets (initial + 3 lines), got %d", len(offsets1))
	}

	// A failing reader exercises the error return path; a subsequent scan must
	// still work, proving the reader came back from the pool intact.
	_, _, err = scan(iotest.ErrReader(errReaderFailure))
	if !errors.Is(err, errReaderFailure) {
		t.Fatalf("error reader scan = %v, want errReaderFailure", err)
	}

	offsets3, _, err := scan(strings.NewReader(content))
	if err != nil {
		t.Fatalf("third scan after error path: %v", err)
	}

	if !slices.Equal(offsets1, offsets3) {
		t.Fatalf("offsets differ after the error path recycled the reader: %v vs %v", offsets1, offsets3)
	}
}

// GetLevelCounts hands out the internal table directly (C2): the snapshot must
// be readable, stable across calls, and extend after a tail append without any
// defensive copy.
func TestGetLevelCountsSnapshot(t *testing.T) {
	var sb strings.Builder

	// An entry is published every levelCountInterval lines, covering the lines
	// counted so far: the snapshot at LineIndex 1000 covers lines 0-999.
	for i := range 1500 {
		switch i {
		case 0:
			sb.WriteString("ERROR one\n")
		case 999:
			sb.WriteString("WARN two\n")
		default:
			sb.WriteString("INFO filler\n")
		}
	}

	idx, _ := writeFile(t, sb.String())

	counts := idx.GetLevelCounts()
	if len(counts) != 2 {
		t.Fatalf("expected 2 level count entries (lines 0 and 1000), got %d", len(counts))
	}

	if counts[0].LineIndex != 0 || counts[0].Errors != 0 || counts[0].Warns != 0 {
		t.Fatalf("first entry = %+v, want {0 0 0}", counts[0])
	}

	if counts[1].LineIndex != 1000 || counts[1].Errors != 1 || counts[1].Warns != 1 {
		t.Fatalf("second entry = %+v, want LineIndex 1000 with 1 error and 1 warn", counts[1])
	}

	// A repeat call must observe identical content.
	again := idx.GetLevelCounts()
	if len(again) != len(counts) {
		t.Fatalf("repeat call returned %d entries, want %d", len(again), len(counts))
	}

	for i := range counts {
		if again[i] != counts[i] {
			t.Fatalf("repeat call entry %d = %+v, want %+v", i, again[i], counts[i])
		}
	}
}
