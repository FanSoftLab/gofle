package cmds

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

// TestUsageArgsWrapsFailures verifies the positional-args wrapper classifies
// validator failures as usage errors while letting successes through.
func TestUsageArgsWrapsFailures(t *testing.T) {
	validator := UsageArgs(cobra.ExactArgs(1))
	cmd := &cobra.Command{}

	if err := validator(cmd, []string{"one"}); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}

	err := validator(cmd, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error for wrong arg count")
	}

	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Fatalf("expected *UsageError, got %T: %v", err, err)
	}
}

// TestFlagErrorFuncClassifies verifies flag parse failures are wrapped as
// usage errors.
func TestFlagErrorFuncClassifies(t *testing.T) {
	fn := FlagErrorFunc()

	err := fn(nil, errors.New("unknown flag: --bogus"))
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Fatalf("expected *UsageError, got %T: %v", err, err)
	}
}
