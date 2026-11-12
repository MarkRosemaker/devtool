package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MarkRosemaker/devtool-engine/event"
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/ghrepo"
	"github.com/google/go-github/v80/github"
)

// testCommand is the command gorepo runs, and failingOutput what it says about
// a failure once gorepo has left out what passed.
const (
	testCommand   = "go test ./... -race -cover -covermode=atomic -coverpkg=./... -coverprofile=cover.out -shuffle=on"
	failingOutput = "-test.shuffle 1794492933930643719\n" +
		"--- FAIL: TestBad (0.00s)\n" +
		"    b_test.go:4: boom\n" +
		"FAIL\n" +
		"FAIL\texample.com/app/b\t0.011s"
	brokenOutput = "# example.com/app/b\n" +
		"b/b.go:3:2: undefined: gone\n" +
		"FAIL\texample.com/app/b [build failed]"
)

func testsFail(out string) error {
	return ghrepo.ExecError{Cmd: testCommand, Out: out, Err: errors.New("exit status 1")}
}

// app is a portfolio of one repository, alice/app.
func app(t *testing.T, cfg string) (*Service, *fakeSource) {
	t.Helper()

	svc, src, _ := appWithEvents(t, cfg)

	return svc, src
}

func appWithEvents(t *testing.T, cfg string) (*Service, *fakeSource, *recorder) {
	t.Helper()

	svc, src, rec := portfolio(t, `{"alice": {"repositories": {"app": {`+cfg+`}}}}`)
	src.add("alice", "app", map[string]string{"go.mod": goMod("github.com/alice/app")})

	return svc, src, rec
}

// issueEvents are the issue events recorded, as "kind issue: title".
func issueEvents(rec *recorder) []string {
	var got []string

	for _, ev := range rec.events {
		if ev.Kind == event.IssueFiled || ev.Kind == event.IssueClosed {
			got = append(got, string(ev.Kind)+" "+ev.Issue+": "+ev.Title)
		}
	}

	return got
}

func labelNames(issue *github.Issue) []string {
	var names []string
	for _, l := range issue.Labels {
		names = append(names, l.GetName())
	}

	return names
}

