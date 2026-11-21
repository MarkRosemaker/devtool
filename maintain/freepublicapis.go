package maintain

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"unicode"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/openapi"
	"github.com/spf13/afero"
	"golang.org/x/mod/modfile"
)

// freePublicAPIsPath is an API library's entry in freepublicapis.com's
// catalog, which says what the API is.
const freePublicAPIsPath = "freepublicapis.json"

// FreePublicAPI is an API as freepublicapis.com's catalog has it.
type FreePublicAPI struct {
	ID            int    `json:"id,omitzero"`
	Emoji         string `json:"emoji,omitzero"`
	Title         string `json:"title,omitzero"`
	Description   string `json:"description,omitzero"`
	Documentation string `json:"documentation,omitzero"`
	Methods       int    `json:"methods,omitzero"`
	Health        int    `json:"health,omitzero"`
	Source        string `json:"source,omitzero"`
}

// LoadFreePublicAPI reads the repository's catalog entry, reporting whether
// it has one.
func LoadFreePublicAPI(fs afero.Fs) (FreePublicAPI, bool, error) {
	var api FreePublicAPI

	b, err := afero.ReadFile(fs, freePublicAPIsPath)
	if errors.Is(err, os.ErrNotExist) {
		return api, false, nil
	} else if err != nil {
		return api, false, err
	}

	if err := json.Unmarshal(b, &api); err != nil {
		return api, false, fmt.Errorf("%s: %w", freePublicAPIsPath, err)
	}

	return api, true, nil
}

// apilibTools are what generate.go runs, as go tool dependencies.
var apilibTools = []string{
	"github.com/MarkRosemaker/openapi-enrich/cmd/openapi-enrich",
	"github.com/MarkRosemaker/openapi-flatten/cmd/openapi-flatten",
	"github.com/MarkRosemaker/openapi-compress/cmd/openapi-compress",
	"github.com/MarkRosemaker/openapi-codegen/cmd/openapi-codegen",
}

// apilibGenerate is the generate.go of an API library: enrich builds the
// specification from the recorded calls, flatten and compress tidy it, and
// codegen writes the client. Split, because go generate reads a directive at
// the start of any line of a source file, string literals included.
const apilibGenerate = goGenerated + "\n\npackage %s\n\n" +
	"//go:" + "generate go tool openapi-enrich -trim-examples=20\n" +
	"//go:" + "generate go tool openapi-flatten\n" +
	"//go:" + "generate go tool openapi-compress -trim-examples=3\n" +
	"//go:" + "generate go tool openapi-codegen -client\n"

// FreePublicAPIsTask lays an API library out from its catalog entry, where
// it is an apilib with a freepublicapis.json:
//
//   - api/openapi.json, started from the entry where there is none, and its
//     title, description and documentation filled in where they are empty.
//     The rest is openapi-enrich's, which builds the specification from the
//     calls api/interactions.json records and rewrites it every make generate,
//     so a value already there is never replaced.
//   - generate.go, which runs openapi-enrich and the tools after it, with the
//     tools as go tool dependencies. One without devtool's marker is the
//     repository's own and is left alone, tools and all.
func FreePublicAPIsTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "freepublicapis",
		Short: "freepublicapis",
		Run: func(ctx context.Context) error {
			return writeFreePublicAPI(ctx, repo)
		},
	}
}

func writeFreePublicAPI(ctx context.Context, repo engine.Repo) error {
	fs := repo.Fs()

	def, _, err := LoadDefinition(fs)
	if err != nil {
		return err
	}

	if def.Kind != KindAPILib {
		return nil
	}

	api, ok, err := LoadFreePublicAPI(fs)
	if err != nil || !ok {
		return err
	}

	if err := fillOpenAPI(fs, api); err != nil {
		return fmt.Errorf("%s: %w", openAPIPath, err)
	}

	return writeAPILibGenerate(ctx, repo)
}

