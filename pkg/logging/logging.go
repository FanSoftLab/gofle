// Package logging centralizes slog initialization for both GoFLE binaries.
//
// Errors are logged with levels (Error/Warn/Info/Debug) to stderr and —
// best-effort — to gofle.log beside the config directory. The file matters for
// the packaged desktop app, whose stderr is discarded when launched from a
// .desktop entry: without it, every Warn/Error would vanish without a trace.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Setup installs the default slog logger writing to stderr and, when dir is
// non-empty, to gofle.log inside dir. A failing log file degrades to
// stderr-only logging. The returned func closes the log file.
//
// Parameters:
//   - dir: The directory where gofle.log should be created. Typically the config directory.
//     If empty, only stderr logging is used.
//
// Returns a cleanup function that closes the log file. Call this on application shutdown.
//
// Example:
//
//	cleanup := logging.Setup(config.DefaultDir())
//	defer cleanup()
//	slog.Info("Application started")
func Setup(dir string) func() {
	writers := []io.Writer{os.Stderr}

	var f *os.File

	if dir != "" {
		lf, err := os.OpenFile(filepath.Join(dir, "gofle.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // user-provided log directory is intended
		if err == nil {
			f = lf
			writers = append(writers, lf)
		}
	}

	h := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(h))

	if f == nil {
		return func() {}
	}

	return func() { _ = f.Close() }
}
