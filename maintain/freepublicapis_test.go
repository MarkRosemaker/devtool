package maintain

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MarkRosemaker/openapi"
)

// gamerpower is go-api-libs/gamerpower's catalog entry.
const gamerpower = `{
  "id": 182,
  "emoji": "🎮",
  "title": "Free Games API",
  "description": "The GamerPower API provides access to free games.",
  "documentation": "https://www.gamerpower.com/api-read",
  "methods": 1,
  "health": 95,
  "source": "https://freepublicapis.com/free-games-api"
}`

// apilibRepo is an API library with a catalog entry, with go.mod having the
// tools generate.go runs where tools is set.
func apilibRepo(t *testing.T, tools bool) *fakeRepo {
	t.Helper()

	repo := &fakeRepo{}
	mod := "module github.com/go-api-libs/sec-edgar\n\ngo 1.27\n"

	if tools {
		mod += "\ntool (\n\t" + strings.Join(apilibTools, "\n\t") + "\n)\n"
	}

	writeFile(t, repo.Fs(), "go.mod", mod)
	writeFile(t, repo.Fs(), DefinitionPath, `{"kind": "apilib"}`+"\n")
	writeFile(t, repo.Fs(), freePublicAPIsPath, gamerpower)

	return repo
}

func runFreePublicAPIs(t *testing.T, repo *fakeRepo) {
	t.Helper()

	if err := FreePublicAPIsTask(repo, false).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func loadSpec(t *testing.T, repo *fakeRepo) *openapi.Document {
	t.Helper()

	doc, err := openapi.LoadFromDataJSON([]byte(readFile(t, repo.Fs(), openAPIPath)))
	if err != nil {
		t.Fatal(err)
	}

	// Valid but for describing nothing yet, which make generate puts right.
	if err := doc.Validate(); err != nil && !errors.Is(err, openapi.ErrEmptyDocument) {
		t.Fatalf("the specification is not valid: %v", err)
	}

	return doc
}

// A new API library gets a specification started from its catalog entry,
// and a generate.go, in its own package, that builds the rest.
func TestFreePublicAPIsStartsTheSpecification(t *testing.T) {
	repo := apilibRepo(t, true)
	runFreePublicAPIs(t, repo)

	doc := loadSpec(t, repo)

	if doc.OpenAPI != "3.1.0" || doc.Info.Title != "Free Games API" || doc.Info.Version != "0.0.1" ||
		doc.Info.Description != "The GamerPower API provides access to free games." {
		t.Errorf("openapi %q, info %+v", doc.OpenAPI, doc.Info)
	}

	if doc.ExternalDocs == nil || doc.ExternalDocs.URL.String() != "https://www.gamerpower.com/api-read" ||
		doc.ExternalDocs.Description != "API documentation" {
		t.Errorf("externalDocs %+v", doc.ExternalDocs)
	}

	gen := readFile(t, repo.Fs(), "generate.go")
	for _, want := range []string{goGenerated, "package secedgar\n", "go tool openapi-enrich", "go tool openapi-codegen -client"} {
		if !strings.Contains(gen, want) {
			t.Errorf("generate.go lacks %q:\n%s", want, gen)
		}
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "generate.go", gen, parser.ParseComments); err != nil {
		t.Errorf("generate.go is not Go: %v", err)
	}

	if len(repo.commands) != 0 {
		t.Errorf("ran %q with the tools already in go.mod", repo.commands)
	}
}

// What the specification already says stays: enrich owns it, and only what
// is empty is filled in, without a write where nothing is.
func TestFreePublicAPIsFillsOnlyWhatIsEmpty(t *testing.T) {
	repo := apilibRepo(t, true)
	writeFile(t, repo.Fs(), openAPIPath, `{"openapi": "3.1.0", "info": {"title": "GamerPower", "version": "2.0.0"}}`)

	runFreePublicAPIs(t, repo)

	doc := loadSpec(t, repo)
	if doc.Info.Title != "GamerPower" || doc.Info.Version != "2.0.0" {
		t.Errorf("replaced what was there: %+v", doc.Info)
	}

	if doc.Info.Description == "" || doc.ExternalDocs == nil {
		t.Errorf("left empty what the catalog has: %+v, %+v", doc.Info, doc.ExternalDocs)
	}

	written := readFile(t, repo.Fs(), openAPIPath)
	runFreePublicAPIs(t, repo)

	if got := readFile(t, repo.Fs(), openAPIPath); got != written {
		t.Errorf("a full specification was rewritten:\n%s", got)
	}
}

// Without the tools in go.mod, they are added and go.mod tidied, vendored
// where the repository vendors.
func TestFreePublicAPIsAddsTheTools(t *testing.T) {
	repo := apilibRepo(t, false)
	writeFile(t, repo.Fs(), "vendor/modules.txt", "")

	runFreePublicAPIs(t, repo)

	want := []string{}
	for _, tool := range apilibTools {
		want = append(want, "go get -tool "+tool)
	}

	want = append(want, "go mod tidy", "go mod vendor")

	if !slices.Equal(repo.commands, want) {
		t.Errorf("ran %q\nwant %q", repo.commands, want)
	}
}

// A generate.go of the repository's own, as go-api-libs/proton builds its
// specification otherwise, is left alone, and so is its go.mod.
func TestFreePublicAPIsLeavesAnOwnGenerate(t *testing.T) {
	repo := apilibRepo(t, false)

	const own = "package main\n\n//go:generate make spec\n"

	writeFile(t, repo.Fs(), "generate.go", own)
	runFreePublicAPIs(t, repo)

	if got := readFile(t, repo.Fs(), "generate.go"); got != own {
		t.Errorf("generate.go was replaced:\n%s", got)
	}

	if len(repo.commands) != 0 {
		t.Errorf("ran %q for a generate.go of the repository's own", repo.commands)
	}
}

// Only an apilib with a catalog entry is laid out.
func TestFreePublicAPIsOnlyWithAnEntry(t *testing.T) {
	for name, setup := range map[string]func(*fakeRepo){
		"no catalog entry": func(r *fakeRepo) { _ = r.Fs().Remove(freePublicAPIsPath) },
		"not an apilib":    func(r *fakeRepo) { writeFile(t, r.Fs(), DefinitionPath, `{"kind": "library"}`) },
	} {
		t.Run(name, func(t *testing.T) {
			repo := apilibRepo(t, false)
			setup(repo)
			runFreePublicAPIs(t, repo)

			for _, f := range []string{openAPIPath, "generate.go"} {
				if ok, _ := afeExists(repo, f); ok {
					t.Errorf("%s written", f)
				}
			}
		})
	}
}

func afeExists(repo *fakeRepo, name string) (bool, error) {
	_, err := repo.Fs().Stat(name)

	return err == nil, nil
}

func TestPackageName(t *testing.T) {
	for module, want := range map[string]string{
		"github.com/go-api-libs/gamerpower": "gamerpower",
		"github.com/go-api-libs/sec-edgar":  "secedgar",
		"github.com/go-api-libs/Yahoo_Fin":  "yahoofin",
		"github.com/go-api-libs/1password":  "api1password",
	} {
		if got := packageName(module); got != want {
			t.Errorf("packageName(%q) = %q, want %q", module, got, want)
		}
	}
}

// The README takes a specification describing nothing yet for one not built
// yet, and still refuses one that is broken otherwise.
func TestReadOpenAPIWaitsForTheSkeleton(t *testing.T) {
	repo := apilibRepo(t, true)
	runFreePublicAPIs(t, repo)

	if doc, err := readOpenAPI(repo.Fs()); err != nil || doc != nil {
		t.Errorf("readOpenAPI(skeleton) = %v, %v, want nothing yet", doc, err)
	}

	writeFile(t, repo.Fs(), openAPIPath, `{"openapi": "3.1.0", "paths": {}}`)

	if _, err := readOpenAPI(repo.Fs()); err == nil {
		t.Error("a specification without info passed")
	}
}

// An API library's README links its documentation and says its health,
// coloured as coverage is.
func TestReadmeHasTheCatalogBadges(t *testing.T) {
	repo := apilibRepo(t, true)

	if err := generateReadme(repo.Fs(), "go-api-libs", "gamerpower", true, 100); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, repo.Fs(), readmePath)
	for _, want := range []string{
		"[![Official Documentation](https://img.shields.io/badge/docs-API-blue)](https://www.gamerpower.com/api-read)",
		"![API Health](https://img.shields.io/badge/API_health-95%25-brightgreen)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the README lacks %s:\n%s", want, got)
		}
	}
}

