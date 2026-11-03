package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
)

// benchExportModel writes an nLine log where every line matches and builds a
// model whose match list covers all lines, mirroring a broad search result
// export. The same file backs all benchmark variants.
func benchExportModel(b *testing.B, nLines int) model {
	b.Helper()

	dir := b.TempDir()
	p := filepath.Join(dir, "export-bench.log")

	f, err := os.Create(p)
	if err != nil {
		b.Fatal(err)
	}

	w := bufio.NewWriterSize(f, 1<<20)
	for i := range nLines {
		fmt.Fprintf(w, "%06d needle some log line with payload %080d\n", i, i)
	}

	if err := w.Flush(); err != nil {
		b.Fatal(err)
	}

	if err := f.Close(); err != nil {
		b.Fatal(err)
	}

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { idx.Close() })

	matches := make([]int64, 0, nLines)
	for i := range int64(nLines) {
		matches = append(matches, i)
	}

	return model{
		idx:         idx,
		se:          search.NewSearchEngine(idx),
		matches:     matches,
		matchCursor: 0,
		ctx:         b.Context(),
	}
}

// BenchmarkTUIExportAllLegacy is the pre-T2 shape: one pread + one string +
// one fmt.Sprintf per line. Kept as the documented baseline so the bulk
// port's gap stays quantified next to BenchmarkExportGetLineLoop (cmd/gofle).
func BenchmarkTUIExportAllLegacy(b *testing.B) {
	m := benchExportModel(b, 100_000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		p := filepath.Join(b.TempDir(), "out-legacy.log")

		f, err := os.Create(p)
		if err != nil {
			b.Fatal(err)
		}

		bw := bufio.NewWriter(f)
		total := m.idx.TotalLines()

		for i := range total {
			line, gerr := m.idx.GetLine(i)
			if gerr != nil {
				b.Fatal(gerr)
			}

			if _, werr := fmt.Fprintf(bw, "%d:%s\n", i+1, line); werr != nil {
				b.Fatal(werr)
			}
		}

		if err := bw.Flush(); err != nil {
			b.Fatal(err)
		}

		if err := f.Close(); err != nil {
			b.Fatal(err)
		}

		if err := os.Remove(p); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTUIExportAllBulk is the T2 path: chunked ScanLines reads through
// doExport's lineWriter — no per-line string, no per-line Sprintf.
func BenchmarkTUIExportAllBulk(b *testing.B) {
	m := benchExportModel(b, 100_000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		p := filepath.Join(b.TempDir(), "out-bulk.log")
		m.doExport(p)

		if m.err != nil {
			b.Fatal(m.err)
		}

		if err := os.Remove(p); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTUIExportMatchesBulk runs the T2 matched-line exporter over the
// full match list (every line matches, so runs are maximally contiguous) and
// exercises the number-prefix scratch buffer.
func BenchmarkTUIExportMatchesBulk(b *testing.B) {
	m := benchExportModel(b, 100_000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		p := filepath.Join(b.TempDir(), "out-matches.log")

		f, err := os.Create(p)
		if err != nil {
			b.Fatal(err)
		}

		lw := &lineWriter{w: bufio.NewWriter(f)}
		written, skipped := m.exportMatchedLines(lw)

		if err := lw.w.Flush(); err != nil {
			b.Fatal(err)
		}

		if err := f.Close(); err != nil {
			b.Fatal(err)
		}

		if err := os.Remove(p); err != nil {
			b.Fatal(err)
		}

		if written != 100_000 || skipped != 0 {
			b.Fatalf("written=%d skipped=%d", written, skipped)
		}
	}
}
