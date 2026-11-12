package ghrepo

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/go-github/v80/github"
)

// OpenIssues returns the open issues, with labels only those carrying all of
// them. Pull requests, which GitHub lists as issues too, are left out.
func (r *Repository) OpenIssues(ctx context.Context, labels ...string) ([]*github.Issue, error) {
	opts := &github.IssueListByRepoOptions{
		State: "open", Labels: labels, ListOptions: github.ListOptions{PerPage: 100},
	}

	var open []*github.Issue

	for {
		issues, resp, err := r.s.github.Issues.ListByRepo(ctx, r.owner, r.name, opts)
		if err != nil {
			return nil, err
		}

		for _, issue := range issues {
			if !issue.IsPullRequest() {
				open = append(open, issue)
			}
		}

		if resp.NextPage == 0 {
			return open, nil
		}

		opts.ListOptions.Page = resp.NextPage
	}
}

// FindOpenIssue returns the first open issue whose body contains text, or nil
// where there is none, reading only those carrying all the labels given.
func (r *Repository) FindOpenIssue(ctx context.Context, text string, labels ...string) (*github.Issue, error) {
	issues, err := r.OpenIssues(ctx, labels...)
	if err != nil {
		return nil, err
	}

	for _, issue := range issues {
		if strings.Contains(issue.GetBody(), text) {
			return issue, nil
		}
	}

	return nil, nil
}

// CloseIssue closes an issue as completed, first commenting why where comment
// says.
func (r *Repository) CloseIssue(ctx context.Context, number int, comment string) error {
	if comment != "" {
		if _, _, err := r.s.github.Issues.CreateComment(ctx, r.owner, r.name, number,
			&github.IssueComment{Body: &comment}); err != nil {
			return err
		}
	}

	_, _, err := r.s.github.Issues.Edit(ctx, r.owner, r.name, number,
		&github.IssueRequest{State: new("closed"), StateReason: new("completed")})

	return err
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
