package maintain

import (
	"context"
	"fmt"
	"slices"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

// UpdateOptions are what the generators cannot work out from the repository.
type UpdateOptions struct {
	// Holder is the copyright holder written into LICENSE.
	Holder string

	// Coverage is the figure the README badge shows and devtool.json records.
	// Zero means nothing was measured for this run: the recorded figure stays,
	// and the README generator falls back to it.
	Coverage float64

	// Version is the devtool build doing the work, recorded only where the
	// run changed something else.
	Version string

	// Description and Topics come off the portfolio's list, and are recorded
	// only where the definition has none of its own: once it has, the
	// repository is where they live.
	Description string
	Topics      []string
}

// UpdateTask returns the one task that brings everything devtool owns up to
// date — LICENSE, README.md, the Makefile, .gitignore, AGENTS.md, CLAUDE.md,
// the lint config and devtool.json — as one commit.
//
// devtool.json is written inside the task, and that is the point of it being
// one. A maintained run commits a task's changes and pushes; a definition
// written after that is left uncommitted, and the next run discards it before
// pulling. Every portfolio run did exactly that, for every repository, until
// this existed.
//
// The same task is the whole of a local rebuild, so the two paths cannot
// disagree about what devtool owns.
func UpdateTask(repo engine.Repo, opts UpdateOptions) engine.Task {
	return engine.Task{
		Name:  "devtool update",
		Short: "update",
		Run: func(ctx context.Context) error {
			return runUpdate(ctx, repo, opts)
		},
	}
}

func runUpdate(ctx context.Context, repo engine.Repo, opts UpdateOptions) error {
	before, err := changedPaths(repo)
	if err != nil {
		return err
	}

	for _, t := range generators(repo, opts) {
		if err := t.Run(ctx); err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
	}

	if err := recordDefinition(repo.Fs(), opts); err != nil {
		return err
	}

	after, err := changedPaths(repo)
	if err != nil {
		return err
	}

	// The version is a passenger. A newer build with no different opinion
	// about this repository has nothing to say about it, and recording it
	// anyway is a commit whose only content is the mark — which, in devtool's
	// own repository, publishes a version for the next run to record, and so
	// on without end.
	if slices.Equal(before, after) {
		return nil
	}

	return RecordVersion(repo.Fs(), opts.Version)
}

// generators are the files devtool owns, in the order they have to be
// written:
//
//   - The licence, README and Makefile first, so a repository is licensed,
//     documented and buildable even if a later generator fails — and so the
//     make targets AGENTS.md points an agent at exist before it is written.
//   - .gitignore after the Makefile, whose targets settle what there is to
//     ignore, and before AGENTS.md, which says so only once the block is
//     really there.
func generators(repo engine.Repo, opts UpdateOptions) []engine.Task {
	return []engine.Task{
		LicenseTask(repo, opts.Holder),
		ReadmeTask(repo, opts.Coverage),
		MakefileTask(repo),
		GitignoreTask(repo),
		AgentsTask(repo),
		ClaudeTask(repo),
		GenLintfile(repo),
	}
}

// changedPaths is what differs from HEAD, sorted so two readings compare.
//
// Paths rather than contents. A run that starts clean — every maintained run,
// and a local one with -commit — is judged exactly. A plain local rebuild may
// start dirty, and there a generator rewriting a file it had already left
// dirty would not register; that would mean the generators are not
// deterministic, which is a fault of its own.
func changedPaths(repo engine.Repo) ([]string, error) {
	files, err := repo.GetChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("reading the worktree: %w", err)
	}

	slices.Sort(files)

	return files, nil
}

// recordDefinition writes what this run knows into devtool.json, leaving the
// file untouched where nothing moved: a byte-identical file is what tells the
// runner there is nothing to commit.
func recordDefinition(fs afero.Fs, opts UpdateOptions) error {
	def, _, err := LoadDefinition(fs)
	if err != nil {
		return err
	}

	next := def

	if opts.Coverage != 0 {
		next.Coverage = opts.Coverage
	}

	if next.Description == "" {
		next.Description = opts.Description
	}

	if len(next.Topics) == 0 {
		next.Topics = opts.Topics
	}

	if next.equal(def) {
		return nil
	}

	return SaveDefinition(fs, next)
}
