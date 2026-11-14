package run

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/MarkRosemaker/ghrepo"
	"github.com/MarkRosemaker/gorepo"
	"github.com/google/go-github/v80/github"
)

// isolateGo keeps the go command off the network and out of the real HOME,
// while keeping the build and module caches.
func isolateGo(t *testing.T) {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "go", "env", "-json", "GOCACHE", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}

	env := map[string]string{}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatal(err)
	}

	isolateGit(t)

	for k, v := range env {
		t.Setenv(k, v)
	}

	// go would otherwise write telemetry counters into HOME after the test.
	writeFile(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "go", "telemetry", "mode"), "off")

	t.Setenv("GOPROXY", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
}

// vendoring is a module that vendors a local one, example.com/dep, of whose
// packages it uses only top.
var vendoring = map[string]string{
	"go.mod": "module example.com/test\n\ngo 1.26\n\n" +
		"require example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n",
	"a.go": "package a\n\nimport \"example.com/dep/top\"\n\nconst Z = top.X\n",

	"dep/go.mod":     "module example.com/dep\n\ngo 1.26\n",
	"dep/top/top.go": "package top\n\nconst X = 1\n",
	"dep/sub/sub.go": "package sub\n\nconst Y = 2\n",
}

func newGoRepository(t *testing.T, files map[string]string) (*repository, string) {
	t.Helper()

	isolateGo(t)

	base := t.TempDir()
	dir := filepath.Join(base, "user", "test")

	for path, content := range files {
		writeFile(t, filepath.Join(dir, path), content)
	}

	gitRun(t, dir, "init", "-q", "-b", "main")
	// Never contacted.
	gitRun(t, dir, "remote", "add", "origin", "https://github.com/user/test.git")

	repo, err := gorepo.NewService(t.Context(), "", ghrepo.WithBaseDir(base)).
		NewRepository(t.Context(), "user", "test", ghrepo.WithGithubRepo(&github.Repository{}))
	if err != nil {
		t.Fatal(err)
	}

	return &repository{Repository: repo}, dir
}

// subTest is a test importing a package of example.com/dep that vendor/ does
// not hold.
const subTest = "package a\n\nimport (\n\t\"testing\"\n\n" +
	"\t\"example.com/dep/sub\"\n)\n\nfunc TestA(t *testing.T) { _ = sub.Y }\n"

// A generator writing a test that imports a package vendor/ lacks leaves the
// tests passing, since the task settles the module files after it.
func TestGenerateSettlesTheModules(t *testing.T) {
	files := map[string]string{"gen.go": "package a\n\n//go:generate cp gen.txt a_test.go\n", "gen.txt": subTest}
	maps.Copy(files, vendoring)

	repo, dir := newGoRepository(t, files)

	if err := repo.GoModVendor(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := repo.generate(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.GoTestCover(t.Context()); err != nil {
		t.Fatalf("the tests fail after generating: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "vendor", "example.com", "dep", "sub", "sub.go")); err != nil {
		t.Errorf("sub not vendored: %v", err)
	}
}

// A module that does not vendor is tidied after generating and left without
// a vendor/.
func TestGenerateDoesNotStartVendoring(t *testing.T) {
	repo, dir := newGoRepository(t, map[string]string{
		"go.mod":  "module example.com/test\n\ngo 1.26\n",
		"gen.go":  "package a\n\n//go:generate cp gen.txt b.go\n",
		"gen.txt": "package a\n\nconst B = 1\n",
	})

	if err := repo.generate(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "b.go")); err != nil {
		t.Errorf("the generator did not run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "vendor")); !os.IsNotExist(err) {
		t.Errorf("vendor/ made: %v", err)
	}
}

// A generator that fails stops the task before anything is settled.
func TestGenerateFails(t *testing.T) {
	repo, _ := newGoRepository(t, map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.26\n",
		"gen.go": "package a\n\n//go:generate false\n",
	})

	if err := repo.generate(t.Context()); err == nil {
		t.Error("a failing generator passed")
	}
}
