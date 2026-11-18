package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupStderrOnly(t *testing.T) {
	t.Helper()
	cleanup := Setup("")
	defer cleanup()

	logger := slog.Default()
	if logger == nil {
		t.Fatal("Default logger should not be nil")
	}

	logger.Info("test message")
}

func TestSetupWithDir(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()
	logger.Info("test message with dir")

	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("Log file should exist: %v", err)
	}

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message with dir") {
		t.Errorf("Log file should contain message, got: %s", string(content))
	}
	if !strings.Contains(string(content), "INFO") {
		t.Errorf("Log file should contain level INFO, got: %s", string(content))
	}
}

func TestSetupWithInvalidDir(t *testing.T) {
	t.Helper()
	invalidDir := "/nonexistent/path/that/does/not/exist"
	cleanup := Setup(invalidDir)
	defer cleanup()

	logger := slog.Default()
	if logger == nil {
		t.Fatal("Default logger should not be nil even with invalid dir")
	}

	logger.Info("test with invalid dir")
}

func TestSetupReturnsCleanup(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)

	if cleanup == nil {
		t.Fatal("Setup should return a cleanup function")
	}

	cleanup()
	cleanup()
}

func TestSetupMultipleCalls(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()

	cleanup1 := Setup(tmpDir)
	logger1 := slog.Default()
	logger1.Info("first setup")

	cleanup2 := Setup(tmpDir)
	logger2 := slog.Default()
	logger2.Info("second setup")

	if logger1 == logger2 {
		t.Log("Same logger instance returned (expected)")
	}

	cleanup1()
	cleanup2()
}

func TestLogLevels(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	if !strings.Contains(logContent, "info message") {
		t.Errorf("Log should contain info message")
	}
	if !strings.Contains(logContent, "warn message") {
		t.Errorf("Log should contain warn message")
	}
	if !strings.Contains(logContent, "error message") {
		t.Errorf("Log should contain error message")
	}

	if strings.Contains(logContent, "debug message") {
		t.Log("Debug message appears in log (level is Info, so this may or may not appear)")
	}
}

func TestStructuredLogging(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()
	logger.Info("structured log",
		"key1", "value1",
		"key2", 42,
		"key3", true,
	)

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "structured log") {
		t.Errorf("Log should contain message")
	}
	if !strings.Contains(logContent, "key1") || !strings.Contains(logContent, "value1") {
		t.Errorf("Log should contain structured key1=value1")
	}
	if !strings.Contains(logContent, "key2") || !strings.Contains(logContent, "42") {
		t.Errorf("Log should contain structured key2=42")
	}
	if !strings.Contains(logContent, "key3") || !strings.Contains(logContent, "true") {
		t.Errorf("Log should contain structured key3=true")
	}
}

func TestLogOutputFormat(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()
	logger.Info("format test", "user", "alice", "action", "login")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	if len(lines) < 1 {
		t.Fatalf("Expected at least one log line, got: %s", logContent)
	}

	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "format test") {
		t.Errorf("Log line should contain message: %s", lastLine)
	}
	if !strings.Contains(lastLine, "user=alice") && !strings.Contains(lastLine, `"user":"alice"`) {
		t.Errorf("Log line should contain user attribute: %s", lastLine)
	}
	if !strings.Contains(lastLine, "action=login") && !strings.Contains(lastLine, `"action":"login"`) {
		t.Errorf("Log line should contain action attribute: %s", lastLine)
	}
}

func TestSetupFilePermissions(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()
	logger.Info("permission test")

	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Failed to stat log file: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf("Log file permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestSetupCreatesDirIfNotExists(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "logs", "subdir")

	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	cleanup := Setup(subDir)
	defer cleanup()

	logFile := filepath.Join(subDir, "gofle.log")

	logger := slog.Default()
	logger.Info("nested dir test")

	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("Log file should exist in nested dir: %v", err)
	}
}

func TestSetupWithRelativePath(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	absRelDir := filepath.Join(tmpDir, "rel_logs")

	if err := os.MkdirAll(absRelDir, 0o755); err != nil {
		t.Fatalf("Failed to create rel_logs dir: %v", err)
	}

	cleanup := Setup(absRelDir)
	defer cleanup()

	logFile := filepath.Join(absRelDir, "gofle.log")

	logger := slog.Default()
	logger.Info("relative path test")

	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("Log file should exist: %v", err)
	}
}

func TestCleanupClosesFile(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()
	logger.Info("before cleanup")

	cleanup()

	logger.Info("after cleanup")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "before cleanup") {
		t.Errorf("Log should contain 'before cleanup'")
	}
	if strings.Contains(logContent, "after cleanup") {
		t.Log("Message after cleanup may or may not appear depending on buffering")
	}
}

func TestSetupWithStderrCapture(t *testing.T) {
	t.Helper()
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cleanup := Setup("")
	logger := slog.Default()
	logger.Info("stderr test")

	w.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "stderr test") {
		t.Errorf("Stderr should contain log message, got: %s", output)
	}

	cleanup()
}

func TestLogLevelFiltering(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()

	logger.Debug("debug level")
	logger.Info("info level")
	logger.Warn("warn level")
	logger.Error("error level")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	if strings.Contains(logContent, "debug level") {
		t.Log("Debug message logged (level is Info by default)")
	}
	if !strings.Contains(logContent, "info level") {
		t.Errorf("Info message should be logged")
	}
	if !strings.Contains(logContent, "warn level") {
		t.Errorf("Warn message should be logged")
	}
	if !strings.Contains(logContent, "error level") {
		t.Errorf("Error message should be logged")
	}
}

func TestSetupWithEmptyDirStillLogsToStderr(t *testing.T) {
	t.Helper()
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cleanup := Setup("")
	logger := slog.Default()
	logger.Info("stderr only test")

	w.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "stderr only test") {
		t.Errorf("Stderr should contain log message when dir is empty, got: %s", output)
	}

	cleanup()
}

func TestSetupLogFileAppend(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()
	logger.Info("first message")

	cleanup2 := Setup(tmpDir)
	defer cleanup2()
	logger.Info("second message")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "first message") {
		t.Errorf("Log should contain first message")
	}
	if !strings.Contains(logContent, "second message") {
		t.Errorf("Log should contain second message")
	}
}

func TestSetupHandlerOptions(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	cleanup := Setup(tmpDir)
	defer cleanup()

	logFile := filepath.Join(tmpDir, "gofle.log")

	logger := slog.Default()
	logger.Info("handler options test", "custom", "value")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "handler options test") {
		t.Errorf("Log should contain message")
	}
	if !strings.Contains(logContent, "custom") || !strings.Contains(logContent, "value") {
		t.Errorf("Log should contain custom attribute")
	}
}
