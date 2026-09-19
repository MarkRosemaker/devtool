package local

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MarkRosemaker/devtool-engine/event"
	"github.com/MarkRosemaker/devtool-engine/selfupdate"
	"github.com/MarkRosemaker/devtool/maintain"
	"github.com/spf13/afero"
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

// TestUpdateRecordsTheVersion: the mark a later run reads to know it is behind
// is written by the run before it, so a local rebuild has to leave one.
func TestUpdateRecordsTheVersion(t *testing.T) {
	const current = "v0.0.0-20260917155344-c047bdcfd843"

	dir := committedRepo(t)

	if err := Update(t.Context(), dir, Options{Version: current}, nopEmitter{}); err != nil {
		t.Fatal(err)
	}

	got, err := maintain.RecordedVersion(afero.NewBasePathFs(afero.NewOsFs(), dir))
	if err != nil {
		t.Fatal(err)
	}

	if got != current {
		t.Errorf("recorded %q, want %q", got, current)
	}
}

// TestVersionRidesAlongWithRealWork is the guard against the record walking
// itself forward. A run that changed nothing leaves the mark alone: the newer
// build evidently has no different opinion about this repository, and writing
// the mark anyway would be a commit whose only content is the mark — which in
// devtool's own repository publishes a version for the next run to record,
// and so on without end.
func TestVersionRidesAlongWithRealWork(t *testing.T) {
	const (
		first  = "v0.0.0-20260915161737-9e10ae8f485b"
		second = "v0.0.0-20260917155344-c047bdcfd843"
	)

	dir := committedRepo(t)
	fs := afero.NewBasePathFs(afero.NewOsFs(), dir)

	// The first run generates everything, so it has plainly done work.
	if err := Update(t.Context(), dir, Options{Version: first}, nopEmitter{}); err != nil {
		t.Fatal(err)
	}

	if got, err := maintain.RecordedVersion(fs); err != nil {
		t.Fatal(err)
	} else if got != first {
		t.Fatalf("the first run recorded %q, want %q", got, first)
	}

	// The second finds every generated file already as it wants it, so there
	// is nothing for the mark to ride along with.
	if err := Update(t.Context(), dir, Options{Version: second}, nopEmitter{}); err != nil {
		t.Fatal(err)
	}

	got, err := maintain.RecordedVersion(fs)
	if err != nil {
		t.Fatal(err)
	}

	if got != first {
		t.Errorf("a run that changed nothing moved the mark to %q, want %q left alone",
			got, first)
	}
}

// committedRepo is a repository a local run will work on: the license task
// dates the copyright from the first commit, so there has to be one.
func committedRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "t@example.com")
	git(t, dir, "config", "user.name", "T")

	write(t, dir, "go.mod", "module example.com/thing\n\ngo 1.27.0\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "first")

	return dir
}

// TestUpdateRefusesWhenBehind: a build that is behind does not write. Behind
// is not a worse opinion but a different one, so its generators undo the
// newer ones' work — which is what happened to this repository's own files
// before the notice became a refusal.
//
// In-process rather than through the binary, because a binary a test builds
// is never a published version — that is the whole reason a local build is
// exempt — so only a caller that can name a version it is not can drive this.
func TestUpdateRefusesWhenBehind(t *testing.T) {
	const (
		recorded = "v0.0.0-20260917155344-c047bdcfd843"
		running  = "v0.0.0-20260915161737-9e10ae8f485b"
	)

	dir := committedRepo(t)
	write(t, dir, "devtool.json", `{"devtoolVersion":"`+recorded+`"}`+"\n")

	err := Update(t.Context(), dir, Options{Version: running}, nopEmitter{})
	if err == nil {
		t.Fatal("a build that is behind was allowed to write")
	}

	for _, want := range []string{"behind", running, recorded, "self-update"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}

	// Nothing was written: the refusal comes before the generators, so the
	// repository is exactly as it was found.
	if got := readFileString(t, filepath.Join(dir, "devtool.json")); !strings.Contains(got, recorded) {
		t.Errorf("the definition was rewritten: %s", got)
	}
}

