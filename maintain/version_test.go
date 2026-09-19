package maintain

import (
	"testing"

	"github.com/spf13/afero"
)

// These are real pseudo-versions of devtool-engine, in the order they were
// published. Invented "v1.2.3" strings would prove nothing: the shape that
// actually runs is a base version plus a timestamp and a commit hash, and the
// hash is what makes this look like it should not work.
const (
	older = "v0.0.0-20260915161737-9e10ae8f485b"
	newer = "v0.0.0-20260917155344-c047bdcfd843"
)

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		want bool
	}{
		{"a later pseudo-version", newer, older, true},
		{"and the other way round", older, newer, false},
		{"the same build", newer, newer, false},

		// Once the module is tagged, a pseudo-version takes the shape
		// "v1.0.1-0.<timestamp>-<hash>" and has to keep ordering.
		{"a tag beats the pseudo-versions below it", "v1.0.0", newer, true},
		{"a pseudo-version above a tag beats it", "v1.0.1-0.20260918091500-abcdef123456", "v1.0.0", true},
		{"two pseudo-versions above the same tag", "v1.0.1-0.20260918091500-abcdef123456", "v1.0.1-0.20260917155344-c047bdcfd843", true},

		// Not knowing is not a reason to complain.
		{"a local build is never behind", devel, newer, false},
		// A "go build" in a dirty worktree carries the commit it came from
		// and "+dirty". semver ignores build metadata when comparing, so
		// without this it would pass for the published build it is not.
		{"a dirty build is not the build it came from", newer + "+dirty", older, false},
		{"and nothing is behind one", older, newer + "+dirty", false},
		{"and never ahead", newer, devel, false},
		{"no version at all", "", newer, false},
		{"nothing recorded", newer, "", false},
		{"something that is not a version", "yesterday's build", newer, false},
		{"against something that is not a version", newer, "yesterday's build", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Newer(tc.a, tc.b); got != tc.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestRecordVersion(t *testing.T) {
	// A build that is behind must be able to read the mark without lowering
	// it, or the first stale run stops everybody else being warned.
	t.Run("an older build leaves a newer mark alone", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, DefinitionPath, `{"devtoolVersion":"`+newer+`"}`+"\n")

		before := readFile(t, fs, DefinitionPath)

		if err := RecordVersion(fs, older); err != nil {
			t.Fatal(err)
		}

		// Byte-identical, not merely equivalent: an unchanged worktree is
		// what tells a commit run there is nothing to commit.
		if got := readFile(t, fs, DefinitionPath); got != before {
			t.Errorf("the file was rewritten:\n%s", got)
		}
	})

	t.Run("a newer build advances it", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, DefinitionPath, `{"devtoolVersion":"`+older+`"}`+"\n")

		if err := RecordVersion(fs, newer); err != nil {
			t.Fatal(err)
		}

		got, err := RecordedVersion(fs)
		if err != nil {
			t.Fatal(err)
		}

		if got != newer {
			t.Errorf("recorded %q, want %q", got, newer)
		}
	})

	t.Run("the same build changes nothing", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, DefinitionPath, `{"devtoolVersion":"`+newer+`"}`+"\n")

		before := readFile(t, fs, DefinitionPath)

		if err := RecordVersion(fs, newer); err != nil {
			t.Fatal(err)
		}

		if got := readFile(t, fs, DefinitionPath); got != before {
			t.Errorf("the file was rewritten:\n%s", got)
		}
	})

	t.Run("a repository with no definition gets one", func(t *testing.T) {
		fs := afero.NewMemMapFs()

		if err := RecordVersion(fs, newer); err != nil {
			t.Fatal(err)
		}

		got, err := RecordedVersion(fs)
		if err != nil {
			t.Fatal(err)
		}

		if got != newer {
			t.Errorf("recorded %q, want %q", got, newer)
		}
	})

	// The definition holds what a repository is, and a version record must
	// not cost it that.
	t.Run("the rest of the definition survives", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		writeFile(t, fs, DefinitionPath,
			`{"description":"A blurb.","topics":["go"],"coverage":96.4,"devtoolVersion":"`+older+`"}`+"\n")

		if err := RecordVersion(fs, newer); err != nil {
			t.Fatal(err)
		}

		def, ok, err := LoadDefinition(fs)
		if err != nil || !ok {
			t.Fatalf("loading: %v, %v", ok, err)
		}

		if def.Description != "A blurb." || len(def.Topics) != 1 || def.Coverage != 96.4 {
			t.Errorf("the definition lost something: %+v", def)
		}
	})

	// A local build is not a published one, so it is nobody's high-water mark.
	t.Run("a local build records nothing", func(t *testing.T) {
		fs := afero.NewMemMapFs()

		if err := RecordVersion(fs, devel); err != nil {
			t.Fatal(err)
		}

		if exists, err := afero.Exists(fs, DefinitionPath); err != nil {
			t.Fatal(err)
		} else if exists {
			t.Errorf("a definition was written for a local build:\n%s",
				readFile(t, fs, DefinitionPath))
		}
	})
}

// TestRecordVersionIgnoresADirtyBuild: the mark is what other machines are
// judged against, so only a build somebody else could install belongs in it.
func TestRecordVersionIgnoresADirtyBuild(t *testing.T) {
	fs := afero.NewMemMapFs()

	if err := RecordVersion(fs, newer+"+dirty"); err != nil {
		t.Fatal(err)
	}

	if exists, err := afero.Exists(fs, DefinitionPath); err != nil {
		t.Fatal(err)
	} else if exists {
		t.Errorf("a dirty build was recorded as the high-water mark:\n%s",
			readFile(t, fs, DefinitionPath))
	}
}

// TestOutdatedReadsALocalBuild is the gap that let a stale build through.
// Newer refuses to reason about a "+dirty" binary, which is right when the
// question is whether to record it and wrong when the question is whether to
// let it write: the commit it came from orders it perfectly well.
func TestOutdatedReadsALocalBuild(t *testing.T) {
	const (
		// The build that reverted this repository's own generated files, and
		// the mark it should have been stopped by.
		dirty    = "v0.0.0-20260918065718-2314b2502699+dirty"
		recorded = "v0.0.0-20260919111501-67d6619ce110"
	)

	if Newer(recorded, dirty) {
		t.Error("Newer should go on exempting a local build, so it never becomes the mark")
	}

	if !Outdated(dirty, recorded) {
		t.Error("a local build a day behind the mark was not reported as outdated")
	}

	if Installable(dirty) {
		t.Error("a local build must not read as something another machine could fetch")
	}

	// And the other direction: a local build ahead of the mark is not behind
	// it, so somebody working on devtool itself is not locked out.
	if Outdated(recorded, dirty) {
		t.Error("a build newer than the mark was reported as outdated")
	}
}

// TestOutdatedSaysNothingWithoutAVersion: not knowing is not a reason to
// refuse. A binary with no version information at all cannot be ordered.
func TestOutdatedSaysNothingWithoutAVersion(t *testing.T) {
	const recorded = "v0.0.0-20260919111501-67d6619ce110"

	for _, current := range []string{"", devel, "not-a-version"} {
		if Outdated(current, recorded) {
			t.Errorf("Outdated(%q, recorded) = true, want false", current)
		}
	}

	if Outdated(recorded, "nonsense") {
		t.Error("an unreadable mark should not stop a run")
	}
}