// fillOpenAPI starts the specification from the catalog entry, or fills in
// what of it is empty, writing only where something changed.
func fillOpenAPI(fs afero.Fs, api FreePublicAPI) error {
	doc := &openapi.Document{OpenAPI: "3.1.0"}

	b, err := afero.ReadFile(fs, openAPIPath)

	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if doc, err = openapi.LoadFromDataJSON(b); err != nil {
			return err
		}
	}

	changed := b == nil

	fill := func(field *string, value string) {
		if *field == "" && value != "" {
			*field, changed = value, true
		}
	}

	if doc.Info == nil {
		doc.Info, changed = &openapi.Info{}, true
	}

	fill(&doc.Info.Title, api.Title)
	fill(&doc.Info.Description, api.Description)
	fill(&doc.Info.Version, "0.0.1")

	if doc.ExternalDocs == nil && api.Documentation != "" {
		u, err := url.Parse(api.Documentation)
		if err != nil {
			return fmt.Errorf("the documentation link %q: %w", api.Documentation, err)
		}

		doc.ExternalDocs, changed = &openapi.ExternalDocs{Description: "API documentation", URL: u}, true
	}

	if ops := countOperations(doc); ops < api.Methods {
		slog.Warn("the specification has fewer operations than freepublicapis.com counts methods: "+
			"record a call to each in api/interactions.json",
			"operations", ops, "methods", api.Methods)
	}

	if !changed {
		return nil
	}

	out, err := doc.ToJSON()
	if err != nil {
		return err
	}

	if err := fs.MkdirAll(path.Dir(openAPIPath), 0o755); err != nil {
		return err
	}

	return afero.WriteFile(fs, openAPIPath, out, 0o644)
}

// countOperations is how many operations the specification describes.
func countOperations(doc *openapi.Document) int {
	n := 0

	for _, item := range doc.Paths {
		for range item.Operations {
			n++
		}
	}

	return n
}

// writeAPILibGenerate writes generate.go where there is none or it is
// devtool's, and adds the tools it runs to go.mod where they are missing.
func writeAPILibGenerate(ctx context.Context, repo engine.Repo) error {
	fs := repo.Fs()

	existing, err := afero.ReadFile(fs, "generate.go")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if existing != nil && !strings.HasPrefix(string(existing), goGenerated) {
		return nil
	}

	module, err := modulePath(fs)
	if err != nil {
		return err
	}

	if err := afero.WriteFile(fs, "generate.go", fmt.Appendf(nil, apilibGenerate, packageName(module)), 0o644); err != nil {
		return err
	}

	missing, err := missingTools(fs, apilibTools)
	if err != nil || len(missing) == 0 {
		return err
	}

	for _, tool := range missing {
		if out, err := repo.ExecCommand(ctx, "go", "get", "-tool", tool); err != nil {
			return fmt.Errorf("go get -tool %s: %w\n%s", tool, err, strings.TrimSpace(string(out)))
		}
	}

	return tidyModules(ctx, repo)
}

// missingTools are those of tools go.mod has no tool directive for.
func missingTools(fs afero.Fs, tools []string) ([]string, error) {
	b, err := afero.ReadFile(fs, "go.mod")
	if err != nil {
		return nil, err
	}

	// Strict: the lax parse skips tool directives.
	mod, err := modfile.Parse("go.mod", b, nil)
	if err != nil {
		return nil, err
	}

	var missing []string

	for _, t := range tools {
		if !slices.ContainsFunc(mod.Tool, func(m *modfile.Tool) bool { return m.Path == t }) {
			missing = append(missing, t)
		}
	}

	return missing, nil
}

// packageName is the Go package name a module path's last element makes:
// lower-case letters and digits, as go-api-libs/sec-edgar is secedgar.
func packageName(module string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}

		return -1
	}, path.Base(module))

	if name == "" || unicode.IsDigit(rune(name[0])) {
		name = "api" + name
	}

	return name
}
