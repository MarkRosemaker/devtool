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
	"path/filepath"
	"strings"

	"github.com/MarkRosemaker/devtool-engine/event"
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/devtool/internal/remote"
	"github.com/MarkRosemaker/devtool/maintain"
	"github.com/spf13/afero"
)

// Options are what cannot be worked out from the directory itself.
type Options struct {
	// Holder is the copyright holder written into LICENSE.
	Holder string

	// Private marks a repository whose README should not carry the badges a
	// public one gets. There is no way to ask git, and this runs without
	// GitHub, so it is the caller's to say.
	Private bool

	// Coverage is the figure the README badge shows. Left at zero, the
	// generator uses what README/badges.md records, and failing that what the
	// current badge already claims.
	Coverage float64

	// Commit turns the rebuild into a run: test, commit each task that
	// changed something, and push once at the end, which is what a
	// maintained run does. Without it nothing is committed and the worktree
	// is left for whoever ran it to read.
	Commit bool
}

// Update rebuilds dir's generated files, emitting an event per task.
func Update(ctx context.Context, dir string, opts Options, events event.Emitter) error {
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

	if opts.Commit {
		return commitRun(ctx, r, opts, events)
	}

	event.Emit(events, event.Event{Kind: event.RunStart, Repos: []string{r.String()}})
	defer event.Emit(events, event.Event{Kind: event.RunDone})

	event.Emit(events, event.Event{Kind: event.RepoStart, Repo: r.String()})

	for _, t := range tasks(r, opts) {
		event.Emit(events, event.Event{
			Kind: event.TaskStart, Repo: r.String(), Task: t.Short,
		})

		if err := t.Run(ctx); err != nil {
			event.Emit(events, event.Event{
				Kind: event.RepoDone, Repo: r.String(),
				Err: fmt.Sprintf("%s: %v", t.Name, err),
			})

			return fmt.Errorf("%s: %w", t.Name, err)
		}

		event.Emit(events, event.Event{
			Kind: event.TaskDone, Repo: r.String(), Task: t.Short,
		})
	}

	event.Emit(events, event.Event{Kind: event.RepoDone, Repo: r.String()})

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

// commitRun hands the repository to the engine's runner, which tests, commits
// each task that changed something, and pushes once at the end.
//
// It refuses a dirty worktree, and that refusal is the whole reason this is
// not simply the same call with a flag: the runner starts by discarding
// whatever it finds uncommitted, which is correct for a checkout it owns on a
// server and catastrophic for the one somebody is working in.
func commitRun(
	ctx context.Context, r *repo, opts Options, events event.Emitter,
) error {
	files, err := r.GetChangedFiles()
	if err != nil {
		return err
	}

	if len(files) > 0 {
		return fmt.Errorf(
			"the worktree has uncommitted changes and -commit would discard them; "+
				"commit or stash first (%d: %s)",
			len(files), strings.Join(files, ", "),
		)
	}

	event.Emit(events, event.Event{Kind: event.RunStart, Repos: []string{r.String()}})
	defer event.Emit(events, event.Event{Kind: event.RunDone})

	res := (&engine.Runner{Inert: maintain.Inert}).Update(ctx, r,
		engine.Spec{Coverage: opts.Coverage},
		func(*engine.Runner, engine.Repo, engine.Spec) []engine.Task {
			return tasks(r, opts)
		}, events)

	return res.Err
}

// Test runs the repository's tests, records the coverage they report in
// README/badges.md, and rewrites the README if the badge moved.
//
// Separate from Update because measuring is the expensive half: a rebuild of
// the generated files should not need the test suite, and a measurement should
// not need a reason.
func Test(ctx context.Context, dir string, opts Options, events event.Emitter) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", dir, err)
	}

	owner, name := identify(ctx, abs)

	r := &repo{
		dir:     abs,
		owner:   owner,
		name:    name,
		private: opts.Private,
		fs:      afero.NewBasePathFs(afero.NewOsFs(), abs),
	}

	event.Emit(events, event.Event{Kind: event.RunStart, Repos: []string{r.String()}})
	defer event.Emit(events, event.Event{Kind: event.RunDone})

	event.Emit(events, event.Event{Kind: event.RepoStart, Repo: r.String()})
	event.Emit(events, event.Event{Kind: event.TaskStart, Repo: r.String(), Task: "test"})

	pct, err := r.GoTestCover(ctx)
	if err != nil {
		event.Emit(events, event.Event{
			Kind: event.RepoDone, Repo: r.String(), Err: err.Error(),
		})

		return err
	}

	event.Emit(events, event.Event{Kind: event.TaskDone, Repo: r.String(), Task: "test"})

	if err := maintain.RecordCoverage(r.fs, pct); err != nil {
		return err
	}

	// The badge is rendered from the figure just recorded, so this is what
	// makes the README agree with it.
	if err := maintain.ReadmeTask(r, pct).Run(ctx); err != nil {
		return fmt.Errorf("rewriting the README: %w", err)
	}

	event.Emit(events, event.Event{
		Kind: event.RepoDone, Repo: r.String(), Coverage: pct,
	})

	return nil
}
