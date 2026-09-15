package maintain

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

// badgesName is the fragment holding the badge row's own frontmatter, and so
// where a repository records the coverage it last measured.
const badgesName = "badges.md"

// coverageLine matches the recorded figure in that frontmatter.
var coverageLine = regexp.MustCompile(`(?m)^coverage:[^\n]*$`)

// RecordCoverage writes pct into README/badges.md as the repository's own
// figure, creating the fragment and its frontmatter when they are not there.
//
// Edited as text rather than marshalled from a struct: the frontmatter may
// hold keys this version does not know about, and a round trip through a typed
// value would drop them and reorder what it kept.
func RecordCoverage(fs afero.Fs, pct float64) error {
	p := path.Join(readmeDir, badgesName)

	existing, err := afero.ReadFile(fs, p)
	if err != nil {
		existing = nil
	}

	updated, err := setCoverage(existing, pct)
	if err != nil {
		return err
	}

	if err := fs.MkdirAll(readmeDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", readmeDir, err)
	}

	if err := afero.WriteFile(fs, p, updated, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", p, err)
	}

	return nil
}

// setCoverage is [RecordCoverage]'s edit, so the three shapes it has to handle
// are testable without a filesystem: no file, a file without frontmatter, and
// a file whose frontmatter may or may not already say.
func setCoverage(existing []byte, pct float64) ([]byte, error) {
	line := "coverage: " + strconv.Itoa(int(pct))

	front, body := splitFrontmatter(existing)

	if front == nil {
		// No frontmatter, and possibly no file: the whole body is kept
		// underneath a block that now exists.
		text := strings.TrimLeft(string(existing), "\n")

		if text == "" {
			return fmt.Appendf(nil, "---\n%s\n---\n", line), nil
		}

		return fmt.Appendf(nil, "---\n%s\n---\n\n%s", line, text), nil
	}

	f := strings.TrimRight(string(front), "\n")

	if coverageLine.MatchString(f) {
		f = coverageLine.ReplaceAllString(f, line)
	} else {
		f += "\n" + line
	}

	rest := strings.TrimLeft(string(body), "\n")
	if rest == "" {
		return fmt.Appendf(nil, "---\n%s\n---\n", f), nil
	}

	return fmt.Appendf(nil, "---\n%s\n---\n\n%s", f, rest), nil
}
