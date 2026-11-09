package run

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/MarkRosemaker/devtool-engine/event"
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/devtool/internal/config"
	"github.com/MarkRosemaker/ghrepo"
	"github.com/MarkRosemaker/gorepo"
	"github.com/google/go-github/v80/github"
)

// fakeSource opens real git repositories under a temporary directory, and
// records what a run would otherwise ask of GitHub.
type fakeSource struct {
	t    *testing.T
	base string
	gh   *gorepo.Service

	mu          sync.Mutex
	prefetched  []string
	prefetchErr map[string]error
	openErr     map[string]error
	coverage    map[string]float64
	public      map[string]bool
	tasks       map[string]func(r *fakeRepo) []engine.Task
	opened      map[string]*fakeRepo
	onPull      map[string]func()
	ran         []string
}

func newFakeSource(t *testing.T) *fakeSource {
	t.Helper()

	isolateGit(t)

	base := t.TempDir()

	return &fakeSource{
		t:           t,
		base:        base,
		gh:          gorepo.NewService(t.Context(), "", ghrepo.WithBaseDir(base)),
		prefetchErr: map[string]error{},
		openErr:     map[string]error{},
		coverage:    map[string]float64{},
		public:      map[string]bool{},
		tasks:       map[string]func(r *fakeRepo) []engine.Task{},
		opened:      map[string]*fakeRepo{},
		onPull:      map[string]func(){},
	}
}

// add creates owner/name as a committed git repository holding files.
func (f *fakeSource) add(owner, name string, files map[string]string) string {
	f.t.Helper()

	dir := filepath.Join(f.base, owner, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}

	gitRun(f.t, dir, "init", "-q", "-b", "main")
	gitRun(f.t, dir, "config", "user.email", "t@example.com")
	gitRun(f.t, dir, "config", "user.name", "T")
	// Never contacted: the fake answers Pull and Push itself.
	gitRun(f.t, dir, "remote", "add", "origin", "https://github.com/"+owner+"/"+name+".git")

	for path, content := range files {
		writeFile(f.t, filepath.Join(dir, path), content)
	}

	gitRun(f.t, dir, "add", "-A")
	gitRun(f.t, dir, "commit", "-qm", "first", "--allow-empty")

	return dir
}

func (f *fakeSource) open(ctx context.Context, owner, name string) (maintainedRepo, error) {
	key := event.Key(owner, name)

	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.openErr[key]; err != nil {
		return nil, err
	}

	// A GitHub repository given up front, so opening never asks the API.
	r, err := f.gh.NewRepository(ctx, owner, name, ghrepo.WithGithubRepo(&github.Repository{}))
	if err != nil {
		return nil, err
	}

	fr := &fakeRepo{repository: &repository{Repository: r}, src: f, key: key}
	f.opened[key] = fr

	return fr, nil
}

func (f *fakeSource) prefetch(_ context.Context, owner string, isOrg bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	kind := "user:"
	if isOrg {
		kind = "org:"
	}

	f.prefetched = append(f.prefetched, kind+owner)

	return f.prefetchErr[owner]
}

func (f *fakeSource) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.ran = append(f.ran, s)
}

func (f *fakeSource) repo(key string) *fakeRepo {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.opened[key]
}

// fakeRepo is a real repository whose network operations are recorded instead.
type fakeRepo struct {
	*repository

	src *fakeSource
	key string

	mu          sync.Mutex
	pushes      int
	description string
}

func (r *fakeRepo) Pull(context.Context) error {
	r.src.mu.Lock()
	pull := r.src.onPull[r.key]
	r.src.mu.Unlock()

	if pull != nil {
		pull()
	}

	return nil
}

func (r *fakeRepo) Push(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pushes++

	return nil
}

func (r *fakeRepo) SetDescription(_ context.Context, descr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.description = descr

	return nil
}

func (r *fakeRepo) SetTopics(context.Context, []string) error { return nil }

func (r *fakeRepo) Private() bool { return !r.src.public[r.key] }

func (r *fakeRepo) GoTestCover(context.Context) (float64, error) {
	r.src.mu.Lock()
	defer r.src.mu.Unlock()

	return r.src.coverage[r.key], nil
}

