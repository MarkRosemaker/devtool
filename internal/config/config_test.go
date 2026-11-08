package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/MarkRosemaker/devtool-engine/event"
	"github.com/MarkRosemaker/devtool/internal/config"
	"github.com/MarkRosemaker/ordmap"
)

func owner(isOrg bool, names ...string) config.Owner {
	var repos ordmap.OrderedMap[string, *config.Repository]
	for _, name := range names {
		repos.Set(name, &config.Repository{})
	}

	return config.Owner{Organization: isOrg, Repositories: repos}
}

// The names here are deliberately not in alphabetical order, in either
// dimension: a run announces this list before it has opened anything, and the
// results come back in the file's order, so a Keys that sorted would label
// every row with the wrong repository.
func TestKeysAreInFileOrder(t *testing.T) {
	var cfg config.Config
	cfg.Set("zeta", owner(false, "second", "first"))
	cfg.Set("alpha", owner(true, "api"))

	want := []string{"zeta/second", "zeta/first", "alpha/api"}

	if got := config.Keys(cfg); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestKeysCountsEveryRepository(t *testing.T) {
	var cfg config.Config
	cfg.Set("alpha", owner(false, "one", "two"))
	cfg.Set("beta", owner(true, "three"))

	if got, want := len(config.Keys(cfg)), config.Count(cfg); got != want {
		t.Errorf("got %d keys, want %d", got, want)
	}
}

func TestKeysOfAnEmptyConfig(t *testing.T) {
	var cfg config.Config

	if got := config.Keys(cfg); len(got) != 0 {
		t.Errorf("got %q, want none", got)
	}
}

// A run reads the list, records coverage and writes it back: everything else,
// order included, has to survive the round trip.
func TestSaveAndLoadKeepTheList(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.DefaultName)

	var cfg config.Config
	cfg.Set("zeta", owner(false, "second", "first"))
	cfg.Set("alpha", owner(true, "api"))
	cfg["zeta"].V.Repositories["first"].V.Coverage = 87.5
	cfg["alpha"].V.Repositories["api"].V.Topics = []string{"go", "api"}

	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"zeta/second", "zeta/first", "alpha/api"}; !slices.Equal(config.Keys(got), want) {
		t.Errorf("keys = %q, want %q", config.Keys(got), want)
	}

	if !got["alpha"].V.Organization || got["zeta"].V.Organization {
		t.Error("the organisation flags did not survive")
	}

	if c := got["zeta"].V.Repositories["first"].V.Coverage; c != 87.5 {
		t.Errorf("coverage = %v, want 87.5", c)
	}

	if topics := got["alpha"].V.Repositories["api"].V.Topics; !slices.Equal(topics, []string{"go", "api"}) {
		t.Errorf("topics = %q", topics)
	}
}

func TestLoadFailsOnAMissingOrBrokenList(t *testing.T) {
	dir := t.TempDir()

	if _, err := config.Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing list was accepted")
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := config.Load(broken); err == nil {
		t.Error("a broken list was accepted")
	}

	if err := config.Save(filepath.Join(dir, "no", "such", "dir.json"), nil); err == nil {
		t.Error("saving into a missing directory succeeded")
	}
}

func TestSpecCarriesTheEntry(t *testing.T) {
	r := config.Repository{Description: "d", Topics: []string{"t"}, Coverage: 81, LintInTests: true}

	got := r.Spec()
	if got.Description != "d" || !slices.Equal(got.Topics, []string{"t"}) ||
		got.Coverage != 81 || !got.LintInTests {
		t.Errorf("spec = %+v", got)
	}
}

func TestStateRemembersAFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.StateName)

	state, err := config.LoadState(path)
	if err != nil || state != nil {
		t.Fatalf("no previous run: got %+v, %v; want nil, nil", state, err)
	}

	for _, tc := range []struct {
		name    string
		results []event.Result
		want    bool
	}{
		{"a clean run", []event.Result{{Name: "a"}, {Name: "b"}}, false},
		{"one failure", []event.Result{{Name: "a"}, {Name: "b", Err: errors.New("boom")}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := config.SaveState(path, tc.results); err != nil {
				t.Fatal(err)
			}

			state, err := config.LoadState(path)
			if err != nil {
				t.Fatal(err)
			}

			if state.HadError != tc.want || state.Time.IsZero() {
				t.Errorf("state = %+v, want HadError %v and a time", state, tc.want)
			}
		})
	}

	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := config.LoadState(path); err == nil {
		t.Error("a broken state file was read as no previous run")
	}
}
