package main

import (
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// benchExportApp builds an App with one tab over a 100k-line log where every
// line matches, so export-search walks the full per-match write path.
func benchExportApp(b *testing.B) (*webAPI, string) {
	b.Helper()

	dir := b.TempDir()
	p := filepath.Join(dir, "bench.log")

	var sb strings.Builder

	for i := range 100_000 {
		sb.WriteString("ERROR line ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(" something went wrong\n")
	}

	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		b.Fatalf("write log: %v", err)
	}

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { idx.Close() })

	matches := make([]int64, 0, 100_000)
	for i := range int64(100_000) {
		matches = append(matches, i)
	}

	tab := &TabState{
		ID:           "tab_bench",
		FilePath:     p,
		FileName:     "bench.log",
		Indexer:      idx,
		Searcher:     search.NewSearchEngine(idx),
		MatchedLines: matches,
	}

	api := &webAPI{app: &App{tabs: map[string]*TabState{tab.ID: tab}}}

	return api, tab.ID
}

// BenchmarkExportSearchHTTP pins the /api/tabs/<id>/export-search handler
// (M3): the baseline writes one pread + one Fprintf per match straight into
// the (unbuffered) ResponseWriter; the fixed path uses bulk ScanLines runs
// plus a bufio.Writer.
func BenchmarkExportSearchHTTP(b *testing.B) {
	api, tabID := benchExportApp(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		w := httptest.NewRecorder()
		api.handleExportSearch(w, tabID)

		if w.Code != 200 {
			b.Fatalf("status %d", w.Code)
		}
	}
}

// benchUploadBody builds the raw multipart body carrying a file part of the
// given size, prepared once so per-iteration setup is just a bytes.Reader.
func benchUploadBody(b *testing.B, size int) ([]byte, string) {
	b.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", "bench-upload.log")
	if err != nil {
		b.Fatal(err)
	}

	line := []byte("2026-08-15T10:00:00.000Z INFO bench upload line payload padding\n")
	payload := bytes.Repeat(line, size/len(line)+1)[:size]

	if _, err := fw.Write(payload); err != nil {
		b.Fatal(err)
	}

	if err := mw.Close(); err != nil {
		b.Fatal(err)
	}

	return buf.Bytes(), mw.FormDataContentType()
}

// BenchmarkUploadMultipartParse isolates the W1 change: ParseMultipartForm's
// maxMemory argument. With maxMemory at the full upload size (the old 512 MB
// value) every part is held in RAM before saveUpload re-writes it to a temp
// file; with the new 8 MB cap, parts larger than the cap spill to disk-backed
// temp files and the resident cost collapses. The 16 MB body below is 2x the
// new cap, so the two variants exercise exactly the spill boundary.
func BenchmarkUploadMultipartParse(b *testing.B) {
	body, ctype := benchUploadBody(b, 16<<20)

	for _, tc := range []struct {
		name      string
		maxMemory int64
	}{
		{"maxmem-512MB", 512 << 20},
		{"maxmem-8MB", 8 << 20},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				r := httptest.NewRequestWithContext(b.Context(), http.MethodPost, "/api/files/upload", bytes.NewReader(body))
				r.Header.Set("Content-Type", ctype)

				if err := r.ParseMultipartForm(tc.maxMemory); err != nil {
					b.Fatal(err)
				}

				r.MultipartForm.RemoveAll()
			}
		})
	}
}
