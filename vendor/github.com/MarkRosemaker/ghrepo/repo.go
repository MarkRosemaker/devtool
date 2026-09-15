package ghrepo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
)

const remoteName = "origin"

// Repository represents a local Git repository linked to a GitHub remote.
type Repository struct {
	muGithub sync.Mutex
	// Use the repository folder as its own file system.
	afero.Fs
	owner         string
	name          string
	path          string // Local filesystem path
	gitrepo       *git.Repository
	defaultBranch plumbing.ReferenceName
	worktree      *git.Worktree
	remote        *git.Remote
	github        *github.Repository
	s             *Service
}

func (r *Repository) String() string { return fmt.Sprintf("%s/%s", r.owner, r.name) }

// HasChanges returns true if there are any unstaged, staged, or untracked changes.
// It returns false only if the working tree is completely clean.
func (r *Repository) HasChanges() (bool, error) {
	status, err := r.worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	return !status.IsClean(), nil
}

// GetChangedFiles returns a list of files that were changed.
func (r *Repository) GetChangedFiles() ([]string, error) {
	s, err := r.worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	changes := []string{}
	for file, status := range s {
		if status.Worktree != git.Unmodified || status.Staging != git.Unmodified {
			changes = append(changes, file)
		}
	}

	return changes, nil
}

// GitStatus returns the git status of the repository.
func (r *Repository) GitStatus() (git.Status, error) { return r.worktree.Status() }

// GitReset performs a git reset in the repository.
func (r *Repository) GitReset(mode git.ResetMode) error {
	return r.worktree.Reset(&git.ResetOptions{Mode: mode})
}

// GitClean performs a git clean in the repository.
func (r *Repository) GitClean() error {
	return r.worktree.Clean(&git.CleanOptions{Dir: true})
}

// Checkout checks out the specified branch.
// func (r *Repository) Checkout(branch string) error {
// 	return r.worktree.Checkout(&git.CheckoutOptions{
// 		Branch: plumbing.NewBranchReferenceName(branch),
// 	})
// }

// IsDefaultBranch returns true if we are on the default branch.
func (r *Repository) IsDefaultBranch() (bool, error) {
	h, err := r.gitrepo.Head()
	if err != nil {
		return false, err
	}

	return h.Name() == r.defaultBranch, nil
}

// CheckoutDefault checks out the default branch (either main or master).
func (r *Repository) CheckoutDefault() error {
	return r.worktree.Checkout(&git.CheckoutOptions{
		Branch: r.defaultBranch,
	})
}

// Pull incorporates changes from a remote repository into the current branch.
func (r *Repository) Pull(ctx context.Context) error {
	if err := r.worktree.PullContext(ctx, &git.PullOptions{
		ClientOptions: r.s.gitOpts,
	}); err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	} else if !errors.Is(err, git.ErrNonFastForwardUpdate) {
		return err
	}

	// We got [git.ErrNonFastForwardUpdate].
	// The error occurs when the local and remote branches have diverged (meaning both have new commits the other doesn't know about). While the Git CLI handles this by performing a merge, go-git simply rejects the operation with a "non-fast-forward update" error.

	if err := r.remote.FetchContext(ctx, &git.FetchOptions{
		RemoteName:    "origin",
		ClientOptions: r.s.gitOpts,
		Force:         true,
	}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetching origin: %w", err)
	}

	ref, err := r.gitrepo.Reference(plumbing.NewRemoteReferenceName("origin", r.defaultBranch.Short()), true)
	if err != nil {
		return fmt.Errorf("getting default branch reference: %w", err)
	}

	if err := r.worktree.Reset(&git.ResetOptions{
		Mode:   git.HardReset,
		Commit: ref.Hash(),
	}); err != nil {
		return fmt.Errorf("resetting: %w", err)
	}

	return nil
}

var errNoDefaultBranch = errors.New("no default branch found")

func getDefaultBranch(r *git.Repository) (plumbing.ReferenceName, error) {
	// Check remote's default branch (origin/HEAD if valid)
	if ref, err := r.Reference(plumbing.NewRemoteReferenceName("origin", "HEAD"), true); err == nil {
		if short, ok := strings.CutPrefix(refString(ref), "refs/remotes/origin/"); ok {
			return plumbing.NewBranchReferenceName(short), nil
		}
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return "", err
	}

	// Check if head is missing, pragmatically assume "main"
	if _, err := r.Head(); errors.Is(err, plumbing.ErrReferenceNotFound) {
		return plumbing.Main, nil
	} else if err != nil {
		return "", err
	}

	// Fallback to existing common branches
	for _, candidate := range []plumbing.ReferenceName{plumbing.Main, plumbing.Master} {
		if _, err := r.Reference(candidate, true); err == nil {
			return candidate, nil
		} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", err
		}
	}

	return "", errNoDefaultBranch
}

