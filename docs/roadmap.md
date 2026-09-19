# Roadmap

Work not yet done. Most of this came over from `portfolio`, where the
generators used to live; entries about the event stream and the runner are on
`devtool-engine`'s roadmap, and entries about the chat are on `patchpal`'s.

## Coverage as a gate

Coverage is measured and recorded every run, but nothing acts on it. The
intended behaviour:

- Treat a repository above 80% as ready to publish, and make sure it is.
- Treat a **drop** below 80% as an error — that is a regression, and worth
  interrupting someone for.
- Treat a repository that has *always* been below 80% as a log line, not an
  error. It is a backlog item, not a regression, and failing every run on it
  would train the notification to be ignored.

The distinction between the last two is the whole point, and it is why
`event.Result` carries `PrevCoverage`.

## The figure a badge shows is one run behind

Coverage is measured after the tasks have run, so the figure a task sees — and
therefore the figure the README badge is generated from — is the one recorded
going in. Fixing it means a task that runs after the measurement, or a second
pass over the repository.

## Repository-owned metadata

`devtool.json` in each repository's root holds `description`, `topics`,
`coverage` and the newest `devtoolVersion` known to have maintained it.
`devtool test` measures coverage and records it; the README badge reads it. The
version is a high-water mark, and a run compares its own build against it to
notice, without asking anything, that it is behind. A maintained run
prefers the definition, falls back to `portfolio`'s `config.json` where a
repository has none, and writes the definition afterwards with whatever it
used — so one full run over the portfolio gives every repository one.

What is left: the engine's `Runner.prepare` pushes `Description` and `Topics`
before any task runs, and takes them from the definition where there is one,
which means reading a file out of a worktree that `prepare` itself pulls. Worth
a look when the fields leave `config.json`, because the ordering is the part
that was always awkward — and it is a change to `devtool-engine`, not here.

## Record how long each step took

`devtool.json` already carries per-repository metadata, and a run now emits a
step-by-step account of what it did. Writing each step's duration there would
let a reader estimate progress from what this repository actually cost last
time, rather than from a static weight — last-run-wins, per repository, no
learning algorithm.

The estimate needs it more than it looks. A repository's total work is not
knowable in advance: `apply` runs the whole suite inside any task that changed
a relevant file, so the number of test runs depends on how many tasks turn out
to commit, which nobody knows until they do. A repository that committed three
tasks last run will probably commit them again, and that is the whole
prediction.

## The tool PATH does not hold on make 3.81

The generated Makefile exports `PATH` with `$(TOOL_BIN)` in front so a recipe
runs the copy `make tools` installed rather than an older one earlier on
somebody's PATH. That works for any recipe line reaching a shell, and not for
one without shell metacharacters: GNU Make execs those directly, and 3.81 —
what Apple ships as `/usr/bin/make` — resolves them against its own PATH
instead of the exported one. `golangci-lint run` and `govulncheck ./...` are
exactly that shape, so on a Mac `make lint` can silently run a linter the
export was written to rule out.

Proven rather than guessed: `command -v housetool` inside the same Makefile
finds the installed copy while the bare recipe word runs the shadowed one.
`TestGenerateMakefileToolPathUnderMake` now asserts only the half that holds
everywhere, and says so.

Two ways out, neither free. Calling the house tools by absolute path —
`$(TOOL_BIN)/golangci-lint` — works on every make and says what it means, but
a machine that never ran `make tools` gets "no such file" rather than falling
back to its own copy. Requiring GNU Make 4.x pushes a setup step onto every
machine and every agent. Left as it is until one of those costs less than the
shadowing does.

## Per-repository task sequences

Every repository currently gets the same sequence. Some want more:
`worldweaver` has its own `.golangci.yml` that the shared lint task ignores.

The sequence is already a function of the repository, so this is a matter of
letting a repository's definition carry an opt-in list rather than a schema
change. `devtool-legacy`'s `required:` predicates — `repo.UsesGo`,
`repo.IsMicroservice`, `always` — are the design for it.

This is also the prerequisite for maintaining a repository that is not a Go
repository at all. As things stand a repository with no `go.mod` takes the
bootstrap branch — `go mod init`, an initial commit, a push — and then a
`Makefile`, a `LICENSE` and an `AGENTS.md` on the next run.

## README/examples.md written from source

`README/examples.md` is, for now, a fragment like any other: prose a human
writes. A repository with an `examples/examples_test.go` already has this
written twice — once as runnable, verified code, once as README prose that
quietly drifts from it. The README task could parse the former (an `ExampleXxx`
function's doc comment and body are exactly a README example's prose and code)
and write `examples.md` itself, the same way it already writes `LICENSE`.
Worth doing once drift between the two has actually been seen, not before —
parsing Go source for this is a distinct, self-contained piece of work.

## Adopting a hand-written CLAUDE.md

