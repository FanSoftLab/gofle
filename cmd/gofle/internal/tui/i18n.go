package tui

// CLI UI is English-only. Desktop localization lives in frontend/src/i18n.ts.
var cliStrings = map[string]string{
	"no_matches_hint": "no matches — showing full log (press / to search)",
	"error_prefix":    "error: ",
	"search_hint":     "Alt+R regex · Alt+I invert · Alt+C case  ·  Tab swap  ·  \\ AND-filter  ·  Enter run · Esc cancel",
	"hint_full":       "↑↓/jk scroll  ·  / search  \\ AND  A advanced  u top  P presets  n/p next  ·  Tab pane  ·  m/s/r view  ·  f tail  g goto  ·  o recent  ·  ? help  q quit",
	"hint_short":      "↑↓/jk · / search · A adv · u top · P · n/p · Tab · m/s/r · f · o · ? · q",
	"hint_mini":       "/ search · A adv · o recent · n/p · q",
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
