package indexer

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"testing"
)

// Benchmark fixture: a deterministic temp log written once per benchmark.
// Every line is exactly 80 bytes ("0"-padded 7-digit counter + '\n').
const (
	benchTotalLines = 200_000
	benchLineBytes  = 80
)

// scanSink keeps the ScanLines callback counter live across iterations.
var scanSink int64

func newBenchIndexer(b *testing.B) *FileIndexer {
	b.Helper()

	f, err := os.CreateTemp(b.TempDir(), "gofle-bench-*.log")
	if err != nil {
		b.Fatalf("failed to create temp file: %v", err)
	}

	path := f.Name()

	w := bufio.NewWriter(f)
	for i := range benchTotalLines {
		if _, err := fmt.Fprintf(w, "%079d\n", i); err != nil {
			b.Fatalf("failed to write line: %v", err)
		}
	}

	if err := w.Flush(); err != nil {
		b.Fatalf("failed to flush: %v", err)
	}

	if err := f.Close(); err != nil {
		b.Fatalf("failed to close temp file: %v", err)
	}

	fi, err := NewFileIndexer(path)
	if err != nil {
		os.Remove(path)
		b.Fatalf("failed to create indexer: %v", err)
	}

	b.Cleanup(func() {
		fi.Close()
		os.Remove(path)
	})

	return fi
}

func BenchmarkGetLine(b *testing.B) {
	fi := newBenchIndexer(b)
	total := int64(benchTotalLines)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		idx := (int64(i) * 7919) % total
		if _, err := fi.GetLine(idx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetLines(b *testing.B) {
	fi := newBenchIndexer(b)
	total := int64(benchTotalLines)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		start := (int64(i) * 97) % total
		if _, err := fi.GetLines(start, 100); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanLines(b *testing.B) {
	fi := newBenchIndexer(b)
	total := int64(benchTotalLines)
	counted := int64(0)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		start := (int64(i) * 2999) % total
		n := int64(0)

		err := fi.ScanLines(start, 3000, func(_ int64, _ []byte) bool {
			n++
			return true
		})
		if err != nil {
			b.Fatal(err)
		}

		counted += n
	}

	scanSink = counted
}

// BenchmarkNewFileIndexer times the full index build (the open path). Before
// the ReadSlice rewrite this cost one heap allocation per scanned line.
func BenchmarkNewFileIndexer(b *testing.B) {
	f, err := os.CreateTemp(b.TempDir(), "gofle-bench-build-*.log")
	if err != nil {
		b.Fatalf("failed to create temp file: %v", err)
	}

	path := f.Name()

	w := bufio.NewWriter(f)
	for i := range benchTotalLines {
		if _, err := fmt.Fprintf(w, "%079d\n", i); err != nil {
			b.Fatalf("failed to write line: %v", err)
		}
	}

	if err := w.Flush(); err != nil {
		b.Fatalf("failed to flush: %v", err)
	}

	if err := f.Close(); err != nil {
		b.Fatalf("failed to close temp file: %v", err)
	}

	b.Cleanup(func() { os.Remove(path) })

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		fi, err := NewFileIndexer(path)
		if err != nil {
			b.Fatal(err)
		}

		fi.Close()
	}
}

// BenchmarkUpdateIndexGrowth exercises the 500ms-cadence tail path: append a
// batch of lines, then let UpdateIndex scan only the appended tail.
func BenchmarkUpdateIndexGrowth(b *testing.B) {
	fi := newBenchIndexer(b)

	f, err := os.OpenFile(fi.FilePath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		b.Fatalf("open for append: %v", err)
	}

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		if _, err := fmt.Fprintf(f, "%079d\n", i); err != nil {
			b.Fatal(err)
		}

		if _, err := fi.UpdateIndex(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScanLinesLongLines64K pins the sub-span batching: chunking by line
// count alone would attempt one ~196 MB ReadAt here (2000 lines x 64 KB).
func BenchmarkScanLinesLongLines64K(b *testing.B) {
	f, err := os.CreateTemp(b.TempDir(), "gofle-bench-long-*.log")
	if err != nil {
		b.Fatalf("create: %v", err)
	}

	path := f.Name()

	const longLine = 64 * 1024

	line := bytes.Repeat([]byte("x"), longLine-1)

	w := bufio.NewWriterSize(f, 1024*1024)
	for range 2000 {
		if _, err := w.Write(line); err != nil {
			b.Fatal(err)
		}

		if err := w.WriteByte('\n'); err != nil {
			b.Fatal(err)
		}
	}

	if err := w.Flush(); err != nil {
		b.Fatal(err)
	}

	if err := f.Close(); err != nil {
		b.Fatal(err)
	}

	fi, err := NewFileIndexer(path)
	if err != nil {
		os.Remove(path)
		b.Fatalf("indexer: %v", err)
	}

	b.Cleanup(func() { fi.Close(); os.Remove(path) })

	counted := int64(0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		n := int64(0)

		err := fi.ScanLines(0, 2000, func(_ int64, _ []byte) bool {
			n++
			return true
		})
		if err != nil {
			b.Fatal(err)
		}

		counted += n
	}

	scanSink = counted
}

// BenchmarkGetLinesPage covers the viewport pagination shape (100-line window).
func BenchmarkGetLinesPage(b *testing.B) {
	fi := newBenchIndexer(b)
	total := int64(benchTotalLines)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		start := (int64(i) * 53) % total
		if items, err := fi.GetLines(start, 100); err != nil || len(items) == 0 {
			b.Fatalf("GetLines: n=%d err=%v", len(items), err)
		}
	}
}
