package run

import (
	"context"
	"fmt"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/gorepo"
	"github.com/go-git/go-git/v6"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
	"golang.org/x/mod/modfile"
)

// repository is one of my repositories, as both the engine and this package
// need it.
//
// [engine.Repo] names only what the engine uses, so the concrete type has
// to be carried here too: the task sequence reaches for gorepo operations the
// engine has no business knowing about — "go mod tidy" is not a thing a
// generic task runner should have heard of.
type repository struct{ *gorepo.Repository }

// The engine's requirements, checked at compile time. This line is what
// replaces a class of bug: when Repo was a concrete type embedding an
// afero.Fs, a missing filesystem method could only be discovered by running
// against a real repository, which is how ClaudeTask shipped broken. Now
// failing to satisfy it does not build.
var _ engine.Repo = (*repository)(nil)

// Fs satisfies [engine.Repo], which asks for the working tree rather than
// embedding it. gorepo embeds the filesystem as a field, so this hands the
// same value over under the name the engine asks by.
func (r *repository) Fs() afero.Fs { return r.Repository.Fs }

// HardReset and Clean satisfy [engine.Repo], which names them without a
// reset mode so the engine needs no git library for the sake of one constant.
// Supplying the constant is this side's job.
func (r *repository) HardReset() error { return r.GitReset(git.HardReset) }
func (r *repository) Clean() error     { return r.GitClean() }

// GoTestCover runs the tests, and where they fail because the module files
// are out of step with the code, settles them and tries once more. A change
// can import a package go.mod does not require or vendor/ does not hold yet,
// as a generated test did on sec-edgar, and that is not the change's fault.
func (r *repository) GoTestCover(ctx context.Context) (float64, error) {
	coverage, err := r.Repository.GoTestCover(ctx)
	if !gorepo.ModulesOutOfStep(err) {
		return coverage, err
	}

	if serr := r.SettleModules(ctx); serr != nil {
		return 0, fmt.Errorf("%w\nsettling the module files: %w", err, serr)
	}

	return r.Repository.GoTestCover(ctx)
}

// maintainedRepo is what a run needs of one repository: what the engine asks
// for, its module's requirements, a commit of named files, and its sequence.
type maintainedRepo interface {
	engine.Repo
	Dependencies() ([]*modfile.Require, error)
	Commit(paths []string, message string) error
	sequence(version string) engine.Sequence

	// The issues a run files about the repository, and their labels.
	FindOpenIssue(ctx context.Context, text string, labels ...string) (*github.Issue, error)
	CreateIssue(ctx context.Context, title, body string, labels ...string) (*github.Issue, error)
	EditIssue(ctx context.Context, number int, title, body string) (*github.Issue, error)
	EnsureLabel(ctx context.Context, name, color, description string) error
	OpenIssues(ctx context.Context, labels ...string) ([]*github.Issue, error)
	CloseIssue(ctx context.Context, number int, comment string) error
}

// repoSource opens repositories and loads their owners' metadata.
type repoSource interface {
	open(ctx context.Context, owner, name string) (maintainedRepo, error)
	prefetch(ctx context.Context, owner string, isOrg bool) error
}

// gitHub is the [repoSource] a run uses: clones under the home directory, with
// their metadata on GitHub.
type gitHub struct{ *gorepo.Service }

func (g gitHub) open(ctx context.Context, owner, name string) (maintainedRepo, error) {
	repo, err := g.NewRepository(ctx, owner, name)
	if err != nil {
		return nil, err
	}

	return &repository{Repository: repo}, nil
}

// prefetch loads an owner's repository metadata in one request, so that opening
// each repository afterwards does not cost one of its own.
func (g gitHub) prefetch(ctx context.Context, owner string, isOrg bool) error {
	if isOrg {
		if err := g.PrefetchOrgRepositories(ctx, owner); err != nil {
			return fmt.Errorf("prefetching repositories for organisation %q: %w", owner, err)
		}

		return nil
	}

	if err := g.PrefetchUserRepositories(ctx, owner); err != nil {
		return fmt.Errorf("prefetching repositories for user %q: %w", owner, err)
	}

	return nil
}
