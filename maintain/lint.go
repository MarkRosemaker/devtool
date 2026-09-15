package maintain

import (
	_ "embed"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
)

// lintSettings is the golangci-lint configuration every repository in the
// portfolio is given, generated from internal/lintgen's choices.
//
//go:embed lint.yaml
var lintSettings []byte

// GenLintfile returns the engine's lint-config task, carrying this owner's
// linter settings.
func GenLintfile(repo engine.Repo) engine.Task {
	return engine.GenLintfile(repo, lintSettings)
}
