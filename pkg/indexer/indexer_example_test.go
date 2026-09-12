package indexer

import (
	"fmt"
	"os"
	"path/filepath"
)

func ExampleNewFileIndexer() {
	dir, _ := os.MkdirTemp("", "indexer-example")
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "example.log")

	content := "2024-01-15 10:00:00 INFO  Starting application\n" +
		"2024-01-15 10:00:01 DEBUG Loading config\n" +
		"2024-01-15 10:00:02 ERROR Failed to connect to database\n"

	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		panic(err)
	}

	idx, err := NewFileIndexer(p)
	if err != nil {
		panic(err)
	}
	defer idx.Close()

	fmt.Printf("Indexed %d lines\n", idx.TotalLines())
	// Output: Indexed 3 lines
}

func ExampleFileIndexer_GetLine() {
	dir, _ := os.MkdirTemp("", "indexer-example")
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "example.log")

	content := "first line\nsecond line\nthird line\n"
	_ = os.WriteFile(p, []byte(content), 0o644)

	idx, _ := NewFileIndexer(p)
	defer idx.Close()

	line, _ := idx.GetLine(1)
	fmt.Println(line)
	// Output: second line
}

func ExampleFileIndexer_GetLines() {
	dir, _ := os.MkdirTemp("", "indexer-example")
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "example.log")

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	_ = os.WriteFile(p, []byte(content), 0o644)

	idx, _ := NewFileIndexer(p)
	defer idx.Close()

	lines, _ := idx.GetLines(1, 3)
	for _, l := range lines {
		fmt.Printf("%d: %s\n", l.Index, l.Content)
	}
	// Output:
	// 1: line 2
	// 2: line 3
	// 3: line 4
}

func ExampleFileIndexer_UpdateIndex() {
	dir, _ := os.MkdirTemp("", "indexer-example")
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "example.log")

	_ = os.WriteFile(p, []byte("a\nb\n"), 0o644)

	idx, _ := NewFileIndexer(p)
	defer idx.Close()

	fmt.Printf("Initial: %d lines\n", idx.TotalLines())

	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("c\nd\ne\n")
	f.Close()

	added, _ := idx.UpdateIndex()
	fmt.Printf("Appended %d lines, total now %d\n", added, idx.TotalLines())
	// Output:
	// Initial: 2 lines
	// Appended 3 lines, total now 5
}

func ExampleFileIndexer_ScanLines() {
	dir, _ := os.MkdirTemp("", "indexer-example")
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "example.log")

	content := "ERROR: connection failed\n" +
		"INFO: retrying\n" +
		"ERROR: timeout\n" +
		"DEBUG: backoff\n"
	_ = os.WriteFile(p, []byte(content), 0o644)

	idx, _ := NewFileIndexer(p)
	defer idx.Close()

	fmt.Println("Scanning for ERROR lines:")
	idx.ScanLines(0, idx.TotalLines(), func(i int64, line []byte) bool {
		if len(line) >= 5 && string(line[:5]) == "ERROR" {
			fmt.Printf("  [%d] %s\n", i, line)
		}
		return true
	})
	// Output:
	// Scanning for ERROR lines:
	//   [0] ERROR: connection failed
	//   [2] ERROR: timeout
}
