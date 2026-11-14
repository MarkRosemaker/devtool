package gorepo

import (
	"context"
	"errors"
	"io/fs"
	"strings"
)

// outOfStep is what the go command says when go.mod, go.sum or vendor/ no
// longer match the code: an import nothing requires or vendors yet, or a
// requirement vendor/modules.txt does not list.
var outOfStep = []string{
	"import lookup disabled by -mod=vendor",
	"inconsistent vendoring",
	"no required module provides package",
	"missing go.sum entry",
	"updates to go.mod needed",
}

// ModulesOutOfStep reports whether err says the module files no longer match
// the code, which [Repository.SettleModules] puts right.
func ModulesOutOfStep(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	for _, s := range outOfStep {
		if strings.Contains(msg, s) {
			return true
		}
	}

	return false
}

// SettleModules brings go.mod and go.sum in line with the code, and vendor/
// where the module vendors.
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
