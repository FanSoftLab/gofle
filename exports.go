package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SaveSearchResults writes the current search matches (prefixed with their
// 1-based line numbers) to a user-chosen file. Returns the saved path, or an
// empty string if cancelled.
//
//nolint:gocyclo // export loop over result rows is inherently branchy
func (a *App) SaveSearchResults(tabID string) (string, error) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("%w: %s", ErrTabNotFound, tabID)
	}

	// Read FileName under the tab lock: RenameTab mutates it concurrently.
	tab.mu.RLock()
	fileName := tab.FileName
	tab.mu.RUnlock()

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Зберегти результати пошуку",
		DefaultFilename: fmt.Sprintf("search_results_%s.log", fileName),
		Filters: []runtime.FileFilter{
			{DisplayName: "Log Files (*.log, *.txt, *.tar.gz, *.tgz)", Pattern: "*.log;*.txt;*.tar.gz;*.tgz"},
			{DisplayName: "Text Files (*.txt)", Pattern: "*.txt"},
			{DisplayName: "Compressed Logs (*.gz, *.tar.gz, *.tgz)", Pattern: "*.gz;*.tar.gz;*.tgz"},
		},
	})

	if err != nil || savePath == "" {
		return "", err
	}

	// Snapshot the matched lines under the tab lock, then export WITHOUT it:
	// holding tab.mu across a long export blocked CloseTab/RenameTab for the
	// whole write. The indexer has its own locking for the reads.
	tab.mu.RLock()
	matched := slices.Clone(tab.MatchedLines)
	tab.mu.RUnlock()

	file, err := os.Create(savePath) //nolint:gosec // user-provided export path is intended
	if err != nil {
		return "", fmt.Errorf("failed to create %s: %w", savePath, err)
	}

	defer func() {
		if cerr := file.Close(); cerr != nil {
			slog.Warn("SaveSearchResults: failed to close output file", "path", savePath, "err", cerr)
		}
	}()

	writer := bufio.NewWriter(file)
	skipped := 0

	var writeErr error

	// Line numbers are appended into one reusable buffer per export instead
	// of fmt.Fprintf per line (H1): Fprintf boxes the line number into an
	// interface (allocating above 255) and runs reflection over the args for
	// every row of a multi-million-match export.
	num := make([]byte, 0, 32)

	// Export via bulk ScanLines over contiguous runs of the (ascending) match
	// list instead of one pread+string per match — measured ~15x on a 1M-match
	// export. Matches beyond the live total (snapshot raced a rotation) count
	// as skipped, matching what GetLine would have reported.
	total := tab.Indexer.TotalLines()

	i := 0
	for i < len(matched) && writeErr == nil {
		if matched[i] >= total {
			skipped++
			i++

			continue
		}

		runEnd := i + 1
		for runEnd < len(matched) && matched[runEnd] == matched[runEnd-1]+1 && matched[runEnd] < total {
			runEnd++
		}

		start, count := matched[i], int64(runEnd-i)

		serr := tab.Indexer.ScanLinesRetry(start, count, func(idx int64, line []byte) bool {
			num = strconv.AppendInt(num[:0], idx+1, 10)
			num = append(num, ']', ' ')
			num = append(num, line...)

			if _, e := writer.Write(num); e != nil {
				writeErr = e

				return false
			}

			if e := writer.WriteByte('\n'); e != nil {
				writeErr = e

				return false
			}

			return true
		})

		if writeErr != nil {
			break
		}

		if serr != nil {
			// Chunk read failed (e.g. raced a rotation): fall back to per-line
			// reads for exactly this run so skip accounting stays precise.
			for k := i; k < runEnd; k++ {
				content, gerr := tab.Indexer.GetLine(matched[k])
				if gerr != nil {
					skipped++
					continue
				}

				num = strconv.AppendInt(num[:0], matched[k]+1, 10)
				num = append(num, ']', ' ')
				num = append(num, content...)

				if _, e := writer.Write(num); e != nil {
					writeErr = e

					break
				}

				if e := writer.WriteByte('\n'); e != nil {
					writeErr = e

					break
				}
			}

			if writeErr != nil {
				break
			}
		}

		i = runEnd
	}

	if writeErr != nil {
		// A line that cannot be read (e.g. raced a rotation) must not vanish
		// silently — the export would look complete while missing hits.
		_ = writer.Flush()
		return "", fmt.Errorf("failed to write search results: %w", writeErr)
	}

	if ferr := writer.Flush(); ferr != nil {
		return "", fmt.Errorf("failed to flush search results: %w", ferr)
	}

	if skipped > 0 {
		return savePath, fmt.Errorf(
			"exported %d of %d matches to %s; %d unreadable lines skipped",
			len(matched)-skipped, len(matched), savePath, skipped)
	}

	return savePath, nil
}

// SaveFullLogAs exports the entire log of a tab to a user-chosen file, read in
// streaming chunks. Returns the saved path, or an empty string if cancelled.
//
//nolint:gocyclo // full-log export loop over chunks is inherently branchy
func (a *App) SaveFullLogAs(tabID string) (string, error) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("%w: %s", ErrTabNotFound, tabID)
	}

	// Read FileName under the tab lock: RenameTab mutates it concurrently.
	tab.mu.RLock()
	fileName := tab.FileName
	tab.mu.RUnlock()

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Зберегти лог як",
		DefaultFilename: fileName,
		Filters: []runtime.FileFilter{
			{DisplayName: "Log Files (*.log, *.txt, *.tar.gz, *.tgz)", Pattern: "*.log;*.txt;*.tar.gz;*.tgz"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})

	if err != nil || savePath == "" {
		return "", err
	}

	total := tab.Indexer.TotalLines()

	file, err := os.Create(savePath) //nolint:gosec // user-provided export path is intended
	if err != nil {
		return "", fmt.Errorf("failed to create %s: %w", savePath, err)
	}

	defer func() {
		if cerr := file.Close(); cerr != nil {
			slog.Warn("SaveFullLogAs: failed to close output file", "path", savePath, "err", cerr)
		}
	}()

	writer := bufio.NewWriter(file)
	// Stream the file in chunks via ScanLines, which reads each range in a
	// single ReadAt instead of one pread per line (GetLines/getLineUnlocked).
	// Exporting a 10M-line log used to issue ~10M syscalls; this collapses it
	// to a handful (O4). The chunk bounds peak memory for very large files.
	const exportChunk = int64(500_000)

	var exportErr error

	for start := int64(0); start < total; start += exportChunk {
		count := min(exportChunk, total-start)

		var werr error

		err := tab.Indexer.ScanLinesRetry(start, count, func(_ int64, line []byte) bool {
			if werr != nil {
				return false
			}

			if _, e := writer.Write(line); e != nil {
				werr = e
				return false
			}

			if e := writer.WriteByte('\n'); e != nil {
				werr = e
				return false
			}

			return true
		})

		if werr != nil {
			exportErr = werr
			break
		}

		if err != nil {
			exportErr = err
			break
		}
	}

	if exportErr != nil {
		_ = writer.Flush()
		return "", exportErr
	}

	if werr := writer.Flush(); werr != nil {
		return "", fmt.Errorf("failed to flush full log export: %w", werr)
	}

	return savePath, nil
}
