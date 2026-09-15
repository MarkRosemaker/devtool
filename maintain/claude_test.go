package maintain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// realFs is a filesystem on disk, which these tests need rather than want:
// symlinking is not on afero.Fs at all, and MemMapFs is one of the
// filesystems that does not offer it.
func realFs(t *testing.T) (afero.Fs, string) {
	t.Helper()

	dir := t.TempDir()

	return afero.NewBasePathFs(afero.NewOsFs(), dir), dir
}

func TestLinkClaude(t *testing.T) {
	t.Run("the link is made, and relative", func(t *testing.T) {
		fs, dir := realFs(t)
		writeFile(t, fs, agentsPath, "# Working here\n")

		if err := linkClaude(fs); err != nil {
			t.Fatal(err)
		}

		// Relative, because an absolute one is committed as this machine's
		// path and broken on every other. afero's own SymlinkIfPossible
		// resolves the target, which is why this task does not use it.
		target, err := os.Readlink(filepath.Join(dir, claudePath))
		if err != nil {
			t.Fatal(err)
		}

		if target != agentsPath {
			t.Errorf("target = %q, want %q", target, agentsPath)
		}

		// And it reads as the file it points at.
		if got := readFile(t, fs, claudePath); got != "# Working here\n" {
			t.Errorf("reading through the link gave %q", got)
		}
	})

	t.Run("running twice leaves the one link", func(t *testing.T) {
		fs, dir := realFs(t)
		writeFile(t, fs, agentsPath, "# Working here\n")

		for range 2 {
			if err := linkClaude(fs); err != nil {
				t.Fatal(err)
			}
		}

		info, err := os.Lstat(filepath.Join(dir, claudePath))
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is not a symlink after a second run: %v", claudePath, info.Mode())
		}
	})

	// Somebody's own CLAUDE.md is not this task's to replace, even though
	// leaving it means the repository has two of these files. Saying so is
	// the roadmap's business, not this task's.
	t.Run("a file somebody wrote is left alone", func(t *testing.T) {
		fs, _ := realFs(t)
		writeFile(t, fs, agentsPath, "# Working here\n")

		const own = "# my own instructions\n"
		writeFile(t, fs, claudePath, own)

		if err := linkClaude(fs); err != nil {
			t.Fatal(err)
		}

		if got := readFile(t, fs, claudePath); got != own {
			t.Errorf("%s = %q, want it untouched", claudePath, got)
		}
	})

	t.Run("a link somewhere else is left alone", func(t *testing.T) {
		fs, dir := realFs(t)
		writeFile(t, fs, agentsPath, "# Working here\n")
		writeFile(t, fs, "OTHER.md", "# elsewhere\n")

		if err := os.Symlink("OTHER.md", filepath.Join(dir, claudePath)); err != nil {
			t.Fatal(err)
		}

		if err := linkClaude(fs); err != nil {
			t.Fatal(err)
		}

		target, err := os.Readlink(filepath.Join(dir, claudePath))
		if err != nil {
			t.Fatal(err)
		}

		if target != "OTHER.md" {
			t.Errorf("target = %q, want the one already there", target)
		}
	})

	// A link read rather than followed, so this counts as taken: otherwise
	// the task would try to make a link where one already is and fail.
	t.Run("a dangling link is left alone", func(t *testing.T) {
		fs, dir := realFs(t)
		writeFile(t, fs, agentsPath, "# Working here\n")

		if err := os.Symlink("GONE.md", filepath.Join(dir, claudePath)); err != nil {
			t.Fatal(err)
		}

		if err := linkClaude(fs); err != nil {
			t.Fatal(err)
		}

		if target, err := os.Readlink(filepath.Join(dir, claudePath)); err != nil {
			t.Fatal(err)
		} else if target != "GONE.md" {
			t.Errorf("target = %q, want the one already there", target)
		}
	})

	// Better no link than one pointing at nothing.
	t.Run("no AGENTS.md, no link", func(t *testing.T) {
		fs, dir := realFs(t)

		if err := linkClaude(fs); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Lstat(filepath.Join(dir, claudePath)); !os.IsNotExist(err) {
			t.Errorf("want no %s at all, got err = %v", claudePath, err)
		}
	})

	t.Run("a filesystem that cannot symlink says so", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, agentsPath, "# Working here\n")

		err := linkClaude(fs)
		if err == nil {
			t.Fatal("expected an error rather than a silently missing link")
		}

		if !strings.Contains(err.Error(), claudePath) {
			t.Errorf("error does not name the file: %v", err)
		}
	})
}

// TestClaudeTaskOnARepo runs the task the way a run does: through
// [ClaudeTask], on a [Repo], rather than handing [linkClaude] a filesystem
// of the test's choosing.
//
// It exists because the two came apart once. Every test above passes a
// *afero.BasePathFs directly and all of them passed while the task failed on
// every repository in the portfolio, since the Repo of the day was an
// afero.Fs by embedding that interface and so had none of the concrete
// filesystem's methods. Repo now asks for the filesystem instead, which
// makes that a compile error rather than a run-time surprise — but the task
// is still worth driving end to end through the constructor a run calls.
func TestClaudeTaskOnARepo(t *testing.T) {
	dir := t.TempDir()
	base := afero.NewBasePathFs(afero.NewOsFs(), dir)

	repo := &fakeRepo{fs: base}

	writeFile(t, repo.Fs(), agentsPath, "# Working here\n")

	if err := ClaudeTask(repo).Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	target, err := os.Readlink(filepath.Join(dir, claudePath))
	if err != nil {
		t.Fatal(err)
	}

	if target != agentsPath {
		t.Errorf("target = %q, want %q", target, agentsPath)
	}
}

// TestClaudeTaskOnARepoLeavesADanglingLink covers the other assertion in
// this file, which failed the same way and more quietly: claudeTaken asks
// for afero.LinkReader, so on the old Repo it fell through to afero.Exists,
// which follows a link and calls a dangling one absent — and the task then
// tried to link over it.
func TestClaudeTaskOnARepoLeavesADanglingLink(t *testing.T) {
	dir := t.TempDir()
	base := afero.NewBasePathFs(afero.NewOsFs(), dir)

	repo := &fakeRepo{fs: base}

	writeFile(t, repo.Fs(), agentsPath, "# Working here\n")

	if err := os.Symlink("GONE.md", filepath.Join(dir, claudePath)); err != nil {
		t.Fatal(err)
	}

	if err := ClaudeTask(repo).Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if target, err := os.Readlink(filepath.Join(dir, claudePath)); err != nil {
		t.Fatal(err)
	} else if target != "GONE.md" {
		t.Errorf("target = %q, want the one already there", target)
	}
}
