package maintain

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"unicode/utf8"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/MarkRosemaker/openapi"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	readmePath = "README.md"
	readmeDir  = "README"

	// readmeLegacyName marks README/ as holding an unprocessed dump of a
	// repository's previous hand-written README: what [ReadmeTask] falls
	// back to when it could not recognise anything in that README worth
	// splitting into fragments.
	readmeLegacyName = "legacy.md"
	disallowLegacy   = true

	// openAPIPath is where a repository describing an API keeps that
	// description.
	openAPIPath = "api/openapi.json"
)

// readmeTmplText is the raw template text, embedded rather than kept as a Go
// string literal: it is expected to grow, and a template is easier to read
// and change as its own file than as a quoted string in the middle of Go
// source. Everything the layout decides — the badge row included — lives
// there rather than being assembled in Go.
//
//go:embed README.md.tmpl
var readmeTmplText string

var readmeTmpl = template.Must(template.New("README.md").Parse(readmeTmplText))

// readmeData is what [readmeTmpl] is executed against.
type readmeData struct {
	// Owner and Name are the repository's, kept apart rather than joined
	// into one "owner/name": the template is the only place that needs them
	// together, and it can say so itself.
	Owner string
	Name  string

	// Private mirrors the repository's own flag: a private repository gets
	// no footer and none of the badges that only mean anything once other
	// people can see the code, the same way [LicenseTask] gives it no
	// licence. The coverage badge it keeps, since that is for whoever owns
	// the repository rather than for the public.
	Private bool

	// Coverage is the percentage the previous run measured, whole rather
	// than fractional: a badge reading "96%" says everything "96.4%" does.
	//
	// It is what the run already knows, not something a fragment carries,
	// so it lags by one run on a repository whose coverage just changed.
	// Moving the figure into the repository itself is what fixes that, and
	// is the roadmap's repository-owned metadata.
	Coverage int

	// OpenAPI is the repository's parsed API description, or nil where it
	// does not describe an API. The whole document rather than a version
	// string: the template gates the badge on it being there at all, and
	// anything else a README wants to say about the API is then a field
	// away rather than another thing to thread through.
	OpenAPI *openapi.Document

	// Logo and Tagline come from a fragment's frontmatter — see
	// [readmeMeta].
	Logo    *readmeLogo
	Tagline string

	// Docs are the conventional documents this repository actually has, for
	// the footer to link. Unlike everything else in the footer these are
	// written whether or not the repository is private: a relative link to
	// a file beside the README works for anybody who can see the repository
	// at all, and a repository nobody outside can read is the one where
	// they are most of what there is to read.
	Docs []conventionalDoc

	// Description, Intro, Features, Usage, Examples and Tail are the bodies
	// of README/description.md, README/intro.md, and so on. Each is empty
	// until that fragment exists.
	Description string
	Intro       string
	Features    string
	Usage       string
	Examples    string
	Tail        string
}

// conventionalDoc is one of the long-form documents every repository here
// is told to keep, and how the README's footer names it.
type conventionalDoc struct {
	// Path is where the document lives, and is also the link, so it is
	// written the way a README's other links are: relative to the root.
	Path string

	// Title and Summary are what the link says and what follows it.
	Title   string
	Summary string
}

// conventionalDocs are the documents the generated AGENTS.md names, in the
// order the footer lists them.
//
// Fixed titles and summaries, rather than each document's own first heading
// the way [AgentsTask] titles a fragment link. The difference is that these
// two paths are a convention: docs/architecture.md means the same thing in
// every repository here, so the README saying it the same way everywhere is
// the point, while an AGENTS/ fragment is whatever that one repository
// needed and has to introduce itself.
//
// This works for the reason logo inference mostly does not: the convention
// is stated first, in AGENTS.md, so the generator is reading an agreed-on
// layout rather than guessing at an arbitrary one.
var conventionalDocs = []conventionalDoc{
	{
		Path:    "docs/architecture.md",
		Title:   "Architecture",
		Summary: "how the parts fit together",
	},
	{
		Path:    "docs/roadmap.md",
		Title:   "Roadmap",
		Summary: "what is planned and not yet done",
	},
}

