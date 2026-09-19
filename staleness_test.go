package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAStaleLocalBuildIsRefused drives the version check through the real
// binary, over the case a test binary can actually be: a build nobody could
// install, behind the mark the repository carries.
//
// That is not a contrived case. It is the one that happened — a "+dirty"
// build a day old rewrote this repository's own generated files back to what
// its older generators wanted — and the first version of the check missed it,
// because the comparison exempted a local build in both directions rather
// than only when raising the mark.
//
// The mark must survive either way: a build that is not published must never
// become what other machines are judged against, nor lower what is there.
func TestAStaleLocalBuildIsRefused(t *testing.T) {
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

	if err := cmd.Run(); err == nil {
		t.Fatalf("a build behind the mark was allowed to write\n--- stdout ---\n%s", stdout)
	}

	// Self-update cannot replace a build nobody published, so the refusal has
	// to name the remedy that does.
	if !strings.Contains(stderr.String(), "rebuild") {
		t.Errorf("the refusal does not say how to fix it:\n%s", stderr)
	}

	after, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(after), recorded) {
		t.Errorf("the mark was lowered by a local build:\n%s", after)
	}
}