// fetchedAgo writes the catalog entry as last fetched so long ago, or as
// never fetched where ago is 0.
func fetchedAgo(t *testing.T, repo *fakeRepo, ago time.Duration) {
	t.Helper()

	entry := gamerpower
	if ago != 0 {
		entry = strings.Replace(entry, "\n}", ",\n  \"fetched\": \""+time.Now().UTC().Add(-ago).Format(fetchedLayout)+"\"\n}", 1)
	}

	writeFile(t, repo.Fs(), freePublicAPIsPath, entry)
}

// fetching replaces what freepublicapis.com answers, counting the calls.
func fetching(t *testing.T, answer FreePublicAPI, err error) *int {
	t.Helper()

	calls := 0
	was := fetchFreePublicAPI

	fetchFreePublicAPI = func(_ context.Context, id int) (FreePublicAPI, error) {
		calls++

		if id != answer.ID {
			t.Errorf("asked for %d, want %d", id, answer.ID)
		}

		return answer, err
	}

	t.Cleanup(func() { fetchFreePublicAPI = was })

	return &calls
}

// A maintained run asks the catalog again for an entry fetched a week ago or
// never, and writes what it answers with the day it asked, indented as the
// entries are. The same entry is then not asked for again on the next run,
// though nothing in it changed.
func TestFreePublicAPIsRefreshesAWeekOldEntry(t *testing.T) {
	for name, ago := range map[string]time.Duration{"a week ago": 8 * 24 * time.Hour, "never": 0} {
		t.Run(name, func(t *testing.T) {
			repo := apilibRepo(t, true)
			fetchedAgo(t, repo, ago)

			api, _, _ := LoadFreePublicAPI(repo.Fs())
			api.Fetched = ""
			calls := fetching(t, api, nil)

			for range 2 {
				if err := FreePublicAPIsTask(repo, true).Run(t.Context()); err != nil {
					t.Fatal(err)
				}
			}

			got := readFile(t, repo.Fs(), freePublicAPIsPath)
			today := time.Now().UTC().Format(fetchedLayout)

			if *calls != 1 || !strings.Contains(got, "\n  \"health\": 95,\n") || !strings.Contains(got, `"fetched": "`+today+`"`) {
				t.Errorf("asked %d times, want once, and the entry reads:\n%s", *calls, got)
			}
		})
	}
}

