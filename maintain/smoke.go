package maintain

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path"
	"text/template"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

// playwrightVersion is the Playwright every smoke test runs on.
const playwrightVersion = "1.63.0"

const (
	smokeHarness  = "harness.js"
	smokePackage  = "package.json"
	smokeWorkflow = ".github/workflows/smoke.yml"
)

var (
	//go:embed smoke_harness.js
	smokeHarnessText []byte

	//go:embed smoke_starter.js
	smokeStarterText []byte

	//go:embed smoke.yml.tmpl
	smokeWorkflowText string

	smokeWorkflowTmpl = template.Must(template.New(smokeWorkflow).Parse(smokeWorkflowText))
)

// SmokeTask gives every web app its smoke test, which make ready and a
// workflow on every pull request run: smoke/harness.js, which serves the app
// with the API answered from api/interactions.json and fails on any error a
// page shows, rewritten every run; Playwright in smoke/package.json, beside
// whatever else the test needs; and, where there is none, a smoke/smoke.js
// that opens the app, for the app to add its screens to.
func SmokeTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "smoke test",
		Short: "smoke",
		Run: func(context.Context) error {
			return writeSmoke(repo.Fs(), repo.Name())
		},
	}
}

func writeSmoke(fs afero.Fs, name string) error {
	def, _, err := LoadDefinition(fs)
	if err != nil {
		return err
	}

	if !hasSmoke(def) {
		return nil
	}

	if err := fs.MkdirAll(smokeDir, 0o755); err != nil {
		return err
	}

	if err := afero.WriteFile(fs, path.Join(smokeDir, smokeHarness), smokeHarnessText, 0o644); err != nil {
		return err
	}

	if err := writeSmokePackage(fs, name); err != nil {
		return err
	}

	script := path.Join(smokeDir, smokeScript)
	if ok, err := afero.Exists(fs, script); err != nil {
		return err
	} else if !ok {
		if err := afero.WriteFile(fs, script, smokeStarterText, 0o644); err != nil {
			return err
		}
	}

	return writeSmokeWorkflow(fs)
}

// writeSmokePackage pins Playwright in smoke/package.json, keeping what else
// the test depends on.
func writeSmokePackage(fs afero.Fs, name string) error {
	file := path.Join(smokeDir, smokePackage)

	pkg := map[string]any{"name": name + "-smoke", "private": true}

	b, err := afero.ReadFile(fs, file)

	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &pkg); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}

	deps, _ := pkg["dependencies"].(map[string]any)
	if deps == nil {
		deps = map[string]any{}
	}

	deps["playwright"] = playwrightVersion
	pkg["dependencies"] = deps

	out, err := json.Marshal(pkg, json.Deterministic(true), jsontext.Multiline(true), jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}

	return afero.WriteFile(fs, file, append(out, '\n'), 0o644)
}

// writeSmokeWorkflow writes the workflow that runs the smoke test on every
// pull request, leaving one somebody wrote alone.
func writeSmokeWorkflow(fs afero.Fs) error {
	existing, err := afero.ReadFile(fs, smokeWorkflow)

	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("reading %s: %w", smokeWorkflow, err)
	case !generatedMarker.Match(firstLine(existing)):
		return nil
	}

	var buf bytes.Buffer
	if err := smokeWorkflowTmpl.Execute(&buf, struct{ Stamp string }{generatedBy}); err != nil {
		return fmt.Errorf("rendering %s: %w", smokeWorkflow, err)
	}

	if err := fs.MkdirAll(path.Dir(smokeWorkflow), 0o755); err != nil {
		return err
	}

	return afero.WriteFile(fs, smokeWorkflow, buf.Bytes(), 0o644)
}
