// Package indexer provides fast, memory-efficient log file indexing and random access.
//
// FileIndexer builds a byte-offset index of a log file on open, enabling O(1) random
// access to any line without loading the entire file into memory. It supports
// incremental tailing (detecting appended lines), line scanning for search, and
// concurrent access from multiple goroutines.
package indexer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Sentinel read-guard errors. Dynamic detail (line index, byte length) is
// attached via %w wrapping so callers can classify failures with errors.Is
// without parsing message text.
var (
	// ErrLineOutOfRange reports a line index outside the indexed range.
	ErrLineOutOfRange = errors.New("line index out of bounds")
	// ErrCorruptOffsetTable reports an inconsistent in-memory offset table
	// (negative spans), e.g. after an unexpected concurrent mutation.
	ErrCorruptOffsetTable = errors.New("corrupt offset table")
	// ErrLineTooLarge reports content spanning more bytes than addressable in
	// one read buffer.
	ErrLineTooLarge = errors.New("line too large to read")
	// ErrIndexRebuilt reports that the offset table was rebuilt (the file was
	// rotated or truncated) while a long ScanLines pass was in flight, so the
	// line indices it was walking no longer denote the same content. The caller
	// must restart the operation against the refreshed index.
	ErrIndexRebuilt = errors.New("index rebuilt during scan")
)

// lineBufPool reuses the per-line read buffers used by the single-line read
// core (readLineUnlocked). A full-file search reads millions of lines; handing each line a
// fresh heap slice is the single largest allocator in the search path, so we
// recycle buffers through a pool instead of allocating one per line.
var lineBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

// scanBufPool reuses the large, contiguous chunk read buffers used by
// ScanLines. A full-file search previously issued one random ReadAt per line
// (~10M pread syscalls on a 10M-line log); ScanLines reads the whole searched
// range in a single ReadAt and reuses this buffer, collapsing the pass to a
// handful of reads.
var scanBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256*1024)
		return &b
	},
}

// maxLineBufSize / maxScanBufSize cap the buffers retained by the pools (M2).
// A line or search chunk larger than the cap is served by a throwaway
// allocation for that one call; the pooled buffer is never grown past the cap,
// so sustained searches over large/varying inputs cannot pin huge buffers in
// the pool and grow memory.
const (
	maxLineBufSize = 64 * 1024
	maxScanBufSize = 1 * 1024 * 1024
	// maxScanReadSize bounds a SINGLE ReadAt issued by the sub-span batching
	// in eachReadSpan. Line-count-bounded chunking alone is byte-unbounded:
	// 3000 lines of 64 KB each would attempt one ~196 MB read, and a 500k-line
	// export chunk on such content would try to allocate tens of GB.
	maxScanReadSize = 1 * 1024 * 1024
)

// releaseLineBuf returns buf to lineBufPool unless it grew past the cap.
func releaseLineBuf(bufPtr *[]byte, buf []byte) {
	if bufPtr == nil {
		return
	}

	if cap(buf) <= maxLineBufSize {
		*bufPtr = buf
		lineBufPool.Put(bufPtr)
	}
}

// releaseScanBuf returns buf to scanBufPool unless it grew past the cap.
func releaseScanBuf(bufPtr *[]byte, buf []byte) {
	if bufPtr == nil {
		return
	}

	if cap(buf) <= maxScanBufSize {
		*bufPtr = buf
		scanBufPool.Put(bufPtr)
	}
}

// LineItem is a single line returned by the indexer, carrying its global
// (0-based) index and the (newline-trimmed) content.
type LineItem struct {
	Index   int64
	Content string
}

