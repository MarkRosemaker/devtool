package gorepo

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/MarkRosemaker/ghrepo"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// Repository represents a local go repository.
type Repository struct{ *ghrepo.Repository }

func (r Repository) IsGoRepo() (bool, error) {
	if _, err := r.Stat("go.mod"); err == nil {
		return true, nil
	} else if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
}

// GoModInit initializes a go module in the repository.
func (r Repository) GoModInit(ctx context.Context) error {
	_, err := r.ExecCommand(ctx, "go", "mod", "init")
	return err
}

// UpdateDependencies updates all dependencies in the repository.
func (r Repository) UpdateDependencies(ctx context.Context) error {
	if err := r.GoGetAll(ctx); err != nil {
		return err
	}

	if err := r.GoModTidy(ctx); err != nil {
		return err
	}

	return r.GoModVendor(ctx)
}

// GoGetAll updates all package dependencies reachable from the module.
func (r Repository) GoGetAll(ctx context.Context) error {
	// NOTE:
	// 	./... = only packages reachable from the module + their real dependencies
	// all = every module ever mentioned in the build list, including ones that only exist for old versions
	_, err := r.ExecCommand(ctx, "go", "get", "-u", "./...")
	return err
}

// UpdateTools updates all go tools in the repository.
func (r Repository) UpdateTools(ctx context.Context) error {
	if err := r.GoGetTools(ctx); err != nil {
		return err
	}

	if err := r.GoModTidy(ctx); err != nil {
		return err
	}

	return r.GoModVendor(ctx)
}

// GoGetTools updates all tool dependencies listed in go.mod.
func (r Repository) GoGetTools(ctx context.Context) error {
	_, err := r.ExecCommand(ctx, "go", "get", "-u", "tool")
	return err
}

// GoModTidy tidies the go module in the repository.
func (r Repository) GoModTidy(ctx context.Context) error {
	_, err := r.ExecCommand(ctx, "go", "mod", "tidy")
	return err
}

// GoModVendor vendors all dependencies in the repository.
func (r Repository) GoModVendor(ctx context.Context) error {
	_, err := r.ExecCommand(ctx, "go", "mod", "vendor")
	return err
}

func (r Repository) Goimports(ctx context.Context) error {
	eg := errgroup.Group{}

	if err := afero.Walk(r, ".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip entire vendor directory (and .git, etc. if desired)
		if info.IsDir() {
			switch filepath.Base(path) {
			case "vendor", ".git":
				return filepath.SkipDir // skips the whole subtree
			default:
				return nil
			}
		}

		// Only process .go files
		if filepath.Ext(path) != ".go" {
			return nil
		}

		// Run goimports -w on this single file
		eg.Go(func() error {
			_, err := r.ExecCommand(ctx, "goimports", "-w", path)
			return err
		})

		return nil
	}); err != nil {
		return err
	}

	return eg.Wait()
}

func (r Repository) Gofumpt(ctx context.Context) error {
	_, err := r.ExecCommand(ctx, "gofumpt", "-extra", "-w", ".")
	return err
}

func (r Repository) GoFix(ctx context.Context) error {
	if _, err := r.ExecCommand(ctx, "go", "fix", "./..."); err != nil {
		return err
	}

	return nil
}

// GoVet runs go vet on the repository
func (r Repository) GoVet(ctx context.Context) error {
	if _, err := r.ExecCommand(ctx, "go", "vet", "./..."); err != nil {
		const noPackagesMsg = "go: warning: \"./...\" matched no packages\nno packages to vet"
		if execErr := (ghrepo.ExecError{}); errors.As(err, &execErr) &&
			execErr.Out == noPackagesMsg {
			return nil
		}

		return err
	}

	return nil
}

func (r Repository) GoGenerate(ctx context.Context) error {
	_, err := r.ExecCommand(ctx, "go", "generate", "./...")
	return err
}
