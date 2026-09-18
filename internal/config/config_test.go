package config_test

import (
	"slices"
	"testing"

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
