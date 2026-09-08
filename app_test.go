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
)

// tab1 is the canonical test tab id reused across multiple test cases.
const tab1 = "tab1"

// Restoring a session must reuse the persisted tab ids, otherwise the saved
// group membership and strip order would point at ids that no longer exist.
func TestNewTabIDUsesExplicitID(t *testing.T) {
	if got := newTabID("tab", "tab_persisted_42"); got != "tab_persisted_42" {
		t.Fatalf("explicit id must be used verbatim, got %q", got)
	}
}

func TestNewTabIDGeneratesPrefixedID(t *testing.T) {
	got := newTabID("tab_ssh", "")
	if !strings.HasPrefix(got, "tab_ssh_") {
		t.Fatalf("generated id should carry the prefix, got %q", got)
	}

	if got == newTabID("tab_ssh", "") {
		t.Fatal("generated ids must be unique")
	}
}

func TestExtractFilePathArgs(t *testing.T) {
	// extractFilePathArgs stats paths relative to the working directory, so
	// create every referenced file under a temp dir and chdir into it for the
	// duration of the test (M3: dir/file.log and subdir/file.log are valid
	// relative paths and must be recognised as existing files).
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	files := []string{
		"file.log", "a.log", "b.log", "c.log",
		filepath.Join("dir", "file.log"),
		filepath.Join("subdir", "file.log"),
	}
	for _, f := range files {
		dir := filepath.Dir(f)
		if dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}

		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	// Absolute path case: reference a real file inside the temp dir.
	absFile := filepath.Join(tmpDir, "abs.log")
	if err := os.WriteFile(absFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", absFile, err)
	}

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"empty", []string{}, []string{}},
		{"binary only", []string{"app", "-flag"}, []string{}},
		{"binary with file", []string{"app", absFile}, []string{absFile}},
		{"binary with multiple files", []string{"app", "a.log", "b.log", "c.log"}, []string{"a.log", "b.log", "c.log"}},
		{"binary with flag and file", []string{"app", "-v", "file.log"}, []string{"file.log"}},
		{"binary with dir and file", []string{"app", "dir/file.log"}, []string{"dir/file.log"}},
		{"binary with non-existent file", []string{"app", "/nonexistent.log"}, []string{}},
		{"binary with relative path", []string{"app", "subdir/file.log"}, []string{"subdir/file.log"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractFilePathArgs(tc.args)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d args, got %d: %v", len(tc.want), len(got), got)
			}

			for i, g := range got {
				if g != tc.want[i] {
					t.Fatalf("arg[%d]: expected %q, got %q", i, tc.want[i], g)
				}
			}
		})
	}
}

func newTestTab(t *testing.T, content string) (*indexer.FileIndexer, *search.SearchEngine, *TabState) {
	t.Helper()
	dir := t.TempDir()

	p := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	se := search.NewSearchEngine(idx)

	return idx, se, &TabState{
		ID:           tab1,
		FilePath:     p,
		FileName:     "log.txt",
		Indexer:      idx,
		Searcher:     se,
		MatchedLines: []int64{},
		AutoScroll:   true,
	}
}

