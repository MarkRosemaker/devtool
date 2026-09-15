// Package local rebuilds the generated files of the repository you are
// standing in.
//
// No GitHub, no network, no commits: it writes the files and leaves the
// worktree dirty for whoever ran it to read the diff and decide. That is the
// difference between this and a maintained run, which tests, commits each task
// separately and pushes — one is a person asking for their README to be
// brought up to date, the other is unattended.
package local

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/devtool/internal/remote"
	"github.com/MarkRosemaker/devtool/maintain"
	"github.com/spf13/afero"
)

// repo is a [engine.Repo] backed by a directory on disk.
//
// The operations a maintained run needs and this one does not — pulling,
// pushing, committing, telling GitHub about a description — do nothing. They
// are unreachable: the tasks below never call them, and no Runner is involved
// to do it on their behalf.
type repo struct {
	dir     string
	owner   string
	name    string
	private bool
	fs      afero.Fs
}

func (r *repo) Owner() string    { return r.owner }
func (r *repo) Name() string     { return r.name }
func (r *repo) String() string   { return r.owner + "/" + r.name }
func (r *repo) Private() bool    { return r.private }
func (r *repo) Fs() afero.Fs     { return r.fs }
func (r *repo) HardReset() error { return nil }
func (r *repo) Clean() error     { return nil }

func (r *repo) ExecCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.dir

	return cmd.CombinedOutput()
}

func (r *repo) Pull(context.Context) error                   { return nil }
func (r *repo) Push(context.Context) error                   { return nil }
func (r *repo) CommitAll(string) error                       { return nil }
func (r *repo) GetChangedFiles() ([]string, error)           { return nil, nil }
func (r *repo) IsGoRepo() (bool, error)                      { return true, nil }
func (r *repo) GoModInit(context.Context) error              { return nil }
func (r *repo) GoTestCover(context.Context) (float64, error) { return 0, nil }
func (r *repo) SetDescription(context.Context, string) error { return nil }
func (r *repo) SetTopics(context.Context, []string) error    { return nil }

var _ engine.Repo = (*repo)(nil)

// Options are what cannot be worked out from the directory itself.
type Options struct {
	// Holder is the copyright holder written into LICENSE.
	Holder string

	// Private marks a repository whose README should not carry the badges a
	// public one gets. There is no way to ask git, and this runs without
	// GitHub, so it is the caller's to say.
	Private bool

	// Coverage is the figure the README badge shows. Left at zero, whatever
	// the current README already claims is carried across.
	Coverage float64
}

// coverageBadge matches the figure in a README this tool wrote, which is the
// only place a local rebuild can learn it from.
var coverageBadge = regexp.MustCompile(`shields\.io/badge/coverage-([0-9.]+)%`)

// keepCoverage reads the coverage out of the README already in dir.
//
// A local rebuild does not measure coverage — that means running the tests —
// and writing zero would quietly downgrade the badge of every repository
// somebody ran this in.
func keepCoverage(fs afero.Fs) float64 {
	existing, err := afero.ReadFile(fs, "README.md")
	if err != nil {
		return 0
	}

	m := coverageBadge.FindSubmatch(existing)
	if m == nil {
		return 0
	}

	pct, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0
	}

	return pct
}

// Update rebuilds dir's generated files, emitting an event per task.
func Update(ctx context.Context, dir string, opts Options, events engine.Emitter) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", dir, err)
	}

	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return fmt.Errorf("%s is not a git repository", abs)
	}

	owner, name := identify(ctx, abs)

	r := &repo{
		dir:     abs,
		owner:   owner,
		name:    name,
		private: opts.Private,
		fs:      afero.NewBasePathFs(afero.NewOsFs(), abs),
	}

	if opts.Coverage == 0 {
		opts.Coverage = keepCoverage(r.fs)
	}

	engine.Emit(events, engine.Event{Kind: engine.RunStart, Repos: []string{r.String()}})
	defer engine.Emit(events, engine.Event{Kind: engine.RunDone})

	engine.Emit(events, engine.Event{Kind: engine.RepoStart, Repo: r.String()})

	for _, t := range tasks(r, opts) {
		engine.Emit(events, engine.Event{
			Kind: engine.TaskStart, Repo: r.String(), Task: t.Short,
		})

		if err := t.Run(ctx); err != nil {
			engine.Emit(events, engine.Event{
				Kind: engine.RepoDone, Repo: r.String(),
				Err: fmt.Sprintf("%s: %v", t.Name, err),
			})

			return fmt.Errorf("%s: %w", t.Name, err)
		}

		engine.Emit(events, engine.Event{
			Kind: engine.TaskDone, Repo: r.String(), Task: t.Short,
		})
	}

	engine.Emit(events, engine.Event{Kind: engine.RepoDone, Repo: r.String()})

	return nil
}

// tasks is what a local rebuild runs: the generators, and nothing that needs
// the network or changes code.
func tasks(r engine.Repo, opts Options) []engine.Task {
	return []engine.Task{
		maintain.LicenseTask(r, opts.Holder),
		maintain.ReadmeTask(r, opts.Coverage),
		maintain.MakefileTask(r),
		maintain.GitignoreTask(r),
		maintain.AgentsTask(r),
		maintain.ClaudeTask(r),
		maintain.GenLintfile(r),
	}
}

// identify names the repository from its git remote, falling back to the
// directory name: a checkout without a remote is still worth rebuilding.
func identify(ctx context.Context, dir string) (owner, name string) {
	owner, name, ok := remote.Of(ctx, dir)
	if !ok {
		return "", filepath.Base(dir)
	}

	return owner, name
}