// Tests failing on main are filed once, and the open issue is brought up to
// date by every run that finds them still failing.
func TestFailingTestsOnMainAreFiledOnce(t *testing.T) {
	svc, src := app(t, "")

	for _, out := range []string{failingOutput, strings.Replace(failingOutput, "boom", "boom again", 1)} {
		src.coverErrs["alice/app"] = []error{testsFail(out)}

		if err := svc.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	issues := src.issues["alice/app"]
	if len(issues) != 1 {
		t.Fatalf("filed %d issues over two runs, want 1", len(issues))
	}

	issue := issues[0]
	if got := issue.GetTitle(); got != "Tests fail on main: TestBad" {
		t.Errorf("title = %q", got)
	}

	if got, want := labelNames(issue), []string{"devtool", "failing on main"}; !slices.Equal(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}

	for _, want := range []string{
		"<!-- devtool:test-failure ", "`main` itself is broken", "- `TestBad`", "boom again",
		testCommand, "`-shuffle=1794492933930643719`", "make test-race", "AGENTS.md",
	} {
		if !strings.Contains(issue.GetBody(), want) {
			t.Errorf("the issue lacks %q:\n%s", want, issue.GetBody())
		}
	}
}

// Tests a task's change breaks are filed against that task, with the change,
// and labelled by whose task it was and whether the build broke.
func TestATaskThatBreaksTheTestsIsFiledWithItsChange(t *testing.T) {
	for _, tc := range []struct {
		task, out, label, title string
	}{
		{"write go.sum", failingOutput, "devtool breaks tests", `Tests fail after "write go.sum": TestBad`},
		{updateDependencies, brokenOutput, "dependency update breaks build", `Build fails after "update dependencies": example.com/app/b`},
	} {
		t.Run(tc.label, func(t *testing.T) {
			svc, src := app(t, "")

			src.tasks["alice/app"] = func(r *fakeRepo) []engine.Task {
				change := writeTask(r, "go.sum", "changed\n")
				change.Name = tc.task

				return []engine.Task{writeTask(r, "NOTES", "changed\n"), change}
			}
			// The suite passes before any change and after the first task, and
			// fails after the second.
			src.coverErrs["alice/app"] = []error{nil, nil, testsFail(tc.out)}

			if err := svc.Run(t.Context()); err != nil {
				t.Fatal(err)
			}

			issues := src.issues["alice/app"]
			if len(issues) != 1 {
				t.Fatalf("filed %d issues, want 1", len(issues))
			}

			issue := issues[0]
			if issue.GetTitle() != tc.title {
				t.Errorf("title = %q, want %q", issue.GetTitle(), tc.title)
			}

			if got, want := labelNames(issue), []string{"devtool", tc.label}; !slices.Equal(got, want) {
				t.Errorf("labels = %v, want %v", got, want)
			}

			for _, want := range []string{"**" + tc.task + "**", "changed: go.sum", "**write NOTES**", tc.out} {
				if !strings.Contains(issue.GetBody(), want) {
					t.Errorf("the issue lacks %q:\n%s", want, issue.GetBody())
				}
			}
		})
	}
}

// A build broken by a dependency update, as a maintained run of lso reported
// it in Telegram, names the package that does not compile, not the nineteen
// that import it.
func TestABrokenBuildNamesThePackageThatDoesNotCompile(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "lso-build-failure.txt"))
	if err != nil {
		t.Fatal(err)
	}

	head, rest, _ := strings.Cut(strings.TrimSuffix(string(text), "\n"), "\ncommand ")
	task, changes, _ := strings.Cut(head, failedAfterChanges)
	out := strings.TrimSuffix(rest[strings.Index(rest, "\n")+1:], "\nexit status 1")

	failure := fmt.Errorf("%s%s%s\n%w", task, failedAfterChanges, changes, testsFail(out))
	if failure.Error() != strings.TrimSuffix(string(text), "\n") {
		t.Fatalf("the error rebuilt from the message reads differently:\n%s", failure)
	}

	r, ok, err := reportOf(event.Result{Err: failure}, engine.Spec{}, facts{Owner: "MarkRosemaker", Name: "lso"})
	if err != nil || !ok {
		t.Fatalf("reportOf = %v, %v", ok, err)
	}

	if want := `Build fails after "update dependencies": internal/handler`; r.title != want {
		t.Errorf("title = %q, want %q", r.title, want)
	}

	if r.label != depsBreakBuild {
		t.Errorf("label = %q, want %q", r.label.name, depsBreakBuild.name)
	}

	for _, want := range []string{
		"failing to build", "- `github.com/MarkRosemaker/lso/internal/handler`\n\nOnly",
		"mismatched types civil.Date and untyped nil", "golang.org/x/crypto v0.58.0 => v0.59.0",
	} {
		if !strings.Contains(r.body, want) {
			t.Errorf("the issue lacks %q:\n%s", want, r.body)
		}
	}
}

// Coverage fallen below the floor is filed, with the figures.
func TestCoverageBelowTheFloorIsFiled(t *testing.T) {
	svc, src := app(t, `"coverage": 85`)
	src.coverage["alice/app"] = 70

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	issues := src.issues["alice/app"]
	if len(issues) != 1 {
		t.Fatalf("filed %d issues, want 1", len(issues))
	}

	issue := issues[0]
	if want := "Coverage fell below 80%: 85.0% → 70.0%"; issue.GetTitle() != want {
		t.Errorf("title = %q, want %q", issue.GetTitle(), want)
	}

	if got, want := labelNames(issue), []string{"devtool", "coverage"}; !slices.Equal(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}

	for _, want := range []string{"<!-- devtool:coverage fell -->", "from 85.0% to 70.0%", "make cover", "-coverpkg=./..."} {
		if !strings.Contains(issue.GetBody(), want) {
			t.Errorf("the issue lacks %q:\n%s", want, issue.GetBody())
		}
	}
}

