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
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/ghrepo"
)

// label is a label devtool puts on the issues it files, created in a
// repository the first time an issue there needs it.
type label struct{ name, color, description string }

// reported is on every issue devtool files, so a person can list them all.
var reported = label{"devtool", "5319e7", "Reported by a maintained devtool run"}

// The labels that say what an issue is about, one to each.
var (
	coverageLabel   = label{"coverage", "fbca04", "Test coverage is below the floor"}
	failingOnMain   = label{"failing on main", "b60205", "The build or the tests fail on main, before any change"}
	depsBreakBuild  = label{"dependency update breaks build", "d93f0b", "Updating dependencies breaks the build"}
	depsBreakTests  = label{"dependency update breaks tests", "d93f0b", "Updating dependencies breaks the tests"}
	toolBreaksBuild = label{"devtool breaks build", "1d76db", "A devtool task breaks the build"}
	toolBreaksTests = label{"devtool breaks tests", "1d76db", "A devtool task breaks the tests"}
)

// updateDependencies is the task that moves dependencies; every other task is
// devtool's own.
const updateDependencies = "update dependencies"

// report is an issue a maintained run files on a repository: what it is about,
// the comment that identifies it, and what it says.
type report struct {
	label       label
	marker      string
	title, body string
}

// facts is what an issue says about the run that found the problem.
type facts struct {
	Owner, Name                      string
	Commit, GoVersion, Version, Time string
}

// ShortCommit is the commit as GitHub abbreviates it.
func (f facts) ShortCommit() string { return f.Commit[:min(len(f.Commit), 12)] }

