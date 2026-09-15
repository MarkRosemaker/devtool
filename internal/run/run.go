// Package run makes one pass over a list of repositories: it works out what to
// maintain and in what order, does the work, and emits what happened.
//
// It reports as events rather than to a person. Whatever is watching — a
// terminal reading JSON Lines, or patchpal rendering them into a chat — is a
// separate process, which is the point: this one is started fresh for every
// run, so a released change takes effect on the next run rather than the next
// restart of something long-lived.
package run

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/MarkRosemaker/devtool-engine/depgraph"
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/devtool/internal/config"
	"github.com/MarkRosemaker/ghrepo"
	"github.com/MarkRosemaker/gorepo"
)

// TokenEnv names the environment variable holding the GitHub token.
const TokenEnv = "GITHUB_TOKEN"

// Service maintains a list of repositories.
type Service struct {
	repos   *gorepo.Service
	runner  *engine.Runner
	events  engine.Emitter
	verbose bool

	// cfg is where the list lives and which repository holds it. The coverage
	// a run measures is written back there, so the next run on any machine
	// starts from it.
	cfg ConfigFile
}

// New builds a service that emits its progress to events.
//
// When verbose is set, the run reports its outcome even if nothing changed;
// otherwise a quiet, uneventful run stays quiet.
func New(ctx context.Context, cfg ConfigFile, events engine.Emitter, verbose bool) (*Service, error) {
	token := os.Getenv(TokenEnv)
	if token == "" {
		return nil, fmt.Errorf("%s is not set", TokenEnv)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locating home directory: %w", err)
	}

	return &Service{
		repos: gorepo.NewService(ctx, token,
			ghrepo.WithBaseDir(filepath.Join(home, "go/src/github.com/")),
			ghrepo.MakeDirAll,
			ghrepo.CloneGit,
			ghrepo.CreateRemote,
			ghrepo.CreateOnGitHub,
		),
		runner:  &engine.Runner{},
		events:  events,
		verbose: verbose,
		cfg:     cfg,
	}, nil
}

// Run makes one pass over the portfolio.
func (s *Service) Run(ctx context.Context) error {
	cfg, err := config.Load(s.cfg.Path)
	if err != nil {
		return err
	}

	previous, err := config.LoadState(s.cfg.StatePath)
	if err != nil {
		slog.WarnContext(ctx, "could not read the last run's state", "error", err)
	}

	logProgress(ctx, config.Count(cfg))

	plan, err := s.plan(ctx, cfg)
	if err != nil {
		return err
	}

	graph, err := depgraph.New(plan.keys, plan.deps)
	if err != nil {
		return fmt.Errorf("ordering repositories: %w", err)
	}

	slog.InfoContext(ctx, "dependency graph built", "repos", len(plan.keys))

	results := s.execute(ctx, graph, plan)

	return s.finish(ctx, cfg, previous, results)
}

// RunOne maintains a single repository, named "owner/name", instead of the
// whole portfolio. A repository the portfolio does not know about yet is not
// an error: it is maintained the same as any other, and joins the portfolio —
// so a future full [Service.Run] picks it up too — once it has actually
// succeeded. A failed bootstrap leaves no trace, for the next attempt to find
// exactly as it left it.
//
// It skips the dependency graph a full Run builds to order every repository:
// nothing else in this run is being maintained for one repository to wait on.
// Past that, it goes through the same execute path as Run, so a
// single-repository update reports exactly like one row of a full run would.
//
// Unlike Run, a failure in the repository itself is returned as an error, not
// only carried in the event stream — this is most often run by a person at a
// terminal watching for the exit code, not unattended.
func (s *Service) RunOne(ctx context.Context, owner, name string) error {
	cfg, err := config.Load(s.cfg.Path)
	if err != nil {
		return err
	}

	repoCfg, org, known := lookupRepo(cfg, owner, name)

	if err := s.prefetch(ctx, owner, org); err != nil {
		return err
	}

	u := &unit{owner: owner, name: name, cfg: repoCfg}
	s.open(ctx, u, nil, map[string][]string{})

	graph, err := depgraph.New([]string{u.key()}, nil)
	if err != nil {
		return fmt.Errorf("ordering repositories: %w", err)
	}

	results := s.execute(ctx, graph, &plan{
		keys:  []string{u.key()},
		units: map[string]*unit{u.key(): u},
	})

	res := results[0]

	if !known && res.Err == nil {
		addRepo(cfg, owner, name, org, repoCfg)

		slog.InfoContext(ctx, "added a new repository to the portfolio", "repo", u.key())
	}

	if err := s.saveConfig(ctx, cfg); err != nil {
		return err
	}

	return res.Err
}

