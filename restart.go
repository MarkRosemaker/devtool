package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

// restartedEnv marks a process started by restartAs. A build that is still
// behind after updating itself once fails rather than updating again, so a
// self-update that installs something other than what was asked for cannot
// loop.
const restartedEnv = "DEVTOOL_RESTARTED"

// exitStatus ends the process with the status of the build it handed over to,
// which has already said everything there was to say.
type exitStatus int

func (s exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(s)) }

// restarter is the restartAs a run may use, or nil once it has been used.
func restarter() func(context.Context, string) error {
	if os.Getenv(restartedEnv) != "" {
		return nil
	}

	return restartAs
}

// restartAs runs this command line again as binary, the build self-update
// just installed, and hands back how it ended: nil, or the exitStatus to
// leave with.
//
// A child rather than syscall.Exec, so it does not depend on the platform; the
// streams are passed through untouched, so a reader of -json sees one run.
func restartAs(ctx context.Context, binary string) error {
	slog.InfoContext(ctx, "running the command again as the new build", "binary", binary)

	cmd := exec.CommandContext(ctx, binary, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	cmd.Env = append(os.Environ(), restartedEnv+"=1")
	// Stopping this process stops the build it handed over to the same way,
	// rather than killing it mid-push.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }

	err := cmd.Run()

	if exited, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitStatus(max(exited.ExitCode(), 1))
	}

	if err != nil {
		return fmt.Errorf("running %s: %w", binary, err)
	}

	return nil
}
