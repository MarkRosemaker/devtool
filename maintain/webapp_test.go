package maintain

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

const webappGoMod = "module example.com/garden\n\ngo 1.27\n\nrequire github.com/MarkRosemaker/webapp v0.0.0\n"

// webappRepo is a web app as a design's export leaves it, declaring how it
// is served.
func webappRepo(t *testing.T, definition string) *fakeRepo {
	t.Helper()

	repo := &fakeRepo{}
	fs := repo.Fs()

	writeFile(t, fs, "go.mod", webappGoMod)
	writeFile(t, fs, DefinitionPath, definition)
	writeFile(t, fs, "frontend/index.html", "<html><head></head><body></body></html>\n")

	return repo
}

const gardenDefinition = `{
	"name": "Pocket Garden",
	"description": "What to water \"when\"",
	"kind": "webapp",
	"webapp": {"port": 8090, "themeColor": "#F4EFE6", "backgroundColor": "#FBF8F2", "auth": true}
}
`

func runWebapp(t *testing.T, repo *fakeRepo) error {
	t.Helper()

	return WebappTask(repo).Run(t.Context())
}

// mainGo reads main.go, which has to be Go.
func mainGo(t *testing.T, fs afero.Fs) string {
	t.Helper()

	src := readFile(t, fs, webappMain)
	if _, err := parser.ParseFile(token.NewFileSet(), webappMain, src, parser.ParseComments); err != nil {
		t.Fatalf("main.go is not Go: %v\n%s", err, src)
	}

	return src
}

// What devtool.json says is what the app is served as, and app.go is where
// the app's own code starts.
func TestWebappWritesMainAndApp(t *testing.T) {
	repo := webappRepo(t, gardenDefinition)
	if err := runWebapp(t, repo); err != nil {
		t.Fatal(err)
	}

	fs := repo.Fs()
	got := mainGo(t, fs)

	for _, want := range []string{
		goGenerated + "\n",
		"package main\n",
		`"github.com/MarkRosemaker/webapp"`,
		"//go:embed frontend\nvar frontend embed.FS\n",
		`Name:            "Pocket Garden",`,
		`Description:     "What to water \"when\"",`,
		"Port:            8090,",
		`ThemeColor:      "#F4EFE6",`,
		`BackgroundColor: "#FBF8F2",`,
		"Auth:            true,",
		"Frontend:        frontend,",
		"}, setup)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("main.go lacks %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "go:generate") {
		t.Errorf("no spec under api/, nothing to generate:\n%s", got)
	}

	app := readFile(t, fs, webappApp)
	if _, err := parser.ParseFile(token.NewFileSet(), webappApp, app, 0); err != nil {
		t.Fatalf("app.go is not Go: %v\n%s", err, app)
	}

	if !strings.Contains(app, "func setup(_ context.Context, mux *http.ServeMux) (http.Handler, func(), error) {\n\treturn mux, func() {}, nil\n}") {
		t.Errorf("app.go has no setup to start from:\n%s", app)
	}

	if strings.Contains(app, "generated") {
		t.Errorf("app.go is the app's own, not generated:\n%s", app)
	}
}

// The go:generate lines follow the specs the app has, each enriched only
// from a file it has.
func TestWebappGeneratesFromTheSpecs(t *testing.T) {
	enrich := "//go:generate go tool openapi-enrich\n"
	openapi := []string{
		"//go:generate go tool openapi-flatten\n",
		"//go:generate go tool openapi-compress\n",
		"//go:generate go tool openapi-codegen -js -out=frontend/src\n",
		"//go:generate go tool openapi-codegen -server -out=pkg/api -pkg=api\n",
	}
	sessions := "//go:generate go tool asyncapi-enrich -spec api/asyncapi.json -sessions api/sessions.json\n"
	asyncapi := []string{
		"//go:generate go tool asyncapi-flatten -spec api/asyncapi.json\n",
		"//go:generate go tool asyncapi-codegen -js -out=frontend/src -name=ws.js\n",
		"//go:generate go tool asyncapi-codegen -server -out=pkg/ws -pkg=ws\n",
	}

	for _, tc := range []struct {
		name          string
		files         []string
		want, without []string
	}{
		{"openapi alone", []string{"api/openapi.json"}, openapi, append([]string{enrich, sessions}, asyncapi...)},
		{"openapi with interactions", []string{"api/openapi.json", interactionsPath}, append(openapi, enrich), asyncapi},
		{"asyncapi alone", []string{"api/asyncapi.json"}, asyncapi, append([]string{sessions, enrich}, openapi...)},
		{
			"both, enriched",
			[]string{"api/openapi.json", interactionsPath, "api/asyncapi.json", "api/sessions.json"},
			append(append(openapi, enrich, sessions), asyncapi...), nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := webappRepo(t, gardenDefinition)
			for _, f := range tc.files {
				writeFile(t, repo.Fs(), f, "{}\n")
			}

			if err := runWebapp(t, repo); err != nil {
				t.Fatal(err)
			}

			got := mainGo(t, repo.Fs())

			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("main.go lacks %q:\n%s", want, got)
				}
			}

			for _, not := range tc.without {
				if strings.Contains(got, not) {
					t.Errorf("main.go has %q:\n%s", not, got)
				}
			}
		})
	}

	t.Run("enriched before flattened, and the server last", func(t *testing.T) {
		repo := webappRepo(t, gardenDefinition)
		writeFile(t, repo.Fs(), "api/openapi.json", "{}\n")
		writeFile(t, repo.Fs(), interactionsPath, "{}\n")

		got := mainGo(t, written(t, repo))
		if !(strings.Index(got, enrich) < strings.Index(got, openapi[0]) &&
			strings.Index(got, openapi[2]) < strings.Index(got, openapi[3])) {
			t.Errorf("out of order:\n%s", got)
		}
	})
}