// readConventionalDocs returns the conventional documents this repository
// actually has, so the footer never offers a link that goes nowhere.
//
// A directory sitting at one of those paths is passed over rather than
// being an error: it is nothing this task wrote or owns, unlike an
// unexpected entry in README/, so there is nothing for it to insist on.
func readConventionalDocs(fs afero.Fs) ([]conventionalDoc, error) {
	var found []conventionalDoc

	for _, doc := range conventionalDocs {
		info, err := fs.Stat(doc.Path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("checking for %s: %w", doc.Path, err)
		}

		if info.Mode().IsRegular() {
			found = append(found, doc)
		}
	}

	return found, nil
}

// readmeMeta is the YAML frontmatter a fragment may open with, for the
// parts of a README that are data rather than prose. It belongs on
// README/description.md, which is the fragment that renders where a logo
// and a tagline go, but it is read from any fragment that carries it
// rather than only that one.
type readmeMeta struct {
	// Tagline is one line, centred under the logo. It is frontmatter rather
	// than a fragment of its own because a single line does not earn a file.
	Tagline string `yaml:"tagline,omitempty"`

	// Logo is the image above the tagline.
	Logo *readmeLogo `yaml:"logo,omitempty"`
}

// readmeLogo is the image a README opens with. Width defaults to 500 in the
// template when left at zero.
type readmeLogo struct {
	Alt    string `yaml:"alt,omitempty"`
	Source string `yaml:"source,omitempty"`
	Width  int    `yaml:"width,omitempty"`
}

// readOpenAPI parses the repository's API description, returning nil where
// it has none.
//
// An invalid document is an error rather than a badge left off: a
// repository publishing a broken API description has a problem worth
// stopping for, and quietly not mentioning it would be the least useful
// thing to do about it.
func readOpenAPI(fs afero.Fs) (*openapi.Document, error) {
	b, err := afero.ReadFile(fs, openAPIPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("%s: %w", openAPIPath, err)
	}

	doc, err := openapi.LoadFromDataJSON(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", openAPIPath, err)
	}

	if err := doc.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", openAPIPath, err)
	}

	return doc, nil
}

// CoverageColor is the shields.io colour for [readmeData.Coverage], the
// scale running from red at nothing covered to bright green at nearly all
// of it.
func (d readmeData) CoverageColor() string {
	switch {
	case d.Coverage >= 90:
		return "brightgreen"
	case d.Coverage >= 75:
		return "green"
	case d.Coverage >= 60:
		return "yellowgreen"
	case d.Coverage >= 45:
		return "yellow"
	case d.Coverage >= 30:
		return "orange"
	default:
		return "red"
	}
}

// hasContent reports whether any fragment has been written yet. The stamp,
// the badges and the footer are written whatever a repository has or has
// not, so they don't count on their own — an otherwise-untouched repository
// would never be recognised as such.
func (d readmeData) hasContent() bool {
	return d.Logo != nil || d.Tagline != "" || d.Description != "" ||
		d.Intro != "" || d.Features != "" || d.Usage != "" ||
		d.Examples != "" || d.Tail != ""
}

// bodyByFragment maps each fragment [ReadmeTask] reads to the [readmeData]
// field its body fills. Reading README/ as a directory and looking each
// entry up here — rather than looking for each name in turn — is what lets
// an entry that is in neither be reported rather than silently skipped.
func (d *readmeData) bodyByFragment() map[string]*string {
	return map[string]*string{
		"description.md": &d.Description,
		"intro.md":       &d.Intro,
		"features.md":    &d.Features,
		"usage.md":       &d.Usage,
		"examples.md":    &d.Examples,
		"tail.md":        &d.Tail,
	}
}

