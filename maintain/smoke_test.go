package maintain

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func runSmoke(t *testing.T, repo *fakeRepo) {
	t.Helper()

	if err := SmokeTask(repo).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// A web app without a smoke test gets one: the harness, Playwright pinned, a
// smoke.js that opens the app, and the workflow that runs it.
func TestSmokeScaffolds(t *testing.T) {
	repo := webappRepo(t, gardenDefinition)
	runSmoke(t, repo)

	fs := repo.Fs()

	if got := readFile(t, fs, "smoke/harness.js"); got != string(smokeHarnessText) {
		t.Errorf("harness.js is not the harness:\n%s", got)
	}

	if got := readFile(t, fs, "smoke/smoke.js"); got != string(smokeStarterText) {
		t.Errorf("smoke.js is not the starter:\n%s", got)
	}

	pkg := readFile(t, fs, "smoke/package.json")
	for _, want := range []string{`"name": "gorepo-smoke"`, `"private": true`, `"playwright": "` + playwrightTool.Default + `"`} {
		if !strings.Contains(pkg, want) {
			t.Errorf("package.json lacks %s:\n%s", want, pkg)
		}
	}

	wf := readFile(t, fs, smokeWorkflow)
	for _, want := range []string{"# " + generatedBy, "pull_request:", "npx playwright install --with-deps chromium", "run: node smoke.js"} {
		if !strings.Contains(wf, want) {
			t.Errorf("the workflow lacks %q:\n%s", want, wf)
		}
	}
}

// The app's own smoke.js and dependencies stay; the harness and the pin are
// devtool's, and are put back.
func TestSmokeKeepsTheAppsOwn(t *testing.T) {
	repo := webappRepo(t, gardenDefinition)
	fs := repo.Fs()

	const own = "require('./harness').run(async (app) => { await app.open('/#/beds'); });\n"

	writeFile(t, fs, "smoke/smoke.js", own)
	writeFile(t, fs, "smoke/harness.js", "// edited\n")
	writeFile(t, fs, "smoke/package.json", `{"name": "garden-smoke", "dependencies": {"playwright": "1.0.0", "react": "18.3.1"}}`)

	runSmoke(t, repo)

	if got := readFile(t, fs, "smoke/smoke.js"); got != own {
		t.Errorf("smoke.js was replaced:\n%s", got)
	}

	if got := readFile(t, fs, "smoke/harness.js"); got != string(smokeHarnessText) {
		t.Errorf("an edited harness was left:\n%s", got)
	}

	pkg := readFile(t, fs, "smoke/package.json")
	for _, want := range []string{`"name": "garden-smoke"`, `"react": "18.3.1"`, `"playwright": "` + playwrightTool.Default + `"`} {
		if !strings.Contains(pkg, want) {
			t.Errorf("package.json lacks %s:\n%s", want, pkg)
		}
	}
}

// A smoke workflow somebody wrote is theirs; the generated one is rewritten.
func TestSmokeWorkflowOwnership(t *testing.T) {
	repo := webappRepo(t, gardenDefinition)
	fs := repo.Fs()

	const own = "name: Smoke\non: push\n"

	writeFile(t, fs, smokeWorkflow, own)
	runSmoke(t, repo)

	if got := readFile(t, fs, smokeWorkflow); got != own {
		t.Errorf("a workflow somebody wrote was replaced:\n%s", got)
	}

	writeFile(t, fs, smokeWorkflow, "# "+generatedBy+"\nname: Old\n")
	runSmoke(t, repo)

	if got := readFile(t, fs, smokeWorkflow); !strings.Contains(got, "name: Smoke") {
		t.Errorf("a generated workflow was not rewritten:\n%s", got)
	}
}

// A repository that is not a web app gets no smoke test.
func TestSmokeOnlyForWebapps(t *testing.T) {
	repo := &fakeRepo{}
	writeFile(t, repo.Fs(), DefinitionPath, `{"kind": "cli"}`)
	runSmoke(t, repo)

	if ok, err := afero.DirExists(repo.Fs(), "smoke"); err != nil || ok {
		t.Errorf("smoke/ written outside a web app (%v)", err)
	}
}