func (r *fakeRepo) sequence(string) engine.Sequence {
	return func(*engine.Runner, engine.Repo, engine.Spec) []engine.Task {
		r.src.mu.Lock()
		build := r.src.tasks[r.key]
		r.src.mu.Unlock()

		if build == nil {
			return nil
		}

		return build(r)
	}
}

func (r *fakeRepo) pushCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.pushes
}

// writeTask writes a file into the repository, recording when it ran.
func writeTask(r *fakeRepo, path, content string) engine.Task {
	return engine.Task{Name: "write " + path, Run: func(context.Context) error {
		r.src.record(r.key)

		return os.WriteFile(filepath.Join(r.src.base, r.key, path), []byte(content), 0o600)
	}}
}

// isolateGit keeps git, and go-git, away from the machine's own configuration.
func isolateGit(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// recorder collects events from every goroutine of a run.
type recorder struct {
	mu     sync.Mutex
	events []event.Event
}

func (r *recorder) Emit(ev event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, ev)
}

func (r *recorder) of(kind event.Kind) []event.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []event.Event

	for _, ev := range r.events {
		if ev.Kind == kind {
			out = append(out, ev)
		}
	}

	return out
}

const portfolioJSON = `{
  "alice": {
    "repositories": {
      "app": {},
      "lib": {"coverage": 70}
    }
  },
  "acme": {
    "organization": true,
    "repositories": {
      "tool": {"coverage": 85}
    }
  }
}
`

// portfolio sets up a list held in alice/portfolio and a service over it.
func portfolio(t *testing.T, list string) (*Service, *fakeSource, *recorder) {
	t.Helper()

	src := newFakeSource(t)
	dir := src.add("alice", "portfolio", map[string]string{config.DefaultName: list})

	rec := &recorder{}

	return &Service{
		repos:   src,
		runner:  &engine.Runner{},
		events:  rec,
		version: "v1.0.0",
		cfg: ConfigFile{
			Path:      filepath.Join(dir, config.DefaultName),
			StatePath: filepath.Join(t.TempDir(), config.StateName),
			Owner:     "alice",
			Name:      "portfolio",
		},
	}, src, rec
}

func goMod(module string, requires ...string) string {
	s := "module " + module + "\n\ngo 1.27.0\n"
	for _, r := range requires {
		s += "\nrequire " + r + " v1.0.0\n"
	}

	return s
}

func TestRunMaintainsThePortfolio(t *testing.T) {
	svc, src, rec := portfolio(t, portfolioJSON)

	src.add("alice", "app", map[string]string{
		"go.mod": goMod("github.com/alice/app", "github.com/alice/lib", "github.com/alice/gone"),
	})
	libDir := src.add("alice", "lib", map[string]string{"go.mod": goMod("github.com/alice/lib")})
	src.add("acme", "tool", map[string]string{"go.mod": goMod("github.com/acme/tool")})

	src.coverage["alice/lib"] = 91.5
	src.coverage["alice/app"] = 88
	src.coverage["acme/tool"] = 85

	src.tasks["alice/lib"] = func(r *fakeRepo) []engine.Task {
		return []engine.Task{writeTask(r, "NOTES", "changed\n")}
	}
	src.tasks["alice/app"] = func(r *fakeRepo) []engine.Task {
		return []engine.Task{{Name: "look", Run: func(context.Context) error {
			r.src.record(r.key)

			return nil
		}}}
	}

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if want := []string{"user:alice", "org:acme"}; !slices.Equal(src.prefetched, want) {
		t.Errorf("prefetched %v, want %v", src.prefetched, want)
	}

	// The run is announced in the list's order, before anything is opened.
	if starts := rec.of(event.RunStart); len(starts) != 1 ||
		!slices.Equal(starts[0].Repos, []string{"alice/app", "alice/lib", "acme/tool"}) {
		t.Errorf("run_start = %+v", starts)
	}

	if last := rec.events[len(rec.events)-1]; last.Kind != event.RunDone {
		t.Errorf("the last event is %q, want run_done", last.Kind)
	}

	var opening []string
	for _, ev := range rec.of(event.RunProgress) {
		opening = append(opening, ev.Repo)
	}

	if want := []string{"alice/app", "alice/lib", "acme/tool"}; !slices.Equal(opening, want) {
		t.Errorf("opened %v, want %v", opening, want)
	}

	if gaps := rec.of(event.Unmaintained); len(gaps) != 1 ||
		gaps[0].Repo != "alice/app" || gaps[0].Dependency != "alice/gone" {
		t.Errorf("unmaintained = %+v, want alice/app needing alice/gone", gaps)
	}

	// app requires lib, so lib is maintained first.
	if want := []string{"alice/lib", "alice/app"}; !slices.Equal(src.ran, want) {
		t.Errorf("tasks ran for %v, want %v", src.ran, want)
	}

	if got := gitRun(t, libDir, "log", "-1", "--format=%s"); got != "write NOTES" {
		t.Errorf("lib's last commit is %q, want the task's", got)
	}

	if n := src.repo("alice/lib").pushCount(); n != 1 {
		t.Errorf("lib pushed %d times, want once", n)
	}

	if n := src.repo("alice/app").pushCount(); n != 0 {
		t.Errorf("app pushed %d times with nothing committed", n)
	}

	cfg, err := config.Load(svc.cfg.Path)
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg["alice"].V.Repositories["lib"].V.Coverage; got != 91.5 {
		t.Errorf("lib's coverage = %v, want the measured 91.5", got)
	}

	if got := config.Keys(cfg); !slices.Equal(got, []string{"alice/app", "alice/lib", "acme/tool"}) {
		t.Errorf("the saved list reads %v, out of order", got)
	}

	// The list's coverage is committed and pushed where it lives.
	listDir := filepath.Dir(svc.cfg.Path)
	if got := gitRun(t, listDir, "log", "-1", "--format=%s"); got != "update config" {
		t.Errorf("the list's last commit is %q", got)
	}

	if got := gitRun(t, listDir, "show", "HEAD:"+config.DefaultName); !strings.Contains(got, "91.5") {
		t.Errorf("the committed list does not hold the new coverage:\n%s", got)
	}

	if n := src.repo("alice/portfolio").pushCount(); n != 1 {
		t.Errorf("the list was pushed %d times, want once", n)
	}

	state, err := config.LoadState(svc.cfg.StatePath)
	if err != nil || state == nil || state.HadError {
		t.Errorf("state = %+v, %v; want a clean run recorded", state, err)
	}
}