// ReadmeTask returns a task that assembles README.md from the repository's
// README/ directory, reading whichever of these already exist:
//
//	README/description.md  the paragraph under the tagline, unheaded
//	README/intro.md        "## Introduction"
//	README/features.md     "## Features"
//	README/usage.md        "## Usage"
//	README/examples.md     "## Examples"
//	README/tail.md         freeform, right before the generated footer
//
// A fragment may open with YAML frontmatter for the parts of a README that
// are data rather than prose — a logo and a tagline, see [readmeMeta] —
// which is why neither has a fragment of its own. A logo's source must name
// a file in the repository, or the task fails rather than publish a README
// with a broken image at the top of it; a remote URL is taken on trust.
//
// A fragment's body is written as if its content already sat at the
// repository root: a link to "docs/architecture.md" resolves correctly once
// embedded, even though the fragment itself lives one directory deeper.
//
// Once any fragment exists, README.md is fully owned by this task —
// rewritten unconditionally every run, the same as [LicenseTask] — and a
// hand edit to it is simply overwritten on the next run.
//
// A repository that has no fragments yet is handled without discarding
// anything:
//
//   - If README/legacy.md exists, nothing is written at all: an earlier run
//     could not make sense of that repository's hand-written README and put
//     it there whole, and it is waiting for a human. Once they have split it
//     up and deleted legacy.md, this task takes over.
//   - Otherwise, if README.md already exists, it predates this task and is
//     split into fragments by its headings (see [splitLegacyReadme]), which
//     the same run then renders back into README.md. A logo the README does
//     not show in a recognisable way is guessed at from the repository root
//     (see [inferLogo]). The split is approximate by design — it gets human
//     review either way — and falls back to README/legacy.md if nothing in
//     the file could be recognised.
//   - Otherwise, README.md is written from scratch: with no fragments at
//     all, that means the stamp and, for a public repository, the badge row
//     and generated footer.
func ReadmeTask(repo engine.Repo, coverage float64) engine.Task {
	return engine.Task{
		Name:  "generate README.md",
		Short: "readme",
		Run: func(context.Context) error {
			return generateReadme(repo.Fs(),
				repo.Owner(), repo.Name(), repo.Private(), coverage)
		},
	}
}

// generateReadme is [ReadmeTask]'s work, factored out so it can run against
// an in-memory filesystem in tests without a real repository.
func generateReadme(fs afero.Fs, owner, name string, private bool, coverage float64) error {
	legacyPath := path.Join(readmeDir, readmeLegacyName)

	exists, err := afero.Exists(fs, legacyPath)
	switch {
	case err != nil:
		return fmt.Errorf("checking for %s: %w", legacyPath, err)
	case !exists:
	case disallowLegacy:
		return fmt.Errorf("%s still exists", readmeDir)
	default:
		return nil
	}

	data, err := collectReadmeData(fs, owner, name, private, coverage)
	if err != nil {
		return err
	}

	if !data.hasContent() {
		split, err := adoptExistingReadme(fs, name)
		if err != nil {
			return err
		}

		switch split {
		case readmeKept:
			// Nothing this task could recognise; README.md is now sitting
			// in README/legacy.md and is a human's to deal with.
			return nil
		case readmeSplit:
			// The fragments exist now, so read them back rather than
			// reconstructing the same data a second way.
			if data, err = collectReadmeData(fs, owner, name, private, coverage); err != nil {
				return err
			}
		case readmeAbsent:
		}
	}

	b, err := renderReadme(data)
	if err != nil {
		return err
	}

	return afero.WriteFile(fs, readmePath, b, 0o644)
}

// readmeAdoption is what [adoptExistingReadme] found to do.
type readmeAdoption int

const (
	// readmeAbsent means there was no README.md to adopt.
	readmeAbsent readmeAdoption = iota

	// readmeSplit means an existing README.md was split into fragments,
	// which the caller can now read.
	readmeSplit

	// readmeKept means an existing README.md could not be split and was
	// kept whole in README/legacy.md instead.
	readmeKept
)

