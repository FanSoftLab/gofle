package main

import (
	"GoFLE/cmd/gofle/internal/cmds"
	"errors"
	"strings"
	"testing"
)

// TestRootNoArgsShowsHelp ensures that invoking gofle with no target prints the
// help (via RunE -> cmd.Help()) instead of erroring or launching the viewer.
func TestRootNoArgsShowsHelp(t *testing.T) {
	rootCmd.SetArgs([]string{})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("root with no args returned error: %v", err)
	}
}

// TestRootSubcommands verifies the consolidated command tree: only ssh and
// recent remain; the removed search/tail/export/view commands are gone. (Their
// absence is also enforced at compile time — referencing them would fail the
// build — but this documents the intended surface.)
func TestRootSubcommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
	}

	for _, want := range []string{"ssh", "list", "version"} {
		if !names[want] {
			t.Errorf("expected subcommand %q to be registered", want)
		}
	}

	for _, removed := range []string{"view", "search", "tail", "export"} {
		if names[removed] {
			t.Errorf("removed subcommand %q should not be registered", removed)
		}
	}
}

// TestUnknownFlagIsUsageError verifies that a flag parse failure surfaces as
// a *cmds.UsageError, which main() maps to exit code 2.
func TestUnknownFlagIsUsageError(t *testing.T) {
	rootCmd.SetArgs([]string{"--bogus-flag"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}

	if _, ok := errors.AsType[*cmds.UsageError](err); !ok {
		t.Fatalf("expected *cmds.UsageError, got %T: %v", err, err)
	}
}

// TestSSHRemoteNowOptional verifies that `gofle ssh somehost` (no --remote) is
// no longer rejected as a required-flag usage error: --remote is optional and an
// omitted value launches the interactive directory browser. The run still fails
// (no stored credentials / non-interactive password read), but NOT on "--remote".
func TestSSHRemoteNowOptional(t *testing.T) {
	rootCmd.SetArgs([]string{"ssh", "somehost"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected a failure for the host-only invocation")
	}

	if strings.Contains(err.Error(), "--remote") {
		t.Fatalf("expected --remote to be optional now; got failure on it: %v", err)
	}

	if ok := func() bool { _, ok := errors.AsType[*cmds.UsageError](err); return ok }(); ok {
		// A usage error is acceptable only if it is about something other than
		// the now-optional --remote flag.
		if strings.Contains(err.Error(), "--remote") {
			t.Fatalf("unexpected --remote usage error: %v", err)
		}
	}
}
