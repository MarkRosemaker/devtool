package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/MarkRosemaker/devtool-engine/event"
	"github.com/MarkRosemaker/ghrepo"
	"github.com/google/go-github/v80/github"
)

// testFailure is a repository's tests failing in a maintained run, told as an
// issue on that repository for whoever fixes it.
type testFailure struct {
	Owner, Name string

	// Task is the task whose change broke the tests, and Changes what it
	// changed. Task is empty where the tests failed on main, before any change.
	Task, Changes string

	// Command is the go test command, Output what it said about what failed,
	// Failing the tests that failed (their packages where none is named), and
	// Seed the shuffle seed that ordered them.
	Command, Output string
	Failing         []string
	Seed            string

	// Unpushed are the earlier tasks of the run that committed, and were not
	// pushed because the run stopped.
	Unpushed []string

	Commit, GoVersion, Version, Time string
}

// The engine's words for the two places a test run fails: on main, before any
// task, and after a task changed something.
const (
	failedBeforeChanges = "testing before any changes: "
	failedAfterChanges  = ": testing after changes:\n"
)

// parseTestFailure reads err as the engine reports a failing test run; ok is
// false for any other failure.
func parseTestFailure(err error) (testFailure, bool) {
	var exec ghrepo.ExecError
	if !errors.As(err, &exec) || !strings.HasPrefix(exec.Cmd, "go test") {
		return testFailure{}, false
	}

	f := testFailure{Command: exec.Cmd, Output: strings.TrimSpace(exec.Out)}

	if msg := err.Error(); !strings.HasPrefix(msg, failedBeforeChanges) {
		before, after, ok := strings.Cut(msg, failedAfterChanges)
		if !ok {
			return testFailure{}, false
		}

		f.Task = before
		f.Changes, _, _ = strings.Cut(after, "\n"+exec.Error())
	}

	f.Failing = failingTests(f.Output)
	if m := shuffleSeed.FindStringSubmatch(f.Output); m != nil {
		f.Seed = m[1]
	}

	return f, true
}

var (
	failedTest    = regexp.MustCompile(`(?m)^\s*--- FAIL: (\S+)`)
	failedPackage = regexp.MustCompile(`(?m)^FAIL\t(\S+)`)
	shuffleSeed   = regexp.MustCompile(`(?m)^-test\.shuffle (\d+)`)
)

// failingTests names what failed: the tests, or the packages where no test is
// named, as when one does not build.
func failingTests(out string) []string {
	var names []string

	for _, re := range []*regexp.Regexp{failedTest, failedPackage} {
		for _, m := range re.FindAllStringSubmatch(out, -1) {
			if !slices.Contains(names, m[1]) {
				names = append(names, m[1])
			}
		}

		if len(names) > 0 {
			return names
		}
	}

	return nil
}

// marker identifies the failure in its issue, so a later run finds the issue
// rather than filing another: the repository, where the tests failed, and
// which tests, none of which a timing or a line number changes.
func (f testFailure) marker() string {
	what := slices.Sorted(slices.Values(f.Failing))
	if len(what) == 0 {
		what = []string{regexp.MustCompile(`\d+`).ReplaceAllString(f.Output, "")}
	}

	sum := sha256.Sum256([]byte(strings.Join(append([]string{f.Owner, f.Name, f.Task}, what...), "\n")))

	return "devtool:test-failure " + hex.EncodeToString(sum[:8])
}

// maxTitled is how many failing tests an issue's title names.
const maxTitled = 3

func (f testFailure) title() string {
	names := strings.Join(f.Failing[:min(len(f.Failing), maxTitled)], ", ")
	if more := len(f.Failing) - maxTitled; more > 0 {
		names += fmt.Sprintf(" and %d more", more)
	}

	if names == "" {
		names = "the suite"
	}

	if f.Task == "" {
		return "Tests fail on main: " + names
	}

	return fmt.Sprintf("Tests fail after %q: %s", f.Task, names)
}

//go:embed testissue.md.tmpl
var testIssueText string

var testIssueTmpl = template.Must(template.New("testissue").Parse(testIssueText))

func (f testFailure) body() (string, error) {
	var b bytes.Buffer

	err := testIssueTmpl.Execute(&b, struct {
		testFailure
		Marker, ShortCommit string
	}{f, f.marker(), f.Commit[:min(len(f.Commit), 12)]})

	return b.String(), err
}

// issueTracker is where a test failure is reported: a test replaces it.
type issueTracker interface {
	// findOpen is the address of the open issue holding marker, or "".
	findOpen(ctx context.Context, owner, name, marker string) (string, error)
	create(ctx context.Context, owner, name, title, body string) (string, error)
}

// reportTestFailure files an issue for a repository whose tests failed, unless
// one for the same failure is open already. Reporting is not the run's work,
// so a failure to report is logged rather than returned.
func (s *Service) reportTestFailure(ctx context.Context, u *unit, res event.Result) {
	if s.issues == nil || res.Err == nil {
		return
	}

	f, ok := parseTestFailure(res.Err)
	if !ok {
		return
	}

	f.Owner, f.Name, f.Unpushed, f.Version = u.owner, u.name, res.Commits, s.version
	f.Commit = commandOutput(ctx, u.repo, "git", "rev-parse", "@{upstream}")
	f.GoVersion = commandOutput(ctx, u.repo, "go", "version")
	f.Time = time.Now().UTC().Format("2006-01-02 15:04 MST")

	log := slog.With("repo", u.key())

	open, err := s.issues.findOpen(ctx, f.Owner, f.Name, f.marker())
	if err != nil {
		log.WarnContext(ctx, "could not look for an issue about the failing tests", "error", err)

		return
	}

	if open != "" {
		log.InfoContext(ctx, "the failing tests are reported already", "issue", open)

		return
	}

	body, err := f.body()
	if err != nil {
		log.WarnContext(ctx, "could not write the issue about the failing tests", "error", err)

		return
	}

	url, err := s.issues.create(ctx, f.Owner, f.Name, f.title(), body)
	if err != nil {
		log.WarnContext(ctx, "could not file an issue about the failing tests", "error", err)

		return
	}

	log.InfoContext(ctx, "filed an issue about the failing tests", "issue", url)
}

// commandOutput is what a command says in the repository, or "unknown".
func commandOutput(ctx context.Context, repo maintainedRepo, name string, args ...string) string {
	out, err := repo.ExecCommand(ctx, name, args...)
	if err != nil {
		return "unknown"
	}

	return strings.TrimSpace(string(out))
}

// gitHubIssues files issues through the GitHub API.
type gitHubIssues struct{ client *github.Client }

func (g gitHubIssues) findOpen(ctx context.Context, owner, name, marker string) (string, error) {
	opts := &github.IssueListByRepoOptions{State: "open", ListOptions: github.ListOptions{PerPage: 100}}

	for {
		issues, resp, err := g.client.Issues.ListByRepo(ctx, owner, name, opts)
		if err != nil {
			return "", err
		}

		for _, issue := range issues {
			if !issue.IsPullRequest() && strings.Contains(issue.GetBody(), marker) {
				return issue.GetHTMLURL(), nil
			}
		}

		if resp.NextPage == 0 {
			return "", nil
		}

		opts.ListOptions.Page = resp.NextPage
	}
}

func (g gitHubIssues) create(ctx context.Context, owner, name, title, body string) (string, error) {
	issue, _, err := g.client.Issues.Create(ctx, owner, name, &github.IssueRequest{Title: &title, Body: &body})

	return issue.GetHTMLURL(), err
}