// FileIndexer maps a log file to an in-memory table of per-line byte offsets so lines can be read on demand via pread without loading the whole file. It is safe for concurrent use and supports incremental tail updates.
type FileIndexer struct {
	filePath string
	fileSize int64
	offsets  []int64
	mu       sync.RWMutex
	file     *os.File
	isClosed bool
	// rebuilt is set by UpdateIndex when a rotation/truncation forced the
	// offset table to be rebuilt from scratch. TakeRebuiltFlag consumes it so
	// callers (the tail poller) can discard stale per-line state that points at
	// now-invalid line indices.
	rebuilt bool
	// gen counts offset-table rebuilds. ScanLines releases fi.mu between read
	// spans (so a tail UpdateIndex is never blocked for a whole scan) and
	// re-checks gen after each re-acquire: a bump means every line index it
	// holds became meaningless mid-pass, which is reported as ErrIndexRebuilt
	// instead of silently returning content from unrelated lines.
	gen uint64
	// scanMu serializes the index-mutating passes (UpdateIndex growth and the
	// rotation rebuild). The rebuild deliberately runs its file scan WITHOUT
	// fi.mu (see rebuildIndex), so fi.mu alone no longer guarantees that two
	// mutators cannot interleave; scanMu does. It is always taken before fi.mu
	// and never held by a reader.
	scanMu sync.Mutex
	// rebuildPending marks a rotation rebuild that started but did not finish
	// (its scan failed). fileSize already matches the rotated file and the table
	// is empty, so the size comparison alone would never trigger another attempt
	// and the file would stay stuck at zero lines; the flag makes the next
	// UpdateIndex retry the rebuild instead.
	rebuildPending bool
}

// NewFileIndexer opens a log file and builds an in-memory byte-offset index.
// The file is scanned once to record the start offset of each line, enabling
// O(1) random access via GetLine and GetLines. The index uses ~8 bytes per line
// (int64 offset), so a 10M-line file needs ~80 MB for the index.
//
// The returned FileIndexer holds an open file handle for pread-based reads.
// Call Close() when done to release the file descriptor.
//
// Returns an error if the file cannot be opened or indexed.
func NewFileIndexer(filePath string) (*FileIndexer, error) {
	file, err := os.Open(filePath) //nolint:gosec // user-provided path is intended
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}

	info, err := file.Stat()
	if err != nil {
		if cerr := file.Close(); cerr != nil {
			slog.Warn("indexer: failed to close file after stat failure", "path", filePath, "err", cerr)
		}

		return nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	// Estimate the line count from file size to pre-size the offset slice. We
	// deliberately use a conservative ~256 bytes/line assumption (O1): this
	// bounds the *initial* allocation (a 10 MB file pre-allocates ~40K int64s ≈
	// 320 KB instead of ~1.3 MB at 64 bytes/line) and avoids brief over-allocation
	// for typical logs. When the real line count is higher the slice grows via
	// amortized append, and the final slices.Clip releases any leftover slack.
	indexer := &FileIndexer{
		filePath: filePath,
		fileSize: info.Size(),
		file:     file,
		offsets:  make([]int64, 0, estimateOffsetCap(info.Size())),
	}

	if err := indexer.buildInitialIndex(); err != nil {
		if cerr := file.Close(); cerr != nil {
			slog.Warn("indexer: failed to close file after index build failure", "path", filePath, "err", cerr)
		}

		return nil, err
	}

	// Release the preallocation slack so a generous line-count estimate doesn't
	// keep millions of unused int64s alive for the whole process lifetime.
	indexer.offsets = slices.Clip(indexer.offsets)

	return indexer, nil
}

// estimateOffsetCap returns the offset-slice capacity to preallocate for a file
// of the given size, assuming a conservative ~256 bytes per line. The result is
// clamped to 1<<24 entries (~16M lines) so an absurd (or corrupt) size can never
// make the int conversion wrap and panic in make(); amortized append covers any
// overshoot. Shared by the initial build and the rotation rebuild so both size
// their table the same way.
func estimateOffsetCap(size int64) int {
	est := max(size/256, 1024)
	est = min(est, 1<<24)

	return int(est)
}

