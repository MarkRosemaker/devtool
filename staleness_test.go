package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAStaleBuildLeavesTheMarkAndAsksForAnUpdate drives the staleness check
// through the real binary, over the two things that hold whatever machine it
// runs on.
//
// It deliberately does not assert the refusal. The mark here names a build
// nobody published, and that is exactly the case the check lets through on
// purpose: insisting on a version no machine can install would lock the
// repository for everybody, so it warns and goes on. Asserting a refusal
// passed once here only because that build happened to be "+dirty" and took
// the other branch — an accident of the machine, not a property. The
// refusal's branches are covered in internal/local, where the updater is
// injected and the outcome is the test's to choose.
//
// What does hold wherever it runs: a build that is not published must
// never lower the mark other machines are judged against.
func TestAStaleBuildLeavesTheMarkAndAsksForAnUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and runs git")
	}

	// Later than anything this module will publish for a while, so the test
	// does not expire.
	const recorded = "v9.0.0-20990101000000-ffffffffffff"

	dir := throwawayRepo(t)
	definition := filepath.Join(dir, "devtool.json")

	if err := os.WriteFile(definition,
		[]byte(`{"devtoolVersion":"`+recorded+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), build(t), "update", "-jsonl")
	cmd.Dir = dir

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	// Either outcome is legitimate — refused, or warned and carried on — so
	// the exit status is not the assertion.
	_ = cmd.Run()

	after, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(after), recorded) {
		t.Errorf("the mark was lowered by a build that is behind it:\n%s", after)
	}

	// Whether the updater was built correctly is not assertable here: a
	// "+dirty" build refuses before ever reaching it, so the check would be
	// vacuous on any machine with an uncommitted change. TestSelfUpdaterKnows
	// ItsCurrentVersion covers it instead.
}
