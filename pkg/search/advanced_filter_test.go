package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"GoFLE/pkg/indexer"
)

func TestApplyAdvancedFilters_Levels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	content := "info hello\nERROR boom\nwarn x\njust text\nFATAL bad\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	se := NewSearchEngine(idx)
	// All lines 0..4
	all := []int64{0, 1, 2, 3, 4}
	out, err := se.ApplyAdvancedFilters(context.Background(), SearchOptions{
		Levels: []string{"ERROR"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// ERROR boom + FATAL bad
	if len(out) != 2 || out[0] != 1 || out[1] != 4 {
		t.Fatalf("ERROR filter got %v want [1 4]", out)
	}
	out, err = se.ApplyAdvancedFilters(context.Background(), SearchOptions{
		Levels: []string{"OTHER"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// "just text" only (info/warn/error/fatal classified)
	if len(out) != 1 || out[0] != 3 {
		t.Fatalf("OTHER filter got %v want [3]", out)
	}
}

func TestApplyAdvancedFilters_Exclude(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	content := "alpha\nbeta noise\ngamma\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	se := NewSearchEngine(idx)
	out, err := se.ApplyAdvancedFilters(context.Background(), SearchOptions{
		ExcludePatterns: []string{"noise"},
	}, []int64{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != 0 || out[1] != 2 {
		t.Fatalf("exclude got %v", out)
	}
}
