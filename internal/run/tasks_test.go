package run

import (
	"testing"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/devtool/maintain"
	"github.com/MarkRosemaker/gorepo"
)

// TestTheMaintainedSequenceWritesTheDefinition pins where devtool.json is
// written in a maintained run: inside the sequence, as part of the first
// task, so the engine commits it with everything else devtool owns.
//
// It used to be written by the service after the engine returned — after the
// commits and the push — so it sat uncommitted in the worktree until the next
// run's prepare discarded it. Not one devtool.json in the portfolio came from
// a run. The end-to-end half of this is in internal/local, which drives the
// same task through the same engine against a real remote; this is the half
// that says the portfolio still uses it.
func TestTheMaintainedSequenceWritesTheDefinition(t *testing.T) {
	// Building the sequence only takes method values; nothing here is called,
	// so a zero repository is enough.
	repo := &repository{Repository: &gorepo.Repository{}}

	seq := tasks(&engine.Runner{}, repo, engine.Spec{}, "v0.0.0-20260917155344-c047bdcfd843")

	want := maintain.UpdateTask(nil, maintain.UpdateOptions{}).Name

	if len(seq) == 0 || seq[0].Name != want {
		t.Fatalf("the sequence does not begin with %q", want)
	}

	for _, task := range seq[1:] {
		if task.Name == want {
			t.Errorf("%q appears twice", want)
		}
	}
}

// TestTheListsCoverageWinsOverTheDefinitions guards the regression this fix
// would otherwise have caused. Once devtool.json actually lands, every
// repository has a coverage figure in it; if that won over the list, the list's
// figure — the one a run measures and writes back every time — would be
// ignored from then on, and every badge would freeze at its first value.
func TestTheListsCoverageWinsOverTheDefinitions(t *testing.T) {
	def := maintain.Definition{
		Description: "from the repository",
		Topics:      []string{"repo"},
		Coverage:    50,
	}

	for _, tc := range []struct {
		name  string
		list  float64
		found bool
		want  float64
	}{
		{"the list has a measurement", 80, true, 80},
		{"the list has none yet", 0, true, 50},
		{"there is no definition", 80, false, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := withDefinition(engine.Spec{Description: "from the list", Coverage: tc.list}, def, tc.found)

			if got.Coverage != tc.want {
				t.Errorf("coverage = %v, want %v", got.Coverage, tc.want)
			}
		})
	}

	// Everything else goes the ordinary way: the repository says what it is.
	got := withDefinition(engine.Spec{Description: "from the list", Coverage: 80}, def, true)
	if got.Description != def.Description || len(got.Topics) != 1 {
		t.Errorf("the definition did not take over description and topics: %+v", got)
	}
}