// adoptExistingReadme takes over a README.md written before this task
// existed: split into fragments where its headings allow, kept whole in
// README/legacy.md where they don't.
//
// It refuses to adopt a README this task wrote, the way the AGENTS.md and
// Makefile generators do. "No fragment exists yet" is not enough on its own:
// a repository with nothing to say gets a generated README carrying only the
// badge row and no fragment to go with it, and the next run would then adopt
// that as though a human had written it — folding the stamp and the badges
// into README/description.md and rendering them inside the next README.
func adoptExistingReadme(fs afero.Fs, name string) (readmeAdoption, error) {
	existing, err := afero.ReadFile(fs, readmePath)
	if errors.Is(err, os.ErrNotExist) {
		return readmeAbsent, nil
	} else if err != nil {
		return 0, fmt.Errorf("reading existing %s: %w", readmePath, err)
	}

	if bytes.Contains(firstLine(existing), []byte(generatedMarker)) {
		return readmeAbsent, nil
	}

	if err := fs.MkdirAll(readmeDir, 0o755); err != nil {
		return 0, fmt.Errorf("creating %s: %w", readmeDir, err)
	}

	// A README that shows its logo some way this cannot recognise still
	// leaves the image itself lying in the repository, so look for one —
	// inference happens here, at adoption, rather than at every render:
	// what it finds is written into frontmatter, where a human reviewing
	// the adoption can see the guess and correct it.
	logo, err := inferLogo(fs, name)
	if err != nil {
		return 0, err
	}

	fragments, err := splitLegacyReadme(existing, logo)
	if err != nil {
		return 0, err
	}

	if len(fragments) == 0 {
		return readmeKept, afero.WriteFile(fs,
			path.Join(readmeDir, readmeLegacyName), existing, 0o644)
	}

	for name, content := range fragments {
		if err := afero.WriteFile(fs, path.Join(readmeDir, name), content, 0o644); err != nil {
			return 0, err
		}
	}

	return readmeSplit, nil
}

// readmeImageExts are the extensions [inferLogo] recognises an image by.
// It is for guessing only: a logo named in frontmatter is whatever the
// author says it is, whatever it is called.
var readmeImageExts = []string{".png", ".jpg", ".jpeg", ".svg", ".gif", ".webp"}

// inferLogo returns a logo for the one image in the repository root, or nil
// if there isn't exactly one.
//
// One image in the root is almost always the project's logo. Several is
// ambiguous and none is the common case, and both mean no logo rather than
// a guess: getting this wrong puts the wrong picture at the top of a
// README, so it only guesses where there is nothing to get wrong.
func inferLogo(fs afero.Fs, name string) (*readmeLogo, error) {
	entries, err := afero.ReadDir(fs, ".")
	if err != nil {
		return nil, fmt.Errorf("reading the repository root: %w", err)
	}

	source := ""

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if !slices.Contains(readmeImageExts, strings.ToLower(path.Ext(entry.Name()))) {
			continue
		}

		if source != "" {
			return nil, nil
		}

		source = entry.Name()
	}

	if source == "" {
		return nil, nil
	}

	return &readmeLogo{Alt: name + " logo", Source: source}, nil
}

// validateLogo checks that a logo's source is something a reader will
// actually see. A source naming a file that is not in the repository
// renders as a broken image on the repository's front page, which is worth
// failing the task over rather than publishing — the fragment naming it is
// a human's to fix. A remote URL is taken on trust, since checking it would
// mean a network request per run.
func validateLogo(fs afero.Fs, logo *readmeLogo) error {
	if logo.Source == "" {
		return errors.New("logo: no source")
	}

	if strings.HasPrefix(logo.Source, "http://") ||
		strings.HasPrefix(logo.Source, "https://") ||
		strings.HasPrefix(logo.Source, "//") {
		return nil
	}

	info, err := fs.Stat(logo.Source)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("logo: source %q is not in the repository", logo.Source)
	} else if err != nil {
		return fmt.Errorf("logo: source %q: %w", logo.Source, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("logo: source %q is not a file", logo.Source)
	}

	return nil
}

