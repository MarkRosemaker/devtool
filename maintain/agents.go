package maintain

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"text/template"
	"unicode/utf8"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const (
	agentsPath = "AGENTS.md"
	agentsDir  = "AGENTS"

	// agentsAdoptedName is where an AGENTS.md written before this task
	// existed ends up: a fragment like any other, linked along with the
	// rest, so nothing an agent was being told goes missing.
	agentsAdoptedName = "legacy.md"

	agentsExt = ".md"
)

//go:embed AGENTS.md.tmpl
var agentsTmplText string

var agentsTmpl = template.Must(template.New(agentsPath).Parse(agentsTmplText))

// agentsHeading matches a fragment's first level-one heading.
var agentsHeading = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

// agentsData is what [agentsTmpl] is executed against.
type agentsData struct {
	// Generated are the root files this repository has that this tool
	// wrote, each paired with the directory to edit instead.
	Generated []generatedRoot

	// Fragments are what AGENTS/ holds, in the order the directory lists
	// them.
	Fragments []agentsFragment

	// Gitignore is true where .gitignore carries this tool's block, which
	// needs saying differently from the rest: the file is part-generated, so
	// the instruction is where in it to write rather than not to.
	Gitignore bool
}

// generatedRoot is one generated root file and the directory it is built
// from.
type generatedRoot struct {
	File string
	Dir  string
}

// agentsFragment is one fragment, as it appears in the list of links.
type agentsFragment struct {
	Title string
	Path  string
}

// AgentsTask returns a task that writes the repository's AGENTS.md: the
// working conventions, and a link to each fragment in AGENTS/.
//
// Links, not the text itself. AGENTS.md is read into nearly every agent
// session, so what it costs is paid over and over, whether or not the task
// at hand needed any of it — while a link costs a line and is followed only
// when it is wanted. That is the one way this differs from README.md, which
// embeds its fragments because a README is meant to be read whole.
//
// The header is the conventions themselves — how to branch, commit, push
// and open a pull request, which root files are generated and where to edit
// instead, which documents have to be kept true, and where review feedback
// goes. Everything general lives there so an agent is told once rather than
// corrected later, which is why it is not short; AGENTS/ is then only what
// is particular to the one repository.
//
// A repository that already had a hand-written AGENTS.md keeps it: the file
// moves verbatim to AGENTS/legacy.md and is linked from the generated one.
func AgentsTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "generate AGENTS.md",
		Short: "agents",
		Run: func(context.Context) error {
			return generateAgents(repo.Fs())
		},
	}
}

// generateAgents is [AgentsTask]'s work, factored out so it can run against
// an in-memory filesystem in tests without a real repository.
func generateAgents(fs afero.Fs) error {
	if err := adoptExistingAgents(fs); err != nil {
		return err
	}

	data, err := collectAgentsData(fs)
	if err != nil {
		return err
	}

	b, err := renderAgents(data)
	if err != nil {
		return err
	}

	return afero.WriteFile(fs, agentsPath, b, 0o644)
}

// adoptExistingAgents moves an AGENTS.md written before this task existed
// into AGENTS/, where it goes on being linked like any other fragment.
func adoptExistingAgents(fs afero.Fs) error {
	existing, err := afero.ReadFile(fs, agentsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("reading existing %s: %w", agentsPath, err)
	}

	if generatedMarker.Match(firstLine(existing)) {
		return nil
	}

	if !utf8.Valid(existing) {
		return fmt.Errorf("%s: not valid UTF-8", agentsPath)
	}

	adopted := path.Join(agentsDir, agentsAdoptedName)

	if taken, err := afero.Exists(fs, adopted); err != nil {
		return fmt.Errorf("checking for %s: %w", adopted, err)
	} else if taken {
		return fmt.Errorf(
			"%s was not written by this task and %s already exists: move one of them aside",
			agentsPath, adopted,
		)
	}

	if err := fs.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", agentsDir, err)
	}

	return afero.WriteFile(fs, adopted, existing, 0o644)
}

