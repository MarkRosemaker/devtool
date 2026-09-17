package maintain

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"unicode/utf8"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const (
	makefilePath = "Makefile"

	// makefileDir is where a repository's own make fragments live.
	//
	// It is not named after the file it feeds, the way README/ is, because
	// it cannot be: a file and a directory of the same name cannot coexist,
	// and the file has to be called Makefile for make to find it.
	makefileDir = "mk"

	// makefileAdoptedName is where a Makefile written before this task
	// existed ends up: a fragment like any other, included along with the
	// rest, so every target it defined goes on working.
	//
	// The name says whose it is. Only this task ever creates it, which is
	// what makes it safe to take the default goal from it — a fragment a
	// human wrote should not change what plain "make" runs just by being
	// there. It is README/legacy.md's opposite number, though a gentler
	// one: that file stops the generator until somebody deals with it,
	// while this one simply goes on working.
	makefileAdoptedName = "legacy.mk"

	// makefileExt is the extension the include below matches.
	makefileExt = ".mk"

	// makefileFlagsName is the fragment included ahead of everything else,
	// for the settings that shape what follows: an export, a compiler flag,
	// a path. Read at the foot of the file with the rest, it would come too
	// late for the variables and recipes that use it.
	makefileFlagsName = "flags.mk"

	// commandDir is where a Go repository keeps the packages it builds into
	// binaries, and binDir is where the built binary goes.
	commandDir = "cmd"
	binDir     = "bin"

	// coverProfile is where the cover target writes what it measured.
	coverProfile = "cover.out"
)

// makefileStamp opens every generated Makefile, so a human or an agent
// reading it finds out, before editing, that the edit will be lost.
const makefileStamp = "# " + generatedBy + " — edit files in mk/ instead."

//go:embed Makefile.tmpl
var makefileTmplText string

var makefileTmpl = template.Must(template.New(makefilePath).Parse(makefileTmplText))

// makefileData is what [makefileTmpl] is executed against.
type makefileData struct {
	// DefaultGoal is what plain "make" runs.
	DefaultGoal string

	// Command is the name of the one package under cmd/ that this
	// repository builds, or empty where there is not exactly one. It names
	// both the binary and the package to build, which is what lets the
	// generated file define BINARY and PKG and a build target that uses
	// them.
	Command string

	// Targets are the ones the generated Makefile defines itself.
	Targets []makeTarget

	// CleanFiles is what clean removes, space separated. A fragment adds to
	// it rather than redefining clean.
	CleanFiles string

	// Tools is true where the generated file installs tools, and so needs
	// the variable that says which Go to build them with.
	Tools bool

	// Flags is mk/flags.mk's path where the repository has one, and empty
	// where it has not, so the generated file carries no include for a file
	// that is not there.
	Flags string

	// Includes are the repository's other fragments, in the order make
	// reads them, named rather than matched: the generator knows what is in
	// mk/, so the generated file says so.
	Includes []string
}

// makeTarget is one target, its prerequisites and its recipe.
type makeTarget struct {
	Name    string
	Prereqs []string
	Recipe  []string

	// Comment goes above the target: one line, and only where the recipe
	// alone would leave a reader guessing.
	Comment string

	// Needs names house targets this one's recipe depends on the behaviour
	// of, rather than merely on the name resolving. Where the repository
	// defines one of them itself, this target is left out too: run drives
	// the binary that this file's own build produces, and a repository that
	// builds its own is the only thing that knows where that lands.
	Needs []string
}

// repoShape is what the repository itself settles about the house targets:
// what there is to build, and whether there is anything to benchmark.
type repoShape struct {
	// Command is the name of the one package under cmd/ this repository
	// builds, empty where there is not exactly one.
	Command string

	// Benchmarks is true where some test file defines one, so a bench
	// target is worth having.
	Benchmarks bool

	// Private is the repository's own visibility, which the generate target
	// has to pass back to devtool: without it a private repository's README
	// is rebuilt as a public one's, and "make ci" then fails on the drift it
	// just created.
	Private bool
}

