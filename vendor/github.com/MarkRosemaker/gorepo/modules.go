package gorepo

import (
	"context"
	"errors"
	"io/fs"
)

// SettleModules brings go.mod and go.sum in line with the code, and vendor/
// where the module vendors: after a change that imports a package go.mod does
// not require or vendor/ does not hold yet, for one.
func (r Repository) SettleModules(ctx context.Context) error {
	if err := r.GoModTidy(ctx); err != nil {
		return err
	}

	if _, err := r.Stat("vendor"); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	return r.GoModVendor(ctx)
}