// A repository that cannot be opened is that repository's failure, not the
// run's: the others are maintained, and the failure is remembered.
func TestRunReportsARepositoryItCannotOpen(t *testing.T) {
	svc, src, rec := portfolio(t, portfolioJSON)

	src.add("alice", "lib", map[string]string{"go.mod": goMod("github.com/alice/lib")})
	src.add("acme", "tool", map[string]string{"go.mod": goMod("github.com/acme/tool")})
	src.openErr["alice/app"] = errors.New("no such repository")
	src.coverage["alice/lib"] = 90
	src.coverage["acme/tool"] = 90

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	var failed, fine []string

	for _, ev := range rec.of(event.RepoDone) {
		if ev.Err != "" {
			failed = append(failed, ev.Repo)

			if !strings.Contains(ev.Err, "no such repository") {
				t.Errorf("the failure does not say why: %q", ev.Err)
			}
		} else {
			fine = append(fine, ev.Repo)
		}
	}

	if !slices.Equal(failed, []string{"alice/app"}) {
		t.Errorf("failed = %v, want alice/app", failed)
	}

	slices.Sort(fine)

	if !slices.Equal(fine, []string{"acme/tool", "alice/lib"}) {
		t.Errorf("maintained = %v, want the other two", fine)
	}

	state, err := config.LoadState(svc.cfg.StatePath)
	if err != nil || state == nil || !state.HadError {
		t.Errorf("state = %+v, %v; want the failure recorded", state, err)
	}
}

func TestRunStopsWhenAnOwnerCannotBeListed(t *testing.T) {
	svc, src, _ := portfolio(t, portfolioJSON)
	src.prefetchErr["acme"] = errors.New("rate limited")

	err := svc.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err = %v, want the listing failure", err)
	}
}

