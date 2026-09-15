package local

import (
	"testing"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

// TestKeepCoverage: a local rebuild does not run the tests, so writing zero
// would quietly downgrade the badge of every repository somebody ran it in.
func TestKeepCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		readme string
		want   float64
		absent bool
	}{
		{
			name:   "reads what the badge claims",
			readme: "![Code Coverage](https://img.shields.io/badge/coverage-80%25-yellow)\n",
			want:   80,
		},
		{
			name:   "a fractional figure",
			readme: "![Code Coverage](https://img.shields.io/badge/coverage-34.7%25-orange)\n",
			want:   34.7,
		},
		{
			name:   "no badge at all",
			readme: "# thing\n",
			want:   0,
		},
		{name: "no README at all", absent: true, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if !tc.absent {
				if err := afero.WriteFile(fs, "README.md", []byte(tc.readme), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if got := keepCoverage(fs); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUpdateRejectsSomethingThatIsNotARepository(t *testing.T) {
	err := Update(t.Context(), t.TempDir(), Options{}, nopEmitter{})
	if err == nil {
		t.Fatal("a directory that is not a repository was accepted")
	}
}

type nopEmitter struct{}

func (nopEmitter) Emit(engine.Event) {}
