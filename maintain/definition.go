package maintain

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/afero"
)

// DefinitionPath is where a repository says what it is, in its own root.
//
// It is the repository's, not the portfolio's: a repository knows its own
// description, the topics it should carry and the coverage its tests last
// reported, and saying so there means the list of repositories to maintain can
// go back to being only a list.
const DefinitionPath = "devtool.json"

// Definition is what a repository says about itself.
//
// Facts, not presentation. What a README looks like — its tagline, its logo,
// the badges it adds of its own — is frontmatter in README/, where a person
// editing the page can see it. What the repository *is* belongs here, where
// something that never renders a page can read it.
type Definition struct {
	// Description and Topics are pushed to the repository's GitHub metadata.
	Description string   `json:"description,omitempty"`
	Topics      []string `json:"topics,omitempty"`

	// Coverage is what the tests last reported, as a percentage. Written by
	// "devtool test" and by a maintained run; read by whatever renders the
	// badge.
	Coverage float64 `json:"coverage,omitempty"`

	// DevtoolVersion is the build that last maintained this repository, so a
	// repository can say which generator produced what it holds.
	DevtoolVersion string `json:"devtoolVersion,omitempty"`

	// Kind is what sort of repository this is. Inferred once where the file
	// has none, and authoritative from then on: a wrong guess is fixed by
	// editing this line, not by rearranging directories.
	Kind Kind `json:"kind,omitempty"`

	// Private is the repository's visibility, as GitHub last reported it to a
	// maintained run. A local run has no GitHub to ask, so it reads it here.
	Private bool `json:"private,omitzero"`

	// Resources are what the repository's code is built around, and devtool
	// generates the code that goes with them.
	Resources Resources `json:"resources,omitzero"`
}

// Resources are a repository's declared resources.
type Resources struct {
	// Databases are laid out as described in [DatabasesTask].
	Databases []Database `json:"databases,omitempty"`
}

// Database is one SQLite database, which lives in databases/<Name>.
type Database struct {
	Name string `json:"name"`
}

// equal reports whether two definitions say the same thing.
func (d Definition) equal(o Definition) bool {
	return d.Description == o.Description &&
		slices.Equal(d.Topics, o.Topics) &&
		d.Coverage == o.Coverage &&
		d.DevtoolVersion == o.DevtoolVersion &&
		d.Kind == o.Kind &&
		d.Private == o.Private &&
		slices.Equal(d.Resources.Databases, o.Resources.Databases)
}

// LoadDefinition reads the repository's definition.
//
// A repository that has none is not an error: every repository predates this
// file, and one is written the first time something has a value to record.
func LoadDefinition(fs afero.Fs) (Definition, bool, error) {
	b, err := afero.ReadFile(fs, DefinitionPath)
	if errors.Is(err, os.ErrNotExist) {
		return Definition{}, false, nil
	} else if err != nil {
		return Definition{}, false, fmt.Errorf("reading %s: %w", DefinitionPath, err)
	}

	var def Definition
	if err := json.Unmarshal(b, &def); err != nil {
		return Definition{}, false, fmt.Errorf("%s: %w", DefinitionPath, err)
	}

	if def.Kind != "" && !slices.Contains(kinds, def.Kind) {
		return Definition{}, false, fmt.Errorf("%s: unknown kind %q, want one of %s",
			DefinitionPath, def.Kind, kindList())
	}

	return def, true, nil
}

// SaveDefinition writes it back, formatted so a run's diff is one line per
// field that moved.
func SaveDefinition(fs afero.Fs, def Definition) error {
	b, err := json.Marshal(def, jsontext.Multiline(true), jsontext.WithIndent("\t"))
	if err != nil {
		return fmt.Errorf("%s: %w", DefinitionPath, err)
	}

	if !strings.HasSuffix(string(b), "\n") {
		b = append(b, '\n')
	}

	if err := afero.WriteFile(fs, DefinitionPath, b, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", DefinitionPath, err)
	}

	return nil
}
