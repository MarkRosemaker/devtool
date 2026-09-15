package gorepo

import (
	"context"

	"github.com/MarkRosemaker/ghrepo"
)

// Service is a repository service for gorepo repositories.
type Service struct {
	s *ghrepo.Service
}

// NewService creates a new gorepo Service.
func NewService(ctx context.Context, githubToken string, opts ...ghrepo.Option) *Service {
	return &Service{s: ghrepo.NewService(ctx, githubToken, opts...)}
}

// NewRepository opens or initializes a repository at the given path.
func (s *Service) NewRepository(ctx context.Context, owner, name string, opts ...ghrepo.Option) (*Repository, error) {
	repo, err := s.s.NewRepository(ctx, owner, name, opts...)
	if err != nil {
		return nil, err
	}

	return &Repository{Repository: repo}, nil
}

// PrefetchUserRepositories fetches all repositories by the given user and caches them.
// This is useful to avoid hitting the GitHub API rate limits when creating multiple repositories.
func (s *Service) PrefetchUserRepositories(ctx context.Context, user string) error {
	return s.s.PrefetchUserRepositories(ctx, user)
}

// PrefetchOrgRepositories fetches all repositories by the given organization and caches them.
// This is useful to avoid hitting the GitHub API rate limits when creating multiple repositories.
func (s *Service) PrefetchOrgRepositories(ctx context.Context, org string) error {
	return s.s.PrefetchOrgRepositories(ctx, org)
}
