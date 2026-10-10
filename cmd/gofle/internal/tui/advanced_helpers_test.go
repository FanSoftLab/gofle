package tui

import "testing"

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a, b , ,c ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestSplitCSVUpper(t *testing.T) {
	got := splitCSVUpper("error, warn")
	if len(got) != 2 || got[0] != "ERROR" || got[1] != "WARN" {
		t.Fatalf("got %v", got)
	}
}

func TestParseYN(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y", false, true},
		{"yes", false, true},
		{"1", false, true},
		{"n", true, false},
		{"no", true, false},
		{"", true, true},
		{"", false, false},
		{"maybe", true, true},
	}
	for _, c := range cases {
		if got := parseYN(c.in, c.def); got != c.want {
			t.Errorf("parseYN(%q,%v)=%v want %v", c.in, c.def, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Fatal("short")
	}
	if truncate("hello world", 5) != "hell…" {
		t.Fatalf("got %q", truncate("hello world", 5))
	}
}

func TestBoolYN(t *testing.T) {
	if boolYN(true) != "y" || boolYN(false) != "n" {
		t.Fatal("boolYN")
	}
}