// lookupRepo finds a repository's configuration by owner and name, and
// reports whether the owner is a GitHub organisation.
//
// A repository not yet in the portfolio is not an error: it comes back as a
// fresh, zero-value configuration and known=false, for RunOne to bootstrap.
// Its owner's Organization flag, when the owner is likewise new, is assumed
// false — there is no cheap way to ask GitHub which an owner is without
// listing every one of its repositories, more than opening a single one
// calls for. That covers the common case of a new personal repository; a new
// organisation can be added once by hand in config.json.
func lookupRepo(cfg config.Config, owner, name string) (repo *config.Repository, isOrg, known bool) {
	o, ok := cfg[owner]
	if !ok {
		return &config.Repository{}, false, false
	}

	r, ok := o.V.Repositories[name]
	if !ok {
		return &config.Repository{}, o.V.Organization, false
	}

	return r.V, o.V.Organization, true
}

// addRepo inserts name into owner's repositories, creating the owner's entry
// first if this is its first repository in the portfolio.
//
// Every other owner, and every other repository of this one, keeps its
// existing position in the file: only the new entries are appended, at the
// end of their respective lists.
func addRepo(cfg config.Config, owner, name string, isOrg bool, repo *config.Repository) {
	o, ok := cfg[owner]
	if !ok {
		cfg.Set(owner, config.Owner{Organization: isOrg})

		o = cfg[owner]
	}

	o.V.Repositories.Set(name, repo)
	cfg[owner] = o
}

// unit is one repository's place in a run.
type unit struct {
	owner, name string
	cfg         *config.Repository

	// repo is nil when the repository could not be opened, in which case err
	// says why. Keeping the failure here lets it be reported as that
	// repository's result rather than aborting the whole run.
	repo *repository
	err  error
}

// key returns the unit's canonical identifier.
func (u *unit) key() string { return engine.Key(u.owner, u.name) }

// plan is the work a run has decided to do, and the order it has to respect.
type plan struct {
	keys  []string            // every repository, in configuration order
	units map[string]*unit    // key → unit
	deps  map[string][]string // key → the keys it depends on
}

// plan opens every configured repository and works out which of them depend on
// which others.
//
// Opening each repository here, once, is also what lets the run reuse it later:
// the dependency read and the maintenance that follows work on the same clone.
func (s *Service) plan(ctx context.Context, cfg config.Config) (*plan, error) {
	count := config.Count(cfg)

	// Module path → key, so a go.mod requirement can be recognised as one of
	// the repositories in this run.
	byModulePath := make(map[string]string, count)
	for ownerName, owner := range cfg.ByIndex() {
		for name := range owner.Repositories.ByIndex() {
			key := engine.Key(ownerName, name)
			byModulePath["github.com/"+key] = key
		}
	}

	p := &plan{
		keys:  make([]string, 0, count),
		units: make(map[string]*unit, count),
		deps:  make(map[string][]string, count),
	}

	for ownerName, owner := range cfg.ByIndex() {
		if err := s.prefetch(ctx, ownerName, owner.Organization); err != nil {
			return nil, err
		}

		for name, repoCfg := range owner.Repositories.ByIndex() {
			u := &unit{owner: ownerName, name: name, cfg: repoCfg}

			p.keys = append(p.keys, u.key())
			p.units[u.key()] = u

			s.open(ctx, u, byModulePath, p.deps)
		}
	}

	return p, nil
}

