package main

import (
	"bytes"
	"encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
)

// TestStdoutCarriesOnlyEvents is the guard for the bug that made this stream
// unreadable: anything at all on stdout besides events breaks the consumer
// that parses them.
//
// It has to run the real binary. The failure was in an init function pointing
// the default slog handler at stdout, so every in-process test — which never
// runs a run through main, and whose own logger is whatever the test binary
// set up — was blind to it. This builds the command, runs it over a throwaway
// repository, and reads the two streams apart.
func TestStdoutCarriesOnlyEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and runs git")
	}

	bin := build(t)
	dir := throwawayRepo(t)

	cmd := exec.CommandContext(t.Context(), bin, "update", "-jsonl")
	cmd.Dir = dir

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n--- stderr ---\n%s", err, stderr)
	}

	// Every line, not most of them: one stray line is all it takes, and the
	// line that gave this away was the thirty-seventh of a run.
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	for i, line := range lines {
		if line == "" {
			t.Errorf("stdout line %d is blank", i+1)

			continue
		}

		var ev struct {
			Kind engine.EventKind `json:"kind"`
		}

		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Errorf("stdout line %d is not an event: %q: %v", i+1, line, err)

			continue
		}

		if !known(ev.Kind) {
			t.Errorf("stdout line %d has kind %q, so it is not an event: %q",
				i+1, ev.Kind, line)
		}
	}
}

// TestLogsGoToStderr is the other half, and the one that fails against the
// handler this change replaces: a run that said nothing anywhere would pass
// the purity check above for the wrong reason.
//
// A failing run is the shape that logs on the shortest path — main logs what
// went wrong before returning — so it is what this drives.
func TestLogsGoToStderr(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}

	cmd := exec.CommandContext(t.Context(), build(t), "update", "MarkRosemaker/openapi")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Run(); err == nil {
		t.Fatal("naming a repository without -config should fail")
	}

	if !strings.Contains(stderr.String(), `"level":"ERROR"`) {
		t.Errorf("the failure was not logged to stderr:\n%s", stderr)
	}

	if stdout.Len() != 0 {
		t.Errorf("a run that emitted nothing still wrote to stdout:\n%s", stdout)
	}
}

// known reports whether k is a kind the engine defines, which is what
// separates an event from some other JSON object.
func known(k engine.EventKind) bool {
	switch k {
	case engine.RunStart, engine.RunDone,
		engine.RepoStart, engine.RepoDone,
		engine.TaskStart, engine.TaskDone:
		return true
	default:
		return false
	}
}

// build compiles the command as it ships, so the test sees the init functions
// a user gets.
func build(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), name)

	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("building %s: %v\n%s", name, err, out)
	}

	return bin
}

// throwawayRepo makes the smallest thing a local run will work on: a git
// repository with a Go module in it. No network and no token, which is why
// this shape of run is the one worth testing.
func throwawayRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	write := func(name, content string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
	write("doc.go", "// Package throwaway is here so the module has one.\npackage throwaway\n")

	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "devtool test"},
		{"config", "user.email", "devtool@example.com"},
		{"add", "."},
		{"commit", "-m", "initial"},
	} {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	return dir
}
