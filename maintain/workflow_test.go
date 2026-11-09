package maintain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// TestWorkflowGolden renders the workflow against a golden file, so a change to
// it is a reviewable diff. Run "go test ./maintain -update" to rewrite it.
func TestWorkflowGolden(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, "vendor/modules.txt", "")
	writeFile(t, fs, DefinitionPath, `{"tools": {"golangci-lint": "v2.15.0"}}`)

	if err := generateWorkflow(fs); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, fs, workflowPath)
	golden := filepath.Join("testdata", "test.yml.golden")

	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run: go test ./maintain -update)", err)
	}

	if got != string(want) {
		t.Errorf("%s does not match:\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}

// A runner reads no private module, so a repository that does not vendor gets
// no workflow, and loses the one it had once it stops.
func TestWorkflowNeedsVendoring(t *testing.T) {
	fs := afero.NewMemMapFs()

	if err := generateWorkflow(fs); err != nil {
		t.Fatal(err)
	}

	if ok, _ := afero.Exists(fs, workflowPath); ok {
		t.Fatal("a workflow was written without vendor/")
	}

	writeFile(t, fs, workflowPath, "# "+generatedBy+" — add a workflow of your own beside it instead.\n")

	if err := generateWorkflow(fs); err != nil {
		t.Fatal(err)
	}

	if ok, _ := afero.Exists(fs, workflowPath); ok {
		t.Error("the generated workflow stayed after vendor/ went")
	}
}

func TestWorkflowLeavesAHandWrittenOneAlone(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, "vendor/modules.txt", "")
	writeFile(t, fs, workflowPath, "name: Test\n")

	if err := generateWorkflow(fs); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, fs, workflowPath); got != "name: Test\n" {
		t.Errorf("a hand-written workflow was rewritten:\n%s", got)
	}
}