// houseTargets are the targets every repository gets, so anything — a
// person, or an agent told to run "make lint" — can count on them.
//
// A repository defining one of these names in a fragment keeps its own and
// this one is left out, since make warns about an overridden recipe on
// every invocation.
func houseTargets(shape repoShape) []makeTarget {
	// Two bundles, one a superset of the other: ci is everything that runs
	// wherever it is asked to, and all adds what needs the network. Both
	// run the race detector; plain "make test" stays quick.
	//
	// fix rather than lint, since it lints as it goes, and before verify so
	// that whatever it changed shows up as drift.
	ci := makeTarget{
		Name:    "ci",
		Comment: "Before every commit. Needs no network beyond the module cache.",
		Prereqs: []string{"fix", "verify", "vet", "test-race"},
	}
	all := makeTarget{
		Name:    "all",
		Comment: "ci, plus the checks that need the network.",
		Prereqs: []string{"ci", "vuln"},
	}

	var build []makeTarget

	if shape.Command != "" {
		ci.Prereqs = append(ci.Prereqs, "build")

		// Through BINARY and PKG, so a fragment can repoint either.
		build = []makeTarget{{
			Name: "build",
			Recipe: []string{
				"mkdir -p " + binDir,
				"go build -o $(BINARY) $(PKG)",
			},
		}, {
			Name:    "run",
			Comment: "ARGS passes flags through, e.g. `make run ARGS=-debug`.",
			Prereqs: []string{"build"},
			Needs:   []string{"build"},
			Recipe:  []string{"$(BINARY) $(ARGS)"},
		}}
	}

	var bench []makeTarget

	if shape.Benchmarks {
		// The dollar is doubled to reach the shell as "^$", which matches
		// no test, so only benchmarks run.
		bench = []makeTarget{{
			Name:   "bench",
			Recipe: []string{"go test -run=^$$ -bench=. -benchmem ./..."},
		}}
	}

	return slices.Concat([]makeTarget{all, ci}, build, []makeTarget{
		{Name: "lint", Recipe: []string{"golangci-lint run"}},
		{Name: "vet", Recipe: []string{"go vet ./..."}},
		{
			Name:    "vuln",
			Comment: "Reads the vulnerability database, so it needs the network.",
			Recipe:  []string{"govulncheck ./..."},
		},
		{Name: "test", Recipe: []string{"go test ./..."}},
		{Name: "test-race", Recipe: []string{"go test -race ./..."}},
	}, bench, []makeTarget{
		{
			Name: "cover",
			Recipe: []string{
				"go test -coverprofile=" + coverProfile + " ./...",
				"go tool cover -func=" + coverProfile,
			},
		},
		// golangci-lint fmt, not gofumpt directly: .golangci.yaml is what
		// says which formatters run and how.
		{Name: "format", Recipe: []string{"golangci-lint fmt"}},
		{
			Name: "fix",
			Recipe: []string{
				"go fix ./...",
				// In golangci-lint v2, run --fix already runs format, so no separate fmt needed.
				"golangci-lint run --fix",
			},
		},
		{Name: "tidy", Recipe: []string{"go mod tidy", "go mod vendor"}},
		{
			Name:   "deps",
			Recipe: []string{"go get -u ./...", "$(MAKE) tidy"},
		},
		{
			Name: "generate",
			Comment: "Everything a tool writes: the go:generate directives, " +
				"then the files devtool owns.",
			Recipe: []string{"go generate ./...", updateCommand(shape.Private)},
		},
		{
			Name:    "verify",
			Comment: "Run on a commit: it reports through git, so your own edits look like drift.",
			Prereqs: []string{"generate"},
			Recipe:  []string{"git diff --exit-code"},
		},
		{
			Name:    "doc",
			Comment: "Serves this module's documentation on :8080, as pkg.go.dev will show it.",
			Recipe:  []string{"pkgsite ."},
		},
		{
			Name:    "tools",
			Comment: "Installs what the targets above shell out to, into $(go env GOPATH)/bin.",
			Recipe:  toolInstalls(),
		},
		{
			Name:    "clean",
			Comment: "A fragment adds its own with CLEAN_FILES += dist.",
			Recipe:  []string{"rm -rf $(CLEAN_FILES)"},
		},
	})
}

