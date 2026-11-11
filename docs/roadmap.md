# Roadmap

See also:
- [What is left to take from devtool-legacy](roadmap/devtool-legacy.md)

Work not yet done. Entries about the event stream and the runner are on
`devtool-engine`'s roadmap, and entries about the chat are on `patchpal`'s.

## A test's Postgres on Windows

`<name>test` starts Postgres on Linux and macOS only. zonky builds it for
Windows too; what is missing there is stopping the server when the test
binary ends, which a shell waiting on a pipe does elsewhere (a job object
would), and running it unprivileged where the test is an administrator.

## Coverage before a repository joins the portfolio

A repository added to `portfolio`'s list should have good coverage, at or
above `maintain.CoverageFloor`, before it joins. The repositories listed now
are fine as they are.

## The figure a badge shows is one run behind

Coverage is measured after the tasks have run, so the figure a task sees — and
therefore the figure the README badge is generated from — is the one recorded
going in. Fixing it means a task that runs after the measurement, or a second
pass over the repository.

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

## A build with no version at all is exempt from the staleness refusal

A build stamped `(devel)` carries nothing to order against the mark — no
commit, no time — so it is waved through. On a machine whose `go build`
produces those, the refusal does nothing for a locally built binary, which is
a good part of what it was written to prevent. `docs/architecture.md` has
what the toolchain stamps and when.

Refusing every unversioned build is the obvious answer and the wrong one: it
would lock whoever is working on devtool itself out of every maintained
repository, on exactly the machine where builds come out unstamped. Worth
revisiting if the stamping turns out to be fixable at the source — `go build
-buildvcs=true` names the reason it was omitted — since the exemption then
stops mattering.

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

## Commands generated from commands.yaml

A command's shape — its name, subcommands and flags — belongs in a
`commands.yaml`, with devtool generating the `cobra.Command`s from it, the way
`devtool-legacy` does. devtool itself would move from its root `main.go` to
`cmd/`. Two things follow:

- The command's name comes from the file. `repoShape` stops reading `cmd/` to
  find it, which is the part the repository kind could not replace.
- Kind inference no longer needs the root `main` package rule, which exists
  only because devtool keeps its command at the root. That rule would go.

## Per-repository task sequences

Every repository gets the same sequence, whatever its kind.

The sequence is already a function of the repository, so this is a matter of
letting a repository's definition carry an opt-in list rather than a schema
change. `devtool.json` already names the repository's kind, which is the
obvious first thing to choose a sequence by. `devtool-legacy`'s `required:` predicates — `repo.UsesGo`,
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

## Cap a repository's outside dependencies

Check `go.mod` against a maximum number of dependencies outside this owner's
own modules. A repository that has
grown past it is worth knowing about, and a task that reports rather than
enforces is the place to start.

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
