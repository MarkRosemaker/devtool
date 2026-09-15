package run

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/MarkRosemaker/devtool/internal/remote"
)

// ConfigFile is the list of repositories to maintain, and the repository that
// holds it.
//
// Both, because a run writes the coverage it measured back into the file and
// commits it, so the next run on any machine starts from it. Which repository
// that is used to be a constant naming one person's; it is worked out from the
// file's own location instead, so anybody's list works the same way.
type ConfigFile struct {
	// Path is the file itself.
	Path string

	// StatePath is where the outcome of the last run is recorded, alongside it.
	StatePath string

	// Owner and Name identify the repository holding Path.
	Owner, Name string
}

// Dir is the directory the file lives in, which is the repository's root
// unless somebody has put the list somewhere odd.
func (c ConfigFile) Dir() string { return filepath.Dir(c.Path) }

// OpenConfigFile works out everything about the list from its path.
func OpenConfigFile(ctx context.Context, path, stateName string) (ConfigFile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("resolving %s: %w", path, err)
	}

	cfg := ConfigFile{
		Path:      abs,
		StatePath: filepath.Join(filepath.Dir(abs), stateName),
	}

	owner, name, ok := remote.Of(ctx, cfg.Dir())
	if !ok {
		return ConfigFile{}, fmt.Errorf(
			"working out which repository holds %s: its directory has no GitHub remote", abs,
		)
	}

	cfg.Owner, cfg.Name = owner, name

	return cfg, nil
}
