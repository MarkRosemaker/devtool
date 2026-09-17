package local

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

// repo is an [engine.Repo] backed by a checkout on this machine.
//
// Everything it does, it does through git and the Go toolchain in that
// directory. What it cannot do is anything on GitHub: setting a description or
// topics needs a token and an API this command deliberately does not use, so
// those report success and change nothing.
type repo struct {
	dir     string
	owner   string
	name    string
	private bool
	fs      afero.Fs
}

func (r *repo) Owner() string { return r.owner }
func (r *repo) Name() string  { return r.name }
func (r *repo) Private() bool { return r.private }
func (r *repo) Fs() afero.Fs  { return r.fs }

func (r *repo) String() string {
	if r.owner == "" {
		return r.name
	}

	return r.owner + "/" + r.name
}

// git runs one git command in the repository and returns its output.
func (r *repo) git(ctx context.Context, args ...string) ([]byte, error) {
	out, err := r.ExecCommand(ctx, "git", args...)
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return out, nil
}

func (r *repo) ExecCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.dir

	return cmd.CombinedOutput()
}

// GetChangedFiles is what the runner reads to decide whether a task did
// anything, so it reports the worktree rather than a fixed answer.
func (r *repo) GetChangedFiles() ([]string, error) {
	out, err := r.git(context.Background(), "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var files []string

	// Trimmed of newlines only: the first column of a porcelain line is a
	// status character that is a space for an unstaged change, and trimming
	// the output as a whole eats it, shifting the path of the first line.
	for line := range strings.SplitSeq(strings.Trim(string(out), "\n"), "\n") {
		if path := porcelainPath(line); path != "" {
			files = append(files, path)
		}
	}

	return files, nil
}

// porcelainPath pulls the path out of one "git status --porcelain" line.
//
// The two status characters and a space come first, and a rename reads
// "old -> new", of which the new name is the one that exists now. Callers
// match these against paths, so the prefix has to go: a line left whole would
// never equal the file it names.
func porcelainPath(line string) string {
	if len(line) < 4 {
		return ""
	}

	path := strings.TrimSpace(line[3:])
	if _, after, found := strings.Cut(path, " -> "); found {
		path = after
	}

	// A path with a space or an oddity in it is quoted by git.
	if unquoted, err := strconv.Unquote(path); err == nil {
		return unquoted
	}

	return path
}

func (r *repo) CommitAll(msg string) error {
	ctx := context.Background()

	if _, err := r.git(ctx, "add", "-A"); err != nil {
		return err
	}

	_, err := r.git(ctx, "commit", "-m", msg)

	return err
}

func (r *repo) Pull(ctx context.Context) error {
	_, err := r.git(ctx, "pull", "--ff-only")

	return err
}

func (r *repo) Push(ctx context.Context) error {
	_, err := r.git(ctx, "push")

	return err
}

func (r *repo) HardReset() error {
	_, err := r.git(context.Background(), "reset", "--hard")

	return err
}

func (r *repo) Clean() error {
	_, err := r.git(context.Background(), "clean", "-fd")

	return err
}

func (r *repo) IsGoRepo() (bool, error) {
	_, err := os.Stat(filepath.Join(r.dir, "go.mod"))
	if os.IsNotExist(err) {
		return false, nil
	}

	return err == nil, err
}

func (r *repo) GoModInit(ctx context.Context) error {
	_, err := r.ExecCommand(ctx, "go", "mod", "init")

	return err
}

// GoTestCover runs the tests and reports the total statement coverage.
func (r *repo) GoTestCover(ctx context.Context) (float64, error) {
	profile := filepath.Join(r.dir, coverProfile)
	defer func() { _ = os.Remove(profile) }()

	if out, err := r.ExecCommand(ctx,
		"go", "test", "-coverprofile="+coverProfile, "./..."); err != nil {
		// A module with no packages is not a failure, only nothing to measure.
		if strings.Contains(string(out), "matched no packages") {
			return 0, nil
		}

		return 0, fmt.Errorf("tests failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	out, err := r.ExecCommand(ctx, "go", "tool", "cover", "-func="+coverProfile)
	if err != nil {
		return 0, fmt.Errorf("reading the coverage profile: %w: %s",
			err, strings.TrimSpace(string(out)))
	}

	return totalCoverage(string(out))
}

// coverProfile is written into the repository and removed again. The generated
// .gitignore already covers it, which is why it can go there rather than
// somewhere temporary: "go tool cover" wants a path the module can see.
const coverProfile = "cover.out"

// totalCoverage reads the figure off the last line of "go tool cover -func",
// which is the total and is the only line worth keeping.
func totalCoverage(out string) (float64, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "total:") {
		return 0, fmt.Errorf("no total in the coverage output: %q", last)
	}

	pct := strings.TrimSuffix(last[strings.LastIndex(last, "\t")+1:], "%")

	f, err := strconv.ParseFloat(strings.TrimSpace(pct), 64)
	if err != nil {
		return 0, fmt.Errorf("reading the coverage total from %q: %w", last, err)
	}

	return f, nil
}

// SetDescription and SetTopics are GitHub's, and this command does not talk to
// GitHub. A maintained run is where a repository's metadata gets pushed.
func (r *repo) SetDescription(context.Context, string) error { return nil }
func (r *repo) SetTopics(context.Context, []string) error    { return nil }

var _ engine.Repo = (*repo)(nil)
