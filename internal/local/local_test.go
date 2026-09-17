package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MarkRosemaker/devtool-engine/event"
)

func TestUpdateRejectsSomethingThatIsNotARepository(t *testing.T) {
	err := Update(t.Context(), t.TempDir(), Options{}, nopEmitter{})
	if err == nil {
		t.Fatal("a directory that is not a repository was accepted")
	}
}

type nopEmitter struct{}

func (nopEmitter) Emit(event.Event) {}

// TestCommitRefusesADirtyWorktree is the one that matters. The engine's runner
// begins by discarding whatever it finds uncommitted — correct for a checkout
// it owns on a server, catastrophic for the one somebody is working in. This
// asserts the refusal happens before anything is touched.
func TestCommitRefusesADirtyWorktree(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "t@example.com")
	git(t, dir, "config", "user.name", "T")

	write(t, dir, "go.mod", "module example.com/thing\n\ngo 1.27.0\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "first")

	// Somebody's work in progress.
	const precious = "package thing // do not lose me\n"
	write(t, dir, "thing.go", precious)

	err := Update(t.Context(), dir, Options{Commit: true}, nopEmitter{})
	if err == nil {
		t.Fatal("a dirty worktree was accepted")
	}

	if !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("unexpected error: %v", err)
	}

	got, readErr := os.ReadFile(filepath.Join(dir, "thing.go"))
	if readErr != nil {
		t.Fatalf("the uncommitted file is gone: %v", readErr)
	}

	if string(got) != precious {
		t.Errorf("the uncommitted file was changed:\n%s", got)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTotalCoverage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		out     string
		want    float64
		wantErr bool
	}{
		{
			name: "reads the total",
			out:  "x.go:1:\tfoo\t100.0%\ntotal:\t(statements)\t83.5%\n",
			want: 83.5,
		},
		{name: "no total line", out: "x.go:1:\tfoo\t100.0%\n", wantErr: true},
		{name: "empty", out: "", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := totalCoverage(tc.out)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPorcelainPath: the engine matches these against paths, so a line left
// with its two status characters on the front would never equal the file it
// names — and every change would look unclaimed, which is the safe answer but
// the wrong one.
func TestPorcelainPath(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{" M README.md", "README.md"},
		{"?? AGENTS.md", "AGENTS.md"},
		{"M  mk/extra.mk", "mk/extra.mk"},
		{"A  thing.go", "thing.go"},
		{"R  old.go -> new.go", "new.go"},
		{`?? "a file with spaces.md"`, "a file with spaces.md"},
		{"", ""},
		{" M", ""},
	} {
		t.Run(tc.line, func(t *testing.T) {
			if got := porcelainPath(tc.line); got != tc.want {
				t.Errorf("porcelainPath(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// TestGetChangedFilesAgainstRealGit runs the parser against output git
// actually produced, rather than against lines written from memory.
func TestGetChangedFilesAgainstRealGit(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "t@example.com")
	git(t, dir, "config", "user.name", "T")
	write(t, dir, "kept.go", "package thing\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "first")

	write(t, dir, "README.md", "# thing\n")
	write(t, dir, "kept.go", "package thing // changed\n")
	write(t, dir, "a file with spaces.md", "x\n")

	r := &repo{dir: dir}

	got, err := r.GetChangedFiles()
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"README.md": true, "kept.go": true, "a file with spaces.md": true,
	}

	if len(got) != len(want) {
		t.Fatalf("got %d files %q, want %d", len(got), got, len(want))
	}

	for _, f := range got {
		if !want[f] {
			t.Errorf("unexpected or unparsed path %q in %q", f, got)
		}
	}
}

// TestGoTestCoverWithNoPackages covers the shape a repository takes once
// everything in it has moved elsewhere: a module kept only so a run does not
// recreate one, with no Go files left. "go test ./..." calls that an error,
// which is not the same as the tests having failed.
func TestGoTestCoverWithNoPackages(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/empty\n\ngo 1.27.0\n")

	got, err := (&repo{dir: dir}).GoTestCover(t.Context())
	if err != nil {
		t.Fatalf("a module with nothing to test should not be a failure: %v", err)
	}

	if got != 0 {
		t.Errorf("coverage = %v, want 0", got)
	}
}