func TestValidateRegex(t *testing.T) {
	app := &App{cfg: &config.AppConfig{SaveTabsExit: true}}

	valid := []string{
		"", "simple", "a", "hello world", "abc123", "a-b", "a_b", "a@b",
		"a.b", "a/b", "a\\b", "a*b", "a?b", "a+b", "^start", "end$",
		"(group)", "[abc]", "a{2,5}", "dot.asterisk", "pipe|a",
		"\\d", "\\w", "\\s",
	}
	for _, q := range valid {
		t.Run(q, func(t *testing.T) {
			res, err := app.ValidateRegex(q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !res.IsValid {
				t.Fatalf("expected %q to be valid", q)
			}
		})
	}

	invalid := []string{
		"(unclosed bracket", "[unclosed range", "\\", "\\x", "\\xgg",
		"a{5,2}",
	}
	for _, p := range invalid {
		t.Run("invalid_"+p, func(t *testing.T) {
			res, err := app.ValidateRegex(p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if res.IsValid {
				t.Fatalf("expected %q to be invalid", p)
			}
		})
	}
}

func TestTabStats(t *testing.T) {
	idx, _, tab := newTestTab(t, "line1\nline2\nline3\n")
	defer idx.Close()

	tab.MatchedLines = []int64{1, 2}
	tab.LastSearchOpts = search.SearchOptions{Query: "error"}
	tab.IsSSH = false

	app := &App{}

	stats := app.tabStats(tab)
	if stats.TabID != tab1 {
		t.Fatalf("TabID=%q", stats.TabID)
	}

	if stats.FileName != "log.txt" {
		t.Fatalf("FileName=%q", stats.FileName)
	}

	if stats.TotalLines != 3 {
		t.Fatalf("TotalLines=%d", stats.TotalLines)
	}

	if stats.MatchedLines != 2 {
		t.Fatalf("MatchedLines=%d", stats.MatchedLines)
	}

	if stats.IsSSH {
		t.Fatal("IsSSH should be false")
	}
}

func TestReuseOpenTab(t *testing.T) {
	idx, _, tab := newTestTab(t, "line1\nline2\n")
	defer idx.Close()

	app := &App{tabs: map[string]*TabState{tab1: tab}, activeTab: ""}

	stats, err := app.reuseOpenTab(tab)
	if err != nil {
		t.Fatalf("reuseOpenTab: %v", err)
	}

	if stats.TabID != tab1 {
		t.Fatalf("TabID=%q", stats.TabID)
	}

	if app.activeTab != tab1 {
		t.Fatalf("activeTab=%q", app.activeTab)
	}
}

func TestFindOpenTab(t *testing.T) {
	idx, _, tab := newTestTab(t, "line1\nline2\n")
	defer idx.Close()

	app := &App{tabs: map[string]*TabState{tab1: tab}}

	if f := app.findOpenTab(tab.FilePath); f == nil || f.ID != tab1 {
		t.Fatal("expected to find tab by path")
	}

	if f := app.findOpenTab("/nonexistent.log"); f != nil {
		t.Fatal("expected nil for missing path")
	}
}

func TestGetSettings(t *testing.T) {
	cfg := &config.AppConfig{Theme: "dark", Language: "en", SaveTabsExit: false, SearchPaneHeight: 100}
	app := &App{cfg: cfg, startupArgs: []string{"file.log"}}

	settings, err := app.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}

	if settings["theme"] != "dark" {
		t.Fatalf("theme=%v", settings["theme"])
	}

	if settings["language"] != "en" {
		t.Fatalf("language=%v", settings["language"])
	}

	if settings["save_tabs_exit"] != false {
		t.Fatalf("save_tabs_exit=%v", settings["save_tabs_exit"])
	}

	if settings["search_pane_height"] != 100 {
		t.Fatalf("search_pane_height=%v", settings["search_pane_height"])
	}

	if sa, ok := settings["startup_args"].([]string); !ok || len(sa) != 1 || sa[0] != "file.log" {
		t.Fatalf("startup_args=%v", settings["startup_args"])
	}
}

func TestSaveSettings(t *testing.T) {
	cfg := config.NewTestConfig(t)
	app := &App{cfg: cfg}

	if err := app.SaveSettings("dark", "en", false); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	if cfg.Theme != "dark" || cfg.Language != "en" || cfg.SaveTabsExit {
		t.Fatalf("fields not set: theme=%q lang=%q saveExit=%v", cfg.Theme, cfg.Language, cfg.SaveTabsExit)
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save: %v", err)
	}
}

// levelStatsApp wires a tab over content into an App the way the Wails
// lifecycle would, minus the runtime context.
func levelStatsApp(t *testing.T, content string) (*App, *TabState, string) {
	t.Helper()
	idx, _, tab := newTestTab(t, content)
	t.Cleanup(func() { _ = idx.Close() })

	return &App{tabs: map[string]*TabState{tab.ID: tab}, activeTab: tab.ID}, tab, tab.FilePath
}

func TestGetLevelStatsCountsMarkers(t *testing.T) {
	app, tab, _ := levelStatsApp(t, strings.Join([]string{
		"INFO starting up",
		"ERROR failed to connect",
		"WARN retrying",
		"FATAL giving up",
		"PANIC runtime failure",
		"DEBUG noise",
		"", // blank line
	}, "\n")+"\n")

	stats, err := app.GetLevelStats(tab.ID)
	if err != nil {
		t.Fatalf("GetLevelStats: %v", err)
	}

	if stats["E"] != 3 {
		t.Fatalf("E=%d, want 3 (ERROR+FATAL+PANIC)", stats["E"])
	}

	if stats["W"] != 1 {
		t.Fatalf("W=%d, want 1", stats["W"])
	}

	if _, err := app.GetLevelStats("nope"); !errors.Is(err, ErrTabNotFound) {
		t.Fatalf("unknown tab error = %v, want ErrTabNotFound", err)
	}
}

// A level marker beyond the inspection window must not be counted: the window is
// what keeps one pathological megabyte-long line from costing a megabyte of
// matching per refresh.
func TestGetLevelStatsHonorsScanWindow(t *testing.T) {
	late := strings.Repeat("x", levelScanWindow) + " ERROR too late\n"
	app, tab, _ := levelStatsApp(t, "ERROR early\n"+late)

	stats, err := app.GetLevelStats(tab.ID)
	if err != nil {
		t.Fatalf("GetLevelStats: %v", err)
	}

	if stats["E"] != 1 {
		t.Fatalf("E=%d, want 1 (the marker past the window is ignored)", stats["E"])
	}
}

// The count is extended incrementally over a tailed file: appended lines are
// added to the cached total, and the lines already counted are neither rescanned
// nor counted twice.
//
//nolint:gocyclo // test exercises many independent append cases
func TestGetLevelStatsIsIncrementalOverAppends(t *testing.T) {
	app, tab, path := levelStatsApp(t, "ERROR one\nWARN two\n")

	stats, err := app.GetLevelStats(tab.ID)
	if err != nil {
		t.Fatalf("first GetLevelStats: %v", err)
	}

	if stats["E"] != 1 || stats["W"] != 1 {
		t.Fatalf("first pass E=%d W=%d, want 1/1", stats["E"], stats["W"])
	}

	// A repeat call with nothing appended must be served from the cache.
	if _, err := app.GetLevelStats(tab.ID); err != nil {
		t.Fatalf("cached GetLevelStats: %v", err)
	}

	if tab.levels.scanned != 2 {
		t.Fatalf("cache consumed %d lines, want 2", tab.levels.scanned)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}

	if _, err := f.WriteString("ERROR three\nINFO four\nWARN five\n"); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := tab.Indexer.UpdateIndex(); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	stats, err = app.GetLevelStats(tab.ID)
	if err != nil {
		t.Fatalf("incremental GetLevelStats: %v", err)
	}

	if stats["E"] != 2 || stats["W"] != 2 {
		t.Fatalf("incremental E=%d W=%d, want 2/2 (no double counting)", stats["E"], stats["W"])
	}

	if tab.levels.scanned != 5 {
		t.Fatalf("cache consumed %d lines, want 5", tab.levels.scanned)
	}
}

// A rotation invalidates every counted line index, so the cache must be dropped
// and the new file counted from scratch instead of added to the old totals.
func TestGetLevelStatsRecountsAfterRotation(t *testing.T) {
	app, tab, path := levelStatsApp(t, "ERROR one\nERROR two\nWARN three\n")

	stats, err := app.GetLevelStats(tab.ID)
	if err != nil {
		t.Fatalf("first GetLevelStats: %v", err)
	}

	if stats["E"] != 2 || stats["W"] != 1 {
		t.Fatalf("first pass E=%d W=%d, want 2/1", stats["E"], stats["W"])
	}

	// logrotate with copytruncate: same path, shorter brand-new content.
	if err := os.WriteFile(path, []byte("WARN fresh\n"), 0o644); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	if _, err := tab.Indexer.UpdateIndex(); err != nil {
		t.Fatalf("UpdateIndex after rotation: %v", err)
	}

	stats, err = app.GetLevelStats(tab.ID)
	if err != nil {
		t.Fatalf("GetLevelStats after rotation: %v", err)
	}

	if stats["E"] != 0 || stats["W"] != 1 {
		t.Fatalf("after rotation E=%d W=%d, want 0/1 (stale counts must be dropped)", stats["E"], stats["W"])
	}
}

// A cancelled app context must abort the counting pass instead of letting a
// full-file scan run on after shutdown — and must report the partial counts
// rather than an error, since the frontend turns any rejection into an error
// toast and a routine shutdown is not a tab failure.
func TestGetLevelStatsHonorsCancelledContext(t *testing.T) {
	var b strings.Builder
	for range levelStatsCancelStride * 3 {
		b.WriteString("ERROR line\n")
	}

	app, tab, _ := levelStatsApp(t, b.String())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	app.ctx = ctx

	stats, err := app.GetLevelStats(tab.ID)
	if err != nil {
		t.Fatalf("GetLevelStats with a cancelled context = %v, want the partial counts", err)
	}
	// Whatever was counted before the abort stays consistent with the cache.
	if tab.levels.scanned != tab.levels.errors {
		t.Fatalf("partial progress is inconsistent: scanned=%d errors=%d", tab.levels.scanned, tab.levels.errors)
	}

	if stats["E"] != tab.levels.errors {
		t.Fatalf("returned E=%d, want the cached partial count %d", stats["E"], tab.levels.errors)
	}

	if tab.levels.scanned >= tab.Indexer.TotalLines() {
		t.Fatalf("scan was not aborted: consumed %d of %d lines", tab.levels.scanned, tab.Indexer.TotalLines())
	}
}