// An entry fetched within the week, a local run, and a catalog that cannot
// be reached all leave the entry as it is.
func TestFreePublicAPIsKeepsTheEntry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ago       time.Duration
		refresh   bool
		err       error
		wantCalls int
	}{
		{"fetched within the week", 6 * 24 * time.Hour, true, nil, 0},
		{"a local run", 30 * 24 * time.Hour, false, nil, 0},
		{"the catalog unreachable", 30 * 24 * time.Hour, true, errors.New("offline"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := apilibRepo(t, true)
			fetchedAgo(t, repo, tc.ago)
			before := readFile(t, repo.Fs(), freePublicAPIsPath)

			api, _, _ := LoadFreePublicAPI(repo.Fs())
			api.Health = 1
			calls := fetching(t, api, tc.err)

			if err := FreePublicAPIsTask(repo, tc.refresh).Run(t.Context()); err != nil {
				t.Fatal(err)
			}

			if *calls != tc.wantCalls {
				t.Errorf("asked %d times, want %d", *calls, tc.wantCalls)
			}

			if got := readFile(t, repo.Fs(), freePublicAPIsPath); got != before {
				t.Errorf("the entry was rewritten:\n%s", got)
			}
		})
	}
}

// A README is headed by an API library's catalog title, with its emoji, then
// by the name devtool.json gives, then by the repository's.
func TestReadmeTitle(t *testing.T) {
	api := &FreePublicAPI{Emoji: "🎮", Title: "Free Games API"}

	for _, tc := range []struct {
		name string
		def  Definition
		api  *FreePublicAPI
		want string
	}{
		{"an API library", Definition{Name: "GamerPower"}, api, "🎮 Free Games API"},
		{"no emoji", Definition{}, &FreePublicAPI{Title: "Free Games API"}, "Free Games API"},
		{"a named app", Definition{Name: "The Atelier"}, nil, "The Atelier"},
		{"anything else", Definition{}, nil, "gorepo"},
	} {
		if got := readmeTitle("gorepo", tc.def, tc.api); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}

	repo := apilibRepo(t, true)
	if err := generateReadme(repo.Fs(), "go-api-libs", "gamerpower", true, 100); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, repo.Fs(), readmePath); !strings.Contains(got, "-->\n# 🎮 Free Games API\n\n[![") {
		t.Errorf("the README is not headed by the catalog title:\n%s", got)
	}
}