// buildInitialIndex scans the entire file once to build the line offset table.
// It is called by NewFileIndexer and takes fi.mu for the duration. fi.fileSize
// is already the full file size (set in NewFileIndexer), which scanIndexUnlocked
// needs to decide whether the final, newline-less line is real content.
// Returns an error if reading the file fails.
func (fi *FileIndexer) buildInitialIndex() error {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	// Start from a clean slate: a fresh index has no offsets yet.
	fi.offsets = fi.offsets[:0]
	if err := fi.scanIndexUnlocked(fi.fileSize); err != nil {
		return err
	}

	fi.offsets = slices.Clip(fi.offsets)

	return nil
}

// scanIndexUnlocked appends line-start offsets for the portion of the file
// after the current end of fi.offsets, up to limit (the file size). Caller must
// hold fi.mu. It assumes fi.offsets already contains the offsets of the
// preceding lines (an empty slice means "scan from the very beginning,
// recording the initial 0 offset"). This is the shared core of the initial
// build and the incremental UpdateIndex growth/truncation paths. limit is
// passed explicitly so callers can build from a freshly-truncated size without
// first stashing it on fi.fileSize.
func (fi *FileIndexer) scanIndexUnlocked(limit int64) error {
	startOffset := int64(0)
	if len(fi.offsets) > 0 {
		startOffset = fi.offsets[len(fi.offsets)-1]
	}

	if _, err := fi.file.Seek(startOffset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek %s: %w", fi.filePath, err)
	}

	if len(fi.offsets) == 0 {
		fi.offsets = append(fi.offsets, 0)
	}

	offsets, err := appendLineOffsets(fi.file, fi.offsets, startOffset, limit, fi.filePath)
	fi.offsets = offsets

	return err
}

// appendLineOffsets scans r sequentially and appends the start offset of every
// complete line it finds to offsets, returning the grown slice. startOffset is
// the absolute file position r begins at (so the recorded offsets are absolute)
// and limit is the file size the scan is bounded by — a trailing chunk without a
// final newline is only recorded as a line while it still ends before limit,
// which is what keeps a half-written last line out of the table until its
// newline arrives.
//
// It takes a plain io.Reader instead of reading fi.file directly so the rotation
// rebuild can drive it from an io.SectionReader (pread-based, no shared file
// offset) on a private slice with fi.mu released, while the incremental growth
// path keeps using the seek-positioned file handle.
func appendLineOffsets(r io.Reader, offsets []int64, startOffset, limit int64, path string) ([]int64, error) {
	reader := bufio.NewReaderSize(r, 64*1024)
	currentOffset := startOffset

	// ReadSlice hands back views into the reader's internal buffer instead of
	// allocating a fresh []byte per line like ReadBytes (~10M allocations when
	// indexing a 10M-line file). The offset table only needs byte counts, so
	// the views never have to be copied; lines longer than the 64 KB buffer
	// arrive in ErrBufferFull fragments accumulated in `consumed`.
	consumed := int64(0)

	for {
		frag, err := reader.ReadSlice('\n')
		consumed += int64(len(frag))

		switch {
		case err == nil:
			// Fragment ends with '\n': the line is complete.
			currentOffset += consumed
			if consumed > 0 {
				offsets = append(offsets, currentOffset)
			}

			consumed = 0
		case errors.Is(err, bufio.ErrBufferFull):
			// Buffer filled mid-line: keep accumulating fragments.
		case errors.Is(err, io.EOF):
			// Trailing content without a final newline: recorded only while
			// still inside the scan limit, matching the original semantics.
			if consumed > 0 && currentOffset+consumed < limit {
				offsets = append(offsets, currentOffset+consumed)
			}

			return offsets, nil
		default:
			return offsets, fmt.Errorf("failed to index %s: %w", path, err)
		}
	}
}