// prefetch loads an owner's repository metadata in one request, so that opening
// each repository afterwards does not cost one of its own.
func (s *Service) prefetch(ctx context.Context, owner string, isOrg bool) error {
	if isOrg {
		if err := s.repos.PrefetchOrgRepositories(ctx, owner); err != nil {
			return fmt.Errorf("prefetching repositories for organisation %q: %w", owner, err)
		}

		return nil
	}

	if err := s.repos.PrefetchUserRepositories(ctx, owner); err != nil {
		return fmt.Errorf("prefetching repositories for user %q: %w", owner, err)
	}

	return nil
}

// open clones or locates the unit's repository and reads its dependencies.
//
// Failure to open is recorded on the unit rather than returned: one unreachable
// repository should be reported as such, not stop the rest of the portfolio from
// being maintained.
func (s *Service) open(ctx context.Context, u *unit, byModulePath map[string]string, deps map[string][]string) {
	repo, err := s.repos.NewRepository(ctx, u.owner, u.name)
	if err != nil {
		u.err = fmt.Errorf("opening repository: %w", err)

		slog.WarnContext(ctx, "could not open repository", "repo", u.key(), "error", err)

		return
	}

	u.repo = &repository{Repository: repo}

	// The dependency read needs the current go.mod, so pull before reading it.
	// A failure here is not fatal: the worktree may simply be stale, and the
	// maintenance pass pulls again and reports properly if it cannot.
	if err := u.repo.Pull(ctx); err != nil {
		slog.WarnContext(ctx, "could not pull before reading dependencies",
			"repo", u.key(), "error", err)
	}

	requires, err := u.repo.Dependencies()
	if err != nil {
		// No go.mod yet. The repository has no dependencies to order it by,
		// and the maintenance pass will initialise the module.
		slog.DebugContext(ctx, "no dependencies to read", "repo", u.key(), "error", err)

		return
	}

	deps[u.key()] = engine.ModuleDeps(requires, byModulePath)
}

// execute maintains every repository, in dependency order and as parallel as
// that order allows, streaming progress as results arrive.
func (s *Service) execute(ctx context.Context, graph *depgraph.Graph, p *plan) []engine.Result {
	rows := make([]engine.Result, len(p.keys))
	for i, key := range p.keys {
		u := p.units[key]
		rows[i] = engine.Result{Owner: u.owner, Name: u.name}
	}

	// The board is kept for the results it returns in configuration order,
	// which is the order a reader expects. Rendering one is the job of
	// whatever is reading the events, in its own process.
	board := engine.NewBoard(rows)

	events := engine.EmitterFunc(func(ev engine.Event) {
		board.Apply(ev)
		s.events.Emit(ev)
	})

	// The run-level pair is this side's to emit: a Runner only ever sees one
	// repository, so only the caller knows where a run begins and ends. It
	// names every repository it covers, because a reader in another process
	// has no other way to know what rows its table should have.
	engine.Emit(events, engine.Event{Kind: engine.RunStart, Repos: p.keys})
	defer engine.Emit(events, engine.Event{Kind: engine.RunDone})

	depgraph.Run(ctx, graph,
		func(ctx context.Context, key string) engine.Result {
			return s.maintain(ctx, p.units[key], events)
		},
		func(res engine.Result) {
			// A repository the engine never saw emits nothing, so its row is
			// filled in here: it failed before there was anything to run.
			if res.Err != nil {
				board.Set(res)
				engine.Emit(events, engine.Event{
					Kind: engine.RepoDone,
					Repo: res.Key(),
					Err:  res.ErrorMessage(),
				})
			}
		})

	// The board holds the results in configuration order, which is the order
	// the reader expects, rather than the order they happened to finish in.
	return board.Results()
}

// maintain brings one repository up to standard and records its new coverage.
func (s *Service) maintain(
	ctx context.Context, u *unit, events engine.Emitter,
) engine.Result {
	if u.err != nil {
		return engine.Result{Owner: u.owner, Name: u.name, Err: u.err}
	}

	res := s.runner.Update(ctx, u.repo, u.cfg.Spec(), u.repo.sequence, events)

	// Write the measured coverage back so the next run can report the change.
	// Each unit owns its own configuration entry, so concurrent runs do not
	// contend here.
	if res.Err == nil {
		u.cfg.Coverage = res.Coverage
	}

	s.pruneOccasionally(ctx, u.repo)

	return res
}

