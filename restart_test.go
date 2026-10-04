package main

import (
	"errors"
	"os"
	"strconv"
	"testing"
)

// childExitEnv makes this test binary stand in for the build restartAs hands
// over to: it exits at once with the status named, or 99 if it was not told
// it had been restarted.
const childExitEnv = "DEVTOOL_TEST_CHILD_EXIT"

func init() {
	code, ok := os.LookupEnv(childExitEnv)
	if !ok {
		return
	}

	if os.Getenv(restartedEnv) == "" {
		os.Exit(99)
	}

	n, _ := strconv.Atoi(code)
	os.Exit(n)
}

func TestRestartAsReportsHowTheNewBuildEnded(t *testing.T) {
	for _, tc := range []struct {
		code string
		want error
	}{
		{"0", nil},
		{"3", exitStatus(3)},
	} {
		t.Run(tc.code, func(t *testing.T) {
			t.Setenv(childExitEnv, tc.code)

			err := restartAs(t.Context(), os.Args[0])
			if !errors.Is(err, tc.want) {
				t.Errorf("restartAs = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRestartAsFailsWhereThereIsNothingToRun(t *testing.T) {
	err := restartAs(t.Context(), "/nonexistent/devtool")

	if _, ok := errors.AsType[exitStatus](err); ok || err == nil {
		t.Errorf("a binary that is not there reads as %v", err)
	}
}

// TestRestartsOnlyOnce: a build that is still behind after handing over once
// refuses instead of updating again, so a self-update that keeps installing
// something stale cannot loop.
func TestRestartsOnlyOnce(t *testing.T) {
	t.Setenv(restartedEnv, "")

	if restarter() == nil {
		t.Error("a first run has nowhere to hand over to")
	}

	t.Setenv(restartedEnv, "1")

	if restarter() != nil {
		t.Error("a restarted build would restart again")
	}
}
