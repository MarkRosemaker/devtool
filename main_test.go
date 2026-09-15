package main

import (
	"errors"
	"strings"
	"testing"
)

// TestRunSaysWhatIsNotHereYet: maintaining repositories has not moved out of
// portfolio, and a usage message alone would read as a mistyped argument.
func TestRunSaysWhatIsNotHereYet(t *testing.T) {
	err := run(t.Context(), nil)
	if !errors.Is(err, errNotYetMoved) {
		t.Fatalf("got %v, want it to report what has not moved", err)
	}

	if !strings.Contains(err.Error(), "portfolio") {
		t.Errorf("the error does not say where it still lives: %v", err)
	}
}

// TestSelfUpdateIsASubcommand keeps "devtool self-update" working without a
// leading dash, which is how patchpal invokes it.
func TestSelfUpdateIsASubcommand(t *testing.T) {
	// A local build is stamped "(devel)", so this reaches selfupdate and is
	// declined there rather than reinstalling anything over the test binary.
	if err := run(t.Context(), []string{"self-update"}); err != nil {
		t.Fatalf("self-update on a local build should decline, not fail: %v", err)
	}
}

func TestVersionFlag(t *testing.T) {
	if err := run(t.Context(), []string{"-version"}); err != nil {
		t.Errorf("-version failed: %v", err)
	}
}