func refString(ref *plumbing.Reference) string {
	switch ref.Type() {
	case plumbing.SymbolicReference:
		return ref.Target().String()
	case plumbing.HashReference:
		return ref.Name().String()
	default:
		return ""
	}
}

// Add adds the file contents of a file in the worktree to the index.
// func (r *Repository) Add(path string) error {
// 	_, err := r.worktree.Add(path)
// 	return err
// }

// // Commit stores the current contents of the index in a new commit along with
// // a log message from the user describing the changes.
// func (r *Repository) Commit(msg string) error {
// 	_, err := r.worktree.Commit(msg, &git.CommitOptions{})
// 	return err
// }

// Commit commits all files that match a certain pattern,
// then commits with the given message.
func (r *Repository) Commit(paths []string, message string) error {
	for _, path := range paths {
		if _, err := r.worktree.Add(path); err != nil {
			return fmt.Errorf("failed to add %q to worktree: %w", path, err)
		}
	}

	if _, err := r.worktree.Commit(message, &git.CommitOptions{}); err != nil &&
		!errors.Is(err, git.ErrEmptyCommit) {
		return fmt.Errorf("commit failed: %w", err)
	}

	return nil
}

// Commit adds all changes, commits with the given message.
func (r *Repository) CommitAll(msg string) error {
	return r.Commit([]string{"."}, msg)
}

// Push pushes to the default remote.
func (r *Repository) Push(ctx context.Context) error {
	if err := r.gitrepo.PushContext(ctx, &git.PushOptions{
		RemoteURL:     fmt.Sprintf("https://github.com/%s/%s.git", r.owner, r.name),
		ClientOptions: r.s.gitOpts,
	}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("push failed: %w", err)
	}

	return nil
}

// func (r *Repository) SetTeamPermission(ctx context.Context, teamSlug, permission string) error {
// 	_, err := r.s.github.Teams.AddTeamRepoBySlug(ctx, r.owner, teamSlug, r.owner, r.name,
// 		&github.TeamAddTeamRepoOptions{Permission: permission})
// 	return err
// }

// SetTopics sets the repository topics on GitHub.
func (r *Repository) SetTopics(ctx context.Context, topics []string) error {
	r.muGithub.Lock()
	defer r.muGithub.Unlock()

	if slices.Equal(r.github.Topics, topics) {
		return nil
	}

	updated, _, err := r.s.github.Repositories.ReplaceAllTopics(ctx, r.owner, r.name, topics)
	if err != nil {
		return err
	}

	r.github.Topics = updated

	return nil
}

// Edit edits the repository on GitHub.
func (r *Repository) Edit(ctx context.Context, update *github.Repository) error {
	r.muGithub.Lock()
	defer r.muGithub.Unlock()

	if !hasChanges(r.github, update) {
		return nil
	}

	repo, _, err := r.s.github.Repositories.Edit(ctx, r.owner, r.name, update)
	if err != nil {
		return err
	}

	r.github = repo

	return nil
}

func hasChanges(initial, update *github.Repository) bool {
	uv := reflect.Indirect(reflect.ValueOf(update))
	iv := reflect.Indirect(reflect.ValueOf(initial))

	for i := 0; i < uv.NumField(); i++ {
		uField := uv.Field(i)
		if uField.IsNil() {
			continue // skip nil fields in update
		}

		iField := iv.Field(i)
		if iField.IsNil() {
			return true // update sets a value where initial is nil
		}

		uVal := reflect.Indirect(uField)
		iVal := reflect.Indirect(iField)
		if !reflect.DeepEqual(uVal.Interface(), iVal.Interface()) {
			return true
		}
	}

	return false
}

// SetDescription changes the repository description on GitHub.
func (r *Repository) SetDescription(ctx context.Context, descr string) error {
	return r.Edit(ctx, &github.Repository{Description: new(descr)})
}

// Name returns the name of the repository.
func (r *Repository) Name() string { return r.name }

// Owner returns the owner of the repository, which may either be a user or an organization.
func (r *Repository) Owner() string { return r.owner }

// Description returns the GitHub description of the repository.
func (r *Repository) Description() string {
	r.muGithub.Lock()
	defer r.muGithub.Unlock()

	if r.github.Description == nil {
		return ""
	}

	return *r.github.Description
}

// Topics returns the GitHub topics of the repository.
func (r *Repository) Topics() []string {
	r.muGithub.Lock()
	defer r.muGithub.Unlock()

	return r.github.Topics
}

// Archived returns whether the repository is archived.
func (r *Repository) Archived() bool {
	r.muGithub.Lock()
	defer r.muGithub.Unlock()

	return *r.github.Archived
}

// Private reports whether the repository is private on GitHub.
//
// A repository GitHub has told us nothing about counts as private: treating an
// unknown repository as public is the answer that leaks, so the safe default is
// the one that holds back.
func (r *Repository) Private() bool {
	r.muGithub.Lock()
	defer r.muGithub.Unlock()

	if r.github == nil || r.github.Private == nil {
		return true
	}

	return *r.github.Private
}
