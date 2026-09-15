package maintain

import (
	"path"
	"strings"
)

// lintfilePath is what the engine's GenLintfile writes, named here because
// this package hands it the settings and so knows where they land.
const lintfilePath = ".golangci.yaml"

// generatedFiles are the files these generators write. A change to one of them
// is a change this tool made, and none of them is read by a Go program at run
// time or by "go test".
var generatedFiles = map[string]bool{
	readmePath:    true,
	makefilePath:  true,
	agentsPath:    true,
	claudePath:    true,
	licensePath:   true,
	gitignorePath: true,
	lintfilePath:  true,
}

// generatedDirs hold the fragments the generators read to write those files.
// Prose and make snippets: the Go build never sees them.
var generatedDirs = []string{readmeDir, agentsDir, makefileDir}

// Inert reports that a change to this path cannot alter what a repository's
// tests do, so the engine need not test before committing it or measure
// coverage again afterwards.
//
// It names only what these generators write and read. Anything else — a Go
// file, an asset, a testdata fixture, a file nobody here has an opinion about
// — is assumed to matter, because it could be embedded and being wrong the
// other way means pushing something untested.
//
// A repository that embeds one of these in its own binary would be misjudged.
// That is why the engine takes this as a parameter rather than assuming it:
// somebody else's devtool says what its own generators write.
func Inert(p string) bool {
	p = path.Clean(strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "./"))

	if generatedFiles[p] {
		return true
	}

	for _, dir := range generatedDirs {
		if strings.HasPrefix(p, dir+"/") {
			return true
		}
	}

	return false
}
