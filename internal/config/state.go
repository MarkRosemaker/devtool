package config

import (
	"errors"
	"io/fs"
	"time"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/jsonutil"
)

// StatePath is where the outcome of the last run is recorded. It is not
// committed: it describes this machine's last run, not the portfolio.
const StateName = "last_run.json"

// State is what one run remembers for the next one.
type State struct {
	Time time.Time `json:"time,omitzero"`

	// HadError is what makes recovery noticeable: a run that succeeds after
	// one that failed is worth saying so, and a run that succeeds after one
	// that also succeeded is not.
	HadError bool `json:"hadError,omitzero"`
}

// LoadState reads the last run's outcome.
//
// A missing file means there is no previous run — the first run on a new
// machine, or after the file was cleared — which is not an error, and comes back
// as a nil State.
func LoadState(path string) (*State, error) {
	state, err := jsonutil.ReadFile[State](path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return &state, nil
}

// SaveState records the outcome of a run.
func SaveState(path string, results []engine.Result) error {
	return jsonutil.WriteFile(path, State{
		Time:     time.Now(),
		HadError: anyFailed(results),
	})
}

// anyFailed reports whether any repository failed.
func anyFailed(results []engine.Result) bool {
	for _, res := range results {
		if res.Err != nil {
			return true
		}
	}

	return false
}
