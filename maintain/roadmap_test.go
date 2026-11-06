package maintain

import (
	"testing"

	"github.com/spf13/afero"
)

func TestLinkRoadmap(t *testing.T) {
	for _, tc := range []struct {
		name    string
		roadmap string // "" for no docs/roadmap.md
		files   map[string]string
		want    string // "" for no docs/roadmap.md afterwards
	}{
		{
			name:    "no docs/roadmap/ leaves the roadmap alone",
			roadmap: "# Roadmap\n\nWork.\n",
			want:    "# Roadmap\n\nWork.\n",
		},
		{
			name: "no docs/roadmap/ and no roadmap writes nothing",
		},
		{
			name:    "files not linked go under the title, named by their heading",
			roadmap: "# Roadmap\n\n## Coverage as a gate\n\nWork.\n",
			files: map[string]string{
				"docs/roadmap/foo.md": "# Postgres everywhere\n\nMore.\n",
				"docs/roadmap/bar.md": "Nothing but prose.\n",
			},
			want: "# Roadmap\n\nSee also:\n- [bar](roadmap/bar.md)\n- [Postgres everywhere](roadmap/foo.md)\n\n" +
				"## Coverage as a gate\n\nWork.\n",
		},
		{
			name:    "a file linked anywhere, either way round, is not added again",
			roadmap: "# Roadmap\n\nAs [foo](roadmap/foo.md#later) says, and [bar](docs/roadmap/bar.md).\n",
			files: map[string]string{
				"docs/roadmap/foo.md": "# Foo\n",
				"docs/roadmap/bar.md": "# Bar\n",
			},
			want: "# Roadmap\n\nAs [foo](roadmap/foo.md#later) says, and [bar](docs/roadmap/bar.md).\n",
		},
		{
			name:    "a new file joins the list, and one whose file has gone leaves it",
			roadmap: "# Roadmap\n\nSee also:\n- [Gone](roadmap/gone.md)\n- [Kept](roadmap/kept.md)\n\nWork.\n",
			files: map[string]string{
				"docs/roadmap/kept.md": "# Kept\n",
				"docs/roadmap/new.md":  "# New\n",
			},
			want: "# Roadmap\n\nSee also:\n- [Kept](roadmap/kept.md)\n- [New](roadmap/new.md)\n\nWork.\n",
		},
		{
			name:    "the list goes once every file it named has gone",
			roadmap: "# Roadmap\n\nSee also:\n- [Gone](roadmap/gone.md)\n\nWork.\n",
			want:    "# Roadmap\n\nWork.\n",
		},
		{
			name:  "a roadmap without a title gets the list at its top",
			files: map[string]string{"docs/roadmap/foo.md": "# Foo\n"},
			want:  "# Roadmap\n\nSee also:\n- [Foo](roadmap/foo.md)\n",
		},
		{
			name:    "an untitled roadmap gets the list first",
			roadmap: "Work.\n",
			files:   map[string]string{"docs/roadmap/sub/foo.md": "## Foo\n"},
			want:    "See also:\n- [Foo](roadmap/sub/foo.md)\n\nWork.\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if tc.roadmap != "" {
				writeFile(t, fs, roadmapPath, tc.roadmap)
			}

			for p, body := range tc.files {
				writeFile(t, fs, p, body)
			}

			if err := linkRoadmap(fs); err != nil {
				t.Fatal(err)
			}

			if tc.want == "" {
				if ok, _ := afero.Exists(fs, roadmapPath); ok {
					t.Errorf("wrote %s:\n%s", roadmapPath, readFile(t, fs, roadmapPath))
				}

				return
			}

			got := readFile(t, fs, roadmapPath)
			if got != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}

			// A second run has nothing to add.
			if err := linkRoadmap(fs); err != nil {
				t.Fatal(err)
			}

			if again := readFile(t, fs, roadmapPath); again != got {
				t.Errorf("a second run changed it:\n%s", again)
			}
		})
	}
}