// Only failing tests and coverage are filed: a run that fails otherwise, or
// does not fail, files nothing.
func TestOtherOutcomesAreNotFiled(t *testing.T) {
	for _, err := range []error{
		nil,
		errors.New("pushing: rejected"),
		fmt.Errorf("testing before any changes: %w", errors.New("no go.mod")),
		ghrepo.ExecError{Cmd: "go mod tidy", Err: errors.New("exit status 1")},
	} {
		if _, ok, _ := reportOf(event.Result{Err: err}, engine.Spec{}, facts{}); ok {
			t.Errorf("%v was filed", err)
		}
	}
}

// The marker names the failure, not the run: timings and seeds do not change it.
func TestTheMarkerIgnoresWhatVariesBetweenRuns(t *testing.T) {
	again := strings.NewReplacer("0.011s", "0.020s", "1794492933930643719", "42").Replace(failingOutput)

	a, _ := parseTestFailure(fmt.Errorf("testing before any changes: %w", testsFail(failingOutput)))
	b, _ := parseTestFailure(fmt.Errorf("testing before any changes: %w", testsFail(again)))

	if a.marker() != b.marker() {
		t.Errorf("the same failure reads as two: %s and %s", a.marker(), b.marker())
	}
}

// Failing tests are filed and told; once a run finds them passing, the issue
// is closed, with a comment saying where, and told too.
func TestAFixedFailureIsClosed(t *testing.T) {
	svc, src, rec := appWithEvents(t, "")

	for range 2 {
		src.coverErrs["alice/app"] = []error{testsFail(failingOutput)}

		if err := svc.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	issue := src.issues["alice/app"][0]

	const url = "https://github.com/alice/app/issues/1"

	want := []string{
		"issue_filed " + url + ": Tests fail on main: TestBad",
		"issue_filed " + url + ": Tests fail on main: TestBad",
	}
	if got := issueEvents(rec); !slices.Equal(got, want) {
		t.Fatalf("after two failing runs, events = %q\nwant %q", got, want)
	}

	if issue.GetState() != "open" {
		t.Fatal("the issue was closed while the tests still failed")
	}

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if issue.GetState() != "closed" {
		t.Fatal("the issue stayed open once the tests passed")
	}

	if got := issueEvents(rec); got[len(got)-1] != "issue_closed "+url+": Tests fail on main: TestBad" {
		t.Errorf("events = %q, want the closing last", got)
	}

	if comment := src.comments[url]; !strings.Contains(comment, "Fixed: devtool v1.0.0 found that the tests pass at") {
		t.Errorf("closed with %q", comment)
	}
}

// Coverage that slipped stays reported while it is below the floor, though no
// run fails over it again, and is closed once it is back.
func TestCoverageIsClosedOnlyOnceBackAtTheFloor(t *testing.T) {
	svc, src := app(t, `"coverage": 85`)

	for _, cov := range []float64{70, 75, 82} {
		src.coverage["alice/app"] = cov

		if err := svc.Run(t.Context()); err != nil {
			t.Fatal(err)
		}

		issues := src.issues["alice/app"]
		if len(issues) != 1 {
			t.Fatalf("at %v%%, %d issues, want 1", cov, len(issues))
		}

		if want := map[bool]string{true: "closed", false: "open"}[cov >= 80]; issues[0].GetState() != want {
			t.Errorf("at %v%%, the issue is %s, want %s", cov, issues[0].GetState(), want)
		}
	}

	if comment := src.comments["https://github.com/alice/app/issues/1"]; !strings.Contains(comment, "coverage is 82.0%, back at the 80% floor") {
		t.Errorf("closed with %q", comment)
	}
}

// A run that fails for another reason shows nothing fixed, and closes nothing.
func TestAnUnrelatedFailureClosesNothing(t *testing.T) {
	svc, src := app(t, "")
	src.coverErrs["alice/app"] = []error{testsFail(failingOutput)}

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	// The tests pass now, but a task fails: no issue is about that.
	src.tasks["alice/app"] = func(*fakeRepo) []engine.Task {
		return []engine.Task{{Name: "go vet", Run: func(context.Context) error { return errors.New("vet: bad") }}}
	}

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if state := src.issues["alice/app"][0].GetState(); state != "open" {
		t.Errorf("the issue is %s after a run that failed otherwise, want open", state)
	}
}