// The list is pulled before it is read: what was merged where it lives is what
// the run maintains, and the coverage commit lands on top of it.
func TestRunPullsItsList(t *testing.T) {
	svc, src, rec := portfolio(t, portfolioJSON)

	src.add("alice", "app", map[string]string{"go.mod": goMod("github.com/alice/app")})
	src.add("alice", "lib", map[string]string{"go.mod": goMod("github.com/alice/lib")})

	listDir := filepath.Dir(svc.cfg.Path)
	src.onPull["alice/portfolio"] = func() {
		writeFile(t, svc.cfg.Path, `{"alice": {"repositories": {"app": {}, "lib": {}}}}`)
		gitRun(t, listDir, "commit", "-qam", "stop maintaining tool")
	}

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if starts := rec.of(event.RunStart); len(starts) != 1 ||
		!slices.Equal(starts[0].Repos, []string{"alice/app", "alice/lib"}) {
		t.Errorf("run_start = %+v, want the pulled list", starts)
	}

	if got := gitRun(t, listDir, "log", "-2", "--format=%s"); got != "update config\nstop maintaining tool" {
		t.Errorf("the list's history reads %q, want the coverage on top of what was pulled", got)
	}
}

func TestRunNeedsItsList(t *testing.T) {
	svc, _, _ := portfolio(t, portfolioJSON)
	svc.cfg.Path = filepath.Join(t.TempDir(), "missing.json")

	if err := svc.Run(t.Context()); err == nil {
		t.Fatal("a missing list was accepted")
	}

	if err := svc.RunOne(t.Context(), "alice", "app"); err == nil {
		t.Fatal("a missing list was accepted by RunOne")
	}
}

func TestRunRejectsACycle(t *testing.T) {
	svc, src, _ := portfolio(t, `{"alice": {"repositories": {"a": {}, "b": {}}}}`)

	src.add("alice", "a", map[string]string{"go.mod": goMod("github.com/alice/a", "github.com/alice/b")})
	src.add("alice", "b", map[string]string{"go.mod": goMod("github.com/alice/b", "github.com/alice/a")})

	err := svc.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "ordering repositories") {
		t.Fatalf("err = %v, want the cycle refused", err)
	}
}

// A clean run after a failing one says so; the state file is what remembers.
func TestRunSaysWhenItRecovered(t *testing.T) {
	svc, src, _ := portfolio(t, `{"alice": {"repositories": {"lib": {}}}}`)
	src.add("alice", "lib", map[string]string{"go.mod": goMod("github.com/alice/lib")})
	src.coverage["alice/lib"] = 90

	writeFile(t, svc.cfg.StatePath, `{"hadError": true}`)

	logs := captureLogs(t)

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(logs.String(), "recovered") {
		t.Errorf("the recovery was not reported:\n%s", logs)
	}

	if !strings.Contains(logs.String(), "nothing to report") {
		t.Errorf("an uneventful run did not say so:\n%s", logs)
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	prev := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	return &buf
}

// What the repository's definition says about itself is pushed to GitHub, and
// a public repository below the floor fails with its figure still recorded.
func TestRunHoldsAPublicRepositoryToTheFloor(t *testing.T) {
	svc, src, _ := portfolio(t, `{"alice": {"repositories": {"lib": {"description": "from the list"}}}}`)

	src.add("alice", "lib", map[string]string{
		"go.mod":       goMod("github.com/alice/lib"),
		"devtool.json": `{"description": "from the repository"}`,
	})
	src.coverage["alice/lib"] = 60
	src.public["alice/lib"] = true

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := src.repo("alice/lib").description; got != "from the repository" {
		t.Errorf("description = %q, want the definition's", got)
	}

	cfg, err := config.Load(svc.cfg.Path)
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg["alice"].V.Repositories["lib"].V.Coverage; got != 60 {
		t.Errorf("coverage = %v, want the failing 60 recorded", got)
	}

	state, err := config.LoadState(svc.cfg.StatePath)
	if err != nil || state == nil || !state.HadError {
		t.Errorf("state = %+v, %v; want the failure recorded", state, err)
	}
}

// The engine discards whatever an interrupted run left behind before it starts.
func TestRunDiscardsWhatAnEarlierRunLeft(t *testing.T) {
	svc, src, _ := portfolio(t, `{"alice": {"repositories": {"lib": {}}}}`)
	dir := src.add("alice", "lib", map[string]string{"go.mod": goMod("github.com/alice/lib")})
	src.coverage["alice/lib"] = 90

	writeFile(t, filepath.Join(dir, "go.mod"), "module broken\n")
	writeFile(t, filepath.Join(dir, "leftover.txt"), "half-written\n")

	if err := svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "leftover.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the untracked leftover is still there: %v", err)
	}

	if got, _ := os.ReadFile(filepath.Join(dir, "go.mod")); !strings.Contains(string(got), "alice/lib") {
		t.Errorf("the modified go.mod was not reset:\n%s", got)
	}
}

