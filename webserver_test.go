package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"GoFLE/pkg/config"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
)

// exportSearchApp wires a tab over content with the given matched line indices
// into a webAPI the way the HTTP layer would, minus the live server.
func exportSearchApp(t *testing.T, content string, matched []int64) (*webAPI, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	tab := &TabState{
		ID:           "tab_export",
		FilePath:     path,
		FileName:     "export",
		Indexer:      idx,
		Searcher:     search.NewSearchEngine(idx),
		MatchedLines: matched,
	}

	api := &webAPI{app: &App{tabs: map[string]*TabState{tab.ID: tab}}}

	return api, tab.ID
}

// The HTTP export must produce the same per-match output as the desktop
// export's format ("%d: %s\n" with the 1-based line number), keep byte-identical
// headers, and group non-contiguous matches into bulk runs — the C3 port of
// exports.go SaveSearchResults.
func TestHandleExportSearchWritesMatchesInOrder(t *testing.T) {
	content := "alpha\nbeta\ngamma\ndelta\n"
	api, tabID := exportSearchApp(t, content, []int64{0, 2, 3})

	w := httptest.NewRecorder()
	api.handleExportSearch(w, tabID)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	want := "1: alpha\n3: gamma\n4: delta\n"
	if got := w.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}

	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="search_results_export.log"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
}

// Matches beyond the live total (snapshot raced a rotation) are skipped, the
// same guard the previous per-line path applied.
func TestHandleExportSearchSkipsMatchesPastTotal(t *testing.T) {
	api, tabID := exportSearchApp(t, "alpha\nbeta\ngamma\n", []int64{0, 1, 5, 9})

	w := httptest.NewRecorder()
	api.handleExportSearch(w, tabID)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	want := "1: alpha\n2: beta\n"
	if got := w.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

// A run whose bulk read fails (here: the indexer was closed, the worst kind of
// read failure) must not abort the export; its unreadable lines are skipped the
// same way the previous per-line path always did.
func TestHandleExportSearchFallsBackWhenScanFails(t *testing.T) {
	api, tabID := exportSearchApp(t, "a\nb\nc\n", []int64{0, 1, 2})
	tab := api.app.tabs[tabID]
	if tab == nil {
		t.Fatal("tab missing from app")
	}

	if err := tab.Indexer.Close(); err != nil {
		t.Fatalf("close indexer: %v", err)
	}

	w := httptest.NewRecorder()
	api.handleExportSearch(w, tabID)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (skips are not an error)", w.Code)
	}

	if got := w.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty (all lines unreadable)", got)
	}
}

