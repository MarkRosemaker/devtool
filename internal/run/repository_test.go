package run

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// A test importing a package vendor/ lacks yet passes once the module files
// are settled, which the test step does by itself, once.
func TestGoTestCoverSettlesTheModules(t *testing.T) {
	repo, dir := newGoRepository(t, vendoring)

	if err := repo.GoModVendor(t.Context()); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(dir, "a_test.go"), "package a\n\nimport (\n\t\"testing\"\n\n"+
		"\t\"example.com/dep/sub\"\n)\n\nfunc TestA(t *testing.T) { _ = sub.Y }\n")

	if _, err := repo.GoTestCover(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "vendor", "example.com", "dep", "sub", "sub.go")); err != nil {
		t.Errorf("sub not vendored: %v", err)
	}
}

// A failing test is not the module files' doing, so nothing is settled.
func TestGoTestCoverLeavesAFailingTest(t *testing.T) {
	repo, dir := newGoRepository(t, map[string]string{
		"go.mod":    "module example.com/test\n\ngo 1.26\n",
		"a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) { t.Fatal(\"boom\") }\n",
	})

	_, err := repo.GoTestCover(t.Context())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("GoTestCover() = %v, want the failure", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "vendor")); !os.IsNotExist(err) {
		t.Errorf("vendor/ made: %v", err)
	}
}

// Where settling fails too, both failures are told.
func TestGoTestCoverCannotSettle(t *testing.T) {
	repo, _ := newGoRepository(t, map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.26\n",
		// Nothing provides it, and with GOPROXY=off nothing can.
		"a_test.go": "package a\n\nimport (\n\t\"testing\"\n\n\t\"example.com/nowhere\"\n)\n\n" +
			"func TestA(t *testing.T) { nowhere.X() }\n",
	})

	_, err := repo.GoTestCover(t.Context())
	if err == nil {
		t.Fatal("passed with an import nothing provides")
	}

	for _, want := range []string{"no required module provides package", "settling the module files"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure lacks %q:\n%v", want, err)
		}
	}
}
