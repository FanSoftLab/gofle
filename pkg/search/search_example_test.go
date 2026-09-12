package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"GoFLE/pkg/indexer"
)

func newExampleIndexer(content string) *indexer.FileIndexer {
	dir, _ := os.MkdirTemp("", "search-example")
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "example.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		panic(err)
	}
	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		panic(err)
	}
	return idx
}

func ExampleSearchEngine_Search() {
	idx := newExampleIndexer("error: connection failed\ninfo: retrying\nerror: timeout\n")
	se := NewSearchEngine(idx)
	defer idx.Close()

	matches, _ := se.Search(SearchOptions{Query: "error"})
	fmt.Printf("Found %d matches\n", len(matches))
	// Output: Found 2 matches
}

func ExampleSearchEngine_SearchWithContext() {
	idx := newExampleIndexer("ERROR: disk full\nWARN: low memory\nINFO: backup started\n")
	se := NewSearchEngine(idx)
	defer idx.Close()

	matches, _ := se.SearchWithContext(context.Background(), SearchOptions{
		Query:         "error",
		CaseSensitive: false,
	})
	fmt.Printf("Case-insensitive matches: %d\n", len(matches))
	// Output: Case-insensitive matches: 1
}

func ExampleSearchEngine_SearchRange() {
	idx := newExampleIndexer("a\nb\nerror\nc\nerror\nd\n")
	se := NewSearchEngine(idx)
	defer idx.Close()

	matches, _ := se.SearchRange(context.Background(), SearchOptions{Query: "error"}, 2, 5)
	fmt.Printf("Matches in range [2,5): %v\n", matches)
	// Output: Matches in range [2,5): [2 4]
}

func Example_regex() {
	idx := newExampleIndexer("req-12345 completed\nreq-67890 failed\nreq-abc invalid\n")
	se := NewSearchEngine(idx)
	defer idx.Close()

	matches, _ := se.SearchWithContext(context.Background(), SearchOptions{
		Query:   `req-\d+`,
		IsRegex: true,
	})
	fmt.Printf("Regex matches: %v\n", matches)
	// Output: Regex matches: [0 1]
}

func Example_invert() {
	idx := newExampleIndexer("error\ninfo\nerror\nwarning\n")
	se := NewSearchEngine(idx)
	defer idx.Close()

	matches, _ := se.SearchWithContext(context.Background(), SearchOptions{
		Query:       "error",
		InvertMatch: true,
	})
	fmt.Printf("Non-error lines: %v\n", matches)
	// Output: Non-error lines: [1 3]
}

func Example_compound() {
	idx := newExampleIndexer("ERROR: timeout\nWARN: timeout\nINFO: connected\nERROR: connection refused\n")
	se := NewSearchEngine(idx)
	defer idx.Close()

	matches, _ := se.SearchWithContext(context.Background(), SearchOptions{
		Query:          "ERROR",
		SecondaryQuery: "timeout",
	})
	fmt.Printf("ERROR lines containing 'timeout': %v\n", matches)
	// Output: ERROR lines containing 'timeout': [0]
}
