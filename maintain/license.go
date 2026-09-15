package maintain

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const licensePath = "LICENSE"

// licenseText is the Apache 2.0 text with the copyright line left open. The
// holder and years are the only parts that vary, so the rest is stored verbatim
// rather than assembled: a licence that does not match the canonical wording is
// one pkg.go.dev may decline to recognise.
//
//go:embed LICENSE.tmpl
var licenseText string

var licenseTmpl = template.Must(template.New(licensePath).Parse(licenseText))

// LicenseTask returns a task that gives the repository the Apache 2.0 licence,
// naming holder in the copyright line with the years running from the
// repository's first commit to now.
//
// Private repositories are skipped. A licence is what lets other people use the
// code, so it only means anything once they can see it — and a repository GitHub
// has told us nothing about reads as private, so an unknown one is left alone
// rather than published to.
func LicenseTask(repo engine.Repo, holder string) engine.Task {
	return engine.Task{
		Name:  "license",
		Short: "license",
		Run: func(ctx context.Context) error {
			if repo.Private() {
				return nil
			}

			years, err := copyrightYears(ctx, repo)
			if err != nil {
				return fmt.Errorf("determining copyright years: %w", err)
			}

			buf := &bytes.Buffer{}
			data := struct{ Years, Holder string }{years, holder}

			if err := licenseTmpl.Execute(buf, data); err != nil {
				return fmt.Errorf("rendering licence: %w", err)
			}

			// Writing it unconditionally is what makes the task idempotent: an
			// unchanged file leaves the worktree clean, so no commit follows.
			return afero.WriteFile(repo.Fs(), licensePath, buf.Bytes(), 0o644)
		},
	}
}

// copyrightYears returns the range covered by the repository's commits:
// "2025-2026" while it spans years, or a single "2026" while it does not.
func copyrightYears(ctx context.Context, repo engine.Repo) (string, error) {
	first, err := firstCommitYear(ctx, repo)
	if err != nil {
		return "", err
	}

	return yearRange(first, time.Now().Year()), nil
}

// yearRange renders the copyright years: a range while the work spans more than
// one year, a single year while it does not. A first year later than the current
// one means a misdated commit, and is treated as the current year rather than
// rendered as a range running backwards.
func yearRange(first, now int) string {
	if first >= now {
		return strconv.Itoa(now)
	}

	return fmt.Sprintf("%d-%d", first, now)
}

// firstCommitYear reads the year of the repository's earliest commit.
//
// --max-parents=0 selects root commits, of which there is normally one; a
// repository that grew by merging separate histories has several, and the
// earliest of them is the one that dates the work.
func firstCommitYear(ctx context.Context, repo engine.Repo) (int, error) {
	out, err := repo.ExecCommand(ctx, "git", "log",
		"--max-parents=0", "--format=%ad", "--date=format:%Y")
	if err != nil {
		return 0, err
	}

	return earliestYear(out)
}

// earliestYear picks the smallest year out of git's output, one year per line.
func earliestYear(out []byte) (int, error) {
	earliest := 0

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		year, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			continue
		}

		if earliest == 0 || year < earliest {
			earliest = year
		}
	}

	if earliest == 0 {
		return 0, fmt.Errorf("no root commit found")
	}

	return earliest, nil
}
