package ghrepo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/client"
	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
	"golang.org/x/oauth2"
)

const maxPerPage = 100

type Service struct {
	githubToken string
	github      *github.Client
	gitOpts     []client.Option
	opts        []Option

	mu    sync.Mutex
	repos map[string]map[string]*github.Repository
}

func NewService(ctx context.Context, githubToken string, opts ...Option) *Service {
	return &Service{
		githubToken: githubToken,
		github: github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken},
		))),
		gitOpts: []client.Option{
			client.WithHTTPAuth(&githttp.BasicAuth{
				// Can be anything non-empty for token auth
				Username: "git",
				// Recommended: GitHub PAT (not raw password)
				Password: githubToken,
			}),
		},
		opts:  opts,
		repos: map[string]map[string]*github.Repository{},
	}
}

// NewRepository opens or initializes a repository at the given path.
func (s *Service) NewRepository(ctx context.Context, owner, name string, opts ...Option) (*Repository, error) {
	// Apply the options
	cfg := &repoConfig{}
	for _, opt := range append(s.opts, opts...) {
		opt(cfg)
	}

	path := filepath.Join(cfg.baseDir, owner, name)
	r := &Repository{
		Fs:     afero.NewBasePathFs(afero.NewOsFs(), path),
		owner:  owner,
		name:   name,
		path:   path,
		github: cmp.Or(cfg.onGithub, s.getRepo(owner, name)),
		s:      s,
	}

	// Make sure it exists on local
	if cfg.mkdirAll {
		if err := os.MkdirAll(path, fs.ModePerm); err != nil {
			return nil, err
		}
	} else if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	// Make sure we have a git repo
	var err error

	r.gitrepo, err = git.PlainOpen(path)
	if err != nil {
		if (!cfg.cloneGit && !cfg.initGit) || !errors.Is(err, git.ErrRepositoryNotExists) {
			return nil, fmt.Errorf("failed to open git repo at %s: %w", path, err)
		}

		if cfg.cloneGit {
			r.gitrepo, err = git.PlainCloneContext(ctx, path, &git.CloneOptions{
				URL:           fmt.Sprintf("https://github.com/%s/%s.git", owner, name),
				ClientOptions: s.gitOpts,
				Progress:      os.Stdout,
			})
			if err != nil {
				return nil, err
			}
		} else if cfg.initGit {
			initOpts := []git.InitOption{}
			if defaultBranch := r.github.GetDefaultBranch(); defaultBranch != "" {
				initOpts = append(initOpts, git.WithDefaultBranch(
					plumbing.NewBranchReferenceName(defaultBranch),
				))
			}

			r.gitrepo, err = git.PlainInit(path, false, initOpts...)
			if err != nil {
				return nil, fmt.Errorf("failed to init git repo at %s: %w", path, err)
			}
		}
	}

	// Get the worktree
	r.worktree, err = r.gitrepo.Worktree()
	if err != nil {
		return nil, err
	}

	r.defaultBranch, err = getDefaultBranch(r.gitrepo)
	if err != nil {
		if !cfg.cloneGit || !errors.Is(err, errNoDefaultBranch) {
			return nil, err
		}

		// Initialize the default branch
		if err := r.worktree.Checkout(&git.CheckoutOptions{
			Branch: plumbing.Main,
			Create: true,
		}); err != nil {
			return nil, fmt.Errorf("failed to create default branch: %w", err)
		}

		r.defaultBranch = plumbing.Main
	}

	// Make sure we have a remote
	r.remote, err = r.gitrepo.Remote(remoteName)
	if err != nil {
		if !cfg.createRemote || !errors.Is(err, git.ErrRemoteNotFound) {
			return nil, fmt.Errorf("failed to get remote: %w", err)
		}

		// Add correct HTTPS remote
		if r.remote, err = r.gitrepo.CreateRemote(&config.RemoteConfig{
			Name: remoteName,
			URLs: []string{fmt.Sprintf("https://github.com/%s/%s.git", owner, name)},
		}); err != nil {
			return nil, fmt.Errorf("failed to create origin remote: %w", err)
		}
	}

	if r.github != nil {
		return r, nil // already have GitHub repo
	}

	ghrepo, rsp, err := s.github.Repositories.Get(ctx, owner, name)
	if err == nil {
		r.github = ghrepo
		return r, nil
	}

	getErr := fmt.Errorf("getting GitHub repository: %w", err)
	if !cfg.createOnGitHub || rsp == nil || rsp.StatusCode != http.StatusNotFound {
		return nil, getErr
	}

	org := ""
	if cfg.ownerIsOrg {
		org = owner
	}

	ghrepo, _, err = s.github.Repositories.Create(ctx, org, &github.Repository{
		Name: new(name),
		// We start out with a private repository until the repository is ready to be published.
		Visibility: new("private"),
	})
	if err != nil {
		return nil, errors.Join(getErr, fmt.Errorf("creating GitHub repository: %w", err))
	}

	r.github = ghrepo

	return r, nil
}

// PrefetchUserRepositories fetches all repositories by the given user and caches them.
// This is useful to avoid hitting the GitHub API rate limits when creating multiple repositories.
func (s *Service) PrefetchUserRepositories(ctx context.Context, user string) error {
	page := 1
	for page > 0 {
		repos, resp, err := s.github.Repositories.ListByUser(ctx, user,
			&github.RepositoryListByUserOptions{
				ListOptions: github.ListOptions{Page: page, PerPage: maxPerPage},
			})
		if err != nil {
			return err
		}

		s.addRepos(user, repos)

		page = resp.NextPage
	}

	return nil
}

// PrefetchOrgRepositories fetches all repositories by the given organization and caches them.
// This is useful to avoid hitting the GitHub API rate limits when creating multiple repositories.
func (s *Service) PrefetchOrgRepositories(ctx context.Context, org string) error {
	page := 1
	for page > 0 {
		repos, resp, err := s.github.Repositories.ListByOrg(ctx, org,
			&github.RepositoryListByOrgOptions{
				ListOptions: github.ListOptions{Page: page, PerPage: maxPerPage},
			})
		if err != nil {
			return err
		}

		s.addRepos(org, repos)

		page = resp.NextPage
	}

	return nil
}

func (s *Service) addRepos(owner string, repos []*github.Repository) {
	if len(repos) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.repos[owner]; !ok {
		s.repos[owner] = map[string]*github.Repository{}
	}

	for _, r := range repos {
		s.repos[owner][r.GetName()] = r
	}
}

func (s *Service) getRepo(owner, name string) *github.Repository {
	s.mu.Lock()
	defer s.mu.Unlock()

	if repos, ok := s.repos[owner]; ok {
		return repos[name]
	}

	return nil
}