// UpdateIndex checks if the file has changed since the last scan and refreshes
// the offset index. It is called periodically by the tail poller.
//
//   - Growth (info.Size() > fileSize): newly appended lines are scanned and
//     their offsets appended; the number of new lines is returned.
//   - Truncation / rotation (info.Size() < fileSize): the previous offset table
//     is invalid, so the index is rebuilt from scratch and the new total line
//     count is returned as "new lines" so the UI refreshes instead of showing
//     stale content.
//   - No change: returns 0.
//
// Concurrent index mutation is serialized by fi.scanMu, not by fi.mu: the
// rebuild path releases the write lock for its (potentially multi-second) file
// scan, so fi.mu no longer spans a whole mutating pass.
//
// Returns an error only if the file stat or a read fails.
func (fi *FileIndexer) UpdateIndex() (int, error) {
	fi.scanMu.Lock()
	defer fi.scanMu.Unlock()

	fi.mu.Lock()

	fi.rebuilt = false
	if fi.isClosed {
		fi.mu.Unlock()
		return 0, os.ErrClosed
	}

	// Stat under the lock, together with the decision it drives. Splitting the
	// two (stat unlocked, then re-lock to act on it) would hand a reader that is
	// queued behind this writer a window in which the file has already been
	// truncated but the offset table still says otherwise — it would resolve
	// offsets past the new EOF and fail on a short read instead of being told
	// the index was rebuilt.
	info, err := fi.file.Stat()
	if err != nil {
		fi.mu.Unlock()
		return 0, fmt.Errorf("failed to stat %s: %w", fi.filePath, err)
	}

	switch {
	// Truncation / log rotation: the file shrank. The existing offsets point
	// past the new end of file, so the table has to be rebuilt from scratch.
	// A previous rebuild that failed mid-scan is retried here too.
	case fi.rebuildPending, info.Size() < fi.fileSize:
		file := fi.file
		newSize := info.Size()
		fi.fileSize = newSize
		// Publish "the old lines are gone" before releasing the lock: every read
		// path derives its bounds from fi.offsets, so for the duration of the
		// unlocked rebuild a reader sees a file with no lines yet (GetLines →
		// empty, GetLine → ErrLineOutOfRange, ScanLines → nil) instead of
		// resolving stale offsets into a short read served as content. Dropping
		// the reference rather than reslicing to [:0] also lets the old table be
		// collected while the new one is built, instead of doubling index memory
		// for the whole rebuild.
		fi.offsets = nil
		fi.gen++
		fi.rebuildPending = true
		fi.mu.Unlock()

		return fi.rebuildIndex(file, newSize)
	case info.Size() == fi.fileSize:
		fi.mu.Unlock()
		return 0, nil
	}

	// Growth: scan only the newly appended tail. This stays under the write
	// lock — it reads just the appended bytes, so the hold is proportional to
	// one poll interval of new data, not to the file size.
	defer fi.mu.Unlock()

	oldLineCount := max(len(fi.offsets)-1, 0)

	fi.fileSize = info.Size()
	if err := fi.scanIndexUnlocked(fi.fileSize); err != nil {
		return 0, err
	}

	return len(fi.offsets) - 1 - oldLineCount, nil
}

// rebuildIndex re-indexes a rotated/truncated file of newSize bytes and installs
// the resulting table, returning the new line count. The caller must hold
// fi.scanMu, must have already invalidated the old table (empty offsets, bumped
// gen, updated fileSize, rebuildPending set) and must NOT hold fi.mu.
//
// The scan runs without fi.mu (M3): re-indexing a freshly rotated multi-GB file
// takes seconds, and doing it under the write lock froze every reader — viewport
// paging, an in-flight search, an export — for the whole pass, on the one event
// where the UI most needs to stay responsive. Only the final swap is locked.
func (fi *FileIndexer) rebuildIndex(file *os.File, newSize int64) (int, error) {
	// Read through a SectionReader so the scan uses pread: it neither disturbs
	// nor depends on the shared file offset, which is what lets it run
	// concurrently with the ReadAt-based readers with no lock held. The section
	// also bounds the scan at newSize, so bytes appended mid-rebuild are left
	// for the next (incremental) poll instead of racing this pass.
	offsets := make([]int64, 0, estimateOffsetCap(newSize))
	offsets = append(offsets, 0)

	offsets, err := appendLineOffsets(io.NewSectionReader(file, 0, newSize), offsets, 0, newSize, fi.filePath)
	if err != nil {
		// rebuildPending stays set: the table is empty and fileSize already
		// matches the file, so only the flag can bring the next poll back here.
		return 0, err
	}

	fi.mu.Lock()
	defer fi.mu.Unlock()
	// Closed during the unlocked scan: the fd is gone, so publishing the table
	// would resurrect a closed indexer.
	if fi.isClosed {
		return 0, os.ErrClosed
	}

	fi.offsets = slices.Clip(offsets)
	fi.rebuildPending = false
	fi.rebuilt = true

	return len(fi.offsets) - 1, nil
}

