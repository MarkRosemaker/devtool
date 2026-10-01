// Package console renders a run's events for a person at a terminal: one line
// per thing worth knowing, as it happens.
//
// The same events go to patchpal as JSON Lines under -json. This is the other
// reader, and it says less: what changed, what was pushed, what failed, and
// how long it took. A step that changed nothing is not news.
package console

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/MarkRosemaker/devtool-engine/event"
)

// openingPhase is the run-level phase devtool reports while it opens and
// clones the list.
const openingPhase = "opening"

// Console is an [event.Emitter] writing for a person. It is safe for
// concurrent use: a run over a list maintains repositories in parallel.
type Console struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time

	start  time.Time
	repos  int
	multi  bool
	pushed int
	failed int

	// changed is every repository something was reported changing in, so one
	// that finishes without is told it had nothing to do.
	changed map[string]bool
}

// New returns a Console writing to w.
func New(w io.Writer) *Console {
	return &Console{w: w, now: time.Now, changed: map[string]bool{}}
}

// Emit renders one event.
func (c *Console) Emit(ev event.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch ev.Kind {
	case event.RunStart:
		c.start, c.repos = c.now(), len(ev.Repos)
		c.multi = c.repos > 1

		if c.multi {
			c.line("", "🚀", fmt.Sprintf("Maintaining %d repositories", c.repos))
		}
	case event.RunProgress:
		if ev.Task == openingPhase && ev.TaskIndex == 1 {
			c.line("", "📂", fmt.Sprintf("Opening %d repositories…", ev.TaskCount))
		}
	case event.RepoStart:
		if !c.multi {
			c.line("", "📦", ev.Repo)
		}
	case event.TaskDone:
		c.taskDone(ev)
	case event.RepoPushed:
		c.changed[ev.Repo] = true
		c.line(ev.Repo, "🚀", "pushed "+strings.Join(ev.Commits, ", "))
	case event.RepoDone:
		c.repoDone(ev)
	case event.RunDone:
		c.runDone()
	case event.TaskStart:
	}
}

// taskDone reports a step that changed something. A failing step is left to
// the repository's own failure, which names the step and says the rest.
func (c *Console) taskDone(ev event.Event) {
	if ev.Err != "" || len(ev.Files) == 0 {
		return
	}

	c.changed[ev.Repo] = true
	c.line(ev.Repo, "✏️ ", ev.Task+": "+strings.Join(ev.Files, ", "))

	for _, m := range ev.Modules {
		c.line(ev.Repo, "  ⬆️ ", moduleLine(m))
	}
}

func (c *Console) repoDone(ev event.Event) {
	if ev.Err != "" {
		c.failed++
		c.line(ev.Repo, "❌", ev.Err)

		return
	}

	if ev.Pushed {
		c.pushed++
	}

	if ev.Coverage > 0 {
		c.line(ev.Repo, "📊", coverage(ev.Coverage, ev.PrevCoverage))
	}

	// In a run over a list, a repository with nothing to do is most of them,
	// and the summary counts them; one at a time, it is the answer.
	if !c.changed[ev.Repo] && !c.multi {
		c.line(ev.Repo, "✨", "nothing to change")
	}
}

func (c *Console) runDone() {
	took := elapsed(c.now().Sub(c.start))

	if !c.multi {
		c.line("", "🏁", "done in "+took)

		return
	}

	summary := fmt.Sprintf("%d repositories · %d pushed", c.repos, c.pushed)
	if c.failed > 0 {
		summary += fmt.Sprintf(" · %d failed", c.failed)
	}

	c.line("", "🏁", summary+" · "+took)
}

// elapsed rounds a duration to what a person reads: tenths of a second, or
// milliseconds where a run took less than one.
func elapsed(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}

	return d.Round(100 * time.Millisecond).String()
}

// line writes one line, naming the repository where several are in flight
// and their lines interleave.
func (c *Console) line(repo, icon, text string) {
	var prefix string

	switch {
	case repo == "":
	case c.multi:
		prefix = repo + " "
	default:
		prefix = "  "
	}

	_, _ = fmt.Fprintf(c.w, "%s%s %s\n", prefix, icon, text)
}

func coverage(now, before float64) string {
	s := fmt.Sprintf("coverage %.1f%%", now)
	if before > 0 && now != before {
		s += fmt.Sprintf(" (%+.1f)", now-before)
	}

	return s
}

func moduleLine(m event.ModuleChange) string {
	switch {
	case m.From == "":
		return m.Path + " " + m.To + " (added)"
	case m.To == "":
		return m.Path + " " + m.From + " (removed)"
	default:
		return m.Path + " " + m.From + " → " + m.To
	}
}
