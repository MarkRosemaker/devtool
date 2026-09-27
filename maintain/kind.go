package maintain

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/spf13/afero"
)

// Kind is what sort of repository this is.
type Kind string

const (
	// KindLibrary is a Go package and nothing more.
	KindLibrary Kind = "library"

	// KindCLI is a command, whether or not it is also a library.
	KindCLI Kind = "cli"

	// KindWebapp is a Go backend with a frontend designed in Claude Design.
	KindWebapp Kind = "webapp"

	// KindAPILib is an API client library under go-api-libs.
	KindAPILib Kind = "apilib"
)

var kinds = []Kind{KindLibrary, KindCLI, KindWebapp, KindAPILib}

func kindList() string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}

	return strings.Join(names, ", ")
}

// apiLibOwner is the organisation every API library lives under.
const apiLibOwner = "go-api-libs"

// inferKind guesses what a repository is from what it holds, for a
// repository whose definition does not say yet. It runs once: the answer is
// recorded, and the record wins from then on.
//
// The owner first, because an API library may well have a cmd/; then
// frontend/, because a web app has a command to serve it too. A command is
// cmd/ or a main package at the root, which is where devtool keeps its own.
func inferKind(fs afero.Fs, owner string) (Kind, error) {
	if owner == apiLibOwner {
		return KindAPILib, nil
	}

	for _, c := range []struct {
		dir  string
		kind Kind
	}{
		{"frontend", KindWebapp},
		{"cmd", KindCLI},
	} {
		info, err := fs.Stat(c.dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("checking for %s/: %w", c.dir, err)
		}

		if info.IsDir() {
			return c.kind, nil
		}
	}

	if isMain, err := rootIsMain(fs); err != nil {
		return "", err
	} else if isMain {
		return KindCLI, nil
	}

	return KindLibrary, nil
}

// rootIsMain reports whether the root package is a command.
func rootIsMain(fs afero.Fs) (bool, error) {
	entries, err := afero.ReadDir(fs, ".")
	if err != nil {
		return false, fmt.Errorf("reading the root directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		b, err := afero.ReadFile(fs, name)
		if err != nil {
			return false, fmt.Errorf("%s: %w", name, err)
		}

		f, err := parser.ParseFile(token.NewFileSet(), name, b, parser.PackageClauseOnly)
		if err != nil {
			continue
		}

		return f.Name.Name == "main", nil
	}

	return false, nil
}