// TotalLines returns the number of lines currently indexed.
// This is O(1) and safe for concurrent use.
func (fi *FileIndexer) TotalLines() int64 {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	if len(fi.offsets) == 0 {
		return 0
	}

	return int64(len(fi.offsets) - 1)
}

// FileSize returns the current size of the indexed file in bytes.
// This is O(1) and safe for concurrent use.
func (fi *FileIndexer) FileSize() int64 {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	return fi.fileSize
}

// FilePath returns the path of the file being indexed.
// This is O(1) and safe for concurrent use.
func (fi *FileIndexer) FilePath() string {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	return fi.filePath
}

// Generation returns a counter that is bumped every time the offset table is
// rebuilt from scratch (a rotation/truncation). Callers that cache anything
// derived from line indices — matched lines, per-line statistics — read it
// alongside TotalLines and discard their cache when it changes, since a rebuild
// makes every previously observed line index denote different content. It is
// O(1) and safe for concurrent use.
func (fi *FileIndexer) Generation() uint64 {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	return fi.gen
}

// TakeRebuiltFlag reports whether the most recent successful UpdateIndex had to
// rebuild the offset table from scratch (rotation/truncation) and resets the
// flag. Callers use it to discard per-line state that now points at invalid
// indices. It is safe for concurrent use.
func (fi *FileIndexer) TakeRebuiltFlag() bool {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	rebuilt := fi.rebuilt
	fi.rebuilt = false

	return rebuilt
}

// readAtFull fills buf with exactly len(buf) bytes from offset via pread. A
// short read means the file shrank between the index update and this read
// (rotation race); serving zero/stale buffer tail as line content would corrupt
// output, so it is an error (io.ErrUnexpectedEOF), not silently padded data.
func (fi *FileIndexer) readAtFull(buf []byte, offset int64) error {
	n, err := fi.file.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read %s at offset %d: %w", fi.filePath, offset, err)
	}

	if n < len(buf) {
		return fmt.Errorf("short read of %s at offset %d: got %d of %d bytes: %w",
			fi.filePath, offset, n, len(buf), io.ErrUnexpectedEOF)
	}

	return nil
}

