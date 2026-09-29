package maintain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// realFs is a filesystem on disk, which these tests need rather than want:
// MemMapFs holds no symlinks at all.
func realFs(t *testing.T) (afero.Fs, string) {
	t.Helper()

	dir := t.TempDir()

	return afero.NewBasePathFs(afero.NewOsFs(), dir), dir
}

func TestUnlinkClaude(t *testing.T) {
	t.Run("the link to AGENTS.md is removed, and AGENTS.md kept", func(t *testing.T) {
		fs, dir := realFs(t)
		writeFile(t, fs, agentsPath, "# Working here\n")

		if err := os.Symlink(agentsPath, filepath.Join(dir, claudePath)); err != nil {
			t.Fatal(err)
		}

		if err := unlinkClaude(fs); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Lstat(filepath.Join(dir, claudePath)); !os.IsNotExist(err) {
			t.Errorf("want no %s, got err = %v", claudePath, err)
		}

		if got := readFile(t, fs, agentsPath); got != "# Working here\n" {
			t.Errorf("%s = %q, want it untouched", agentsPath, got)
		}
	})

	t.Run("a file somebody wrote is left alone", func(t *testing.T) {
		fs, _ := realFs(t)

		const own = "# my own instructions\n"
		writeFile(t, fs, claudePath, own)

		if err := unlinkClaude(fs); err != nil {
			t.Fatal(err)
		}

		if got := readFile(t, fs, claudePath); got != own {
			t.Errorf("%s = %q, want it untouched", claudePath, got)
		}
	})

	for _, target := range []string{"OTHER.md", "GONE.md"} {
		t.Run("a link of somebody's own to "+target+" is left alone", func(t *testing.T) {
			fs, dir := realFs(t)
			writeFile(t, fs, "OTHER.md", "# elsewhere\n")

			if err := os.Symlink(target, filepath.Join(dir, claudePath)); err != nil {
				t.Fatal(err)
			}

			if err := unlinkClaude(fs); err != nil {
				t.Fatal(err)
			}

			if got, err := os.Readlink(filepath.Join(dir, claudePath)); err != nil {
				t.Fatal(err)
			} else if got != target {
				t.Errorf("target = %q, want %q left", got, target)
			}
		})
	}

	t.Run("nothing there is nothing to do", func(t *testing.T) {
		fs, _ := realFs(t)

		if err := unlinkClaude(fs); err != nil {
			t.Fatal(err)
		}

		if err := unlinkClaude(afero.NewMemMapFs()); err != nil {
			t.Fatal(err)
		}
	})
}

// TestClaudeTaskOnARepo runs the task the way a run does: through
// [ClaudeTask], on a [Repo]. Reading a link needs a method the concrete
// filesystem has and a Repo embedding afero.Fs once did not, which is how the
// old linking task shipped broken while every test of its inner function
// passed.
func TestClaudeTaskOnARepo(t *testing.T) {
	dir := t.TempDir()
	repo := &fakeRepo{fs: afero.NewBasePathFs(afero.NewOsFs(), dir)}

	writeFile(t, repo.Fs(), agentsPath, "# Working here\n")

	if err := os.Symlink(agentsPath, filepath.Join(dir, claudePath)); err != nil {
		t.Fatal(err)
	}

	if err := ClaudeTask(repo).Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(dir, claudePath)); !os.IsNotExist(err) {
		t.Errorf("want no %s, got err = %v", claudePath, err)
	}
}