// Missing tab: same 404 as before the bulk path.
func TestHandleExportSearchMissingTab(t *testing.T) {
	api := &webAPI{app: &App{tabs: map[string]*TabState{}}}

	w := httptest.NewRecorder()
	api.handleExportSearch(w, "nope")

	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// A contiguous run must be emitted in ascending order, and a mid-file gap must
// start a new run rather than being merged into one range.
func TestHandleExportSearchRunsAreOrderedAndSplit(t *testing.T) {
	var sb strings.Builder

	for i := range 80 {
		fmt.Fprintf(&sb, "row %d\n", i)
	}

	api, tabID := exportSearchApp(t, sb.String(), []int64{0, 1, 2, 3, 40, 41, 42, 70, 71})

	w := httptest.NewRecorder()
	api.handleExportSearch(w, tabID)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	want := "1: row 0\n2: row 1\n3: row 2\n4: row 3\n" +
		"41: row 40\n42: row 41\n43: row 42\n" +
		"71: row 70\n72: row 71\n"

	if got := w.Body.String(); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
}

// saveUpload lands the uploaded bytes at the returned path.
func TestSaveUploadStoresContent(t *testing.T) {
	path, err := saveUpload(strings.NewReader("uploaded bytes\n"), "upload.log")
	if err != nil {
		t.Fatalf("saveUpload: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}

	if string(got) != "uploaded bytes\n" {
		t.Fatalf("content = %q, want %q", got, "uploaded bytes\n")
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

// A client-supplied filename containing a path separator must not make the
// rename below escape the temp directory (W2): saveUpload basenames the name
// and rejects degenerate ones.
func TestSaveUploadRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{
		"../../etc/evil.log",
		"..\\..\\windows\\evil.log",
		"sub/dir/evil.log",
		"",
		".",
		"/",
	} {
		path, err := saveUpload(strings.NewReader("x"), name)
		if err == nil {
			t.Errorf("saveUpload(%q) = %q, want error", name, path)
			if path != "" {
				_ = os.Remove(path)
			}

			continue
		}

		if path != "" {
			t.Errorf("saveUpload(%q) returned a path alongside an error", name)
		}
	}
}

// A 16 MB upload (2x the new 8 MB multipart memory cap) must still open a tab
// end-to-end through handleUpload: parts larger than the cap spill to
// disk-backed temp files that saveUpload streams via io.Copy (W1).
func TestHandleUploadLargeFileSpillsToDisk(t *testing.T) {
	cfg := config.NewTestConfig(t)

	app := &App{
		cfg:  cfg,
		tabs: make(map[string]*TabState),
		hub:  NewEventHub(),
	}

	api := &webAPI{app: app}

	line := []byte("2026-08-15T10:00:00.000Z INFO upload spill test line padding\n")
	payload := bytes.Repeat(line, (16<<20)/len(line)+1)[:(16 << 20)]

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", "spill-test.log")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fw.Write(payload); err != nil {
		t.Fatal(err)
	}

	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/files/upload", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())

	// Exercise the same middleware chain the server installs so the
	// MaxBytesReader wrapper is part of the path.
	w := httptest.NewRecorder()
	withCORS(http.HandlerFunc(api.handleUpload)).ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	if len(app.tabs) != 1 {
		t.Fatalf("open tabs = %d, want 1", len(app.tabs))
	}

	for _, tab := range app.tabs {
		if tab.Indexer.TotalLines() <= 0 {
			t.Fatal("uploaded file produced an empty index")
		}

		_ = tab.Indexer.Close()
	}
}

// Overflowing a subscriber's 64-slot buffer must be counted, not silent (W3).
func TestEventHubDropCounting(t *testing.T) {
	h := NewEventHub()
	ch := h.Subscribe()

	for range 100 { // 100 > 64-slot buffer, nobody consumes
		h.Broadcast("evt", map[string]any{"n": 1})
	}

	if got := h.Drops(); got == 0 {
		t.Error("Drops() = 0 after overflowing an unconsumed subscriber")
	}

	// The subscriber still receives the first buffered events in order.
	for range 64 {
		select {
		case <-ch:
		default:
			t.Fatal("buffered events missing")
		}
	}

	select {
	case <-ch:
		t.Fatal("unexpected extra event")
	default:
	}
}

// An abruptly disconnected client (TCP close without a WebSocket close
// handshake) must end the handler promptly via the read pump, releasing the
// goroutine and its subscriber channel (W3).
func TestHandleWSAbruptClientDisconnect(t *testing.T) {
	app := &App{cfg: config.NewTestConfig(t), hub: NewEventHub()}
	api := &webAPI{app: app}

	srv := httptest.NewServer(http.HandlerFunc(api.handleWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	handlerReturned := make(chan struct{})
	go func() {
		// The handler goroutine is the one ServeHTTP runs on; re-running
		// handleWS on its own conn would race the server's — instead detect
		// teardown by the hub subscriber count dropping back to zero.
		t0 := time.Now()
		for range 100 {
			app.hub.mu.RLock()
			n := len(app.hub.subs)
			app.hub.mu.RUnlock()

			if n == 0 {
				close(handlerReturned)

				return
			}

			time.Sleep(10 * time.Millisecond)
		}
		t.Logf("subscriber still registered after %v", time.Since(t0))
	}()

	// Abrupt TCP-level close, no WS close frame: with no read deadline this
	// used to leave ReadMessage blocked for hours; the plain TCP close still
	// errors immediately, and the deadline path covers the half-open case.
	conn.UnderlyingConn().Close()

	select {
	case <-handlerReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("WS handler did not release the subscriber after client disconnect")
	}
}