// pruneRate is how often a repository gets its object store tidied. Repacking
// every repository every run would dominate the run time for a saving that only
// matters over weeks, so it is spread thinly instead.
const pruneRate = 0.05

// pruneOccasionally compacts the repository's git objects now and then.
//
// A failure is logged and otherwise ignored: housekeeping that did not happen is
// not a reason to report the repository as failed.
func (s *Service) pruneOccasionally(ctx context.Context, repo *repository) {
	if rand.Float32() >= pruneRate {
		return
	}

	start := time.Now()

	if _, err := repo.ExecCommand(ctx, "git", "maintenance", "run",
		"--task=gc", "--task=loose-objects", "--task=pack-refs"); err != nil {
		slog.WarnContext(ctx, "pruning failed", "repo", repo.String(), "error", err)

		return
	}

	slog.InfoContext(ctx, "pruned", "repo", repo.String(), "duration", time.Since(start))
}

// finish records the run, reports it, and commits the coverage it measured.
func (s *Service) finish(
	ctx context.Context,
	cfg config.Config,
	previous *config.State,
	results []engine.Result,
) error {
	failed := false
	notable := s.verbose

	for _, res := range results {
		failed = failed || res.Err != nil
		notable = notable || res.Notable()
	}

	if err := config.SaveState(s.cfg.StatePath, results); err != nil {
		slog.WarnContext(ctx, "could not record this run's state", "error", err)
	}

	// A clean run after a failing one is worth saying out loud; a clean run
	// after a clean one is not, or the signal stops meaning anything.
	if previous != nil && previous.HadError && !failed {
		slog.InfoContext(ctx, "recovered: this run is clean after a failing one")
	}

	if !notable {
		slog.InfoContext(ctx, "nothing to report")
	}

	return s.saveConfig(ctx, cfg)
}

// saveConfig writes the configuration and commits it to the repository holding
// it, so the coverage this run measured is available to the next run on any
// machine.
func (s *Service) saveConfig(ctx context.Context, cfg config.Config) error {
	if err := config.Save(s.cfg.Path, cfg); err != nil {
		return err
	}

	return s.commitConfig(ctx)
}

// commitConfig commits the coverage figures this run measured, so the next run
// on any machine starts from them.
func (s *Service) commitConfig(ctx context.Context) error {
	repo, err := s.repos.NewRepository(ctx, s.cfg.Owner, s.cfg.Name)
	if err != nil {
		return fmt.Errorf("opening the repository holding the config: %w", err)
	}

	if err := repo.Commit([]string{filepath.Base(s.cfg.Path)}, "update config"); err != nil {
		return fmt.Errorf("committing the config: %w", err)
	}

	if err := repo.Push(ctx); err != nil {
		return fmt.Errorf("pushing the config: %w", err)
	}

	return nil
}

// goalStart is when the one-repository-a-month goal began.
var goalStart = time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)

// baseGoal is the three repositories the portfolio started with, less the
// portfolio itself, which is infrastructure rather than a published library.
const baseGoal = 3 - 1

// logProgress records how the portfolio stands against the goal of publishing
// one open source repository a month.
func logProgress(ctx context.Context, count int) {
	goal := baseGoal + fullMonthsSince(goalStart, time.Now())

	slog.InfoContext(ctx, "starting to process open source repositories",
		"num_repos", count, "num_repos_goal", goal)
}

// fullMonthsSince counts the whole months between two times, ignoring any
// partial month at the end.
func fullMonthsSince(start, end time.Time) int {
	if end.Before(start) {
		return 0
	}

	startYear, startMonth, startDay := start.Date()
	endYear, endMonth, endDay := end.Date()

	months := (endYear-startYear)*12 + int(endMonth-startMonth)

	// The current month only counts once its day of the month has come round.
	if endDay < startDay {
		months--
	}

	return max(months, 0)
}