// readLineUnlocked is the shared core of GetLineBytes, GetLine and
// callers of getLine-style reads; the caller must hold fi.mu.RLock. It returns the
// newline-trimmed line aliasing a pooled read buffer together with bufPtr, the
// pooled-buffer handle needed to give that buffer back via
// releaseLineBuf(bufPtr, line); callers must do so exactly once after
// materializing their copy. On error no buffer is returned — it has already
// been released inside the core.
func (fi *FileIndexer) readLineUnlocked(lineIndex int64) (line []byte, bufPtr *[]byte, err error) {
	if lineIndex < 0 || lineIndex >= int64(len(fi.offsets)-1) {
		return nil, nil, fmt.Errorf("%w: %d", ErrLineOutOfRange, lineIndex)
	}

	startOffset := fi.offsets[lineIndex]
	endOffset := fi.offsets[lineIndex+1]

	length := endOffset - startOffset
	if length < 0 {
		return nil, nil, fmt.Errorf("%w: line %d", ErrCorruptOffsetTable, lineIndex)
	}

	n := int(length)
	if int64(n) != length {
		return nil, nil, fmt.Errorf("%w: line %d (%d bytes)", ErrLineTooLarge, lineIndex, length)
	}

	bufPtr, _ = lineBufPool.Get().(*[]byte)
	if bufPtr == nil {
		b := make([]byte, 0, 256)
		bufPtr = &b
	}

	buf := (*bufPtr)[:0]
	if cap(buf) < n {
		buf = make([]byte, n)
	} else {
		buf = buf[:n]
	}

	if err := fi.readAtFull(buf, startOffset); err != nil {
		releaseLineBuf(bufPtr, buf)
		return nil, nil, err
	}

	if len(buf) > 0 && buf[len(buf)-1] == '\n' {
		buf = buf[:len(buf)-1]
	}

	if len(buf) > 0 && buf[len(buf)-1] == '\r' {
		buf = buf[:len(buf)-1]
	}

	return buf, bufPtr, nil
}

// GetLineBytes returns the content of a single line by its 0-based index as a freshly allocated, caller-owned byte slice that is safe to retain. It avoids GetLine's string-then-[]byte double copy by copying the line out of the pooled read buffer once.
// GetLine: where GetLine materialises a string (string(buf)) and callers that
// only need bytes would then re-copy with []byte(content), GetLineBytes returns
// the bytes directly in a single copy. The returned slice is safe to retain.
func (fi *FileIndexer) GetLineBytes(lineIndex int64) ([]byte, error) {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	line, bufPtr, err := fi.readLineUnlocked(lineIndex)
	if err != nil {
		return nil, err
	}
	defer releaseLineBuf(bufPtr, line)

	// Copy out of the pooled buffer into a caller-owned slice so the pool can
	// be released safely. One copy, unlike GetLine's string + []byte chain.
	out := make([]byte, len(line))
	copy(out, line)

	return out, nil
}

// GetLine returns the content of a single line by its 0-based index.
// Lines are read lazily via pread from the open file handle; content is
// newline-trimmed (LF and trailing CR are stripped).
//
// Returns an error if lineIndex is out of bounds or the read fails. The
// out-of-bounds case wraps ErrLineOutOfRange for errors.Is matching.
func (fi *FileIndexer) GetLine(lineIndex int64) (string, error) {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	line, bufPtr, err := fi.readLineUnlocked(lineIndex)
	if err != nil {
		return "", err
	}
	defer releaseLineBuf(bufPtr, line)

	// string(buf) copies the bytes into an immutable string, so the pooled
	// buffer can safely be returned to the pool afterwards.
	return string(line), nil
}

// GetLineInto fetches the same line as GetLine but writes the line content
// directly into the caller-provided *strings.Builder instead of allocating a
// fresh string. It reuses the identical pool/lookup logic as GetLine so the
// semantics (newline-trimming, bounds, errors) match exactly. The builder's
// existing contents are preserved; the line bytes are appended. The caller may
// reuse the builder across calls to avoid per-row allocations on hot paths.
func (fi *FileIndexer) GetLineInto(lineIndex int64, b *strings.Builder) error {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	line, bufPtr, err := fi.readLineUnlocked(lineIndex)
	if err != nil {
		return err
	}
	defer releaseLineBuf(bufPtr, line)

	b.Write(line)

	return nil
}