// houseTools are installed globally, latest of each, rather than pinned as
// module dependencies: pinning one means vendoring it into every
// repository, and the version worth running is the current one — a new
// finding is worth hearing about the day it lands.
var houseTools = []string{
	"github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
	"golang.org/x/vuln/cmd/govulncheck",
	"golang.org/x/pkgsite/cmd/pkgsite",
	// The generator itself, installed like any other tool rather than
	// vendored into every repository it writes.
	"github.com/MarkRosemaker/devtool",
}

// toolInstalls is the recipe that installs [houseTools].
//
// Through GOTOOLCHAIN, because a tool that analyses this module has to be
// built with a Go at least as new as the one the module targets, and
// "go install" otherwise honours the tool's own pinned toolchain, which can
// be older: golangci-lint and govulncheck both refuse to run at all when it
// is.
//
// Where they land is then put on the Makefile's own PATH, since installing a
// current tool is no use while an older copy earlier on somebody's PATH is
// the one that runs — which is the same failure, arriving by a different
// route. GOBIN is where "go install" writes when it is set, so the PATH line
// reads that first and falls back to GOPATH/bin rather than assuming.
func toolInstalls() []string {
	installs := make([]string, 0, len(houseTools))
	for _, tool := range houseTools {
		installs = append(installs, "GOTOOLCHAIN=$(TOOL_GO) go install "+tool+"@latest")
	}

	return installs
}

// defaultGoal is what plain "make" runs where nothing says otherwise. It is
// always defined, since houseTargets defines it.
const defaultGoal = "all"

// benchmarkFunc matches a benchmark declaration at the start of a line, as
// a top-level function is.
var benchmarkFunc = regexp.MustCompile(`(?m)^func Benchmark[A-Z_]`)

