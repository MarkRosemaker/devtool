package maintain

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestSetCoverage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing string
		want     string
	}{
		{
			name: "no file at all",
			want: "---\ncoverage: 83\n---\n",
		},
		{
			name:     "frontmatter without the key",
			existing: "---\ntagline: a thing\n---\n\nsome badges\n",
			want:     "---\ntagline: a thing\ncoverage: 83\n---\n\nsome badges\n",
		},
		{
			name:     "frontmatter that already says",
			existing: "---\ncoverage: 12\ntagline: a thing\n---\n\nsome badges\n",
			want:     "---\ncoverage: 83\ntagline: a thing\n---\n\nsome badges\n",
		},
		{
			name:     "a body with no frontmatter",
			existing: "some badges\n",
			want:     "---\ncoverage: 83\n---\n\nsome badges\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := setCoverage([]byte(tc.existing), 83.4)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

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

	got := readFile(t, fs, readmePath)
	if !strings.Contains(got, "coverage-83%25") {
		t.Errorf("the badge does not carry the recorded figure:\n%s", got)
	}
}

// TestRecordCoverageKeepsWhatItDoesNotKnow: the frontmatter may hold keys this
// version has never heard of, and a measurement must not drop them.
func TestRecordCoverageKeepsWhatItDoesNotKnow(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, "README/badges.md",
		"---\nsomethingNew: keep me\ncoverage: 1\n---\n\n![extra](x)\n")

	if err := RecordCoverage(fs, 99); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, fs, "README/badges.md")

	for _, want := range []string{"somethingNew: keep me", "coverage: 99", "![extra](x)"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is gone:\n%s", want, got)
		}
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

// TestBadgesFragmentJoinsTheRow: badges.md is a fragment like any other, and
// whatever it holds beyond its frontmatter belongs on the badge row.
func TestBadgesFragmentJoinsTheRow(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, "README/badges.md",
		"---\ncoverage: 42\n---\n\n[![Extra](https://example.com/b.svg)](https://example.com)\n")

	if err := generateReadme(fs, "MarkRosemaker", "thing", false, 0); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, fs, readmePath)
	if !strings.Contains(got, "coverage-42%25") {
		t.Errorf("the recorded figure is missing:\n%s", got)
	}

	if !strings.Contains(got, "[![Extra](https://example.com/b.svg)]") {
		t.Errorf("the fragment's own badges are missing:\n%s", got)
	}
}
