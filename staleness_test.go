package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestALocalBuildIsNobodysHighWaterMark drives the version record through the
// real binary, over the case a test binary can actually be: a build nobody
// could install.
//
// It is the property the notice rests on from the other side. A build that is
// not published must not become the mark other machines are judged against,
// and must not lower a mark that is already there — otherwise the first local
// run in a repository stops every other machine being told it is behind.
//
// The notice itself is asserted in internal/local, where a test can name a
// version it is not; here it could only ever be a local build.
func TestALocalBuildIsNobodysHighWaterMark(t *testing.T) {
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

	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n--- stderr ---\n%s", err, stderr)
	}

	after, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(after), recorded) {
		t.Errorf("the mark was lowered by a local build:\n%s", after)
	}

	// The run still did its work, and the stream is still only events: the
	// version record is a task like any other.
	if !strings.Contains(stdout.String(), `"kind":"run_done"`) {
		t.Errorf("the run did not finish:\n%s", stdout)
	}
}