// collectAgentsData reads AGENTS/ and works out which root files this
// repository has that were generated.
func collectAgentsData(fs afero.Fs) (agentsData, error) {
	fragments, err := readAgentsFragments(fs)
	if err != nil {
		return agentsData{}, err
	}

	generated, err := generatedRoots(fs)
	if err != nil {
		return agentsData{}, err
	}

	blocked, err := hasGitignoreBlock(fs)
	if err != nil {
		return agentsData{}, err
	}

	return agentsData{
		Generated: generated,
		Fragments: fragments,
		Gitignore: blocked,
	}, nil
}

// hasGitignoreBlock reports whether .gitignore carries [GitignoreTask]'s
// block, so a repository whose .gitignore is wholly its own is not told
// otherwise.
func hasGitignoreBlock(fs afero.Fs) (bool, error) {
	b, err := afero.ReadFile(fs, gitignorePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("reading %s: %w", gitignorePath, err)
	}

	return bytes.Contains(b, []byte(gitignoreOpen)), nil
}

// readAgentsFragments reads AGENTS/, returning one entry per fragment.
//
// An entry that is not a fragment is an error rather than something passed
// over, the same as in README/ and mk/: a file sitting there looks for all
// the world like an agent is being told to read it.
func readAgentsFragments(fs afero.Fs) ([]agentsFragment, error) {
	entries, err := afero.ReadDir(fs, agentsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("reading %s: %w", agentsDir, err)
	}

	var fragments []agentsFragment

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		p := path.Join(agentsDir, entry.Name())

		if !entry.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: not a regular file", p)
		}

		if path.Ext(entry.Name()) != agentsExt {
			return nil, fmt.Errorf("%s: not a %s fragment", p, agentsExt)
		}

		b, err := afero.ReadFile(fs, p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}

		if !utf8.Valid(b) {
			return nil, fmt.Errorf("%s: not valid UTF-8", p)
		}

		fragments = append(fragments, agentsFragment{
			Title: fragmentTitle(entry.Name(), b),
			Path:  p,
		})
	}

	return fragments, nil
}

// fragmentTitle is what the link says: the fragment's own first heading
// where it has one, since that tells an agent whether opening the file is
// worth it, and the file name otherwise.
func fragmentTitle(name string, content []byte) string {
	if m := agentsHeading.FindSubmatch(content); m != nil {
		return strings.TrimSpace(string(m[1]))
	}

	return strings.TrimSuffix(name, agentsExt)
}

// generatedRootDirs are the root files this package generates, each with
// the directory it is built from.
var generatedRootDirs = []generatedRoot{
	{File: agentsPath, Dir: agentsDir + "/"},
	{File: claudePath, Dir: agentsDir + "/"},
	{File: readmePath, Dir: readmeDir + "/"},
	{File: makefilePath, Dir: makefileDir + "/"},
}

// generatedRoots returns the root files this repository has that carry the
// marker, so an agent is only told to leave alone what really is generated:
// a repository whose README this tool has not taken over still has one
// somebody edits by hand.
//
// AGENTS.md counts whatever it looks like now, since this task is about to
// write it.
func generatedRoots(fs afero.Fs) ([]generatedRoot, error) {
	var found []generatedRoot

	for _, root := range generatedRootDirs {
		if root.File == agentsPath {
			found = append(found, root)

			continue
		}

		b, err := afero.ReadFile(fs, root.File)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("reading %s: %w", root.File, err)
		}

		if generatedMarker.Match(firstLine(b)) {
			found = append(found, root)
		}
	}

	return found, nil
}

// firstLine returns b up to its first newline.
func firstLine(b []byte) []byte {
	line, _, _ := bytes.Cut(b, []byte("\n"))

	return line
}

// renderAgents executes [agentsTmpl] against data.
func renderAgents(data agentsData) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := agentsTmpl.Execute(buf, data); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", agentsPath, err)
	}

	return append(bytes.TrimSpace(buf.Bytes()), '\n'), nil
}
