package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

// TestKeepCoverage: a local rebuild does not run the tests, so writing zero
// would quietly downgrade the badge of every repository somebody ran it in.
func TestKeepCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		readme string
		want   float64
		absent bool
	}{
		{
			name:   "reads what the badge claims",
			readme: "![Code Coverage](https://img.shields.io/badge/coverage-80%25-yellow)\n",
			want:   80,
		},
		{
			name:   "a fractional figure",
			readme: "![Code Coverage](https://img.shields.io/badge/coverage-34.7%25-orange)\n",
			want:   34.7,
		},
		{
			name:   "no badge at all",
			readme: "# thing\n",
			want:   0,
		},
		{name: "no README at all", absent: true, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if !tc.absent {
				if err := afero.WriteFile(fs, "README.md", []byte(tc.readme), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if got := keepCoverage(fs); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUpdateRejectsSomethingThatIsNotARepository(t *testing.T) {
	err := Update(t.Context(), t.TempDir(), Options{}, nopEmitter{})
	if err == nil {
		t.Fatal("a directory that is not a repository was accepted")
	}
}

type nopEmitter struct{}

func (nopEmitter) Emit(engine.Event) {}

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