// legacyFragmentByHeading maps the "## " headings a hand-written README is
// likely to use to the fragment each one becomes. Anything else keeps its
// heading and goes to tail.md, so nothing is dropped for not being
// recognised.
var legacyFragmentByHeading = map[string]string{
	"introduction":    "intro.md",
	"intro":           "intro.md",
	"about":           "intro.md",
	"features":        "features.md",
	"usage":           "usage.md",
	"getting started": "usage.md",
	"installation":    "usage.md",
	"examples":        "examples.md",
	"example":         "examples.md",
}

// legacyHeadingGenerated are the headings the template writes itself, so a
// README that already has them does not hand them back as fragment content
// for the generated versions to duplicate.
var legacyHeadingGenerated = map[string]bool{
	"additional information": true,
	"contributing":           true,
	"license":                true,
	"licence":                true,
}

var (
	// legacyTagline matches the centred tagline this template writes, so a
	// README previously generated elsewhere in the same shape hands its
	// tagline back as frontmatter rather than as prose.
	legacyTagline = regexp.MustCompile(`(?is)<h3 align="center">\s*(.*?)\s*</h3>`)

	// legacyLogo matches the centred logo block, and legacyAttrs its
	// attributes. Each accepts a quoted or an unquoted value — hand-written
	// HTML has both — which is why the value is three alternatives rather
	// than one pattern: an unquoted value ends at whitespace, a quoted one
	// only at its closing quote, and treating them alike truncates any
	// value with a space in it.
	legacyLogo  = regexp.MustCompile(`(?is)<p align="center">\s*(<img[^>]*>)\s*</p>`)
	legacyAttrs = map[string]*regexp.Regexp{
		"alt":   legacyAttr("alt"),
		"src":   legacyAttr("src"),
		"width": legacyAttr("width"),
	}

	// legacyBadgeLine matches a line that is nothing but linked badge
	// images, which the template writes for itself.
	legacyBadgeLine = regexp.MustCompile(`^\s*(?:\[!\[[^\]]*\]\([^)]*\)\]\([^)]*\)\s*)+$`)
)

// splitLegacyReadme splits a hand-written README into the fragments
// [ReadmeTask] reads, keyed by fragment name.
//
// It is deliberately approximate: a heading it does not recognise keeps its
// heading and joins tail.md rather than being dropped, the headings and
// badges the template writes itself are left out so the next render does not
// double them up, and the result gets human review either way. An empty
// result means nothing was recognised at all, which is the caller's cue to
// keep the file whole instead.
//
// fallbackLogo, which may be nil, is used when the README shows no logo
// this can recognise.
func splitLegacyReadme(content []byte, fallbackLogo *readmeLogo) (map[string][]byte, error) {
	var (
		meta     readmeMeta
		sections = map[string]*bytes.Buffer{}
	)

	// Anything before the first heading is the unheaded paragraph the
	// template puts under the tagline.
	target := "description.md"

	// A fenced code block can hold a line starting with "## " — a shell
	// comment, or Markdown quoting Markdown — which is not a heading, and
	// splitting a section there would cut the block in half.
	fenced := false

	for line := range strings.SplitSeq(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}

		if heading, ok := legacyHeading(line); ok && !fenced {
			switch {
			case legacyHeadingGenerated[heading]:
				target = ""
			case legacyFragmentByHeading[heading] != "":
				target = legacyFragmentByHeading[heading]
			default:
				target = "tail.md"
				appendLine(sections, target, line)
			}

			continue
		}

		if target == "" {
			continue
		}

		appendLine(sections, target, line)
	}

	// The identity blocks are matched over the whole description rather
	// than line by line, since each spans several lines.
	if buf, ok := sections["description.md"]; ok {
		sections["description.md"] = bytes.NewBuffer(extractIdentity(buf.Bytes(), &meta))
	}

	if meta.Logo == nil {
		meta.Logo = fallbackLogo
	}

	return assembleLegacyFragments(sections, meta)
}