// testFailure is a repository's tests failing in a maintained run.
type testFailure struct {
	facts

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

	Marker string
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
	exec, ok := errors.AsType[ghrepo.ExecError](err)
	if !ok || !strings.HasPrefix(exec.Cmd, "go test") {
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
	failedBuild   = regexp.MustCompile(`\[(build|setup) failed\]`)
	shuffleSeed   = regexp.MustCompile(`(?m)^-test\.shuffle (\d+)`)
	digits        = regexp.MustCompile(`\d+`)
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

// Build reports whether a package failed to build, rather than its tests.
func (f testFailure) Build() bool { return failedBuild.MatchString(f.Output) }

// label is what the failure is about: main itself, or the task that broke
// the build or the tests.
func (f testFailure) label() label {
	build := f.Build()

	switch {
	case f.Task == "":
		return failingOnMain
	case f.Task == updateDependencies && build:
		return depsBreakBuild
	case f.Task == updateDependencies:
		return depsBreakTests
	case build:
		return toolBreaksBuild
	default:
		return toolBreaksTests
	}
}

// marker identifies the failure in its issue, so a later run finds the issue
// rather than filing another: where the tests failed, and which, none of which
// a timing or a line number changes.
func (f testFailure) marker() string {
	what := slices.Sorted(slices.Values(f.Failing))
	if len(what) == 0 {
		what = []string{digits.ReplaceAllString(f.Output, "")}
	}

	sum := sha256.Sum256([]byte(strings.Join(append([]string{f.Task}, what...), "\n")))

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

	what := "Tests fail"
	if f.Build() {
		what = "Build fails"
	}

	if f.Task == "" {
		return what + " on main: " + names
	}

	return fmt.Sprintf("%s after %q: %s", what, f.Task, names)
}

// coverageSlip is a repository's coverage below the floor a run holds it to:
// fallen below it in this run, or below what a public repository needs.
type coverageSlip struct {
	facts

	Floor, Prev, Coverage float64
	Public                bool

	Marker string
}

// parseCoverageSlip reads err as the engine reports coverage below a floor.
func parseCoverageSlip(err error, spec engine.Spec, res event.Result) (coverageSlip, bool) {
	c := coverageSlip{Prev: res.PrevCoverage, Coverage: res.Coverage}

	switch {
	case errors.Is(err, engine.ErrPublicCoverage):
		c.Public, c.Floor = true, spec.PublicCoverage
	case errors.Is(err, engine.ErrCoverageFell):
		c.Floor = spec.CoverageFloor
	default:
		return coverageSlip{}, false
	}

	return c, true
}

func (c coverageSlip) marker() string {
	if c.Public {
		return "devtool:coverage public"
	}

	return "devtool:coverage fell"
}

func (c coverageSlip) title() string {
	if c.Public {
		return fmt.Sprintf("Coverage is below the %g%% a public repository needs: %.1f%%", c.Floor, c.Coverage)
	}

	return fmt.Sprintf("Coverage fell below %g%%: %.1f%% → %.1f%%", c.Floor, c.Prev, c.Coverage)
}

var (
	//go:embed issue_tests.md.tmpl
	testsIssueText string
	//go:embed issue_coverage.md.tmpl
	coverageIssueText string

	testsIssue    = template.Must(template.New("tests").Parse(testsIssueText))
	coverageIssue = template.Must(template.New("coverage").Parse(coverageIssueText))
)

// reportOf is the issue a repository's result calls for, if any: its tests
// failed, or its coverage is below a floor. Any other failure is told in the
// run's report alone.
func reportOf(res event.Result, spec engine.Spec, run facts) (report, bool, error) {
	if res.Err == nil {
		return report{}, false, nil
	}

	var (
		r    report
		tmpl *template.Template
		data any
	)

	if f, ok := parseTestFailure(res.Err); ok {
		f.facts, f.Unpushed = run, res.Commits
		f.Marker = f.marker()
		r, tmpl, data = report{label: f.label(), marker: f.Marker, title: f.title()}, testsIssue, f
	} else if c, ok := parseCoverageSlip(res.Err, spec, res); ok {
		c.facts = run
		c.Marker = c.marker()
		r, tmpl, data = report{label: coverageLabel, marker: c.Marker, title: c.title()}, coverageIssue, c
	} else {
		return report{}, false, nil
	}

	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		return report{}, false, err
	}

	r.body = b.String()

	return r, true, nil
}

// fileIssue reports a repository's failing tests or slipping coverage as an
// issue on it, refreshing the open one where the same problem was reported
// before. Reporting is not the run's work, so a failure to report is logged
// rather than returned.
func (s *Service) fileIssue(ctx context.Context, u *unit, spec engine.Spec, res event.Result) {
	if res.Err == nil {
		return
	}

	log := slog.With("repo", u.key())

	r, ok, err := reportOf(res, spec, facts{
		Owner: u.owner, Name: u.name, Version: s.version,
		Commit:    commandOutput(ctx, u.repo, "git", "rev-parse", "@{upstream}"),
		GoVersion: commandOutput(ctx, u.repo, "go", "version"),
		Time:      time.Now().UTC().Format("2006-01-02 15:04 MST"),
	})
	if err != nil {
		log.WarnContext(ctx, "could not write an issue", "error", err)

		return
	} else if !ok {
		return
	}

	if err := s.fileReport(ctx, u.repo, r); err != nil {
		log.WarnContext(ctx, "could not file an issue", "title", r.title, "error", err)
	}
}

// fileReport files r on repo, or refreshes the open issue it was filed as.
func (s *Service) fileReport(ctx context.Context, repo maintainedRepo, r report) error {
	for _, l := range []label{reported, r.label} {
		if err := repo.EnsureLabel(ctx, l.name, l.color, l.description); err != nil {
			return fmt.Errorf("creating label %q: %w", l.name, err)
		}
	}

	open, err := repo.FindOpenIssue(ctx, r.marker, reported.name)
	if err != nil {
		return fmt.Errorf("looking for the issue: %w", err)
	}

	if open != nil {
		if _, err := repo.EditIssue(ctx, open.GetNumber(), r.title, r.body); err != nil {
			return fmt.Errorf("refreshing %s: %w", open.GetHTMLURL(), err)
		}

		slog.InfoContext(ctx, "refreshed the issue", "repo", repo.String(), "issue", open.GetHTMLURL())

		return nil
	}

	issue, err := repo.CreateIssue(ctx, r.title, r.body, reported.name, r.label.name)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "filed an issue", "repo", repo.String(), "issue", issue.GetHTMLURL())

	return nil
}

// commandOutput is what a command says in the repository, or "unknown".
func commandOutput(ctx context.Context, repo maintainedRepo, name string, args ...string) string {
	out, err := repo.ExecCommand(ctx, name, args...)
	if err != nil {
		return "unknown"
	}

	return strings.TrimSpace(string(out))
}
