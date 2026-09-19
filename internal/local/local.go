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
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/MarkRosemaker/devtool-engine/event"
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/devtool-engine/selfupdate"
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

	// Version is the build doing the work, as the toolchain reports it. It is
	// recorded in the repository's definition and compared against what is
	// already there, which is how a build that is behind gets told so.
	//
	// A parameter rather than a call to selfupdate.Version, because a test
	// has to be able to be a build it is not.
	Version string

	// SelfUpdate fetches the newest published build, and is called only when
	// the run finds itself behind the one that last maintained the
	// repository. Nil means not to try, and the refusal then says to update
	// by hand.
	//
	// A function rather than the updater itself, so a test can be behind
	// without a network, and so the caller keeps its own say over which
	// module and whether to go direct.
	SelfUpdate func(context.Context) (selfupdate.Outcome, error)
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

	if err := refuseIfBehind(ctx, r.fs, opts); err != nil {
		return err
	}

	// Captured before anything runs, so the version task can tell whether
	// this run changed the repository at all.
	before, err := r.state(ctx)
	if err != nil {
		return err
	}

	if opts.Commit {
		return commitRun(ctx, r, opts, before, events)
	}

	event.Emit(events, event.Event{Kind: event.RunStart, Repos: []string{r.String()}})
	defer event.Emit(events, event.Event{Kind: event.RunDone})

	event.Emit(events, event.Event{Kind: event.RepoStart, Repo: r.String()})

	for _, t := range tasks(r, opts, before) {
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
func tasks(r *repo, opts Options, before string) []engine.Task {
	return []engine.Task{
		maintain.LicenseTask(r, opts.Holder),
		maintain.ReadmeTask(r, opts.Coverage),
		maintain.MakefileTask(r),
		maintain.GitignoreTask(r),
		maintain.AgentsTask(r),
		maintain.ClaudeTask(r),
		maintain.GenLintfile(r),
		maintain.VersionTask(r, opts.Version, func() (bool, error) {
			now, err := r.state(context.Background())

			return now != before, err
		}),
	}
}

// refuseIfBehind stops a run whose generators are older than the ones that
// last maintained this repository, fetching the newer build on the way out.
//
// The comparison itself costs nothing: it is against a version already on
// disk, put there by whichever build last ran here.
//
// This was a notice, on the reasoning that files written by a build which is
// behind are corrected by the next run that is not. That reasoning was wrong.
// Behind is not a worse opinion, it is a different one, so the older
// generators rewrite what the newer ones wrote and the repository goes
// backwards — which happened here: a stale devtool reverted twenty-five lines
// of this repository's own generated files, and the warning in the log did
// not stop it. Refusing does, and it puts the update in front of whoever runs
// next rather than leaving it to them to notice.
//
// The one thing it must never do is refuse forever. A repository recording a
// version nobody can install would be unusable on every machine, so a record
// this build cannot catch up to goes back to being a notice.
func refuseIfBehind(ctx context.Context, fs afero.Fs, opts Options) error {
	recorded, err := maintain.RecordedVersion(fs)
	if err != nil {
		slog.DebugContext(ctx, "could not read the repository's definition", "error", err)

		return nil
	}

	if !maintain.Outdated(opts.Version, recorded) {
		return nil
	}

	// A local build cannot be replaced by self-update — it refuses, rightly,
	// to overwrite something nobody published — so the remedy is the other
	// one. This is the case that actually bit: the binary that reverted this
	// repository's generated files was a "+dirty" build a day behind.
	if !maintain.Installable(opts.Version) {
		return behind(opts.Version, recorded,
			"it is a local build, so rebuild it from a current checkout")
	}

	if opts.SelfUpdate == nil {
		return behind(opts.Version, recorded, "run "+updateHint)
	}

	slog.InfoContext(ctx, "behind the build that last maintained this repository, updating",
		"running", opts.Version, "recorded", recorded)

	out, err := opts.SelfUpdate(ctx)

	switch {
	case err != nil:
		return behind(opts.Version, recorded, fmt.Sprintf("could not update: %v", err))
	case out.Updated:
		return behind(opts.Version, recorded,
			fmt.Sprintf("updated to %s, so run the command again", out.Latest))
	}

	// There is nothing newer to fetch, so the record names a build that was
	// never published — a local one that leaked into the file, most likely.
	// Refusing on it would lock the repository for everybody.
	slog.WarnContext(ctx, "this repository records a devtool build that cannot be installed",
		"running", opts.Version, "recorded", recorded, "reason", out.Reason)

	return nil
}

// updateHint is the command that fixes it, named once.
const updateHint = "devtool self-update"

// behind builds the refusal, saying what is wrong before what to do about it.
func behind(current, recorded, remedy string) error {
	return fmt.Errorf(
		"devtool %s is behind %s, which last maintained this repository: "+
			"its generators would write over the newer ones' output; %s",
		current, recorded, remedy,
	)
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
	ctx context.Context, r *repo, opts Options, before string, events event.Emitter,
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
			return tasks(r, opts, before)
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
