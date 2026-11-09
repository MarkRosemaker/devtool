package gorepo

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"

	"github.com/MarkRosemaker/ghrepo"
	golangcijson "github.com/MarkRosemaker/golangci-json"
	"github.com/golangci/golangci-lint/v2/pkg/config"
)

func (r Repository) GolangCILint(ctx context.Context) error {
	return r.golangCILint(ctx, false, nil)
}

func (r Repository) GolangCILintFix(ctx context.Context) error {
	return r.golangCILint(ctx, true, nil)
}

// GolangCILintWithLinters runs golangci-lint with these linters only; a setting left at its zero value keeps
// golangci-lint's default, so a default of true cannot be turned off.
func (r Repository) GolangCILintWithLinters(ctx context.Context, fix bool, settings config.Linters) error {
	return r.golangCILint(ctx, fix, &settings)
}

func (r Repository) golangCILint(ctx context.Context, fix bool, linters *config.Linters) error {
	args := []string{"run"}
	if fix {
		args = append(args, "--fix")
	}

	if linters != nil {
		tmp, err := os.CreateTemp("", "golangci-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name()) //nolint:errcheck

		if err := writeConfig(tmp, *linters); err != nil {
			return err
		}
		defer tmp.Close() //nolint:errcheck

		args = append(args, "-c", tmp.Name())
	}

	if _, err := r.ExecCommand(ctx, "golangci-lint", args...); err != nil {
		const noPackagesMsg = "level=error msg=\"Running error: context loading failed: no go files to analyze: running `go mod tidy` may solve the problem\""
		if execErr := (ghrepo.ExecError{}); errors.As(err, &execErr) &&
			execErr.Out == noPackagesMsg {
			return nil
		}

		return err
	}

	return nil
}

// writeConfig writes a golangci-lint configuration that runs linters, through golangci-json, which writes it as
// golangci-lint reads it. A setting left at its zero value takes golangci-lint's default first: golangci-json writes a
// zero as meant, so godot's period, true by default, would otherwise be turned off.
func writeConfig(w io.Writer, linters config.Linters) error {
	withDefaults(reflect.ValueOf(&linters).Elem(), reflect.ValueOf(config.NewDefault().Linters))

	return golangcijson.MarshalWrite(w, config.Config{Version: "2", Linters: linters})
}

// withDefaults sets each field of v left at its zero value to the one def holds, field by field through structs, so
// what a caller set stays.
func withDefaults(v, def reflect.Value) {
	switch {
	case v.Kind() == reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				withDefaults(v.Field(i), def.Field(i))
			}
		}
	case v.IsZero() && !def.IsZero():
		v.Set(def)
	}
}
