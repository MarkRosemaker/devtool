package ghrepo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ExecError represents an error that occurred while executing a command in the repository.
type ExecError struct {
	// Cmd is the full command that was attempted (e.g. "go mod tidy").
	Cmd string
	// Out is the combined stdout and stderr output from the command, if any.
	Out string
	// Err is the underlying error returned by exec.Command.
	Err error
}

// Error returns a string representation of the ExecError, including the command, output, and underlying error.
func (e ExecError) Error() string {
	if e.Out != "" {
		return fmt.Sprintf("command %q failed:\n%s\n%v", e.Cmd, e.Out, e.Err)
	}

	return fmt.Sprintf("command %q failed: %v", e.Cmd, e.Err)
}

// Unwrap allows errors.Is and errors.As to work with ExecError by unwrapping the underlying error.
func (e ExecError) Unwrap() error { return e.Err }

// ExecCommand runs a command in the repository's root directory.
// It returns the combined stdout + stderr as a string.
// The command name and arguments are passed separately (like exec.Command).
func (r *Repository) ExecCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.path

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, ExecError{
			Cmd: strings.Join(append([]string{name}, args...), " "),
			Out: string(bytes.TrimSpace(out)),
			Err: err,
		}
	}

	return out, nil
}