// TestUpdateFetchesTheBuildItIsBehind: the refusal is not a dead end. It
// tries the update first, so whoever runs next — a person or an agent — is
// already carrying the build they were told to get.
func TestUpdateFetchesTheBuildItIsBehind(t *testing.T) {
	const (
		recorded = "v0.0.0-20260917155344-c047bdcfd843"
		running  = "v0.0.0-20260915161737-9e10ae8f485b"
	)

	dir := committedRepo(t)
	write(t, dir, "devtool.json", `{"devtoolVersion":"`+recorded+`"}`+"\n")

	called := 0
	opts := Options{
		Version: running,
		SelfUpdate: func(context.Context) (selfupdate.Outcome, error) {
			called++

			return selfupdate.Outcome{Current: running, Latest: recorded, Updated: true}, nil
		},
	}

	err := Update(t.Context(), dir, opts, nopEmitter{})
	if err == nil {
		t.Fatal("a build that is behind was allowed to write")
	}

	if called != 1 {
		t.Errorf("the updater ran %d times, want 1", called)
	}

	// The process is still the old binary, so the run cannot simply carry on.
	if !strings.Contains(err.Error(), "run the command again") {
		t.Errorf("the refusal does not say what to do next: %v", err)
	}
}

// TestUpdateRunsWhenTheRecordCannotBeInstalled is the guard against locking a
// repository out of every machine. A version nobody published — a local build
// that leaked into the file — must not refuse for ever, so once the update
// reports there is nothing newer, the run goes ahead.
func TestUpdateRunsWhenTheRecordCannotBeInstalled(t *testing.T) {
	const (
		recorded = "v9.0.0-20990101000000-ffffffffffff"
		running  = "v0.0.0-20260915161737-9e10ae8f485b"
	)

	logged := captureLog(t)

	dir := committedRepo(t)
	write(t, dir, "devtool.json", `{"devtoolVersion":"`+recorded+`"}`+"\n")

	opts := Options{
		Version: running,
		SelfUpdate: func(context.Context) (selfupdate.Outcome, error) {
			return selfupdate.Outcome{Current: running, Reason: "already the latest"}, nil
		},
	}

	if err := Update(t.Context(), dir, opts, nopEmitter{}); err != nil {
		t.Fatalf("a record nobody can install must not lock the repository: %v", err)
	}

	if !strings.Contains(logged.String(), "cannot be installed") {
		t.Errorf("it went ahead without saying why:\n%s", logged)
	}
}

// TestUpdateSaysNothingWhenCurrent is the half that keeps this free: a run
// that learns nothing costs nothing and says nothing.
func TestUpdateSaysNothingWhenCurrent(t *testing.T) {
	const current = "v0.0.0-20260917155344-c047bdcfd843"

	for _, tc := range []struct {
		name     string
		recorded string
	}{
		{"nothing recorded", ""},
		{"the same build", current},
		{"an older build", "v0.0.0-20260915161737-9e10ae8f485b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logged := captureLog(t)

			dir := committedRepo(t)
			if tc.recorded != "" {
				write(t, dir, "devtool.json", `{"devtoolVersion":"`+tc.recorded+`"}`+"\n")
			}

			if err := Update(t.Context(), dir, Options{Version: current}, nopEmitter{}); err != nil {
				t.Fatal(err)
			}

			if strings.Contains(logged.String(), "behind") {
				t.Errorf("a run that is not behind said so anyway:\n%s", logged)
			}
		})
	}
}

// captureLog redirects the default logger for one test and hands back what it
// was written, restoring whatever was there before.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	before := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	t.Cleanup(func() { slog.SetDefault(before) })

	return buf
}

// readFileString reads a file the test wrote or a run produced.
func readFileString(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}
