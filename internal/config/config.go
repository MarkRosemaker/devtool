// Package config reads and writes the list of repositories to maintain: which
// they are, what metadata each should carry, and the coverage last measured for
// it.
//
// The file is both input and output. A run reads it to know what to do, and
// writes it back with the coverage it measured, so the next run can report the
// change. Where it lives is the caller's to say — it belongs to whoever owns
// the list, not to this tool.
package config

import (
	"encoding/json/jsontext"
	"fmt"

	"github.com/MarkRosemaker/devtool-engine/event"
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/jsonutil"
	"github.com/MarkRosemaker/ordmap"
)

// DefaultName is the file a caller that named no path looks for, in the
// working directory.
const DefaultName = "config.json"

// Config maps GitHub owner names to their configuration.
//
// The order is the order in the file, and it is the order results are reported
// in, so the file doubles as the reader's index into the output.
type Config = ordmap.OrderedMap[string, Owner]

// Owner is a GitHub user or organisation whose repositories are maintained.
type Owner struct {
	// Organization marks an owner that is a GitHub organisation rather than a
	// user. The two are listed through different API endpoints.
	Organization bool `json:"organization,omitempty"`

	Repositories ordmap.OrderedMap[string, *Repository] `json:"repositories"`
}

// Repository is the desired state of one repository.
type Repository struct {
	Description string   `json:"description,omitempty"`
	Topics      []string `json:"topics,omitempty"`

	// Coverage is the test coverage measured by the last run. It is written
	// back after every run, which is what makes a coverage delta reportable.
	Coverage float64 `json:"coverage,omitempty"`

	// LintInTests marks a repository whose test suite runs the linter itself.
	LintInTests bool `json:"lintInTests,omitempty"`
}

// Spec converts the configuration into what the maintenance engine needs,
// keeping the engine independent of this file's shape.
func (r *Repository) Spec() engine.Spec {
	return engine.Spec{
		Description: r.Description,
		Topics:      r.Topics,
		Coverage:    r.Coverage,
		LintInTests: r.LintInTests,
	}
}

// Load reads the configuration from path.
func Load(path string) (Config, error) {
	cfg, err := jsonutil.ReadFile[Config](path)
	if err != nil {
		return cfg, fmt.Errorf("loading config: %w", err)
	}

	return cfg, nil
}

// Save writes the configuration back to path, formatted so the diff of a run
// shows only the values that actually changed.
func Save(path string, cfg Config) error {
	if err := jsonutil.WriteFile(path, cfg, jsontext.Multiline(true)); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	return nil
}

// Keys returns every repository the configuration covers, as "owner/name", in
// the file's own order.
//
// That order is the one a reader expects to see a run's repositories in, and
// it is fixed by the configuration rather than by anything a run discovers —
// which is what lets a run name what it covers before it has opened anything.
func Keys(cfg Config) []string {
	keys := make([]string, 0, Count(cfg))
	for ownerName, owner := range cfg.ByIndex() {
		for name := range owner.Repositories.ByIndex() {
			keys = append(keys, event.Key(ownerName, name))
		}
	}

	return keys
}

// Count returns how many repositories the configuration covers.
func Count(cfg Config) int {
	count := 0
	for _, owner := range cfg.ByIndex() {
		count += len(owner.Repositories)
	}

	return count
}
