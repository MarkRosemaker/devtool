package main

import (
	"flag"
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
		err := maintained(t.Context(), "does-not-exist.json", target, true, false, emitter(false))
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

// TestUpdateWithAListNeedsATarget: "devtool update" with nothing after it
// rebuilds the repository you are standing in, so with a list it has to say
// which — maintaining thirty repositories unattended is not what somebody who
// typed two words and forgot the third meant.
func TestUpdateWithAListNeedsATarget(t *testing.T) {
	err := dispatch(t.Context(), []string{"update", "-config=somewhere.json"})
	if err == nil {
		t.Fatal("a list with no target was accepted")
	}

	if !strings.Contains(err.Error(), "all") {
		t.Errorf("the error does not say what to name: %v", err)
	}
}

// TestMaintainedNeedsAllSpelled: only "all" means all.
func TestMaintainedNeedsAllSpelled(t *testing.T) {
	if err := maintained(t.Context(), "does-not-exist.json", "", true, false, emitter(false)); err == nil {
		t.Error("an empty target was taken for all")
	}
}

// TestMaintainedNeedsCommit: a maintained run commits and pushes to every
// repository on the list, so it says so out loud rather than being the
// default.
func TestMaintainedNeedsCommit(t *testing.T) {
	err := maintained(t.Context(), "somewhere.json", "all", false, false, emitter(false))
	if err == nil {
		t.Fatal("a maintained run without -commit was accepted")
	}

	if !strings.Contains(err.Error(), "-commit") {
		t.Errorf("the error does not say what is missing: %v", err)
	}
}

// TestParseFlagsAfterAPositional is the bug this exists to prevent. Go's flag
// package stops at the first argument that is not a flag, so
// "update all -config=... -commit -jsonl" parsed "all" and silently ignored
// every flag after it — which is exactly how a person, and patchpal, type it.
func TestParseFlagsAfterAPositional(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantCfg    string
		wantCommit bool
		wantPos    []string
	}{
		{
			name:    "flags after the positional",
			args:    []string{"all", "-config=c.json", "-commit"},
			wantCfg: "c.json", wantCommit: true, wantPos: []string{"all"},
		},
		{
			name:    "flags before it",
			args:    []string{"-config=c.json", "-commit", "all"},
			wantCfg: "c.json", wantCommit: true, wantPos: []string{"all"},
		},
		{
			name:    "on both sides",
			args:    []string{"-commit", "all", "-config=c.json"},
			wantCfg: "c.json", wantCommit: true, wantPos: []string{"all"},
		},
		{
			name:    "a flag value given as its own argument",
			args:    []string{"all", "-config", "c.json"},
			wantCfg: "c.json", wantPos: []string{"all"},
		},
		{
			name:    "no positional at all",
			args:    []string{"-config=c.json"},
			wantCfg: "c.json", wantPos: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			cfg := fs.String("config", "", "")
			commit := fs.Bool("commit", false, "")

			pos, err := parseFlags(fs, tc.args)
			if err != nil {
				t.Fatal(err)
			}

			if *cfg != tc.wantCfg {
				t.Errorf("-config = %q, want %q", *cfg, tc.wantCfg)
			}

			if *commit != tc.wantCommit {
				t.Errorf("-commit = %v, want %v", *commit, tc.wantCommit)
			}

			if strings.Join(pos, ",") != strings.Join(tc.wantPos, ",") {
				t.Errorf("positional = %q, want %q", pos, tc.wantPos)
			}
		})
	}
}

// TestUpdateAllReachesTheList drives the command line as it is actually typed,
// through dispatch, rather than calling maintained with arguments already
// separated. The unit tests all did the latter, which is why the parsing bug
// survived them.
func TestUpdateAllReachesTheList(t *testing.T) {
	err := dispatch(t.Context(),
		[]string{"update", "all", "-config=does-not-exist.json", "-commit", "-jsonl"})
	if err == nil {
		t.Fatal("expected a failure about the list, not success")
	}

	// It must get as far as trying to use the list. The old failure was the
	// parser never seeing -config at all.
	if strings.Contains(err.Error(), "naming a repository needs -config") {
		t.Errorf("the flags after the positional were ignored: %v", err)
	}
}
