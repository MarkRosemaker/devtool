package gorepo

import (
	"fmt"

	"github.com/spf13/afero"
	"golang.org/x/mod/modfile"
)

// Dependencies returns the list of required modules declared in go.mod.
func (r Repository) Dependencies() ([]*modfile.Require, error) {
	data, err := afero.ReadFile(r, "go.mod")
	if err != nil {
		return nil, fmt.Errorf("reading go.mod: %w", err)
	}

	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing go.mod: %w", err)
	}

	return f.Require, nil
}
