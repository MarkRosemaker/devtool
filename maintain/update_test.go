package maintain

import (
	"slices"
	"testing"
)

// TestDescribeUpdate: one commit, reported as the parts that made it, in the
// order they run.
func TestDescribeUpdate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  []string
	}{
		{"coverage moved", []string{"README.md", "devtool.json"}, []string{"definition", "readme"}},
		{
			"everything, out of order",
			[]string{".golangci.yaml", "CLAUDE.md", "AGENTS.md", ".gitignore", "Makefile", "LICENSE"},
			[]string{"license", "makefile", "gitignore", "agents", "claude", "lintgen"},
		},
		{
			"adopted into fragments",
			[]string{"mk/legacy.mk", "AGENTS/rules.md", "README/badges.md"},
			[]string{"readme", "makefile", "agents"},
		},
		// A name that only starts like a directory is not in it.
		{"not a fragment", []string{"README.md.orig"}, []string{"update"}},
		{"something unclaimed", []string{"README.md", "main.go"}, []string{"readme", "update"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeUpdate(tc.files); !slices.Equal(got, tc.want) {
				t.Errorf("describeUpdate(%v) = %v, want %v", tc.files, got, tc.want)
			}
		})
	}
}

// TestUpdateTaskDescribesItself: the runner only asks a task that says it can.
func TestUpdateTaskDescribesItself(t *testing.T) {
	if UpdateTask(nil, UpdateOptions{}).Describe == nil {
		t.Error("UpdateTask does not describe its commits")
	}
}