// GetLines returns a range of lines [start, start+count) as LineItems.
// Out-of-range or zero-count requests return an empty slice (not an error),
// so callers can paginate past EOF without special casing.
func (fi *FileIndexer) GetLines(start, count int64) ([]LineItem, error) {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	total := int64(len(fi.offsets) - 1)
	if start < 0 || start >= total || count <= 0 {
		return []LineItem{}, nil
	}

	end := min(start+count, total)

	result := make([]LineItem, 0, end-start)

	err := fi.eachReadSpan(start, end, func(i, j, spanStart int64, buf []byte) bool {
		for k := i; k < j; k++ {
			lineStart := int(fi.offsets[k] - spanStart)

			lineEnd := int(fi.offsets[k+1] - spanStart)
			if lineEnd > lineStart && buf[lineEnd-1] == '\n' {
				lineEnd--
			}

			if lineEnd > lineStart && buf[lineEnd-1] == '\r' {
				lineEnd--
			}

			result = append(result, LineItem{Index: k, Content: string(buf[lineStart:lineEnd])})
		}

		return false
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ScanLines reads the contiguous byte range covering lines [start, start+count)
// in byte-budgeted bulk ReadAts (eachReadSpan) and invokes fn once per line with
// a byte slice of the newline-trimmed content (no string allocation). The slice
// passed to fn is only valid until fn returns; fn must not retain it. Returning
// false from fn stops iteration early. This is the hot path for a full-file
// search: one sequential pass replaces millions of per-line preads.
//
// Unlike the bounded readers, ScanLines does NOT hold fi.mu for the whole pass:
// a full-file scan (search, export, level stats) runs for seconds, and holding
// the read lock that long blocks the tail poller's UpdateIndex — which needs the
// write lock — so live tailing froze for the entire duration of a search. The
// lock is taken per read span instead, which bounds a waiting writer to a single
// bulk read (at most maxScanReadSize) while keeping every offsets access and
// every fn call under the lock. Appended lines cannot disturb an in-flight scan
// (the range was clamped up front and offsets only grow), but a rotation rebuilds
// the table, which is detected via gen and reported as ErrIndexRebuilt.
func (fi *FileIndexer) ScanLines(start, count int64, fn func(idx int64, line []byte) bool) error { //nolint:gocyclo // chunked scan loop is inherently branchy
	if start < 0 || count <= 0 {
		return nil
	}

	fi.mu.RLock()

	if fi.isClosed {
		fi.mu.RUnlock()
		return os.ErrClosed
	}

	total := int64(len(fi.offsets) - 1)
	gen := fi.gen
	fi.mu.RUnlock()

	if start >= total {
		return nil
	}

	end := min(start+count, total)

	spanFn := func(i, j, spanStart int64, buf []byte) bool {
		for k := i; k < j; k++ {
			lineStart := int(fi.offsets[k] - spanStart)

			lineEnd := int(fi.offsets[k+1] - spanStart)
			if lineEnd > lineStart && buf[lineEnd-1] == '\n' {
				lineEnd--
			}

			if lineEnd > lineStart && buf[lineEnd-1] == '\r' {
				lineEnd--
			}

			if !fn(k, buf[lineStart:lineEnd]) {
				return true
			}
		}

		return false
	}

	for i := start; i < end; {
		fi.mu.RLock()

		if fi.isClosed {
			fi.mu.RUnlock()
			return os.ErrClosed
		}

		if fi.gen != gen {
			fi.mu.RUnlock()
			return fmt.Errorf("%w: %s at line %d", ErrIndexRebuilt, fi.filePath, i)
		}

		next, stop, err := fi.readSpanLocked(i, end, spanFn)
		fi.mu.RUnlock()

		if err != nil {
			return err
		}

		if stop {
			return nil
		}

		i = next
	}

	return nil
}

// maxScanRebuildRetries bounds how many times a ScanLines* operation transparently
// restarts after the offset table is rebuilt mid-pass (a live-tail rotation while
// the read lock is released between spans). Rotations are routine on a streaming
// log, so a single rebuild is retried instead of aborted; only a pathological file
// that rotates on every span exhausts the retries and surfaces ErrIndexRebuilt.
const maxScanRebuildRetries = 32

// ScanLinesRetry runs ScanLines, transparently restarting on ErrIndexRebuilt
// (the offset table was rebuilt by a live-tail rotation mid-pass). It returns the
// first non-rebuild error, or ErrIndexRebuilt once maxScanRebuildRetries attempts
// have all hit a rebuild. Callers that need the raw rebuild signal use ScanLines.
//
// Without this, a routine rotation during a search/export/level-stats aborted the
// whole operation even though the old whole-scan read lock had previously made the
// event impossible — the scan simply never saw a rotation. Retrying restores that
// invisible behavior for the common case.
func (fi *FileIndexer) ScanLinesRetry(start, count int64, fn func(idx int64, line []byte) bool) error {
	var err error
	for attempt := 0; attempt <= maxScanRebuildRetries; attempt++ {
		err = fi.ScanLines(start, count, fn)
		if !errors.Is(err, ErrIndexRebuilt) {
			return err
		}
	}

	return err
}

// eachReadSpan walks [start, end) span by span with fi.mu held by the caller for
// the whole walk. It backs the bounded readers (GetLines), which read at most a
// viewport/page worth of lines; ScanLines drives readSpanLocked directly so it
// can release the lock between spans.
func (fi *FileIndexer) eachReadSpan(start, end int64, fn func(i, j, spanStart int64, buf []byte) bool) error {
	for i := start; i < end; {
		next, stop, err := fi.readSpanLocked(i, end, fn)
		if err != nil {
			return err
		}

		if stop {
			return nil
		}

		i = next
	}

	return nil
}

// readSpanLocked reads ONE sub-span of at most maxScanReadSize bytes starting at
// line i (a lone line longer than the budget forms its own oversized span, so
// huge lines stay readable via throwaway allocations), issues a single pooled
// bulk read for it, and calls fn(i, j, spanStart, buf) with the buffer holding
// exactly the bytes of lines [i, j). It returns the next line index to read and
// whether fn asked to stop. Caller must hold fi.mu (read or write) — fn reads
// fi.offsets. Splitting the walk per span bounds transient memory regardless of
// line length, where a single-ReadAt-per-chunk design sized one buffer from the
// whole chunk's byte span (196 MB for a 3000-line chunk of 64 KB lines).
func (fi *FileIndexer) readSpanLocked(i, end int64, fn func(i, j, spanStart int64, buf []byte) bool) (next int64, stop bool, err error) {
	offs := fi.offsets
	spanStart := offs[i]
	// Binary-search the first line whose END offset exceeds the byte budget:
	// O(log n) keeps span construction free next to the actual read.
	k := int64(sort.Search(int(end-i), func(k int) bool {
		return offs[int(i)+k+1] > spanStart+maxScanReadSize
	}))
	if k == 0 {
		k = 1 // lone oversized line
	}

	j := min(i+k, end)

	spanEnd := offs[j]

	length := spanEnd - spanStart
	if length < 0 {
		return 0, false, fmt.Errorf("%w: range %d-%d", ErrCorruptOffsetTable, i, j)
	}

	n := int(length)
	if int64(n) != length {
		return 0, false, fmt.Errorf("%w: range %d-%d (%d bytes)", ErrLineTooLarge, i, j, length)
	}

	bufPtr, _ := scanBufPool.Get().(*[]byte)
	if bufPtr == nil {
		b := make([]byte, 0, 256*1024)
		bufPtr = &b
	}

	buf := (*bufPtr)[:0]
	if cap(buf) < n {
		buf = make([]byte, n)
	} else {
		buf = buf[:n]
	}

	if rerr := fi.readAtFull(buf, spanStart); rerr != nil {
		releaseScanBuf(bufPtr, buf)
		return 0, false, rerr
	}

	stop = fn(i, j, spanStart, buf)

	*bufPtr = buf
	releaseScanBuf(bufPtr, buf)

	return j, stop, nil
}

// Close releases the open file handle and marks the indexer closed.
// Subsequent calls to GetLine/GetLines/UpdateIndex return errors or no-ops.
// Safe to call multiple times.
func (fi *FileIndexer) Close() error {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if fi.isClosed {
		return nil
	}

	fi.isClosed = true
	if fi.file != nil {
		return fi.file.Close()
	}

	return nil
}
