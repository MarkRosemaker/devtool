package main

import (
	"strings"
	"testing"
)

// TestSelfUpdateIsASubcommand keeps "devtool self-update" working without a
// leading dash, which is how patchpal invokes it. A test binary is a local
// build, so selfupdate declines rather than reinstalling anything over it.
func TestSelfUpdateIsASubcommand(t *testing.T) {
	if err := dispatch(t.Context(), []string{"self-update"}); err != nil {
		t.Fatalf("self-update on a local build should decline, not fail: %v", err)
	}
}

func TestVersionFlag(t *testing.T) {
	if err := dispatch(t.Context(), []string{"-version"}); err != nil {
		t.Errorf("-version failed: %v", err)
	}
}

func TestUnknownCommand(t *testing.T) {
	err := dispatch(t.Context(), []string{"frobnicate"})
	if err == nil {
		t.Fatal("an unknown command was accepted")
	}

	if !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("the error does not name the command: %v", err)
	}
}

// TestNamingARepositoryNeedsAList: without -config there is only the
// repository you are standing in, so naming another one is a mistake worth
// catching before anything runs.
func TestNamingARepositoryNeedsAList(t *testing.T) {
	err := dispatch(t.Context(), []string{"update", "MarkRosemaker/openapi"})
	if err == nil {
		t.Fatal("a repository was accepted without a list")
	}

	if !strings.Contains(err.Error(), "-config") {
		t.Errorf("the error does not say what is missing: %v", err)
	}
}

func TestMaintainedRejectsAMalformedRepository(t *testing.T) {
	for _, target := range []string{"nope", "/beta", "user/", "a/b/c"} {
		err := maintained(t.Context(), "does-not-exist.json", target, false, emitter(false))
		if err == nil {
			t.Errorf("%q was accepted", target)
		}
	}
}

func TestEmitter(t *testing.T) {
	if emitter(false) == nil || emitter(true) == nil {
		t.Error("an emitter is always needed, even when nothing reads it")
	}
}
