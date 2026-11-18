package main

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenFileCreatesTab(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	app := &App{
		cfg:       config.NewTestConfig(t),
		tabs:      make(map[string]*TabState),
		activeTab: "",
	}

	stats, err := app.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	if stats.FileName != "app.log" {
		t.Fatalf("FileName=%q, want app.log", stats.FileName)
	}
	if stats.TotalLines != 3 {
		t.Fatalf("TotalLines=%d, want 3", stats.TotalLines)
	}

	// Tab should be registered.
	if len(app.tabs) != 1 {
		t.Fatalf("tabs len=%d, want 1", len(app.tabs))
	}

	var tab *TabState
	for _, t := range app.tabs {
		tab = t
	}
	if tab == nil {
		t.Fatal("no tab registered")
	}
	if tab.FilePath != path {
		t.Fatalf("tab.FilePath=%q, want %q", tab.FilePath, path)
	}
	if tab.AutoScroll != true {
		t.Fatal("AutoScroll should default to true")
	}
}

func TestOpenFileReusesExistingTab(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	app := &App{
		cfg:       config.NewTestConfig(t),
		tabs:      make(map[string]*TabState),
		activeTab: "",
	}

	stats1, err := app.OpenFile(path)
	if err != nil {
		t.Fatalf("first OpenFile: %v", err)
	}

	stats2, err := app.OpenFile(path)
	if err != nil {
		t.Fatalf("second OpenFile: %v", err)
	}

	if stats1.TabID != stats2.TabID {
		t.Fatalf("tab IDs differ: %q vs %q, want same (reuse)", stats1.TabID, stats2.TabID)
	}

	if len(app.tabs) != 1 {
		t.Fatalf("tabs len=%d, want 1 (no duplicate)", len(app.tabs))
	}
}

func TestCloseTab(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}

	tab := &TabState{
		ID:         "tab1",
		FilePath:   path,
		FileName:   "app.log",
		Indexer:    idx,
		AutoScroll: true,
	}

	app := &App{
		tabs:      map[string]*TabState{"tab1": tab},
		activeTab: "tab1",
	}

	ok, err := app.CloseTab("tab1")
	if err != nil {
		t.Fatalf("CloseTab: %v", err)
	}
	if !ok {
		t.Fatal("CloseTab should return true for existing tab")
	}

	if _, exists := app.tabs["tab1"]; exists {
		t.Fatal("tab should be removed from tabs map")
	}
	if app.activeTab != "" {
		t.Fatalf("activeTab=%q, want empty after closing active tab", app.activeTab)
	}
}

func TestCloseTabMissing(t *testing.T) {
	app := &App{tabs: make(map[string]*TabState)}

	ok, err := app.CloseTab("nonexistent")
	if err != nil {
		t.Fatalf("CloseTab: %v", err)
	}
	if ok {
		t.Fatal("CloseTab should return false for missing tab")
	}
}

func TestSetActiveTab(t *testing.T) {
	tab := &TabState{
		ID:       "tab1",
		FilePath: "/var/log/app.log",
		FileName: "app.log",
	}

	app := &App{
		tabs:      map[string]*TabState{"tab1": tab},
		activeTab: "",
	}

	ok, err := app.SetActiveTab("tab1")
	if err != nil {
		t.Fatalf("SetActiveTab: %v", err)
	}
	if !ok {
		t.Fatal("SetActiveTab should return true for existing tab")
	}
	if app.activeTab != "tab1" {
		t.Fatalf("activeTab=%q, want tab1", app.activeTab)
	}
}

func TestSetActiveTabMissing(t *testing.T) {
	app := &App{tabs: make(map[string]*TabState)}

	ok, err := app.SetActiveTab("nonexistent")
	if err != nil {
		t.Fatalf("SetActiveTab: %v", err)
	}
	if ok {
		t.Fatal("SetActiveTab should return false for missing tab")
	}
}

func TestRenameTab(t *testing.T) {
	tab := &TabState{
		ID:       "tab1",
		FilePath: "/var/log/app.log",
		FileName: "app.log",
	}

	app := &App{tabs: map[string]*TabState{"tab1": tab}}

	ok, err := app.RenameTab("tab1", "renamed.log")
	if err != nil {
		t.Fatalf("RenameTab: %v", err)
	}
	if !ok {
		t.Fatal("RenameTab should return true for existing tab")
	}

	tab.mu.RLock()
	defer tab.mu.RUnlock()
	if tab.FileName != "renamed.log" {
		t.Fatalf("FileName=%q, want renamed.log", tab.FileName)
	}
}

func TestRenameTabMissing(t *testing.T) {
	app := &App{tabs: make(map[string]*TabState)}

	ok, err := app.RenameTab("nonexistent", "new.log")
	if err != nil {
		t.Fatalf("RenameTab: %v", err)
	}
	if ok {
		t.Fatal("RenameTab should return false for missing tab")
	}
}

func TestGetLogLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	defer idx.Close()

	tab := &TabState{
		ID:         "tab1",
		FilePath:   path,
		FileName:   "app.log",
		Indexer:    idx,
		AutoScroll: true,
	}

	app := &App{tabs: map[string]*TabState{"tab1": tab}}

	lines, err := app.GetLogLines("tab1", 0, 3)
	if err != nil {
		t.Fatalf("GetLogLines: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("lines len=%d, want 3", len(lines))
	}
	if lines[0].Content != "line1" {
		t.Fatalf("line[0]=%q, want line1", lines[0].Content)
	}
	if lines[1].Content != "line2" {
		t.Fatalf("line[1]=%q, want line2", lines[1].Content)
	}
	if lines[2].Content != "line3" {
		t.Fatalf("line[2]=%q, want line3", lines[2].Content)
	}
}

func TestGetLogLinesMissingTab(t *testing.T) {
	app := &App{tabs: make(map[string]*TabState)}

	lines, err := app.GetLogLines("nonexistent", 0, 10)
	if err != nil {
		t.Fatalf("GetLogLines: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("lines len=%d, want 0 for unknown tab", len(lines))
	}
}

func TestSearchLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	content := "INFO starting\nERROR failed\nWARN retrying\nERROR again\nINFO done\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	defer idx.Close()

	tab := &TabState{
		ID:           "tab1",
		FilePath:     path,
		FileName:     "app.log",
		Indexer:      idx,
		Searcher:     search.NewSearchEngine(idx),
		MatchedLines: make([]int64, 0),
		AutoScroll:   true,
	}

	app := &App{tabs: map[string]*TabState{"tab1": tab}}

	count, err := app.SearchLines("tab1", "ERROR", false, false, false, "")
	if err != nil {
		t.Fatalf("SearchLines: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d, want 2 (two ERROR lines)", count)
	}

	tab.mu.RLock()
	if len(tab.MatchedLines) != 2 {
		t.Fatalf("MatchedLines len=%d, want 2", len(tab.MatchedLines))
	}
	tab.mu.RUnlock()
}

func TestSearchLinesMissingTab(t *testing.T) {
	app := &App{tabs: make(map[string]*TabState)}

	count, err := app.SearchLines("nonexistent", "test", false, false, false, "")
	if err != nil {
		t.Fatalf("SearchLines: %v", err)
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0 for unknown tab", count)
	}
}

func TestSearchLinesEmptyQueryClears(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("ERROR line\nINFO line\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	defer idx.Close()

	tab := &TabState{
		ID:           "tab1",
		FilePath:     path,
		FileName:     "app.log",
		Indexer:      idx,
		Searcher:     search.NewSearchEngine(idx),
		MatchedLines: []int64{0},
		AutoScroll:   true,
	}

	app := &App{tabs: map[string]*TabState{"tab1": tab}}

	count, err := app.SearchLines("tab1", "", false, false, false, "")
	if err != nil {
		t.Fatalf("SearchLines: %v", err)
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0 for empty query", count)
	}

	tab.mu.RLock()
	if len(tab.MatchedLines) != 0 {
		t.Fatalf("MatchedLines len=%d, want 0 after empty query", len(tab.MatchedLines))
	}
	tab.mu.RUnlock()
}

func TestGetFilteredLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	defer idx.Close()

	tab := &TabState{
		ID:           "tab1",
		FilePath:     path,
		FileName:     "app.log",
		Indexer:      idx,
		MatchedLines: []int64{0, 2, 4},
		MatchSet:     map[int64]struct{}{0: {}, 2: {}, 4: {}},
		AutoScroll:   true,
	}

	app := &App{tabs: map[string]*TabState{"tab1": tab}}

	lines, err := app.GetFilteredLines("tab1", 0, 2)
	if err != nil {
		t.Fatalf("GetFilteredLines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines len=%d, want 2", len(lines))
	}
	if lines[0].Index != 0 {
		t.Fatalf("line[0].Index=%d, want 0", lines[0].Index)
	}
	if lines[1].Index != 2 {
		t.Fatalf("line[1].Index=%d, want 2", lines[1].Index)
	}
}