// legacyHeading reports whether line is a top-level "## " heading, and the
// heading's text folded for comparison. A deeper heading belongs to
// whichever section it is already inside.
func legacyHeading(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "## ")
	if !ok || strings.HasPrefix(rest, "#") {
		return "", false
	}

	return strings.ToLower(strings.TrimSpace(strings.Trim(rest, "#* "))), true
}

// appendLine adds line to the named section, creating it on first use.
func appendLine(sections map[string]*bytes.Buffer, name, line string) {
	buf, ok := sections[name]
	if !ok {
		buf = &bytes.Buffer{}
		sections[name] = buf
	}

	buf.WriteString(line)
	buf.WriteByte('\n')
}

// extractIdentity pulls the title, badges, logo and tagline out of a
// README's opening, filling in meta, and returns what is left: the prose.
func extractIdentity(description []byte, meta *readmeMeta) []byte {
	if m := legacyTagline.FindSubmatch(description); m != nil {
		meta.Tagline = strings.TrimSpace(string(m[1]))
		description = legacyTagline.ReplaceAll(description, nil)
	}

	if m := legacyLogo.FindSubmatch(description); m != nil {
		img := m[1]
		logo := &readmeLogo{
			Alt:    firstSubmatch(legacyAttrs["alt"], img),
			Source: firstSubmatch(legacyAttrs["src"], img),
		}

		// A logo sized by height rather than width keeps no width of its
		// own; the template's default applies instead.
		if w := firstSubmatch(legacyAttrs["width"], img); w != "" {
			logo.Width = atoiOrZero(w)
		}

		if logo.Source != "" {
			meta.Logo = logo
		}

		description = legacyLogo.ReplaceAll(description, nil)
	}

	var kept []string

	for line := range strings.SplitSeq(string(description), "\n") {
		trimmed := strings.TrimSpace(line)

		// The title is the repository's name, which GitHub shows above the
		// README anyway and the template does not write.
		if strings.HasPrefix(trimmed, "# ") {
			continue
		}

		if legacyBadgeLine.MatchString(line) {
			continue
		}

		kept = append(kept, line)
	}

	return []byte(strings.Join(kept, "\n"))
}

