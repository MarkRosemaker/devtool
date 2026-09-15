package maintain

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestGenerateGitignore(t *testing.T) {
	t.Run("a repository with no .gitignore gets the house rules", func(t *testing.T) {
		got := generateGitignoreAndRead(t, afero.NewMemMapFs())

		if want := gitignoreOpen + "\n" + gitignoreRules + gitignoreClose + "\n"; got != want {
			t.Errorf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	// The rules are the point, so losing one quietly is the failure worth a
	// test of its own.
	t.Run("the house rules are all there", func(t *testing.T) {
		got := generateGitignoreAndRead(t, afero.NewMemMapFs())

		for _, want := range []string{
			"*.exe", "*.exe~", "*.dll", "*.so", "*.dylib",
			"*.out",
			".idea", ".vscode", ".schemas", ".history",
			"bin/",
			"*.log",
			"*.DS_Store",
		} {
			if !strings.Contains(got, "\n"+want+"\n") {
				t.Errorf("the house rules no longer state %q:\n%s", want, got)
			}
		}
	})

	// Below, so that they can still override: in a .gitignore the last
	// matching pattern is the one that decides.
	t.Run("rules the repository wrote are kept, below the block", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath, "# Runtime state\nlast_run.json\n")

		checkOrder(t, generateGitignoreAndRead(t, fs), []string{
			gitignoreOpen,
			gitignoreClose,
			"# Runtime state",
			"last_run.json",
		})
	})

	// The case every repository here is in: a .gitignore that is the Go
	// boilerplate the house rules were drawn from, plus a few of its own.
	t.Run("taking the file over does not leave the boilerplate twice", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath, "# IDE directories\n.idea\n.vscode\n\n"+
			"# Build and Environment\ndist/\n/portfolio\n\n"+
			"# macOS files\n*.DS_Store\n")

		got := generateGitignoreAndRead(t, fs)

		// The group that was wholly house rules goes, heading and all —
		// leaving the one occurrence the block itself states.
		if n := strings.Count(got, "# IDE directories"); n != 1 {
			t.Errorf("a heading left over nothing should have gone:\n%s", got)
		}

		if n := strings.Count(got, ".idea"); n != 1 {
			t.Errorf(".idea appears %d times, want 1:\n%s", n, got)
		}

		if n := strings.Count(got, "*.DS_Store"); n != 1 {
			t.Errorf("*.DS_Store appears %d times, want 1:\n%s", n, got)
		}

		// The group that had rules of its own keeps them, and its heading.
		checkOrder(t, got, []string{
			gitignoreClose,
			"# Build and Environment",
			"dist/",
			"/portfolio",
		})
	})

	// Only verbatim rules are dropped, so nothing the repository wrote that
	// cannot be pointed at in the block goes.
	t.Run("a rule that merely overlaps a house rule is kept", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath, "cover.out\n*.test\n!bin/keep-this\n")

		got := generateGitignoreAndRead(t, fs)

		// cover.out overlaps "*.out" and *.test overlaps nothing; both are
		// the repository's, and a negation especially so.
		for _, want := range []string{"cover.out", "*.test", "!bin/keep-this"} {
			if !strings.Contains(got, want) {
				t.Errorf("lost %q:\n%s", want, got)
			}
		}
	})

	t.Run("running twice changes nothing the second time", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath, "# mine\n.idea\nlast_run.json\n")

		first := generateGitignoreAndRead(t, fs)
		second := generateGitignoreAndRead(t, fs)

		if first != second {
			t.Errorf("output changed between runs:\nfirst:\n%s\nsecond:\n%s", first, second)
		}
	})

	// The tidying happens once, when the file is taken over. A rule written
	// below the block afterwards is the repository's, whatever it says.
	t.Run("a house rule added below the block later is left there", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath,
			gitignoreOpen+"\n"+gitignoreRules+gitignoreClose+"\n\n.idea\n")

		if got := generateGitignoreAndRead(t, fs); !strings.HasSuffix(got, "\n.idea\n") {
			t.Errorf("the tail is the repository's after adoption:\n%s", got)
		}
	})

	// A repository that moved the block meant to, and a run that dragged it
	// back would be undoing somebody's arrangement every time.
	t.Run("a block the repository moved stays where it was put", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath,
			"# mine first\nlast_run.json\n\n"+gitignoreOpen+"\n*.log\n"+gitignoreClose+"\n")

		checkOrder(t, generateGitignoreAndRead(t, fs), []string{
			"# mine first",
			"last_run.json",
			gitignoreOpen,
			"*.exe",
			gitignoreClose,
		})
	})

	// Stale contents inside the block are this task's to correct — that is
	// the whole reason the block is marked.
	t.Run("the block is brought up to date, not appended to", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath,
			gitignoreOpen+"\nsomething-stale\n"+gitignoreClose+"\nmine\n")

		got := generateGitignoreAndRead(t, fs)

		if strings.Contains(got, "something-stale") {
			t.Errorf("a line inside the block is not the repository's to keep:\n%s", got)
		}

		if !strings.Contains(got, "*.exe") {
			t.Errorf("the house rules should have replaced it:\n%s", got)
		}

		if !strings.Contains(got, "mine") {
			t.Errorf("the line below the block is the repository's:\n%s", got)
		}
	})

	t.Run("half a block is an error rather than a guess", func(t *testing.T) {
		for _, tc := range []struct{ name, content string }{
			{"open with no close", gitignoreOpen + "\n*.log\nmine\n"},
			{"close with no open", "mine\n" + gitignoreClose + "\n"},
			{"the wrong way round", gitignoreClose + "\n*.log\n" + gitignoreOpen + "\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fs := afero.NewMemMapFs()
				writeFile(t, fs, gitignorePath, tc.content)

				if err := generateGitignore(fs); err == nil {
					t.Fatal("expected an error rather than a reading of half a block")
				}

				if got := readFile(t, fs, gitignorePath); got != tc.content {
					t.Errorf("the file should be untouched, got:\n%s", got)
				}
			})
		}
	})

	t.Run("non-UTF-8 content is rejected", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		if err := afero.WriteFile(fs, gitignorePath, []byte{0xff, 0xfe, 0x00}, 0o644); err != nil {
			t.Fatal(err)
		}

		if err := generateGitignore(fs); err == nil {
			t.Fatal("expected an error for non-UTF-8 content, got nil")
		}
	})

	t.Run("a file with no trailing newline gains one", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, gitignorePath, "last_run.json")

		if got := generateGitignoreAndRead(t, fs); !strings.HasSuffix(got, "\n") {
			t.Errorf("want a final newline, got %q", got)
		}
	})
}

