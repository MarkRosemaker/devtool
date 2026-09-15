package maintain

import "testing"

func TestInert(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		// What these generators write.
		{"README.md", true},
		{"Makefile", true},
		{"AGENTS.md", true},
		{"CLAUDE.md", true},
		{"LICENSE", true},
		{".gitignore", true},
		{".golangci.yaml", true},
		{"devtool.json", true},
		{"./README.md", true},

		// What they read to write it.
		{"README/description.md", true},
		{"AGENTS/conventions.md", true},
		{"mk/extra.mk", true},

		// Anything that is or could reach the build.
		{"thing.go", false},
		{"thing_test.go", false},
		{"go.mod", false},
		{"go.sum", false},
		{"maintain/README.md.tmpl", false},
		{"testdata/README.full.golden", false},
		{"assets/banner.png", false},
		{"internal/thing/README.md", false},

		// A directory whose name merely starts the same way is not the one.
		{"READMEs/other.md", false},
		{"mkfile.go", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := Inert(tc.path); got != tc.want {
				t.Errorf("Inert(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestInertNamesOnlyWhatIsWritten keeps the list honest: every file the
// generators write has to be in it, or a run tests over its own output for
// nothing.
func TestInertNamesOnlyWhatIsWritten(t *testing.T) {
	for _, p := range []string{
		readmePath, makefilePath, agentsPath, claudePath,
		licensePath, gitignorePath, lintfilePath, DefinitionPath,
	} {
		if !Inert(p) {
			t.Errorf("%s is written by these generators but not called inert", p)
		}
	}
}
