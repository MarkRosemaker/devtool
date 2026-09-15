package run

import (
	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/gorepo"
	"github.com/go-git/go-git/v6"
	"github.com/spf13/afero"
)

// repository is one of my repositories, as both the engine and this package
// need it.
//
// [engine.Repo] names only what the engine uses, so the concrete type has
// to be carried here too: the task sequence reaches for gorepo operations the
// engine has no business knowing about — "go mod tidy" is not a thing a
// generic task runner should have heard of.
type repository struct{ *gorepo.Repository }

// The engine's requirements, checked at compile time. This line is what
// replaces a class of bug: when Repo was a concrete type embedding an
// afero.Fs, a missing filesystem method could only be discovered by running
// against a real repository, which is how ClaudeTask shipped broken. Now
// failing to satisfy it does not build.
var _ engine.Repo = (*repository)(nil)

// Fs satisfies [engine.Repo], which asks for the working tree rather than
// embedding it. gorepo embeds the filesystem as a field, so this hands the
// same value over under the name the engine asks by.
func (r *repository) Fs() afero.Fs { return r.Repository.Fs }

// HardReset and Clean satisfy [engine.Repo], which names them without a
// reset mode so the engine needs no git library for the sake of one constant.
// Supplying the constant is this side's job.
func (r *repository) HardReset() error { return r.GitReset(git.HardReset) }
func (r *repository) Clean() error     { return r.GitClean() }