// TestGenerateGitignoreAgainstGit checks the result against the program that
// has to read it. Everything else here asserts about text; this asserts that
// git agrees — that what the generated Makefile builds really does stay out
// of "git add -A", and that a rule the repository put below the block really
// does win over the house rule above it.
func TestGenerateGitignoreAgainstGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	fs := afero.NewBasePathFs(afero.NewOsFs(), dir)

	git := func(t *testing.T, args ...string) string {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}

		return string(out)
	}

	git(t, "init", "-q", ".")

	// On a real filesystem, unlike the in-memory one, a parent directory has
	// to be there first.
	for _, d := range []string{commandDir + "/app", binDir} {
		if err := fs.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// A repository as it would be found after a "make build" and a "make
	// cover", with one house rule deliberately overridden.
	writeFile(t, fs, commandDir+"/app/main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, fs, binDir+"/app", "not really a binary\n")
	writeFile(t, fs, coverProfile, "mode: set\n")
	writeFile(t, fs, "important.log", "wanted in git\n")
	writeFile(t, fs, binDir+"/keep-this", "wanted, but cannot be had this way\n")
	writeFile(t, fs, gitignorePath, "!important.log\n!"+binDir+"/keep-this\n")

	if err := generateGitignore(fs); err != nil {
		t.Fatal(err)
	}

	git(t, "add", "-A")
	staged := git(t, "status", "--short")

	t.Run("git ignores what the Makefile writes", func(t *testing.T) {
		for _, unwanted := range []string{binDir + "/app", coverProfile} {
			if strings.Contains(staged, unwanted) {
				t.Errorf("%s was staged by git add -A:\n%s", unwanted, staged)
			}
		}

		// And the source still is, so this has not ignored everything.
		if !strings.Contains(staged, commandDir+"/app/main.go") {
			t.Errorf("the source should be staged:\n%s", staged)
		}
	})

	// Why the block is at the head: were the house rules last, "*.log" would
	// be the final word and this file could not be tracked.
	t.Run("a rule below the block overrides the house rule above it", func(t *testing.T) {
		if !strings.Contains(staged, "important.log") {
			t.Errorf("the repository's negation should have won:\n%s", staged)
		}
	})

	// The exception the doc comment names, pinned here because it is git's
	// behaviour this task tells people to work around: "bin/" excludes the
	// directory, git does not descend into it, and so no negation below can
	// reach inside. "bin/*" instead of the house rule is the way to do it.
	t.Run("a negation cannot reach inside an excluded directory", func(t *testing.T) {
		if strings.Contains(staged, binDir+"/keep-this") {
			t.Errorf("git is documented not to descend into bin/:\n%s", staged)
		}
	})
}

// generateGitignoreAndRead runs the task against fs and returns what it
// wrote.
func generateGitignoreAndRead(t *testing.T, fs afero.Fs) string {
	t.Helper()

	if err := generateGitignore(fs); err != nil {
		t.Fatal(err)
	}

	return readFile(t, fs, gitignorePath)
}