func TestGetFilteredLinesOutOfRange(t *testing.T) {
	tab := &TabState{
		ID:           "tab1",
		FilePath:     "/var/log/app.log",
		FileName:     "app.log",
		MatchedLines: []int64{0},
		AutoScroll:   true,
	}

	app := &App{tabs: map[string]*TabState{"tab1": tab}}

	// start beyond total matches
	lines, err := app.GetFilteredLines("tab1", 10, 5)
	if err != nil {
		t.Fatalf("GetFilteredLines: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("lines len=%d, want 0 for out-of-range start", len(lines))
	}

	// count <= 0
	lines, err = app.GetFilteredLines("tab1", 0, 0)
	if err != nil {
		t.Fatalf("GetFilteredLines: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("lines len=%d, want 0 for count<=0", len(lines))
	}
}

func TestGetFilteredLinesMissingTab(t *testing.T) {
	app := &App{tabs: make(map[string]*TabState)}

	lines, err := app.GetFilteredLines("nonexistent", 0, 10)
	if err != nil {
		t.Fatalf("GetFilteredLines: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("lines len=%d, want 0 for unknown tab", len(lines))
	}
}

func TestSetAutoScroll(t *testing.T) {
	tab := &TabState{
		ID:         "tab1",
		FilePath:   "/var/log/app.log",
		FileName:   "app.log",
		AutoScroll: false,
	}

	app := &App{tabs: map[string]*TabState{"tab1": tab}}

	app.SetAutoScroll("tab1", true)

	tab.mu.RLock()
	defer tab.mu.RUnlock()
	if tab.AutoScroll != true {
		t.Fatalf("AutoScroll=%v, want true", tab.AutoScroll)
	}
}

func TestSetAutoScrollMissingTab(t *testing.T) {
	app := &App{tabs: make(map[string]*TabState)}

	// Should not panic.
	app.SetAutoScroll("nonexistent", true)
}

func TestCountLevelMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	content := strings.Join([]string{
		"INFO starting",
		"ERROR failed",
		"WARN retrying",
		"FATAL giving up",
		"DEBUG noise",
		"",
	}, "\n") + "\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	defer idx.Close()

	errs, warns, scanned, err := countLevelMarkers(context.Background(), idx, 0, idx.TotalLines())
	if err != nil {
		t.Fatalf("countLevelMarkers: %v", err)
	}
	if errs != 2 {
		t.Fatalf("errs=%d, want 2 (ERROR + FATAL)", errs)
	}
	if warns != 1 {
		t.Fatalf("warns=%d, want 1 (WARN)", warns)
	}
	if scanned != idx.TotalLines() {
		t.Fatalf("scanned=%d, want %d", scanned, idx.TotalLines())
	}
}

func TestCountLevelMarkersPartialRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	content := strings.Join([]string{
		"ERROR line 0",
		"ERROR line 1",
		"ERROR line 2",
		"ERROR line 3",
		"ERROR line 4",
	}, "\n") + "\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	defer idx.Close()

	// Count only lines 1-3 (start=1, count=3): should find ERROR at lines 1,2,3.
	errs, warns, scanned, err := countLevelMarkers(context.Background(), idx, 1, 3)
	if err != nil {
		t.Fatalf("countLevelMarkers: %v", err)
	}
	if errs != 3 {
		t.Fatalf("errs=%d, want 3 (three ERROR lines in range)", errs)
	}
	if warns != 0 {
		t.Fatalf("warns=%d, want 0", warns)
	}
	if scanned != 4 {
		t.Fatalf("scanned=%d, want 4 (exclusive end index)", scanned)
	}
}

func TestCountLevelMarkersCancelled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// Write enough lines that the cancel stride will trigger.
	var b strings.Builder
	for range levelStatsCancelStride * 3 {
		b.WriteString("ERROR line\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("NewFileIndexer: %v", err)
	}
	defer idx.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, _, _, err = countLevelMarkers(ctx, idx, 0, idx.TotalLines())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestLooksLikeRegex(t *testing.T) {
	regexChars := []string{".", "*", "+", "?", "|", "(", ")", "[", "]", "{", "}", "^", "$", "\\"}
	for _, r := range regexChars {
		if !looksLikeRegex(r) {
			t.Fatalf("looksLikeRegex(%q) should be true", r)
		}
	}

	literals := []string{"hello", "error", "123", "failed to connect", "no special chars"}
	for _, l := range literals {
		if looksLikeRegex(l) {
			t.Fatalf("looksLikeRegex(%q) should be false", l)
		}
	}

	if looksLikeRegex("") {
		t.Fatal("looksLikeRegex(empty) should be false")
	}
}

func TestConnLimitParamsDefaults(t *testing.T) {
	app := &App{cfg: nil}

	maxAttempts, window, lockout := app.connLimitParams()
	if maxAttempts != 5 {
		t.Fatalf("maxAttempts=%d, want 5", maxAttempts)
	}
	if window != 15*time.Minute {
		t.Fatalf("window=%v, want 15m", window)
	}
	if lockout != 5*time.Minute {
		t.Fatalf("lockout=%v, want 5m", lockout)
	}
}

func TestRemoteTimeoutsDefaults(t *testing.T) {
	app := &App{cfg: nil}

	connect, stall := app.remoteTimeouts()
	if connect != 10*time.Second {
		t.Fatalf("connect=%v, want 10s", connect)
	}
	if stall != 60*time.Second {
		t.Fatalf("stall=%v, want 60s", stall)
	}
}

func TestAutoAcceptHostKeyDefault(t *testing.T) {
	app := &App{cfg: nil}
	if app.autoAcceptHostKey() {
		t.Fatal("autoAcceptHostKey should be false when cfg is nil")
	}
}

func TestCloseRemoteStreamerAsyncNil(t *testing.T) {
	// Should not panic with nil streamer.
	closeRemoteStreamerAsync(nil)
}