// legacyAttr builds the pattern matching one HTML attribute's value,
// however it was quoted.
func legacyAttr(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + name + `=(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
}

// firstSubmatch returns re's first non-empty submatch in b, or "": which
// group holds the value depends on how it was quoted.
func firstSubmatch(re *regexp.Regexp, b []byte) string {
	m := re.FindSubmatch(b)
	if m == nil {
		return ""
	}

	for _, group := range m[1:] {
		if v := strings.TrimSpace(string(group)); v != "" {
			return v
		}
	}

	return ""
}

// atoiOrZero parses a positive integer, treating anything else — an
// overflowing width, say — as unset, so the template's default applies.
func atoiOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}

	return n
}

// assembleLegacyFragments renders the split sections as fragment files,
// dropping the ones that came out empty and putting meta, if there is any,
// in description.md's frontmatter.
func assembleLegacyFragments(sections map[string]*bytes.Buffer, meta readmeMeta) (map[string][]byte, error) {
	fragments := map[string][]byte{}

	for name, buf := range sections {
		body := strings.TrimSpace(buf.String())
		if body == "" {
			continue
		}

		fragments[name] = []byte(body + "\n")
	}

	if meta.Tagline == "" && meta.Logo == nil {
		return fragments, nil
	}

	front, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("rendering frontmatter: %w", err)
	}

	fragments["description.md"] = slices.Concat(
		[]byte("---\n"), front, []byte("---\n\n"), fragments["description.md"],
	)

	return fragments, nil
}

// collectReadmeData reads README/ and fills in a [readmeData] for
// [readmeTmpl].
//
// Every entry in the directory has to be one this task knows what to do
// with: an entry it does not recognise is an error rather than something
// skipped, because a file sitting in README/ looks for all the world like
// it is being rendered, and silently ignoring it would leave whoever wrote
// it wondering where their words went.
func collectReadmeData(fs afero.Fs, owner, name string, private bool, coverage float64) (readmeData, error) {
	data := readmeData{
		Owner: owner, Name: name, Private: private,
		Coverage: int(coverage),
	}
	bodies := data.bodyByFragment()

	openAPI, err := readOpenAPI(fs)
	if err != nil {
		return readmeData{}, err
	}

	data.OpenAPI = openAPI

	docs, err := readConventionalDocs(fs)
	if err != nil {
		return readmeData{}, err
	}

	data.Docs = docs

	entries, err := afero.ReadDir(fs, readmeDir)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	} else if err != nil {
		return readmeData{}, fmt.Errorf("reading %s: %w", readmeDir, err)
	}

	for _, entry := range entries {
		// A dotfile is not something anybody expects rendered, so it is
		// neither read nor complained about.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		p := path.Join(readmeDir, entry.Name())

		// legacy.md is this task's own doing, and is dealt with before
		// anything gets here.
		if entry.Name() == readmeLegacyName {
			continue
		}

		body, ok := bodies[entry.Name()]
		if !ok {
			return readmeData{}, fmt.Errorf(
				"%s: not a fragment this knows how to render; expected one of %s",
				p, strings.Join(slices.Sorted(maps.Keys(bodies)), ", "),
			)
		}

		if !entry.Mode().IsRegular() {
			return readmeData{}, fmt.Errorf("%s: not a regular file", p)
		}

		b, err := afero.ReadFile(fs, p)
		if err != nil {
			return readmeData{}, fmt.Errorf("%s: %w", p, err)
		}

		if !utf8.Valid(b) {
			return readmeData{}, fmt.Errorf("%s: not valid UTF-8", p)
		}

		front, text := splitFrontmatter(b)

		if front != nil {
			if err := data.applyFrontmatter(fs, front); err != nil {
				return readmeData{}, fmt.Errorf("%s: %w", p, err)
			}
		}

		*body = strings.TrimSpace(string(text))
	}

	return data, nil
}

// splitFrontmatter splits a fragment's leading YAML frontmatter — the block
// between a "---" line and the next one — from its Markdown body. A
// fragment without frontmatter, or with an unclosed block that is therefore
// something else, is all body.
func splitFrontmatter(b []byte) (front, body []byte) {
	const delim = "---\n"

	if !bytes.HasPrefix(b, []byte(delim)) {
		return nil, b
	}

	rest := b[len(delim):]

	if i := bytes.Index(rest, []byte("\n"+delim)); i >= 0 {
		return rest[:i+1], rest[i+len("\n"+delim):]
	}

	// A block closed by the very last line, with no newline after it: the
	// newline before "---" belongs to the frontmatter, so only the
	// delimiter itself comes off.
	if bytes.HasSuffix(rest, []byte("\n---")) {
		return rest[:len(rest)-len("---")], nil
	}

	return nil, b
}

// applyFrontmatter reads a fragment's frontmatter into d. A value already
// set by an earlier fragment is left alone only if this one does not carry
// it, so the last fragment to name something wins — deterministic, since
// the fragments are read in the order the directory lists them.
func (d *readmeData) applyFrontmatter(fs afero.Fs, front []byte) error {
	var meta readmeMeta
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return err
	}

	if meta.Tagline != "" {
		d.Tagline = meta.Tagline
	}

	if meta.Logo != nil {
		if err := validateLogo(fs, meta.Logo); err != nil {
			return err
		}

		d.Logo = meta.Logo
	}

	return nil
}

// renderReadme executes [readmeTmpl] against data.
//
// The template's own trim markers are what keep the blank lines right — a
// section that does not render contributes nothing at all, rather than
// leaving the blank lines around it behind — so there is nothing to tidy up
// here beyond ending the file in exactly one newline. See
// TestReadmeTemplateWhitespace, which holds the template to that.
func renderReadme(data readmeData) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := readmeTmpl.Execute(buf, data); err != nil {
		return nil, fmt.Errorf("rendering README.md: %w", err)
	}

	return append(bytes.TrimSpace(buf.Bytes()), '\n'), nil
}
