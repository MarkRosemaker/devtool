package ghrepo

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/go-github/v80/github"
)

// FindOpenIssue returns the first open issue whose body contains text, or nil
// where there is none. With labels, only issues carrying all of them are
// read. Pull requests, which GitHub lists as issues too, are passed over.
func (r *Repository) FindOpenIssue(ctx context.Context, text string, labels ...string) (*github.Issue, error) {
	opts := &github.IssueListByRepoOptions{
		State: "open", Labels: labels, ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		issues, resp, err := r.s.github.Issues.ListByRepo(ctx, r.owner, r.name, opts)
		if err != nil {
			return nil, err
		}

		for _, issue := range issues {
			if !issue.IsPullRequest() && strings.Contains(issue.GetBody(), text) {
				return issue, nil
			}
		}

		if resp.NextPage == 0 {
			return nil, nil
		}

		opts.ListOptions.Page = resp.NextPage
	}
}

// CreateIssue opens an issue with the labels given, which must exist (see
// [Repository.EnsureLabel]).
func (r *Repository) CreateIssue(ctx context.Context, title, body string, labels ...string) (*github.Issue, error) {
	req := &github.IssueRequest{Title: &title, Body: &body}
	if len(labels) > 0 {
		req.Labels = &labels
	}

	issue, _, err := r.s.github.Issues.Create(ctx, r.owner, r.name, req)

	return issue, err
}

// EditIssue replaces an issue's title and body.
func (r *Repository) EditIssue(ctx context.Context, number int, title, body string) (*github.Issue, error) {
	issue, _, err := r.s.github.Issues.Edit(ctx, r.owner, r.name, number,
		&github.IssueRequest{Title: &title, Body: &body})

	return issue, err
}

// EnsureLabel creates the label where the repository does not have it yet,
// with color (six hex digits, no #) and description. One it has is left as it
// is, so a colour somebody chose stays.
func (r *Repository) EnsureLabel(ctx context.Context, name, color, description string) error {
	_, _, err := r.s.github.Issues.GetLabel(ctx, r.owner, r.name, name)
	if errResp, ok := errors.AsType[*github.ErrorResponse](err); !ok || errResp.Response.StatusCode != http.StatusNotFound {
		return err
	}

	_, _, err = r.s.github.Issues.CreateLabel(ctx, r.owner, r.name,
		&github.Label{Name: &name, Color: &color, Description: &description})

	return err
}
