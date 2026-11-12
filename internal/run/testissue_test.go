package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/ghrepo"
)

// fakeIssues files issues in memory.
type fakeIssues struct {
	mu     sync.Mutex
	filed  []fakeIssue
	reject error
}

type fakeIssue struct{ repo, title, body string }

func (f *fakeIssues) findOpen(_ context.Context, owner, name, marker string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, issue := range f.filed {
		if issue.repo == owner+"/"+name && strings.Contains(issue.body, marker) {
			return fmt.Sprintf("https://github.com/%s/%s/issues/%d", owner, name, i+1), nil
		}
	}

	return "", nil
}

func (f *fakeIssues) create(_ context.Context, owner, name, title, body string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.reject != nil {
		return "", f.reject
	}

	f.filed = append(f.filed, fakeIssue{owner + "/" + name, title, body})

	return fmt.Sprintf("https://github.com/%s/%s/issues/%d", owner, name, len(f.filed)), nil
}

// testCommand is the command gorepo runs, and failingOutput what it says about
// a failure once gorepo has left out what passed.
const (
	testCommand   = "go test ./... -race -cover -covermode=atomic -coverpkg=./... -coverprofile=cover.out -shuffle=on"
	failingOutput = "-test.shuffle 1794492933930643719\n" +
		"--- FAIL: TestBad (0.00s)\n" +
		"    b_test.go:4: boom\n" +
		"FAIL\n" +
		"FAIL\texample.com/app/b\t0.011s"
)

func testsFail() error {
	return ghrepo.ExecError{Cmd: testCommand, Out: failingOutput, Err: errors.New("exit status 1")}
}

// Tests failing on main are filed once, however many runs find them.
func TestFailingTestsOnMainAreFiledOnce(t *testing.T) {
	svc, src, _ := portfolio(t, `{"alice": {"repositories": {"app": {}}}}`)
	src.add("alice", "app", map[string]string{"go.mod": goMod("github.com/alice/app")})

	issues := &fakeIssues{}
	svc.issues = issues

	for range 2 {
		src.coverErrs["alice/app"] = []error{testsFail()}

		if err := svc.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	if len(issues.filed) != 1 {
		t.Fatalf("filed %d issues over two runs, want 1", len(issues.filed))
	}

	issue := issues.filed[0]
	if issue.repo != "alice/app" || issue.title != "Tests fail on main: TestBad" {
		t.Errorf("filed %q on %s", issue.title, issue.repo)
	}

	for _, want := range []string{
		"<!-- devtool:test-failure ", "`main` itself is broken", "- `TestBad`",
		failingOutput, testCommand, "`-shuffle=1794492933930643719`", "make test-race", "AGENTS.md",
	} {
		if !strings.Contains(issue.body, want) {
			t.Errorf("the issue lacks %q:\n%s", want, issue.body)
		}
	}
}

// Tests a task's change breaks are filed against that task, with the change.
func TestTestsATaskBreaksAreFiledWithTheChange(t *testing.T) {
	svc, src, _ := portfolio(t, `{"alice": {"repositories": {"app": {}}}}`)
	src.add("alice", "app", map[string]string{"go.mod": goMod("github.com/alice/app")})

	src.tasks["alice/app"] = func(r *fakeRepo) []engine.Task {
		return []engine.Task{writeTask(r, "NOTES", "changed\n"), writeTask(r, "go.sum", "changed\n")}
	}
	// The suite passes before any change and after the first task, and fails
	// after the second.
	src.coverErrs["alice/app"] = []error{nil, nil, testsFail()}

	issues := &fakeIssues{}
	svc.issues = issues

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if len(issues.filed) != 1 {
		t.Fatalf("filed %d issues, want 1", len(issues.filed))
	}

	issue := issues.filed[0]
	if want := `Tests fail after "write go.sum": TestBad`; issue.title != want {
		t.Errorf("title = %q, want %q", issue.title, want)
	}

	for _, want := range []string{"**write go.sum**", "changed: go.sum", "**write NOTES**", failingOutput} {
		if !strings.Contains(issue.body, want) {
			t.Errorf("the issue lacks %q:\n%s", want, issue.body)
		}
	}
}

// Only failing tests are filed: a run that fails otherwise files nothing.
func TestOtherFailuresAreNotFiled(t *testing.T) {
	for _, err := range []error{
		errors.New("pushing: rejected"),
		fmt.Errorf("testing before any changes: %w", errors.New("no go.mod")),
		ghrepo.ExecError{Cmd: "go mod tidy", Err: errors.New("exit status 1")},
	} {
		if _, ok := parseTestFailure(err); ok {
			t.Errorf("%q read as failing tests", err)
		}
	}
}

// The marker names the failure, not the run: timings and seeds do not change it.
func TestTheMarkerIgnoresWhatVariesBetweenRuns(t *testing.T) {
	again := strings.NewReplacer("0.011s", "0.020s", "1794492933930643719", "42").Replace(failingOutput)

	a, _ := parseTestFailure(fmt.Errorf("testing before any changes: %w", testsFail()))
	b, _ := parseTestFailure(fmt.Errorf("testing before any changes: %w",
		ghrepo.ExecError{Cmd: testCommand, Out: again, Err: errors.New("exit status 1")}))

	if a.marker() != b.marker() {
		t.Errorf("the same failure reads as two: %s and %s", a.marker(), b.marker())
	}
}
