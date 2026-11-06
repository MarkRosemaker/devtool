package maintain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const (
	roadmapPath = "docs/roadmap.md"

	// roadmapDir holds the roadmap's larger items, a file each, which
	// docs/roadmap.md links to from its head.
	roadmapDir = "docs/roadmap"

	roadmapSeeAlso = "See also:"
)

var (
	// markdownLink matches an inline link's target.
	markdownLink = regexp.MustCompile(`\]\(([^)\s]+)`)

	// markdownHeading matches a heading of any level.
	markdownHeading = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*#*\s*$`)

	// seeAlsoItem matches one line of the See also list.
	seeAlsoItem = regexp.MustCompile(`^- \[.*\]\(([^)\s]+)\)\s*$`)
)

// RoadmapTask returns a task that links every file in docs/roadmap/ from the
// head of docs/roadmap.md, so the roadmap is read whole from one place.
//
// It adds only the links the file lacks, under "See also:" after its title,
// each named by the linked file's first heading, and drops a link of that
// list whose file has gone. The rest of the roadmap is the owner's.
func RoadmapTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "link the roadmap's files",
		Short: "roadmap",
		Run: func(context.Context) error {
			return linkRoadmap(repo.Fs())
		},
	}
}

// roadmapLink is one item of the See also list.
type roadmapLink struct {
	title, target string
}

func (l roadmapLink) String() string { return "- [" + l.title + "](" + l.target + ")" }

// linkRoadmap is [RoadmapTask]'s work, factored out so a test can hand it a
// filesystem of its own.
func linkRoadmap(fs afero.Fs) error {
	files, err := roadmapFiles(fs)
	if err != nil {
		return err
	}

	b, err := afero.ReadFile(fs, roadmapPath)
	if errors.Is(err, os.ErrNotExist) {
		if len(files) == 0 {
			return nil
		}

		b = []byte("# Roadmap\n")
	} else if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")

	// The title stays first, and the list goes under it.
	head := 0
	for head < len(lines) && strings.TrimSpace(lines[head]) == "" {
		head++
	}

	if head < len(lines) && strings.HasPrefix(lines[head], "# ") {
		head++
	} else {
		head = 0
	}

	start := head
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}

	var kept []string

	end := start

	if start < len(lines) && strings.TrimSpace(lines[start]) == roadmapSeeAlso {
		end = start + 1

		for ; end < len(lines); end++ {
			m := seeAlsoItem.FindStringSubmatch(lines[end])
			if m == nil {
				break
			}

			// A link into docs/roadmap/ whose file has gone goes with it.
			if file, ok := roadmapTarget(m[1]); ok && !slices.Contains(files, file) {
				continue
			}

			kept = append(kept, lines[end])
		}
	}

	linked := map[string]bool{}

	for _, m := range markdownLink.FindAllStringSubmatch(string(b), -1) {
		if file, ok := roadmapTarget(m[1]); ok {
			linked[file] = true
		}
	}

	var added []string

	for _, file := range files {
		if linked[file] {
			continue
		}

		title, err := roadmapTitle(fs, file)
		if err != nil {
			return err
		}

		added = append(added, roadmapLink{title, strings.TrimPrefix(file, "docs/")}.String())
	}

	removed := end > start && len(kept) < end-start-1
	if len(added) == 0 && !removed {
		return nil
	}

	items := append(kept, added...)

	out := slices.Clone(lines[:head])
	if head > 0 {
		out = append(out, "")
	}

	if len(items) > 0 {
		out = append(out, roadmapSeeAlso)
		out = append(out, items...)
	}

	rest := lines[end:]
	for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}

	if len(rest) > 0 {
		if len(items) > 0 {
			out = append(out, "")
		}

		out = append(out, rest...)
	}

	return afero.WriteFile(fs, roadmapPath, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

// roadmapFiles are the Markdown files under docs/roadmap/, sorted.
func roadmapFiles(fs afero.Fs) ([]string, error) {
	var files []string

	err := afero.Walk(fs, roadmapDir, func(p string, info os.FileInfo, err error) error {
		switch {
		case errors.Is(err, os.ErrNotExist) && p == roadmapDir:
			return nil
		case err != nil:
			return err
		case !info.IsDir() && path.Ext(p) == ".md":
			files = append(files, path.Clean(p))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", roadmapDir, err)
	}

	slices.Sort(files)

	return files, nil
}

// roadmapTarget is the file under docs/roadmap/ a link in docs/roadmap.md
// points to, written relative to it or, as it often is, to the root.
func roadmapTarget(target string) (string, bool) {
	target, _, _ = strings.Cut(target, "#")
	if strings.Contains(target, "://") {
		return "", false
	}

	for _, p := range []string{path.Join("docs", target), path.Clean(target)} {
		if strings.HasPrefix(p, roadmapDir+"/") {
			return p, true
		}
	}

	return "", false
}

// roadmapTitle is a file's first heading, or its name where it has none.
func roadmapTitle(fs afero.Fs, file string) (string, error) {
	b, err := afero.ReadFile(fs, file)
	if err != nil {
		return "", err
	}

	if m := markdownHeading.FindSubmatch(b); m != nil {
		return string(m[1]), nil
	}

	return strings.TrimSuffix(path.Base(file), ".md"), nil
}