The Claude task leaves anything already at `CLAUDE.md` alone, which is right as
far as it goes but leaves a repository that had one holding two agent files
that can disagree — and an agent reading the Claude-specific name gets the
stale one.

The other three generators adopt rather than leave: the file would move to
`AGENTS/claude.md` and be linked from the generated `AGENTS.md`. That belongs
in the task that writes `AGENTS.md` rather than in the one that writes
`CLAUDE.md`, since only the former can link the fragment in the same run — done
from the latter it would lag a run behind.

## Pre-commit hooks

Check that each repository has pre-commit configured, and add it where missing.
A natural task: idempotent, writes a file, commits only when it changed.
`devtool-legacy`'s `githooks.go` and its `PreCommitHook.tpl` are the design.

## Cap a repository's outside dependencies

Carried over from `portfolio`'s `TODO.md`: check `go.mod` against a maximum
number of dependencies outside this owner's own modules. A repository that has
grown past it is worth knowing about, and a task that reports rather than
enforces is the place to start.

## Cannibalising devtool-legacy

`MarkRosemaker/devtool-legacy` is a fork of an earlier tool solving the same
problem, kept to compare implementations against and shrunk as each feature is
replaced. Its `commands.yaml` is the inventory.

Worth taking and not yet here: `release <level>` (a semver bump plus assets),
`version-file`, `apply-version-upgrades` with `gomod_update.go` and
`internal/version/`, `init`/`new <name>` with `repository.go`, `update.go` and
`github.go`.

`self-update` was taken, and not as the fork had it — see `devtool-engine`'s
roadmap. So `release <level>` is no longer a prerequisite for shipping a fix;
it is worth having for people who are not us, and nothing is waiting on it.

Two corrections to a first reading that went by filenames:

- `internal/cmd/smart_command.go` looked like the `required:` predicate
  machinery worth porting. It is ten lines wrapping `cobra.Command` with an
  `IsRequired func() bool`. Take the idea; there is no code to take.
- `internal/golangci/golangci.go` looked superseded by the lint config
  generator. Compared, and kept. Of the 75 linters it enables, 62 still exist
  in golangci-lint v2 and 13 do not: `deadcode`, `structcheck`, `varcheck`,
  `ifshort`, `exhaustivestruct` and `typecheck` are gone outright,
  `exportloopref` is now `copyloopvar`, `gosimple` and `stylecheck` folded into
  `staticcheck`, `tenv` into `usetesting`, `goerr113` into `err113`, `gomnd`
  into `mnd`, and `wsl` into `wsl_v5`. Its `printfFuncs` names another
  organisation's logger and is worth nothing here.

  The two files take opposite postures, which is the real difference:
  `golangci.go` starts from `config.NewDefault()` and opts *out*, while ours
  starts from `Default: none` and opts *in* to seven. The settings worth
  reading before extending ours are `nolintlint` (`RequireExplanation`,
  `RequireSpecific`), `godot` (`Scope: all`, `Capital`, which is close to what
  the comment conventions ask for anyway), `errcheck`
  (`CheckAssignToBlank`, `CheckTypeAssertions`), `unparam.CheckExported`, and
  the complexity ceilings — `cyclop` 10, `gocognit`/`gocyclo` 20, `nestif` 4,
  `nakedret` 10. `exhaustive.DefaultSignifiesExhaustive` and `tagalign` we
  already set the same way.

**"Out of scope" meant now, not forever.** The pruning that took
`devtool-legacy` from 91 files to 55 cut the microservice, CI and website
generators because this portfolio has no microservices in it today, not because
generating a CircleCI config, a Dockerfile or a service skeleton is a bad idea.
Every deleted file is readable at `devtool-legacy@293f881`, the commit before
the pruning, which is on `master`'s own history rather than dangling. When one
of them is wanted, a generator graduates to its own `devtool-<name>` repository
rather than reverting into the fork — `devtool-circleci` or `devtool-service`
is the shape. The `required:` predicates above are the other half of that:
`repo.IsMicroservice` is precisely the condition such a generator would run
under.

## Report the goal

`logProgress` computes how many repositories the portfolio should have by now
and only logs it. It could go in the table's footer.

## Organisation detection for a wholly new owner

`RunOne` bootstraps an unknown repository under a *known* owner correctly — it
reuses that owner's existing `Organization` flag. A repository under an owner
the portfolio has never seen at all is assumed personal, because there is no
cheap way to ask GitHub which kind of account a name belongs to without listing
every one of its repositories. Fine for the common case; a new organisation
still needs one manual edit to `config.json` the first time.

## Test coverage

What is untested is what touches a repository from the outside:

- `internal/run`'s dependency discovery and module-path mapping.
- `internal/run`'s failure-to-open path, recorded on the unit.

`internal/local` is covered where it matters, including the refusal to touch a
dirty worktree.
