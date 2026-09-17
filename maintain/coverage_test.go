package maintain

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// TestRecordCoverageRoundTrips is the property that matters: what is written
// is what the generator reads back, so the badge and the record agree.
func TestRecordCoverageRoundTrips(t *testing.T) {
	fs := afero.NewMemMapFs()

	if err := RecordCoverage(fs, 83.4); err != nil {
		t.Fatal(err)
	}

	if err := generateReadme(fs, "MarkRosemaker", "thing", false, 0); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, fs, readmePath); !strings.Contains(got, "coverage-83%25") {
		t.Errorf("the badge does not carry the recorded figure:\n%s", got)
	}
}

// TestRecordCoverageKeepsTheRest: a measurement must not disturb what a
// repository says about itself.
func TestRecordCoverageKeepsTheRest(t *testing.T) {
	fs := afero.NewMemMapFs()

	if err := SaveDefinition(fs, Definition{
		Description:    "a thing that does things",
		Topics:         []string{"go", "openapi"},
		Coverage:       1,
		DevtoolVersion: "v0.0.0-earlier",
	}); err != nil {
		t.Fatal(err)
	}

	if err := RecordCoverage(fs, 99); err != nil {
		t.Fatal(err)
	}

	def, ok, err := LoadDefinition(fs)
	if err != nil || !ok {
		t.Fatalf("loading: %v, found %v", err, ok)
	}

	if def.Coverage != 99 {
		t.Errorf("Coverage = %v, want 99", def.Coverage)
	}

	if def.Description != "a thing that does things" {
		t.Errorf("the description was lost: %q", def.Description)
	}

	if len(def.Topics) != 2 {
		t.Errorf("the topics were lost: %q", def.Topics)
	}

	if def.DevtoolVersion != "v0.0.0-earlier" {
		t.Errorf("the version was lost: %q", def.DevtoolVersion)
	}
}

// TestMeasuredCoverageBeatsTheRecord: a caller that just ran the tests has the
// newer figure.
func TestMeasuredCoverageBeatsTheRecord(t *testing.T) {
	fs := afero.NewMemMapFs()

	if err := RecordCoverage(fs, 10); err != nil {
		t.Fatal(err)
	}

	if err := generateReadme(fs, "MarkRosemaker", "thing", false, 55); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, fs, readmePath); !strings.Contains(got, "coverage-55%25") {
		t.Errorf("the stored figure overrode a fresh measurement:\n%s", got)
	}
}

// TestCoverageFallsBackToTheBadge covers every repository that predates
// devtool.json: with nothing measured and nothing recorded, a rebuild must not
// reset the badge to zero.
func TestCoverageFallsBackToTheBadge(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, readmePath,
		"![Code Coverage](https://img.shields.io/badge/coverage-77%25-yellowgreen)\n")

	if err := generateReadme(fs, "MarkRosemaker", "thing", false, 0); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, fs, readmePath); !strings.Contains(got, "coverage-77%25") {
		t.Errorf("the badge was reset:\n%s", got)
	}
}

// TestBadgesFragmentJoinsTheRow: badges.md stays a fragment, for the badges a
// repository adds of its own. Only the figure moved out of it.
func TestBadgesFragmentJoinsTheRow(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, "README/badges.md",
		"[![Extra](https://example.com/b.svg)](https://example.com)\n")

	if err := generateReadme(fs, "MarkRosemaker", "thing", false, 42); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, fs, readmePath)
	if !strings.Contains(got, "[![Extra](https://example.com/b.svg)]") {
		t.Errorf("the fragment's own badges are missing:\n%s", got)
	}
}

func TestDefinitionRoundTrips(t *testing.T) {
	fs := afero.NewMemMapFs()

	if _, ok, err := LoadDefinition(fs); err != nil || ok {
		t.Fatalf("a repository with no definition: err %v, found %v", err, ok)
	}

	want := Definition{
		Description: "a thing", Topics: []string{"go"},
		Coverage: 61.5, DevtoolVersion: "v1",
	}

	if err := SaveDefinition(fs, want); err != nil {
		t.Fatal(err)
	}

	got, ok, err := LoadDefinition(fs)
	if err != nil || !ok {
		t.Fatalf("err %v, found %v", err, ok)
	}

	if got.Description != want.Description || got.Coverage != want.Coverage ||
		got.DevtoolVersion != want.DevtoolVersion || len(got.Topics) != 1 {
		t.Errorf("got %+v, want %+v", got, want)
	}

	if b := readFile(t, fs, DefinitionPath); !strings.HasSuffix(b, "\n") {
		t.Error("the file should end in a newline")
	}
}
