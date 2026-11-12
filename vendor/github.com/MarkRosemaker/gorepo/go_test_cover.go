package gorepo

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/MarkRosemaker/ghrepo"
)

var reCover = regexp.MustCompile(`^total:\t+\(statements\)\t+([0-9]+.[0-9])%$`)

func (r *Repository) GoTestCover(ctx context.Context) (float64, error) {
	const coverFile = "cover.out"
	defer func() { _ = r.Remove(coverFile) }() // always clean up, even on early errors

	// run go test with coverage
	if _, err := r.ExecCommand(
		ctx, "go", "test", "./...",
		"-race", // enable race detection
		// coverage of every package a test reaches, not only its own, written to cover.out
		"-cover", "-covermode=atomic", "-coverpkg=./...", "-coverprofile="+coverFile,
		"-shuffle=on", // shuffle tests
	); err != nil {
		const noTestPackagesMsg = "go: warning: \"./...\" matched no packages\nno packages to test"

		execErr, ok := errors.AsType[ghrepo.ExecError](err)
		if !ok {
			return 0, err
		}

		if execErr.Out == noTestPackagesMsg {
			return 0, nil
		}

		execErr.Out = failures(execErr.Out)

		return 0, execErr
	}

	// get coverage from the go test
	out, err := r.ExecCommand(ctx, "go", "tool", "cover", "-func="+coverFile)
	if err != nil {
		return 0, fmt.Errorf("getting go test coverage: %w", err)
	}

	sc := bufio.NewScanner(bytes.NewReader(out))

	// scan until we find the total coverage line
	totalLine, err := getTotalLine(sc)
	if err != nil {
		return 0, err
	}

	match := reCover.FindStringSubmatch(totalLine)
	if match == nil {
		return 0, fmt.Errorf("failed to parse go coverage line %q", totalLine)
	}

	coverage, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parsing coverage %q", match[1])
	}

	return coverage, nil
}

func getTotalLine(sc *bufio.Scanner) (string, error) {
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "total:") {
			return sc.Text(), nil
		}
	}

	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("scanning coverage output: %w", err)
	}

	return "", errors.New("no 'total:' line found in coverage report")
}

// failures is go test's output without what passed, so a failure reads as only
// what failed.
func failures(out string) string {
	var kept []string

	for line := range strings.Lines(out) {
		line = strings.TrimSuffix(line, "\n")
		if !passed(line) {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n")
}

// passed reports whether line is go test's word on a package with nothing to
// look at: one that passed, has no tests, or only states its coverage.
func passed(line string) bool {
	return strings.HasPrefix(line, "ok  \t") ||
		strings.HasPrefix(line, "?   \t") ||
		strings.HasPrefix(line, "coverage: ") ||
		strings.HasPrefix(line, "\t") && strings.Contains(line, "\tcoverage: ")
}
