package run

import (
	"testing"
	"time"

	"github.com/MarkRosemaker/devtool/internal/config"
	"github.com/MarkRosemaker/ordmap"
)

func TestFullMonthsSince(t *testing.T) {
	date := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	}

	for _, tc := range []struct {
		name       string
		start, end time.Time
		want       int
	}{
		{"same day", date(2025, time.January, 1), date(2025, time.January, 1), 0},
		{"part of a month does not count", date(2025, time.January, 1), date(2025, time.January, 31), 0},
		{"one whole month", date(2025, time.January, 1), date(2025, time.February, 1), 1},
		{"a day short of a month", date(2025, time.January, 15), date(2025, time.February, 14), 0},
		{"a year", date(2025, time.January, 1), date(2026, time.January, 1), 12},
		{"across a year boundary", date(2025, time.November, 10), date(2026, time.February, 10), 3},
		// A clock behind the start date must not produce a negative goal.
		{"end before start", date(2025, time.June, 1), date(2025, time.January, 1), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := fullMonthsSince(tc.start, tc.end); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func testConfig() config.Config {
	var repos ordmap.OrderedMap[string, *config.Repository]
	repos.Set("gorepo", &config.Repository{Coverage: 42})

	var orgRepos ordmap.OrderedMap[string, *config.Repository]
	orgRepos.Set("api", &config.Repository{Coverage: 100})

	var cfg config.Config
	cfg.Set("MarkRosemaker", config.Owner{Repositories: repos})
	cfg.Set("go-api-libs", config.Owner{Organization: true, Repositories: orgRepos})

	return cfg
}

func TestLookupRepo(t *testing.T) {
	cfg := testConfig()

	t.Run("found", func(t *testing.T) {
		repo, org, known := lookupRepo(cfg, "MarkRosemaker", "gorepo")

		if !known {
			t.Fatal("gorepo should be known")
		}

		if repo.Coverage != 42 {
			t.Errorf("got coverage %v, want 42", repo.Coverage)
		}

		if org {
			t.Error("MarkRosemaker should not be reported as an organisation")
		}
	})

	t.Run("organisation is reported", func(t *testing.T) {
		_, org, known := lookupRepo(cfg, "go-api-libs", "api")

		if !known {
			t.Fatal("api should be known")
		}

		if !org {
			t.Error("go-api-libs should be reported as an organisation")
		}
	})

	t.Run("unknown repository under a known, organisation owner", func(t *testing.T) {
		repo, org, known := lookupRepo(cfg, "go-api-libs", "new-lib")

		if known {
			t.Fatal("new-lib should not be known")
		}

		if !org {
			t.Error("an unknown repository under go-api-libs should still be reported as belonging to an organisation")
		}

		if repo.Coverage != 0 || repo.Description != "" {
			t.Errorf("a bootstrap repository should be the zero value, got %+v", repo)
		}
	})

	t.Run("unknown owner defaults to not an organisation", func(t *testing.T) {
		_, org, known := lookupRepo(cfg, "nobody", "x")

		if known {
			t.Fatal("a repository under an unknown owner should not be known")
		}

		if org {
			t.Error("an unknown owner should default to Organization: false")
		}
	})
}

func TestAddRepo(t *testing.T) {
	t.Run("new repository under a known owner keeps every existing position", func(t *testing.T) {
		cfg := testConfig()

		addRepo(cfg, "MarkRosemaker", "newrepo", false, &config.Repository{Coverage: 7})

		repo, org, known := lookupRepo(cfg, "MarkRosemaker", "newrepo")
		if !known || repo.Coverage != 7 || org {
			t.Fatalf("got (%+v, %v, %v)", repo, org, known)
		}

		// The owner order in the file, and gorepo's place within
		// MarkRosemaker, must both survive: a bootstrap is an addition, not
		// a reshuffle of everything already there.
		wantOwners := []string{"MarkRosemaker", "go-api-libs"}
		for i, o := range orderedKeys(cfg) {
			if o != wantOwners[i] {
				t.Errorf("owner %d is %q, want %q", i, o, wantOwners[i])
			}
		}

		wantRepos := []string{"gorepo", "newrepo"}
		for i, r := range orderedKeys(cfg["MarkRosemaker"].V.Repositories) {
			if r != wantRepos[i] {
				t.Errorf("repository %d is %q, want %q", i, r, wantRepos[i])
			}
		}
	})

	t.Run("new owner is appended", func(t *testing.T) {
		cfg := testConfig()

		addRepo(cfg, "newowner", "firstrepo", true, &config.Repository{})

		repo, org, known := lookupRepo(cfg, "newowner", "firstrepo")
		if !known || !org {
			t.Fatalf("got (%+v, %v, %v)", repo, org, known)
		}

		wantOwners := []string{"MarkRosemaker", "go-api-libs", "newowner"}
		for i, o := range orderedKeys(cfg) {
			if o != wantOwners[i] {
				t.Errorf("owner %d is %q, want %q", i, o, wantOwners[i])
			}
		}
	})
}

// orderedKeys returns m's keys in the order ByIndex reports them.
func orderedKeys[V any](m ordmap.OrderedMap[string, V]) []string {
	var keys []string
	for k := range m.ByIndex() {
		keys = append(keys, k)
	}

	return keys
}
