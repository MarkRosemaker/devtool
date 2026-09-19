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

	bin := build(t)

	// A binary carrying no version at all cannot be ordered against the mark,
	// so on a machine that builds one there is nothing here to exercise.
	//
	// Whether there is a version is the machine's to decide, not this test's:
	// the toolchain stamps the commit it built from, and silently omits it
	// wherever it cannot read git — "go build -buildvcs=true" names the
	// reason instead of staying quiet. Skipping beats failing for something
	// the code under test did not do, and the comparison itself is covered in
	// internal/local, where a test can name a version it is not.
	if v := binVersion(t, bin); strings.Contains(v, "(devel)") {
		t.Skipf("this build carries no version information (%s), so it is behind nothing", v)
	}

	dir := throwawayRepo(t)
	definition := filepath.Join(dir, "devtool.json")

	if err := os.WriteFile(definition,
		[]byte(`{"devtoolVersion":"`+recorded+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), bin, "update", "-jsonl")
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

// binVersion asks the binary what it is, which is the only thing that knows:
// the version comes from the build info the toolchain stamped into it.
func binVersion(t *testing.T, bin string) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), bin, "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("%s -version: %v\n%s", bin, err, out)
	}

	return strings.TrimSpace(string(out))
}
