package tui

// CLI UI is English-only. Desktop localization lives in frontend/src/i18n.ts.
var cliStrings = map[string]string{
	"no_matches_hint": "no matches — showing full log (press / to search)",
	"error_prefix":    "error: ",
	"search_hint":     "Alt+R regex · Alt+I invert · Alt+C case  ·  Tab swap  ·  \\ AND-filter  ·  Enter run · Esc cancel",
	"hint_full":       "↑↓/jk scroll  ·  / search  \\ AND  n/p next  [ ] ends  ·  m mode  s split  r filter  ·  v select  c copy  x clear  ·  f tail  g goto  e export  o recent  ·  ? help  q quit",
	"hint_short":      "↑↓/jk · / search · \\ AND · n/p · m/s/r · f tail · ? help · q quit",
	"hint_mini":       "/ search · n/p · m mode · q quit",
	"status_split":    "split",
	"status_results":  "results",
	"status_normal":   "log",
	"status_tail_on":  "ON",
	"status_tail_off": "OFF",
	"status_filter":   "filter",
	"status_matches":  "matches",
	"status_lines":    "lines",
	"status_top":      "top",
	"mode_normal":     "normal mode",
	"mode_split":      "split view · Tab = switch pane",
	"mode_results":    "results-only mode",
	"no_matches_mode": "no matches — press / to search first",
}

// tr returns an English CLI UI string (desktop uses its own i18n).
func tr(_ model, key string) string {
	if s, ok := cliStrings[key]; ok {
		return s
	}
	return key
}