func TestRunFailsWhenTheListCannotBeCommitted(t *testing.T) {
	svc, src, _ := portfolio(t, `{"alice": {"repositories": {}}}`)
	src.openErr["alice/portfolio"] = errors.New("gone")

	err := svc.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "holding the config") {
		t.Fatalf("err = %v, want the list's repository named", err)
	}
}

func TestRunOne(t *testing.T) {
	for _, tc := range []struct {
		name        string
		owner, repo string
		fail        bool
		wantOrg     bool
		wantJoined  bool
		wantFailure bool
	}{
		{name: "a listed repository", owner: "alice", repo: "lib", wantJoined: true},
		{name: "a new repository joins the list", owner: "acme", repo: "new", wantOrg: true, wantJoined: true},
		{name: "a new owner joins as a user", owner: "bob", repo: "first", wantJoined: true},
		{name: "a failed bootstrap leaves no trace", owner: "bob", repo: "first", fail: true, wantFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, src, rec := portfolio(t, portfolioJSON)
			key := event.Key(tc.owner, tc.repo)

			src.add(tc.owner, tc.repo, map[string]string{"go.mod": goMod("github.com/" + key)})
			src.coverage[key] = 95

			if tc.fail {
				src.tasks[key] = func(*fakeRepo) []engine.Task {
					return []engine.Task{{Name: "break", Run: func(context.Context) error {
						return errors.New("broken")
					}}}
				}
			}

			err := svc.RunOne(t.Context(), tc.owner, tc.repo)
			if gotFailure := err != nil; gotFailure != tc.wantFailure {
				t.Fatalf("err = %v, want failure %v", err, tc.wantFailure)
			}

			if want := []string{map[bool]string{true: "org:", false: "user:"}[tc.wantOrg] + tc.owner}; !slices.Equal(src.prefetched, want) {
				t.Errorf("prefetched %v, want %v", src.prefetched, want)
			}

			if starts := rec.of(event.RunStart); len(starts) != 1 || !slices.Equal(starts[0].Repos, []string{key}) {
				t.Errorf("run_start = %+v, want only %s", starts, key)
			}

			cfg, err := config.Load(svc.cfg.Path)
			if err != nil {
				t.Fatal(err)
			}

			repo, org, known := lookupRepo(cfg, tc.owner, tc.repo)
			if known != tc.wantJoined {
				t.Fatalf("in the list: %v, want %v", known, tc.wantJoined)
			}

			if known && (repo.Coverage != 95 || org != tc.wantOrg) {
				t.Errorf("recorded %+v (organisation %v)", repo, org)
			}
		})
	}
}

func TestNewNeedsAToken(t *testing.T) {
	t.Setenv(TokenEnv, "")

	if _, err := New(t.Context(), ConfigFile{}, &recorder{}, false, "v1"); err == nil ||
		!strings.Contains(err.Error(), TokenEnv) {
		t.Fatalf("err = %v, want the missing token named", err)
	}

	t.Setenv(TokenEnv, "token")
	t.Setenv("HOME", t.TempDir())

	svc, err := New(t.Context(), ConfigFile{Owner: "alice"}, &recorder{}, true, "v1")
	if err != nil {
		t.Fatal(err)
	}

	if !svc.verbose || svc.version != "v1" || svc.cfg.Owner != "alice" || svc.repos == nil {
		t.Errorf("service = %+v", svc)
	}
}

func TestOpenConfigFile(t *testing.T) {
	isolateGit(t)

	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "remote", "add", "origin", "git@github.com:alice/portfolio.git")

	cfg, err := OpenConfigFile(t.Context(), filepath.Join(dir, config.DefaultName), config.StateName)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Owner != "alice" || cfg.Name != "portfolio" {
		t.Errorf("held by %s/%s, want alice/portfolio", cfg.Owner, cfg.Name)
	}

	if cfg.StatePath != filepath.Join(cfg.Dir(), config.StateName) {
		t.Errorf("state at %s, want beside the list", cfg.StatePath)
	}

	if _, err := OpenConfigFile(t.Context(), filepath.Join(t.TempDir(), "x.json"), config.StateName); err == nil {
		t.Error("a list outside any repository was accepted")
	}
}
