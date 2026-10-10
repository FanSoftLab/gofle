package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportGUIPresets(t *testing.T) {
	dir := t.TempDir()
	// Minimal config dir with secret key so Load works via NewTestConfig if available.
	c := NewTestConfig(t)
	_ = dir
	raw := []byte(`[
	  {
	    "name": "errors",
	    "filter": {
	      "query": "ERROR",
	      "nearby_query": "timeout",
	      "nearby_mode": "either",
	      "nearby_window": 5,
	      "exclude_patterns": ["debug"],
	      "levels": ["ERROR", "FATAL"]
	    }
	  },
	  {
	    "name": "warns",
	    "filter": {
	      "query": "WARN",
	      "case_sensitive": true
	    }
	  }
	]`)
	n, err := c.ImportGUIPresets(raw)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("imported %d want 2", n)
	}
	presets := c.GetSearchPresets()
	if len(presets) < 2 {
		t.Fatalf("got %d presets", len(presets))
	}
	var found bool
	for _, p := range presets {
		if p.Name == "errors" && p.NearbyQuery == "timeout" && len(p.ExcludePatterns) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors preset not found: %+v", presets)
	}
}

func TestImportGUIPresetsBadJSON(t *testing.T) {
	c := NewTestConfig(t)
	_, err := c.ImportGUIPresets([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewTestConfigIsolates(t *testing.T) {
	c := NewTestConfig(t)
	if c == nil {
		t.Fatal("nil")
	}
	// Ensure we can write presets file in the temp dir.
	if err := c.AddSearchPreset(SearchPreset{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.configDir, "search_presets.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