// written runs the task and returns the filesystem it wrote to.
func written(t *testing.T, repo *fakeRepo) afero.Fs {
	t.Helper()

	if err := runWebapp(t, repo); err != nil {
		t.Fatal(err)
	}

	return repo.Fs()
}

// main.go follows devtool.json from then on, and app.go is left to the app.
func TestWebappRewritesMainOnly(t *testing.T) {
	repo := webappRepo(t, gardenDefinition)
	fs := written(t, repo)

	own := "package main\n\n// the app's own setup\n"
	writeFile(t, fs, webappApp, own)
	writeFile(t, fs, DefinitionPath, strings.Replace(gardenDefinition, "8090", "8091", 1))

	got := mainGo(t, written(t, repo))
	if !strings.Contains(got, "8091") {
		t.Errorf("main.go did not follow devtool.json:\n%s", got)
	}

	if readFile(t, fs, webappApp) != own {
		t.Error("app.go was rewritten")
	}

	t.Run("nor written again once gone", func(t *testing.T) {
		if err := fs.Remove(webappApp); err != nil {
			t.Fatal(err)
		}

		if ok, _ := afero.Exists(written(t, repo), webappApp); ok {
			t.Error("app.go was written again: setup may live elsewhere by now")
		}
	})

	t.Run("an app.go there before main.go is kept", func(t *testing.T) {
		repo := webappRepo(t, gardenDefinition)
		writeFile(t, repo.Fs(), webappApp, own)

		if readFile(t, written(t, repo), webappApp) != own {
			t.Error("app.go was overwritten")
		}
	})
}

// A repository without a webapp section is left alone, whatever its kind.
func TestWebappNeedsItsSection(t *testing.T) {
	repo := webappRepo(t, `{"kind": "webapp", "name": "Pocket Garden"}`)
	writeFile(t, repo.Fs(), webappMain, "package main\n\nfunc main() {}\n")

	if err := runWebapp(t, repo); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, repo.Fs(), webappMain); got != "package main\n\nfunc main() {}\n" {
		t.Errorf("main.go was touched:\n%s", got)
	}

	if ok, _ := afero.Exists(repo.Fs(), webappApp); ok {
		t.Error("app.go was written")
	}
}

// What main.go could not be written or built from is refused, saying what
// to change, and nothing is written.
func TestWebappRefuses(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(t *testing.T, fs afero.Fs)
	}{
		{"not a web app", `"kind": "webapp"`, func(t *testing.T, fs afero.Fs) {
			writeFile(t, fs, DefinitionPath, strings.Replace(gardenDefinition, `"kind": "webapp"`, `"kind": "cli"`, 1))
		}},
		{"no name", `"name"`, func(t *testing.T, fs afero.Fs) {
			writeFile(t, fs, DefinitionPath, strings.Replace(gardenDefinition, `"name": "Pocket Garden",`, "", 1))
		}},
		{"no port", `"port" 0`, func(t *testing.T, fs afero.Fs) {
			writeFile(t, fs, DefinitionPath, strings.Replace(gardenDefinition, `"port": 8090, `, "", 1))
		}},
		{"no such port", `"port" 70000`, func(t *testing.T, fs afero.Fs) {
			writeFile(t, fs, DefinitionPath, strings.Replace(gardenDefinition, "8090", "70000", 1))
		}},
		{"no frontend", "frontend/", func(t *testing.T, fs afero.Fs) {
			if err := fs.RemoveAll("frontend"); err != nil {
				t.Fatal(err)
			}
		}},
		{"no library", "go get it", func(t *testing.T, fs afero.Fs) {
			writeFile(t, fs, "go.mod", "module example.com/garden\n\ngo 1.27\n")
		}},
		{"a main.go of its own", "move what it does into setup", func(t *testing.T, fs afero.Fs) {
			writeFile(t, fs, webappMain, "package main\n\nfunc main() {}\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := webappRepo(t, gardenDefinition)
			tc.change(t, repo.Fs())

			before := snapshot(t, repo.Fs())

			err := runWebapp(t, repo)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want one saying %q", err, tc.want)
			}

			if after := snapshot(t, repo.Fs()); len(after) != len(before) {
				t.Errorf("wrote files: %d before, %d after", len(before), len(after))
			} else if after[webappMain] != before[webappMain] {
				t.Errorf("main.go changed:\n%s", after[webappMain])
			}
		})
	}
}

// devtool.json keeps the name and the webapp section, and a run that changes
// only them changes the file.
func TestDefinitionKeepsTheWebapp(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, DefinitionPath, gardenDefinition)

	def, _, err := LoadDefinition(fs)
	if err != nil {
		t.Fatal(err)
	}

	if err := SaveDefinition(fs, def); err != nil {
		t.Fatal(err)
	}

	again, _, err := LoadDefinition(fs)
	if err != nil {
		t.Fatal(err)
	}

	if !again.equal(def) || again.Name != "Pocket Garden" || again.Webapp == nil || *again.Webapp != *def.Webapp {
		t.Errorf("lost in saving: %+v", again)
	}

	moved := again
	moved.Webapp = &Webapp{Port: 8091}

	unnamed := again
	unnamed.Name = ""

	for name, d := range map[string]Definition{"port": moved, "name": unnamed, "section": {Name: def.Name, Description: def.Description, Kind: def.Kind}} {
		if d.equal(def) {
			t.Errorf("a change to the %s reads as no change", name)
		}
	}
}
