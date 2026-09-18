package maintain

import (
	"context"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
	"golang.org/x/mod/semver"
)

// devel is what the toolchain stamps on a binary with no version information
// at all. selfupdate refuses to replace one, and nothing here compares one.
const devel = "(devel)"

// Newer reports whether a is a later build than b.
//
// Both are module versions as the toolchain reports them, which for an
// untagged module is a pseudo-version: "v0.0.0-20260917155344-c047bdcfd843",
// a base version and a prerelease holding a timestamp and the commit. That is
// valid semver, and semver orders it the way it reads — the timestamp is
// fixed-width and leads the one prerelease identifier, so comparing the
// identifiers as text compares the builds by date. The commit hash only ever
// decides a tie within the same second, which one module cannot produce.
//
// Anything unusable is not newer: an empty version, a local build, or a string
// semver does not recognise. Not knowing is not a reason to complain.
func Newer(a, b string) bool {
	if !usable(a) || !usable(b) {
		return false
	}

	return semver.Compare(a, b) > 0
}

// usable reports whether v names a published build this can reason about.
//
// Build metadata disqualifies it, and that is the case that matters in
// practice: a "go build" in a dirty worktree is stamped with the commit it
// came from plus "+dirty", which semver accepts and then ignores when
// comparing. Taken at face value it would record a local, uncommitted build as
// the newest one to have maintained the repository, and every other machine
// would be told it is behind something nobody can install.
func usable(v string) bool {
	return v != "" && v != devel && semver.IsValid(v) && semver.Build(v) == ""
}

// RecordedVersion is the devtool build this repository's definition names, or
// "" where there is no definition or it says nothing.
func RecordedVersion(fs afero.Fs) (string, error) {
	def, ok, err := LoadDefinition(fs)
	if err != nil || !ok {
		return "", err
	}

	return def.DevtoolVersion, nil
}

// RecordVersion raises the repository's recorded devtool build to current,
// leaving it alone where what is recorded is already the same or later.
//
// A high-water mark rather than a stamp of whoever ran last, and that is the
// point: it is what tells a build it is behind, so a stale one must be able to
// read it without lowering it for everybody else.
func RecordVersion(fs afero.Fs, current string) error {
	def, _, err := LoadDefinition(fs)
	if err != nil {
		return err
	}

	if !usable(current) {
		return nil
	}

	// An unusable record is replaced rather than compared against, so a
	// repository carrying nothing or carrying nonsense still gets a mark.
	if usable(def.DevtoolVersion) && !Newer(current, def.DevtoolVersion) {
		return nil
	}

	def.DevtoolVersion = current

	return SaveDefinition(fs, def)
}

// VersionTask returns a task that records the devtool build maintaining this
// repository, so a build that is behind can be told it is.
func VersionTask(repo engine.Repo, current string) engine.Task {
	return engine.Task{
		Name:  "record the devtool version",
		Short: "version",
		Run: func(context.Context) error {
			return RecordVersion(repo.Fs(), current)
		},
	}
}
