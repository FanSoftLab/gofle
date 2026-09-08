package shared

import (
	"fmt"
	"io"
	"os"
)

// ColorMode controls when matched-line output is colorized.
type ColorMode int

const (
	// ColorAuto colorizes only when stdout is a terminal.
	ColorAuto ColorMode = iota
	// ColorAlways colorizes regardless of output destination.
	ColorAlways
	// ColorNever never colorizes.
	ColorNever
)

// IsTTY reports whether os.Stdout is a character device (i.e. a terminal
// rather than a pipe or redirected file). Used for auto color/format detection.
func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

// ResolveColor maps a ColorMode to a concrete boolean: in ColorAuto mode it
// returns true only when the output is a terminal.
func ResolveColor(mode ColorMode) bool {
	switch mode {
	case ColorAuto:
		return IsTTY()
	case ColorAlways:
		return true
	case ColorNever:
		return false
	}

	return IsTTY()
}

// MaxLineNumWidth returns the number of decimal digits needed to render total
// as a line number, for gutter padding.
func MaxLineNumWidth(total int64) int {
	if total < 0 {
		total = -total
	}

	w := 1

	for total >= 10 {
		total /= 10
		w++
	}

	return w
}

// PrintMatchedLine writes a single matched line as "LINENO:content" to w. When
// gutterWidth > 0 the line number is right-padded to that width. The color flag
// is a hook for future highlighting (v1 prints plain text).
func PrintMatchedLine(w io.Writer, lineNum int64, content string, _ bool, gutterWidth int) {
	// Write failures belong to the caller's flush/error handling; this helper
	// has no error return by contract.
	if gutterWidth > 0 {
		_, _ = fmt.Fprintf(w, "%*d:%s\n", gutterWidth, lineNum, content)
		return
	}

	_, _ = fmt.Fprintf(w, "%d:%s\n", lineNum, content)
}
