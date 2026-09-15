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
//   - The licence, README, Makefile and AGENTS.md come first, so a
//     repository is properly licensed, documented and buildable even if a
//     later task fails — and so the make targets AGENTS.md points an agent
//     at exist before it is written.
//   - .gitignore comes after the Makefile, whose targets settle what there
//     is to ignore, and before AGENTS.md, which says so only once the block
//     is really there.
//   - Dependencies and tools are updated before the code is reformatted, so the
//     formatters see the code the new versions produce.
//   - Formatting runs after the fixers, so anything they rewrote is left tidy.
//   - The single-linter fixes come last, one task each, so every commit carries
//     exactly one kind of change and is easy to read later.
func tasks(r *engine.Runner, repo *repository, spec engine.Spec) []engine.Task {
	return []engine.Task{
		maintain.LicenseTask(repo, licenseHolder),
		maintain.ReadmeTask(repo, spec.Coverage),
		maintain.MakefileTask(repo),
		maintain.GitignoreTask(repo),
		maintain.AgentsTask(repo),
		maintain.ClaudeTask(repo),
		{Name: "update dependencies", Short: "deps", Run: repo.UpdateDependencies},
		{Name: "go fix", Short: "fix", Run: repo.GoFix},
		{Name: "go vet", Short: "vet", Run: repo.GoVet},
		maintain.GenLintfile(repo),
		{Name: "golang-ci lint fix", Short: "lintfix", Run: func(ctx context.Context) error {
			return r.Serialise(repo.GolangCILintFix)(ctx)
		}},
		{Name: "update tools", Short: "tools", Run: repo.UpdateTools},
		{Name: "go generate", Short: "generate", Run: repo.GoGenerate},
	}
}

// sequence is the [engine.Sequence] the engine calls for this repository.
//
// It ignores the [engine.Repo] the engine hands it, having the concrete
// repository already: [tasks] reaches for gorepo operations that
// engine.Repo deliberately does not name.
func (repo *repository) sequence(
	r *engine.Runner, _ engine.Repo, spec engine.Spec,
) []engine.Task {
	return tasks(r, repo, spec)
}