// makeTargetLine matches a line defining a target: a name, then a colon
// that is not an assignment. A recipe line is tab-indented and so cannot
// match, a "." name is one of make's own, and the character after the
// colons may be neither "=" nor another colon — allowing a colon there
// would let "::=" match by taking only the first.
var makeTargetLine = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_./%+-]*)\s*:+([^=:]|$)`)

// MakefileTask returns a task that writes the repository's Makefile: the
// targets every repository gets, and an include of each fragment in mk/.
//
// The generated file includes the fragments rather than containing them,
// since make's own include merges them into one database — "make foo"
// reaches any target any fragment defines, and a path inside one resolves
// from the repository root where make was run. They are named rather than
// matched, so a fragment added by hand is not read until the next run
// regenerates the file.
//
// A repository that already had a hand-written Makefile keeps everything in
// it: the file moves verbatim to mk/legacy.mk and is included from there,
// with .DEFAULT_GOAL set to its own first target so plain "make" goes on
// meaning what it meant. A Makefile this task wrote carries the stamp and
// is simply rewritten.
//
// .DEFAULT_GOAL is set explicitly and before the includes: the first target
// make sees would otherwise become the default, which a fragment could take
// just by being included.
func MakefileTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "generate Makefile",
		Short: "makefile",
		Run: func(context.Context) error {
			return generateMakefile(repo.Fs(), repo.Private())
		},
	}
}

// updateCommand is how the generate target invokes devtool over this
// repository, which has to be how a person would invoke it by hand.
func updateCommand(private bool) string {
	if private {
		return "devtool update -private"
	}

	return "devtool update"
}

// generateMakefile is [MakefileTask]'s work, factored out so it can run
// against an in-memory filesystem in tests without a real repository.
func generateMakefile(fs afero.Fs, private bool) error {
	if err := adoptExistingMakefile(fs); err != nil {
		return err
	}

	data, err := collectMakefileData(fs, private)
	if err != nil {
		return err
	}

	b, err := renderMakefile(data)
	if err != nil {
		return err
	}

	return afero.WriteFile(fs, makefilePath, b, 0o644)
}

// adoptExistingMakefile moves a Makefile written before this task existed
// into mk/, where it goes on being included like any other fragment.
//
// A Makefile this task wrote carries the stamp and is left for the caller to
// overwrite. One that does not is somebody's work, and is never overwritten
// — if mk/legacy.mk is somehow taken already, that is for a human to sort
// out rather than for this to guess at.
func adoptExistingMakefile(fs afero.Fs) error {
	existing, err := afero.ReadFile(fs, makefilePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("reading existing %s: %w", makefilePath, err)
	}

	// By the marker rather than by this run's own stamp: a Makefile an older
	// generator wrote is still this task's, and matching the exact stamp
	// would move it to mk/ the first time the generator is renamed.
	if generatedMarker.Match(firstLine(existing)) {
		return nil
	}

	if !utf8.Valid(existing) {
		return fmt.Errorf("%s: not valid UTF-8", makefilePath)
	}

	adopted := path.Join(makefileDir, makefileAdoptedName)

	if taken, err := afero.Exists(fs, adopted); err != nil {
		return fmt.Errorf("checking for %s: %w", adopted, err)
	} else if taken {
		return fmt.Errorf(
			"%s was not written by this task and %s already exists: move one of them aside",
			makefilePath, adopted,
		)
	}

	if err := fs.MkdirAll(makefileDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", makefileDir, err)
	}

	return afero.WriteFile(fs, adopted, existing, 0o644)
}

// collectMakefileData reads mk/ and works out what the generated Makefile
// should say: which of the house targets the repository has not already
// defined for itself, and what plain "make" should run.
func collectMakefileData(fs afero.Fs, private bool) (makefileData, error) {
	fragments, err := readMakeFragments(fs)
	if err != nil {
		return makefileData{}, err
	}

	shape, err := readRepoShape(fs)
	if err != nil {
		return makefileData{}, err
	}

	shape.Private = private

	cleanFiles := []string{coverProfile}
	if shape.Command != "" {
		cleanFiles = append([]string{binDir}, cleanFiles...)
	}

	data := makefileData{
		DefaultGoal: defaultGoal,
		Command:     shape.Command,
		CleanFiles:  strings.Join(cleanFiles, " "),
		Flags:       fragments.Flags,
		Includes:    fragments.Paths,
	}

	// An adopted Makefile's own first target stays the default, so plain
	// "make" in a repository that had one goes on meaning what it meant.
	if fragments.AdoptedFirst != "" {
		data.DefaultGoal = fragments.AdoptedFirst
	}

	for _, target := range houseTargets(shape) {
		if fragments.Defined[target.Name] {
			continue
		}

		if target.Name == "tools" {
			data.Tools = true
		}

		// Where what this one relies on is the repository's, so is this.
		if slices.ContainsFunc(target.Needs, func(name string) bool {
			return fragments.Defined[name]
		}) {
			continue
		}

		data.Targets = append(data.Targets, target)
	}

	return data, nil
}

// readRepoShape works out what the repository itself settles about the
// house targets.
func readRepoShape(fs afero.Fs) (repoShape, error) {
	command, err := repoCommand(fs)
	if err != nil {
		return repoShape{}, err
	}

	benchmarks, err := hasBenchmarks(fs)
	if err != nil {
		return repoShape{}, err
	}

	return repoShape{Command: command, Benchmarks: benchmarks}, nil
}

// errBenchmarkFound stops the walk in [hasBenchmarks] at the first
// benchmark, there being nothing more to learn.
//
// A sentinel rather than filepath.SkipAll: [afero.Walk] follows the older
// filepath.Walk, which does not know that one and would hand it back as a
// real error — as it did, on the first repository whose tree happened to
// have a file sorting after the one with the benchmark in it.
var errBenchmarkFound = errors.New("benchmark found")

// hasBenchmarks reports whether any test file defines a benchmark, so that
// a bench target is only written where there is something for it to run.
func hasBenchmarks(fs afero.Fs) (bool, error) {
	err := afero.Walk(fs, ".", func(p string, info os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case info.IsDir():
			// Nothing under vendor/ is this repository's to benchmark, and
			// a dotted directory is not source.
			if base := filepath.Base(p); base == "vendor" ||
				(strings.HasPrefix(base, ".") && base != ".") {
				return filepath.SkipDir
			}

			return nil
		case !strings.HasSuffix(p, "_test.go"):
			return nil
		}

		b, err := afero.ReadFile(fs, p)
		if err != nil {
			return err
		}

		if benchmarkFunc.Match(b) {
			return errBenchmarkFound
		}

		return nil
	})

	switch {
	case errors.Is(err, errBenchmarkFound):
		return true, nil
	case err != nil:
		return false, fmt.Errorf("looking for benchmarks: %w", err)
	default:
		return false, nil
	}
}

// repoCommand returns the name of the one package under cmd/ that this
// repository builds, or "" where there is not exactly one of them.
//
// Exactly one is the case worth acting on: the binary is the repository,
// and its name is settled. Several means picking one, and picking wrong
// would put a build target in every repository that quietly builds the
// wrong thing, so a repository with several says what it wants in a
// fragment of its own instead.
func repoCommand(fs afero.Fs) (string, error) {
	entries, err := afero.ReadDir(fs, commandDir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("reading %s: %w", commandDir, err)
	}

	found := ""

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if found != "" {
			return "", nil
		}

		found = entry.Name()
	}

	return found, nil
}

// makeFragments is what mk/ holds.
type makeFragments struct {
	// Flags is mk/flags.mk's path, where there is one.
	Flags string

	// Paths are the rest, in the order make will read them, which is the
	// order the directory lists them in.
	Paths []string

	// Defined names every target they define between them, so a house
	// target of the same name can be left to whichever fragment defines it.
	Defined map[string]bool

	// AdoptedFirst is the first target mk/legacy.mk defines — the one an
	// adopted Makefile ran by default.
	AdoptedFirst string
}

// readMakeFragments reads everything in mk/.
//
// An entry that is not a fragment is an error rather than something passed
// over: a file sitting in mk/ looks for all the world like part of the
// build, and nothing would otherwise say that it is not.
func readMakeFragments(fs afero.Fs) (makeFragments, error) {
	entries, err := afero.ReadDir(fs, makefileDir)
	if errors.Is(err, os.ErrNotExist) {
		return makeFragments{}, nil
	} else if err != nil {
		return makeFragments{}, fmt.Errorf("reading %s: %w", makefileDir, err)
	}

	found := makeFragments{Defined: map[string]bool{}}

	for _, entry := range entries {
		// A dotfile is nothing anybody expects make to read.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		p := path.Join(makefileDir, entry.Name())

		if !entry.Mode().IsRegular() {
			return makeFragments{}, fmt.Errorf("%s: not a regular file", p)
		}

		if path.Ext(entry.Name()) != makefileExt {
			return makeFragments{}, fmt.Errorf(
				"%s: not a %s fragment, so it would never be included", p, makefileExt,
			)
		}

		b, err := afero.ReadFile(fs, p)
		if err != nil {
			return makeFragments{}, fmt.Errorf("%s: %w", p, err)
		}

		if !utf8.Valid(b) {
			return makeFragments{}, fmt.Errorf("%s: not valid UTF-8", p)
		}

		targets := makeTargets(b)
		for _, name := range targets {
			found.Defined[name] = true
		}

		switch entry.Name() {
		case makefileFlagsName:
			// Included ahead of everything else, so not again with the rest.
			found.Flags = p
		case makefileAdoptedName:
			found.Paths = append(found.Paths, p)

			if len(targets) > 0 {
				found.AdoptedFirst = targets[0]
			}
		default:
			found.Paths = append(found.Paths, p)
		}
	}

	return found, nil
}

// makeTargets returns the names of the targets a fragment defines, in the
// order it defines them.
func makeTargets(fragment []byte) []string {
	var names []string

	for line := range strings.SplitSeq(string(fragment), "\n") {
		m := makeTargetLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		if !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}

	return names
}

// renderMakefile executes [makefileTmpl] against data, ending the file in
// exactly one newline.
func renderMakefile(data makefileData) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := makefileTmpl.Execute(buf, data); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", makefilePath, err)
	}

	return append(bytes.TrimRight(buf.Bytes(), "\n"), '\n'), nil
}
