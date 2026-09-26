package run

import (
	"context"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/devtool/maintain"
)

// licenseHolder is who the copyright is asserted by: the legal person, with the
// GitHub handle after it so the notice connects to where the work lives. A
// notice naming only a pseudonym would leave the holder to establish that link
// later, if it ever mattered.
const licenseHolder = "Marco Rösler (MarkRosemaker)"

// tasks is the sequence applied to every repository, in order.
//
// The order is not arbitrary:
//
//   - Everything devtool owns comes first, as one task and so one commit —
//     licence, README, Makefile, .gitignore, AGENTS.md, CLAUDE.md, the lint
//     config and devtool.json. First, so a repository is licensed, documented
//     and buildable even if a later task fails. One task, so devtool.json is
//     committed with the rest instead of being written after the push and
//     discarded by the next run, which is what happened before.
//   - Dependencies and tools are updated before the code is reformatted, so the
//     formatters see the code the new versions produce.
//   - Formatting runs after the fixers, so anything they rewrote is left tidy.
//   - The single-linter fixes come last, one task each, so every commit carries
//     exactly one kind of change and is easy to read later.
func tasks(
	r *engine.Runner, repo *repository, spec engine.Spec, version string,
) []engine.Task {
	return []engine.Task{
		maintain.UpdateTask(repo, maintain.UpdateOptions{
			Holder:      licenseHolder,
			Coverage:    spec.Coverage,
			Version:     version,
			Description: spec.Description,
			Topics:      spec.Topics,
		}),
		{Name: "update dependencies", Short: "deps", Run: repo.UpdateDependencies},
		{Name: "go fix", Short: "fix", Run: repo.GoFix},
		{Name: "go vet", Short: "vet", Run: repo.GoVet},
		{Name: "golang-ci lint fix", Short: "lintfix", Run: func(ctx context.Context) error {
			return r.Serialise(repo.GolangCILintFix)(ctx)
		}},
		{Name: "update tools", Short: "tools", Run: repo.UpdateTools},
		{Name: "go generate", Short: "generate", Run: repo.GoGenerate},
	}
}

// sequence is the [engine.Sequence] the engine calls for this repository,
// carrying the build doing the work, which the repository cannot know.
//
// It ignores the [engine.Repo] the engine hands it, having the concrete
// repository already: [tasks] reaches for gorepo operations that
// engine.Repo deliberately does not name.
func (repo *repository) sequence(version string) engine.Sequence {
	return func(r *engine.Runner, _ engine.Repo, spec engine.Spec) []engine.Task {
		return tasks(r, repo, spec, version)
	}
}
