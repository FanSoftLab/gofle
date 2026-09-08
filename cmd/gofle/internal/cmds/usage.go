package cmds

import (
	"github.com/spf13/cobra"
)

// UsageError marks a failure caused by how the CLI was invoked — an unknown
// flag, a bad flag value, or wrong positional arguments — as opposed to a
// runtime failure. main() maps it to exit code 2 (Unix usage convention) so
// scripts can distinguish "I called gofle wrong" from "the data is bad".
type UsageError struct {
	Err error
}

// Error implements the error interface, returning the wrapped usage error.
func (e *UsageError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying usage error so callers can inspect it with
// errors.As.
func (e *UsageError) Unwrap() error { return e.Err }

// NewUsageError wraps err as a usage error.
func NewUsageError(err error) error {
	return &UsageError{Err: err}
}

// UsageArgs wraps a positional-args validator so its failures are classified
// as usage errors.
func UsageArgs(validator cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validator(cmd, args); err != nil {
			return &UsageError{Err: err}
		}

		return nil
	}
}

// FlagErrorFunc returns a cobra flag-error handler that classifies flag parse
// failures as usage errors. Set it on the root command via SetFlagErrorFunc;
// children inherit it.
func FlagErrorFunc() func(*cobra.Command, error) error {
	return func(_ *cobra.Command, err error) error {
		return &UsageError{Err: err}
	}
}
