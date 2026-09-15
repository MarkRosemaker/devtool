package main

import (
	"runtime/debug"
	"strings"
)

// name is what this binary calls itself, in its version line and wherever a
// message has to say which tool is speaking.
const name = "devtool"

// modulePath is what "go install" would be given to install this binary, and
// so what self-update reinstalls. It is the module path rather than anything
// derived, because a binary that guesses where it came from can guess wrong.
const modulePath = "github.com/MarkRosemaker/devtool"

// shortRevLen is how much of a commit hash is worth printing: enough to look
// one up, not so much that it crowds the line.
const shortRevLen = 12

// buildVersion describes this binary, from what the Go toolchain stamped into
// it when it was built.
//
// Read rather than generated. A version written into a file has to be kept in
// step with reality by whoever remembers to, and the failure this exists to
// diagnose is precisely a binary that is not what its source says.
//
// What comes out, for each way this gets built:
//
//	go install …@v1.2.3    devtool v1.2.3 go1.27.0
//	go build, at a tag     devtool v1.2.3 ba90a3303e96 go1.27.0
//	go build, in between   devtool v0.0.0-20260914185320-1720b3baf577+dirty go1.27.0
//
// The revision is printed only where the version does not already carry it. A
// build between tags gets a pseudo-version with the commit in it, so repeating
// the commit would only make the line longer; a build at a tag gets the bare
// tag, where naming the commit is the difference between a released binary and
// somebody's local one.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return name + " (built without build information)"
	}

	var revision string

	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			revision = s.Value
		}
	}

	return formatVersion(info.Main.Version, revision, info.GoVersion)
}

// formatVersion is [buildVersion] without the reading, so the three cases in
// that comment are checked rather than asserted.
func formatVersion(version, revision, goVersion string) string {
	parts := []string{name, version}

	if rev := revision[:min(len(revision), shortRevLen)]; rev != "" &&
		!strings.Contains(version, rev) {
		parts = append(parts, rev)
	}

	// The Go version too: a tool built with an older Go than the module it
	// analyses refuses to run at all, which has cost an afternoon before.
	return strings.Join(append(parts, goVersion), " ")
}
