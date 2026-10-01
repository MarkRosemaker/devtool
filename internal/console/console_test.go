package console

import (
	"bytes"
	"testing"
	"time"

	"github.com/MarkRosemaker/devtool-engine/event"
)

func render(events ...event.Event) string {
	buf := &bytes.Buffer{}
	c := New(buf)

	clock := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time {
		t := clock
		clock = clock.Add(1200 * time.Millisecond)

		return t
	}

	for _, ev := range events {
		c.Emit(ev)
	}

	return buf.String()
}

func TestOneRepository(t *testing.T) {
	const repo = "user/alpha"

	got := render(
		event.Event{Kind: event.RunStart, Repos: []string{repo}},
		event.Event{Kind: event.RepoStart, Repo: repo},
		event.Event{Kind: event.TaskStart, Repo: repo, Task: "update"},
		event.Event{
			Kind: event.TaskDone, Repo: repo, Task: "update",
			Files: []string{"README.md", "devtool.json"},
		},
		// A step that changed nothing is not news.
		event.Event{Kind: event.TaskDone, Repo: repo, Task: "vet"},
		event.Event{
			Kind: event.TaskDone, Repo: repo, Task: "deps",
			Files:   []string{"go.mod", "go.sum"},
			Modules: []event.ModuleChange{{Path: "golang.org/x/mod", From: "v0.40.0", To: "v0.41.0"}},
		},
		event.Event{Kind: event.RepoPushed, Repo: repo, Commits: []string{"readme", "deps"}},
		event.Event{Kind: event.RepoDone, Repo: repo, Pushed: true, Coverage: 81.6, PrevCoverage: 80},
		event.Event{Kind: event.RunDone},
	)

	want := `📦 user/alpha
  ✏️  update: README.md, devtool.json
  ✏️  deps: go.mod, go.sum
    ⬆️  golang.org/x/mod v0.40.0 → v0.41.0
  🚀 pushed readme, deps
  📊 coverage 81.6% (+1.6)
🏁 done in 1.2s
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestOneRepositoryWithNothingToDo(t *testing.T) {
	const repo = "user/alpha"

	got := render(
		event.Event{Kind: event.RunStart, Repos: []string{repo}},
		event.Event{Kind: event.RepoStart, Repo: repo},
		event.Event{Kind: event.TaskDone, Repo: repo, Task: "update"},
		event.Event{Kind: event.RepoDone, Repo: repo},
		event.Event{Kind: event.RunDone},
	)

	want := "📦 user/alpha\n  ✨ nothing to change\n🏁 done in 1.2s\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// TestAList: repositories run side by side, so each line names its own, and
// the ones with nothing to do are counted rather than listed.
func TestAList(t *testing.T) {
	got := render(
		event.Event{Kind: event.RunStart, Repos: []string{"user/alpha", "user/beta", "user/gamma"}},
		event.Event{Kind: event.RunProgress, Task: "opening", TaskIndex: 1, TaskCount: 3},
		event.Event{Kind: event.RunProgress, Task: "opening", TaskIndex: 2, TaskCount: 3},
		event.Event{Kind: event.RepoStart, Repo: "user/alpha"},
		event.Event{Kind: event.RepoStart, Repo: "user/beta"},
		event.Event{
			Kind: event.TaskDone, Repo: "user/alpha", Task: "update", Committed: true,
			Files: []string{"README.md"},
		},
		event.Event{Kind: event.RepoPushed, Repo: "user/alpha", Commits: []string{"readme"}},
		event.Event{Kind: event.RepoDone, Repo: "user/alpha", Pushed: true},
		event.Event{Kind: event.RepoDone, Repo: "user/beta"},
		event.Event{Kind: event.RepoDone, Repo: "user/gamma", Err: "deps: go get: timeout"},
		event.Event{Kind: event.RunDone},
	)

	want := `🚀 Maintaining 3 repositories
📂 Opening 3 repositories…
user/alpha ✏️  update: README.md
user/alpha 🚀 pushed readme
user/gamma ❌ deps: go get: timeout
🏁 3 repositories · 1 pushed · 1 failed · 1.2s
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestElapsed(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{312400 * time.Microsecond, "312ms"},
		{1234 * time.Millisecond, "1.2s"},
		{252 * time.Second, "4m12s"},
	} {
		if got := elapsed(tc.d); got != tc.want {
			t.Errorf("elapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
