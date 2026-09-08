package main

import (
	"GoFLE/pkg/indexer"
	"bufio"
	"fmt"
	"os"
	"testing"
)

// The benchmarks mirror the two export shapes over the same fixture so the
// GetLine-per-line vs ScanLines-bulk decision stays quantified. Production
// paths live behind Wails bindings; these document the ~15x gap measured when
// exports moved to ScanLines (see perf/export-paths).

func newBenchFixture(b *testing.B) *indexer.FileIndexer {
	b.Helper()

	f, err := os.CreateTemp(b.TempDir(), "gofle-export-bench-*.log")
	if err != nil {
		b.Fatal(err)
	}

	path := f.Name()

	w := bufio.NewWriterSize(f, 1<<20)
	for i := range 200_000 {
		fmt.Fprintf(w, "%06d some log line with payload %080d\n", i, i)
	}

	w.Flush()
	f.Close()

	fi, err := indexer.NewFileIndexer(path)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { fi.Close(); os.Remove(path) })

	return fi
}

func sink() *bufio.Writer { return bufio.NewWriter(&discard{}) }

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// BenchmarkExportGetLineLoop is the legacy shape: one pread + one string per line.
func BenchmarkExportGetLineLoop(b *testing.B) {
	fi := newBenchFixture(b)
	total := fi.TotalLines()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		w := sink()

		for i := range total {
			line, err := fi.GetLine(i)
			if err != nil {
				b.Fatal(err)
			}

			fmt.Fprintf(w, "%d:%s\n", i+1, line)
		}

		w.Flush()
	}
}

// BenchmarkExportScanChunks is the new shape: bulk chunked reads.
func BenchmarkExportScanChunks(b *testing.B) {
	fi := newBenchFixture(b)
	total := fi.TotalLines()

	const chunk = int64(500_000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		w := sink()

		for start := int64(0); start < total; start += chunk {
			n := min(chunk, total-start)

			err := fi.ScanLines(start, n, func(idx int64, line []byte) bool {
				fmt.Fprintf(w, "[%d] %s\n", idx+1, line)
				return true
			})
			if err != nil {
				b.Fatal(err)
			}
		}

		w.Flush()
	}
}
