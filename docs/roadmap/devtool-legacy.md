# What is left to take from devtool-legacy

`MarkRosemaker/devtool-legacy` is a fork of an earlier tool solving the same
problem as this one. This file is the inventory of what it still holds that
devtool does not, how each piece would fit devtool as it is now, and what
should not be carried over. It replaces the "Cannibalising devtool-legacy"
and "Pre-commit hooks" sections of `docs/roadmap.md`.

Delete a section when its work lands. When only "Not taking" and "Out of scope"
remain, `devtool-legacy` can be archived: every file this document cites is
named by path, and the generators it no longer holds are readable at
`devtool-legacy@293f881`.

Paths below are `devtool-legacy`'s unless they start with `maintain/`,
`internal/run/` or `main.go`, which are devtool's.

## Inventory

Every command in `devtool-legacy`'s `commands.yaml`, and where it stands.

| Legacy command | Status | Section |
| --- | --- | --- |
| `release <level>`, `release upload-assets` | To take | [Releases](#releases) |
| `apply-version-upgrades` | To take | [Upgrades between devtool versions](#upgrades-between-devtool-versions) |
| `gomod update` (`gomod_update.go`, `internal/version/`) | To take | [A floor for the go directive](#a-floor-for-the-go-directive) |
| `init`, `new <name>`, `repository` | To take | [Starting a repository](#starting-a-repository) |
| `github` | To take | [GitHub repository settings](#github-repository-settings) |
| `githooks` | To take | [Pre-commit hooks](#pre-commit-hooks) |
| `validate` | To take, smaller | [Validating devtool.json](#validating-devtooljson) |
| `commands` (`commands.yaml`, `internal/app/`) | On the main roadmap | "Commands generated from commands.yaml" |
| `required:` predicates (`smart_command.go`) | On the main roadmap | "Per-repository task sequences" |
| `update` (`update.go`) | Absorbed | [Absorbed already](#absorbed-already) |
| `self-update`, `version`, `readme`, `git`, `golangci-lint`, `gomod create`, `gomod tidy-vendor`, `generate`, `format gofiles`, `build` | Absorbed | [Absorbed already](#absorbed-already) |
| `version-file` | Not taking | [Not taking](#not-taking) |
| `info-yaml` | Not taking | [Not taking](#not-taking) |
| `run` | Not taking | [Not taking](#not-taking) |
| `clients`, `http` | Not taking | [Not taking](#not-taking) |
| `circleci`, `dockerfile`, `mainfile`, `destroy` | Out of scope | [Out of scope for now](#out-of-scope-for-now-not-forever) |

---

## Releases

`internal/cmd/release.go` and `internal/cmd/release_upload-assets.go`.

`self-update` already ships a fix without a release: it reinstalls the module
at its latest version through the proxy (see `devtool-engine/selfupdate`). A
release is for people who are not us — a tagged version to depend on, and
binaries for somebody without a Go toolchain. Nothing is waiting on it, which
is why it sits here rather than at the top of the main roadmap.

### What the legacy command does

1. Checks out `master` and pulls.
2. Works out the next version from `info.yaml`'s `version` and a level
   (`major`, `minor`, `patch`), or takes one from `-v`.
3. Asks for confirmation (`survey.Confirm`).
4. Writes the new version into `info.yaml` and regenerates
   `<name>/version.gen.go`, commits both as `Release vX.Y.Z` and pushes.
5. Cross-compiles every `cmd/<app>` for `darwin/amd64`, `darwin/arm64` and
   `linux/amd64` into `dist/`.
6. Creates a GitHub release with a fixed body of headed sections, as a
   prerelease, and opens it in a browser.
7. Uploads each file in `dist/` through `ghrepo`'s `UploadReleaseBinary`,
   which zips the binary and uploads a SHA-256 checksum next to it.

### How it should look here

The version moves out of a file and into the tag. devtool already refuses to
keep a version in a file for itself (`version.go`: "Read rather than
generated") and reports builds from what the toolchain stamps, which for a
tagged module is the tag. So step 4 disappears: a release is a tag and a
GitHub release, and commits nothing.

```
devtool release patch            # v1.4.2 → v1.4.3
devtool release minor            # v1.4.2 → v1.5.0
devtool release -version=v2.0.0  # exactly this
devtool release patch -dry-run   # say what would happen, touch nothing
```

A subcommand beside `self-update` in `main.go`'s `dispatch`, local only: it
releases the repository you are standing in, never one from the list. A
maintained run must not cut releases on its own.

Steps:

1. Refuse a dirty worktree, a branch other than the default, or a default
   branch behind its remote. The legacy command checked out and pulled for
   you; refusing is safer in a checkout somebody is working in, which is the
   same reasoning `internal/local` uses for `-commit`.
2. Read the latest version from the tags, not from GitHub releases: a
   repository can have tags without releases, and `go` resolves versions from
   tags. No tags means `v0.0.0`.
3. Compute the next version. `golang.org/x/mod/semver` is already a
   dependency and does not increment, so a small function does it.
4. Run `make ci`, so nothing is released that would not pass a commit.
5. Tag, push the tag.
6. Build binaries for a `cli` repository only (see `maintain/kind.go`). A
   library has nothing to build.
7. Create the GitHub release with the notes, and upload the assets.

```go
// nextVersion is the version after latest at the given level. latest is a
// semver tag such as "v1.4.2", or "" where the repository has none.
func nextVersion(latest, level string) (string, error) {
	if latest == "" {
		latest = "v0.0.0"
	}

	if !semver.IsValid(latest) || semver.Prerelease(latest) != "" {
		return "", fmt.Errorf("latest tag %q is not a release version", latest)
	}

	var major, minor, patch int
	if _, err := fmt.Sscanf(semver.Canonical(latest), "v%d.%d.%d", &major, &minor, &patch); err != nil {
		return "", fmt.Errorf("reading %q: %w", latest, err)
	}

	switch level {
	case "major":
		return fmt.Sprintf("v%d.0.0", major+1), nil
	case "minor":
		return fmt.Sprintf("v%d.%d.0", major, minor+1), nil
	case "patch":
		return fmt.Sprintf("v%d.%d.%d", major, minor, patch+1), nil
	default:
		return "", fmt.Errorf("level %q is not major, minor or patch", level)
	}
}
```

The latest tag, from git rather than the GitHub API, so a release works
before the repository has ever been pushed to:

```go
// latestTag is the highest semver tag in the repository, or "".
func latestTag(ctx context.Context, repo engine.Repo) (string, error) {
	out, err := repo.ExecCommand(ctx, "git", "tag", "--list", "v*")
	if err != nil {
		return "", err
	}

	var latest string

	for tag := range strings.FieldsSeq(string(out)) {
		if semver.IsValid(tag) && semver.Prerelease(tag) == "" &&
			(latest == "" || semver.Compare(tag, latest) > 0) {
			latest = tag
		}
	}

	return latest, nil
}
```

The build matrix. The legacy list had no Windows target, and its
`release_test.go` only checked that the machine running the tests was in it.
devtool's generated Makefile already runs on Windows, so the binary should too.

```go
// targets are what a release builds a cli repository for: `go tool dist
// list` has every combination.
var targets = []struct{ goos, goarch string }{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"windows", "amd64"},
}

// build cross-compiles the command for one target into dist/.
func build(ctx context.Context, repo engine.Repo, name, goos, goarch string) (string, error) {
	out := filepath.Join("dist", fmt.Sprintf("%s-%s-%s", name, goos, goarch))
	if goos == "windows" {
		out += ".exe"
	}

	// -trimpath and no VCS stamping drift: the tag is checked out, so the
	// toolchain stamps it, and buildVersion reports it.
	_, err := repo.ExecCommand(ctx, "env", "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0",
		"go", "build", "-trimpath", "-o", out, ".")

	return out, err
}
```

`env` does not exist on Windows. Running the release from Windows means
setting the variables on the `exec.Cmd` instead, which `engine.Repo`'s
`ExecCommand` does not offer today. Either add an environment parameter to
the engine, or release from macOS or Linux only and say so.

The build path is `.` because devtool keeps its command at the root. Once
"Commands generated from commands.yaml" moves it to `cmd/`, the path comes
from there, and every `cmd/<app>` gets its own binaries as the legacy command
did (its `binaryName` named an `app` directory after the repository).

The release itself, through `ghrepo`, which still has the legacy calls:

```go
rel, err := gh.CreateRelease(ctx, &github.RepositoryRelease{
	TagName:    github.Ptr(version),
	Name:       github.Ptr(version),
	Body:       github.Ptr(notes),
	Draft:      github.Ptr(false),
	Prerelease: github.Ptr(semver.Major(version) == "v0"),
})
if err != nil {
	return fmt.Errorf("creating release %s: %w", version, err)
}

for _, path := range binaries {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	suffix := ""
	if strings.HasSuffix(path, ".exe") {
		suffix = ".exe"
	}

	if err := gh.UploadReleaseBinary(ctx, int(rel.GetID()), path, info, suffix); err != nil {
		return fmt.Errorf("uploading %s: %w (retry with: devtool release upload-assets %d)",
			path, err, rel.GetID())
	}
}
```

Keep `upload-assets <release ID>` as a subcommand. Uploads fail halfway more
often than anything else here, and the legacy error message already told you
how to retry; that was the best part of the design.

### Release notes

The legacy body was empty headed sections (`Highlights`, `New Features`, `Bug
Fixes`, `Updates & Refactors`, `Docs`) for somebody to fill in on GitHub
afterwards. Better to write them from what is there: the merge commits of
pull requests since the previous tag.

```go
// notes lists the pull requests merged since the previous tag, newest first.
func notes(ctx context.Context, repo engine.Repo, previous string) (string, error) {
	rng := "HEAD"
	if previous != "" {
		rng = previous + "..HEAD"
	}

	out, err := repo.ExecCommand(ctx, "git", "log", "--merges", "--first-parent",
		"--format=- %b (%s)", rng)
	if err != nil {
		return "", err
	}

	return "## Changes\n\n" + string(out), nil
}
```

Squash merges change this: the subject line is the pull request title, and
there are no merge commits. `devtool-legacy`'s `repoSettings` turned squash
merging on and merge commits off, and [GitHub repository settings](#github-repository-settings)
proposes the same, so the notes should read `git log --first-parent
--format='- %s'` instead. Pick the format after settling the settings.

### Not to take from the legacy command

- The base directory `/Users/mark/go/src/github.com/` written into
  `ghrepo.WithBaseDir`, and the branch name `master` with its
  `TODO: allow for main`. Read the default branch from the repository.
- The interactive confirmation. `-dry-run` says what would happen; a prompt
  stops a release run from a script or an agent.
- Opening the release in a browser. Print the URL.
- Comparing the URL it built by hand against the one GitHub returned, with a
  `TODO` either way. Use the returned `HTMLURL`.
- Writing the version into `info.yaml` and `version.gen.go`, and committing
  it. See [Not taking](#not-taking).

---

## Upgrades between devtool versions

`internal/cmd/apply-version-upgrades.go` and `internal/cmd/upgrades/upgrade.go`.

A one-off change to how a repository is laid out, applied once to every
repository that was last maintained by a build older than the change. The
legacy `UpgradeList` was empty by the time it was forked, but the shape is
right, and devtool has the piece it was keyed on: `devtoolVersion` in
`devtool.json`, the build that last maintained the repository.

What it is for, concretely: a file devtool used to own and no longer does, a
fragment directory that is renamed, a `devtool.json` field that moves. Today
each such change is code that stays in a generator forever, checking for the
old shape on every run. An upgrade runs once and is deleted when no
repository is older than it.

### Shape

```go
// An upgrade changes a repository from how an older devtool left it to how
// this one expects it. It runs once per repository: on the first run of a
// build at or after Since, in a repository last maintained by a build
// before it.
type upgrade struct {
	// Since is the first build that expects the new shape, as a module
	// version: a tag or a pseudo-version.
	Since string

	// Name says what changed, in the run's report and the commit message.
	Name string

	// Run makes the change. Like any generator it must be idempotent, since
	// a repository with no recorded version gets every upgrade.
	Run func(fs afero.Fs) error
}

// upgrades are applied in order. Delete one when every repository in the
// portfolio has been maintained by a build at or after its Since.
var upgrades = []upgrade{
	// {Since: "v0.0.0-20261201000000-abcdef123456", Name: "move README/badges.md into README/intro.md", Run: …},
}
```

```go
// pending are the upgrades a repository last maintained by recorded needs.
func pending(recorded string) []upgrade {
	var due []upgrade

	for _, u := range upgrades {
		// No recorded version, or one that cannot be ordered, gets every
		// upgrade: each is idempotent, and missing one is worse.
		if recorded == "" || Outdated(recorded, u.Since) || !comparable(recorded) {
			due = append(due, u)
		}
	}

	return due
}
```

`Outdated(current, recorded)` in `maintain/version.go` already answers "is
this build older than that one" with the rules worked out the hard way —
pseudo-versions order by timestamp, `+dirty` builds order by their commit,
`(devel)` is not comparable. Reuse it rather than writing a second
comparison.

### Where it runs

First in `UpdateTask`, before any generator: the generators should see the
new shape. And the recorded version is only moved forward where the build is
`Installable`, which `UpdateTask` already ensures. So a local `+dirty` build
applies upgrades but never records that it did, and the next real build
applies them again — harmless, since each is idempotent.

Report each upgrade as its own name through `Describe`, so a run reads
`upgrade: move README/badges.md, readme → pushed`. Add the files an upgrade
touches to `owners` in `maintain/update.go`, as for any generator.

---

## A floor for the go directive

`internal/cmd/gomod_update.go` and `internal/version/versions.go`.

The legacy tool held one Go version for every repository (`version.Go`) and
raised a repository's `go` directive to that version's major and minor where
it was lower, with `go mod edit -go=1.21`.

devtool has nothing equivalent. The "update dependencies" task runs `go get
-u ./...`, `go mod tidy` and `go mod vendor` (`gorepo.UpdateDependencies`),
which raises the `go` line only when a dependency demands it. A repository
with few dependencies keeps whatever `go` line it was created with, and loses
language features (range-over-func, `errors.AsType`, `strings.FieldsSeq`)
that every other repository in the portfolio can use. devtool's own `go.mod`
says `go 1.27.0`; nothing makes the others follow.

### Where the floor comes from

The legacy constant had to be bumped by hand and was three years stale
(`1.21.0`) when it was forked. Better to read it off the build doing the
work: the Go version devtool was compiled with, which `buildVersion` already
reports. Every machine that maintains the portfolio has at least that Go,
because it built devtool.

```go
// goFloor is the lowest go directive a maintained repository keeps: the
// language version of the toolchain this build of devtool was made with,
// as "1.27". A repository already above it is left alone.
func goFloor() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}

	// "go1.27.1" → "go1.27"; a development toolchain ("devel …") gives no floor.
	lang := version.Lang(info.GoVersion)
	if lang == "" {
		return "", false
	}

	return strings.TrimPrefix(lang, "go"), true
}
```

`version` is the standard library's `go/version`, which takes the
`go1.27.1` form the toolchain reports and orders Go versions correctly
(`go1.9` before `go1.10`), which comparing them as text does not.

### The task

```go
// GoDirectiveTask raises go.mod's go line to the floor where it is lower.
func GoDirectiveTask(repo engine.Repo, floor string) engine.Task {
	return engine.Task{
		Name:  "raise the go directive to " + floor,
		Short: "go",
		Run: func(ctx context.Context) error {
			src, err := afero.ReadFile(repo.Fs(), "go.mod")
			if err != nil {
				return err
			}

			mod, err := modfile.ParseLax("go.mod", src, nil)
			if err != nil {
				return err
			}

			if mod.Go != nil && version.Compare("go"+mod.Go.Version, "go"+floor) >= 0 {
				return nil // already there; an unchanged worktree commits nothing
			}

			// go mod edit rather than modfile's own writer: it keeps the
			// toolchain line and the formatting the go command would.
			if _, err := repo.ExecCommand(ctx, "go", "mod", "edit", "-go="+floor); err != nil {
				return err
			}

			_, err = repo.ExecCommand(ctx, "go", "mod", "tidy")
			return err
		},
	}
}
```

Place it before "update dependencies" in `internal/run/tasks.go`, so the
dependency update runs against the new language version and `go fix` (which
comes after) can modernise code the raised version allows.

### Two things to check before writing it

- **The Makefile's `TOOL_GO`.** The generated Makefile installs tools with
  the machine's own Go (`TOOL_GO := $(shell go env GOVERSION)`, then
  `tools: export GOTOOLCHAIN := $(TOOL_GO)`). That is the same reasoning as
  the floor — use what this machine has — so the two agree as long as
  devtool was built on the machine that runs it, which `self-update` ensures.
- **Libraries other people import.** Raising the `go` line of a public
  library raises it for every module that depends on it. For this
  portfolio's own consumers that is the point; for strangers it is a
  breaking change in all but name. Consider exempting `library` and `apilib`
  repositories that are public, or raising them only to the oldest Go
  release still supported upstream (the current one minus one).

The other two constants in `internal/version/versions.go` need nothing:
`GolangciLint` is pinned per repository through `devtool.json`'s `tools`, and
`Docker` belongs to the out-of-scope Dockerfile generator.

---

## Starting a repository

`internal/cmd/new.go`, `internal/cmd/init.go` and `internal/cmd/repository.go`.

### What the legacy commands do

`new <name>` validates the name (lower-case letters and dashes, runs of dashes
collapsed, at most 45 characters), refuses to run inside another git
repository or over an existing directory, creates the directory, changes into
it, and runs `init`. `init` runs three steps — ask the user to fill in
`info.yaml`, `git init`, `go mod init` — and then the whole `update` sequence.

### What devtool already does

A maintained run already bootstraps: `RunOne` on a repository with no
`go.mod` runs `go mod init`, commits and pushes, and the next run writes the
`Makefile`, `LICENSE` and `AGENTS.md`. That is a server-side, after-the-fact
start. What is missing is the local, one-command start — `devtool new
markdown-lint` in `~/go/src/github.com/MarkRosemaker/` and a repository that
passes `make ci` a second later.

### Shape

```
devtool new MarkRosemaker/markdown-lint              # kind inferred as now
devtool new go-api-libs/weather -kind=apilib
devtool new MarkRosemaker/markdown-lint -github      # also create it on GitHub, private
```

The owner is required: the module path, the README, the license holder line
and whether the repository is an organisation's all depend on it, and
guessing it from the working directory is the kind of guess `devtool.json`'s
`kind` was introduced to stop.

```go
// newRepository creates and initialises a repository in ./name.
func newRepository(ctx context.Context, owner, name string, kind maintain.Kind) error {
	if err := validName(name); err != nil {
		return err
	}

	if _, err := os.Stat(name); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s already exists", name)
	}

	if inGitRepository(".") {
		return errors.New("refusing to create a repository inside another one")
	}

	if err := os.Mkdir(name, 0o755); err != nil {
		return err
	}

	steps := [][]string{
		{"git", "init", "--initial-branch=main"},
		{"go", "mod", "init", "github.com/" + owner + "/" + name},
	}
	for _, s := range steps {
		cmd := exec.CommandContext(ctx, s[0], s[1:]...)
		cmd.Dir = name
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(s, " "), err, out)
		}
	}

	// devtool.json first, with the kind, so the generators read it rather
	// than infer it from an empty directory.
	def := maintain.Definition{Kind: kind}
	if err := maintain.SaveDefinition(afero.NewBasePathFs(afero.NewOsFs(), name), def); err != nil {
		return err
	}

	// Then exactly what "devtool update" does in a repository you stand in.
	return local.Run(ctx, local.Options{Dir: name /* … */})
}

var (
	reSlug   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	maxSlugN = 45
)

func validName(name string) error {
	switch {
	case len(name) > maxSlugN:
		return fmt.Errorf("%q is %d characters too long", name, len(name)-maxSlugN)
	case !reSlug.MatchString(name):
		return fmt.Errorf("%q is not lower-case words joined by single dashes", name)
	}

	return nil
}
```

The legacy pattern `^[a-z-]+$` rejected digits, which rules out names this
portfolio already has (`json2yaml`, `yaml2json`, `oauth2local`), and accepted
a leading or trailing dash. The pattern above fixes both.

`local.Options` has no directory today — `internal/local` works on the
repository you are standing in. Either `os.Chdir` as the legacy command did,
or give `local.Run` a directory, which is cleaner and testable.

### -github

Creating the repository on GitHub, private by default, with the
[repository settings](#github-repository-settings) applied, its description
from `devtool.json`, and `origin` pointed at it. `ghrepo` has no create call
today; that belongs there, as `Service.CreateRepository(ctx, owner, name,
private bool)`, using the organisation endpoint where `config.json` says the
owner is one.

### And then the portfolio

A new repository is not maintained until `portfolio`'s `config.json` names
it. `new` should not edit that file — its owner adds repositories through
patchpal — but it can print the line to send:

```
created MarkRosemaker/markdown-lint
add it to the portfolio with:  /run MarkRosemaker/markdown-lint
```

### Not to take

The interactive `info.yaml` questionnaire (`internal/repo/ask_user.go`,
`library/ask_user.go`, `service/ask_user.go`). Everything it asked — team,
service tier, library subtype — belonged to the previous organisation, and
what devtool needs is one flag, `-kind`.

---

## GitHub repository settings

`internal/cmd/github.go`.

devtool already pushes a repository's description and topics to GitHub during
a maintained run. It does not touch the repository's settings, so each
repository keeps whatever it was created with. The legacy `repoSettings`
enforced a merge policy, and some of it is policy this portfolio already
states in prose without enforcing:

- `DeleteBranchOnMerge: true`. The `AGENTS.md` template says "Merging deletes
  the branch"; this is the setting that makes it true.
- `AllowSquashMerge: true`, `AllowMergeCommit: false`, `AllowRebaseMerge:
  false`. One commit per pull request on `main`, which matches "one commit per
  piece of work".
- `AllowAutoMerge: true` and `AllowUpdateBranch: true`. Lets a pull request
  merge itself once checks pass, and offers the "Update branch" button.
- `HasWiki`, `HasProjects`, `HasDownloads`: false. `docs/` is the wiki.

### The task

A task in `internal/run/tasks.go`, beside the description and topics, and a
no-op in `internal/local` the way `SetTopics` is today
(`internal/local/repo.go`: "SetDescription and SetTopics are GitHub's").

```go
// settings is the merge policy every maintained repository has on GitHub.
// Visibility is not among them: it is read from GitHub and recorded, never
// set.
func settings() *github.Repository {
	return &github.Repository{
		AllowSquashMerge:    github.Ptr(true),
		AllowMergeCommit:    github.Ptr(false),
		AllowRebaseMerge:    github.Ptr(false),
		AllowAutoMerge:      github.Ptr(true),
		AllowUpdateBranch:   github.Ptr(true),
		DeleteBranchOnMerge: github.Ptr(true),
		HasWiki:             github.Ptr(false),
		HasProjects:         github.Ptr(false),
		HasDownloads:        github.Ptr(false),

		// The squash commit's message is the pull request's title and body,
		// so the log reads as the list of pull requests.
		SquashMergeCommitTitle:   github.Ptr("PR_TITLE"),
		SquashMergeCommitMessage: github.Ptr("PR_BODY"),
	}
}

// applySettings edits the repository only where it differs, so an
// unchanged repository costs one read and no write.
func applySettings(ctx context.Context, r *ghrepo.Repository) error {
	want := settings()
	if matches(r.GitHub(), want) {
		return nil
	}

	return r.Edit(ctx, want)
}
```

`matches` compares only the fields `settings` sets; `ghrepo.Repository` would
need to expose the `*github.Repository` it already holds (it reads
`Description`, `Topics`, `Archived` and `Private` from it).

Settings are not files, so this task never produces a commit, and the
engine's rule — an unchanged worktree means nothing to commit — holds. Report
an edit as a log line at info, so a run's account says when a repository's
settings were corrected.

### Not to take

- `Private: true`. The legacy tool made every repository private on every
  run. devtool reads visibility from GitHub and records it in `devtool.json`;
  setting it would make public repositories private.
- `Language: "Go"`. GitHub computes the language; the field is read-only and
  the legacy call silently ignored it.
- The commented-out branch protection, which required one approving review.
  With one person working, required reviews mean bypassing protection on
  every merge. If protection is wanted, require the status checks only, and
  through a ruleset rather than classic branch protection.

---

## Pre-commit hooks

`internal/cmd/githooks.go` and `internal/templates/PreCommitHook.tpl`.

### What the legacy hook does

Runs `go mod tidy`, `gofumpt -l -w .`, `gosec`, `golangci-lint run` and
`gotestsum` with a cover profile, all in parallel, then fails the commit if
any failed or if total coverage is below a hard-coded 95%. It wrote the hook
straight into `.git/hooks/pre-commit`, and `update.go` stopped running it by
default ("do not run again by default, TODO: only for portfolio").

### What should replace it

The generated Makefile already has the gate: `make ci`, which "needs no
network beyond the module cache", runs generate, fix, vet, the race tests and
`verify`. The `AGENTS.md` template already tells every agent to run it before
every commit. The hook should be that and nothing else, so there is one
definition of "fit to commit":

```sh
#!/bin/sh
# Generated by devtool. Runs the same gate the AGENTS.md asks for.
# Skip once with: git commit --no-verify
exec make ci
```

Running the linters again separately, as the legacy hook did, would make the
hook and `make ci` drift apart the first time either changes. The coverage
check belongs in `make ci` too if it belongs anywhere: `maintain.CoverageFloor`
is 80, and the legacy hook's 95 contradicted it.

### Where the hook lives

`.git/hooks/` is not versioned, so a maintained run — which commits files —
cannot install it, and a hook written only by a local run reaches only the
machines where somebody ran devtool locally.

Commit the hook, and point git at it:

- A generator writes `.githooks/pre-commit`, mode `0755`, like every other
  file devtool owns: deterministic, with the generated-file marker.
- `git config core.hooksPath .githooks` makes a clone use it. That is
  per-clone configuration, so set it in two places: a local `devtool update`
  sets it in the repository it runs in, and the generated Makefile gets a
  `hooks` target for everyone else.

```make
# Uses the committed hooks in .githooks/. Once per clone.
hooks:
	git config core.hooksPath .githooks
```

```go
// HooksTask writes .githooks/pre-commit.
func HooksTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "write the pre-commit hook",
		Short: "hooks",
		Run: func(context.Context) error {
			return writeIfChanged(repo.Fs(), ".githooks/pre-commit", preCommit, 0o755)
		},
	}
}
```

### Two things to decide first

- **Agents in a sandbox.** A hook that runs `make ci` makes every commit
  take as long as the test suite. For a person that is the point; for an
  agent that already ran `make ci` before committing, as instructed, it runs
  the suite twice. Agents can pass `--no-verify`, but then the hook protects
  only people, which might be the right answer anyway.
- **Repositories whose `make ci` is slow.** A repository whose tests take
  minutes makes a hook people disable. A `devtool.json` opt-out, or a hook
  that runs `make ready` (no `verify`) instead, are the obvious escapes.

The roadmap entry this replaces asked for "pre-commit configured", which could mean the
[pre-commit](https://pre-commit.com) framework and its
`.pre-commit-config.yaml`. That brings a Python dependency to a Go
portfolio for nothing `make ci` does not already do; prefer the plain hook.

---

## Validating devtool.json

`internal/cmd/validate.go`.

The legacy validation checked one thing — that `team` was a known team — and
could print its findings as a JSON body for a GitHub comment (`-o github`),
with `-strict` turning warnings into a failing exit code. The team check is
the previous organisation's, but the command is worth having in a smaller
form, because `devtool.json` now carries things a typo silently breaks:

- a `kind` that is not one of `library`, `cli`, `webapp`, `apilib`;
- a database `type` that is not `sqlite` or `postgres`;
- a `webapp` section without `name` or `port` (refused at generation time
  today, which is late);
- a `tools` pin that is not a version.

```
devtool validate            # findings on stderr, exit 1 on any error
devtool validate -strict    # warnings fail too
```

`LoadDefinition` is the place to start: whatever it already refuses is half
of this. Unknown fields are worth a warning — `encoding/json` drops them, so
`"kidn": "cli"` reads as no kind at all and the kind is inferred instead.

```go
dec := json.NewDecoder(bytes.NewReader(src))
dec.DisallowUnknownFields() // only in validate; loading stays lenient
```

The GitHub comment output is not worth taking: nothing here posts validation
results to pull requests, and a failing `make ci` already says the same
thing in CI.

---

## Absorbed already

Nothing to take; listed so the inventory is complete.

- **`update`** (`update.go`) ran a fixed sequence of "smart commands", each
  skipped unless its `required:` predicate held, and printed how long any
  step over half a second took. `UpdateTask` plus the engine's runner are the
  sequence; the predicates are "Per-repository task sequences" on the main
  roadmap; per-step timing is "Record how long each step took". Its last act,
  writing the devtool version into the definition file, is what
  `devtoolVersion` in `devtool.json` does now.
- **`self-update`**: done differently, in `devtool-engine/selfupdate`, through
  the module proxy rather than release assets.
- **`version`**: `devtool version` and `-version`, from build information.
- **`readme`, `git` (`.gitignore`/`.gitattributes`), `golangci-lint`**: the
  README, `.gitignore` and lint config generators in `maintain/`.
- **`gomod create`**: the bootstrap branch of a maintained run.
- **`gomod tidy-vendor`, `generate`, `format gofiles`**: the "update
  dependencies", "go generate" and "golang-ci lint fix" tasks, and `make
  tidy`/`make format`.
- **`build`**, which also retried after `go get`-ting a missing package or
  re-vendoring on an "inconsistent vendoring" error: the Makefile builds, and
  the dependency task keeps `vendor/` consistent before anything builds.
  Retrying around a broken `go.mod` hid the problem rather than fixing it.

### internal/golangci/golangci.go

Not superseded by the lint config generator, and stays until its settings
have been read. Of the 75 linters it enables, 62 still exist in golangci-lint
v2 and 13 do not: `deadcode`, `structcheck`, `varcheck`, `ifshort`,
`exhaustivestruct` and `typecheck` are gone outright, `exportloopref` is now
`copyloopvar`, `gosimple` and `stylecheck` folded into `staticcheck`, `tenv`
into `usetesting`, `goerr113` into `err113`, `gomnd` into `mnd`, and `wsl`
into `wsl_v5`. Its `printfFuncs` names another organisation's logger and is
worth nothing here.

The two files take opposite postures, which is the real difference:
`golangci.go` starts from `config.NewDefault()` and opts *out*, while ours
starts from `Default: none` and opts *in*. The settings worth reading before
extending ours are `nolintlint` (`RequireExplanation`, `RequireSpecific`),
`godot` (`Scope: all`, `Capital`, close to what the comment conventions ask
for anyway), `errcheck` (`CheckAssignToBlank`, `CheckTypeAssertions`),
`unparam.CheckExported`, and the complexity ceilings — `cyclop` 10,
`gocognit`/`gocyclo` 20, `nestif` 4, `nakedret` 10.
`exhaustive.DefaultSignifiesExhaustive` and `tagalign` we already set the
same way.

Adding one of these is a change to `internal/lintgen`, which writes
`maintain/lint.yaml` from a Go value:

```go
// in the value internal/lintgen marshals
Linters: config.Linters{
	Default: "none",
	Enable:  append(enabled, "nolintlint"),
	Settings: config.LintersSettings{
		NoLintLint: config.NoLintLintSettings{
			RequireExplanation: true,
			RequireSpecific:    true,
		},
	},
},
```

Each linter added fails `make ci` in every repository that violates it on the
next run, so enable one at a time and fix the portfolio in the same pass, or
the next maintained run goes red thirty times.

---

## Not taking

- **`version-file`**, which generated `<name>/version.gen.go` holding a
  `Version` and a `GoVersion`, and was rewritten on every release. devtool
  reads its version from the build information the toolchain stamps
  (`version.go`), and says why: "A version written into a file has to be kept
  in step with reality by whoever remembers to, and the failure this exists
  to diagnose is precisely a binary that is not what its source says." The
  same holds for every other repository. A library that wants to report its
  own version can read `debug.ReadBuildInfo()`'s `Deps` for its module path.
- **`info-yaml`**, the interactive questionnaire that wrote `info.yaml`.
  `devtool.json` replaced the file; its fields are inferred or set by hand.
- **`run`**, which built the binary and ran it, waiting for Ctrl+C. `go run
  .` does this; a web app's Makefile builds `bin/<repo>`.
- **`clients`** (OpenAPI client generation through oapi-codegen) and
  **`http`** (HTTP scaffolding from `api/openapi.yaml`): `openapi-codegen`
  and the `go-api-libs` pipeline, and the web app generator's `go:generate`
  lines for a server in `pkg/api`.

---

## Out of scope for now, not forever

`devtool-legacy` no longer has its microservice, CI and website generators —
`circleci` (with `internal/circleci/` and its workflow and cache model),
`dockerfile`, `mainfile` (a service's `cmd/app/main.gen.go` and
`internal/app/service.go`), `destroy` (a CircleCI config that tears a service
down on merge) — because this portfolio has no microservices today, not
because generating them is a bad idea. They were deleted in `9bf7f15` and are
readable at `devtool-legacy@293f881`:

```bash
git -C devtool-legacy show 293f881:internal/cmd/dockerfile.go
git -C devtool-legacy show 293f881:internal/circleci/workflows.go
```

When one of them is wanted, it graduates to its own `devtool-<name>`
repository rather than reverting into the fork — `devtool-circleci` or
`devtool-service` is the shape — and runs under a predicate like the legacy
`repo.IsMicroservice`, which is what "Per-repository task sequences" would
provide. A GitHub Actions workflow would replace the CircleCI generator
rather than port it.

Archive `devtool-legacy` before that day comes if it is otherwise empty: an
archived repository is still readable, and `293f881` does not move.
