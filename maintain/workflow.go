package maintain

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"text/template"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const workflowPath = ".github/workflows/test.yml"

//go:embed test.yml.tmpl
var workflowText string

var workflowTmpl = template.Must(template.New(workflowPath).Parse(workflowText))

// WorkflowTask returns a task that writes the GitHub Actions workflow which
// lints and tests every pull request, with golangci-lint at the version
// devtool.json pins.
//
// Only where the repository vendors its dependencies: a runner can read no
// private module, so without vendor/ a private dependency fails the job. A
// workflow somebody wrote is left alone.
func WorkflowTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "generate test workflow",
		Short: "workflow",
		Run: func(context.Context) error {
			return generateWorkflow(repo.Fs())
		},
	}
}

// generateWorkflow is [WorkflowTask]'s work, factored out so it can run
// against an in-memory filesystem in tests without a real repository.
func generateWorkflow(fs afero.Fs) error {
	existing, err := afero.ReadFile(fs, workflowPath)

	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("reading %s: %w", workflowPath, err)
	case !generatedMarker.Match(firstLine(existing)):
		return nil
	}

	vendored, err := afero.Exists(fs, "vendor/modules.txt")
	if err != nil {
		return err
	}

	if !vendored {
		if existing != nil {
			return fs.Remove(workflowPath)
		}

		return nil
	}

	def, _, err := LoadDefinition(fs)
	if err != nil {
		return err
	}

	b, err := renderWorkflow(def)
	if err != nil {
		return err
	}

	if err := fs.MkdirAll(path.Dir(workflowPath), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", path.Dir(workflowPath), err)
	}

	return afero.WriteFile(fs, workflowPath, b, 0o644)
}

// renderWorkflow is the workflow for a repository with this definition.
func renderWorkflow(def Definition) ([]byte, error) {
	i := slices.IndexFunc(houseTools, func(t Tool) bool { return t.Name == "golangci-lint" })
	lint := houseTools[i]

	version := def.Tools[lint.Name]
	if version == "" {
		version = "latest"
	}

	buf := &bytes.Buffer{}
	if err := workflowTmpl.Execute(buf, struct{ Stamp, Lint string }{
		Stamp: generatedBy,
		Lint:  lint.Package + "@" + version,
	}); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", workflowPath, err)
	}

	return buf.Bytes(), nil
}

// hasGeneratedWorkflow reports whether the test workflow is this task's.
func hasGeneratedWorkflow(fs afero.Fs) (bool, error) {
	b, err := afero.ReadFile(fs, workflowPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("reading %s: %w", workflowPath, err)
	}

	return generatedMarker.Match(firstLine(b)), nil
}
