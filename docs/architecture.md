# Architecture

How this repository is put together, and the parts of it that have caught
somebody out.

## What it is

`devtool` brings Go repositories up to a standard and keeps them there. It
writes the files an owner would otherwise write by hand — `LICENSE`,
`README.md`, `Makefile`, `.gitignore`, `AGENTS.md`, `CLAUDE.md`, the
golangci-lint config — and, when asked to, tests, commits and pushes them.

It is one of three:

| | |
| --- | --- |
| `devtool-engine` | the machinery nobody else would want changed: the task, the repository interface, the runner that applies a sequence of tasks, the event stream, the dependency graph, self-update |
| `devtool` | this repository — one owner's opinions, written as generators, plus the executable |
| `patchpal` | a long-running process that starts `devtool` for every run and renders its events into a Telegram chat |

The split exists so a merged change reaches the next run rather than the next
restart: `patchpal` invokes the binary fresh each time.

## Two ways in

`main.go` dispatches on the first word, matching a subcommand before parsing
flags so `devtool update` needs no dash. An empty command line means
`update`, which is the common case.

**`internal/local`** is the repository you are standing in. No GitHub, no
network, no commits: it writes the files and leaves the worktree dirty for
whoever ran it to read. With `-commit` it becomes a maintained run instead —
test, commit each task that changed something, push once — and refuses a
dirty worktree, because the engine's runner starts by discarding whatever it
finds uncommitted. That is correct for a checkout it owns on a server and
catastrophic for the one somebody is working in.

**`internal/run`** is the list. It opens every repository named in
`config.json`, reads each `go.mod` to work out which of them depend on which
others, orders them with the engine's dependency graph, and maintains them as
parallel as that order allows. It reports as events rather than to a person,
because whatever is watching is a separate process.

Both emit the same event stream, so one repository reports exactly like one
row of a full run.

## The generators

`maintain` holds them, one per file it owns, each an `engine.Task` with a
short label the event stream carries. They are deterministic: running one
twice produces the same bytes, which is what lets an unchanged worktree mean
"nothing to commit".

Every generated file opens with a marker naming the generator, matched by a
regular expression rather than an exact string, so the name can change
without every file already written reading as hand-written and being adopted
instead of regenerated.

`internal/lintgen` writes `maintain/lint.yaml` from a Go value, through
`go generate`. Never hand-edited.

## Versions, and how a stale build does damage

Each repository's `devtool.json` records the newest devtool build known to
have maintained it. A run compares itself against that mark and **refuses**
if it is behind.

This is a refusal rather than a warning because behind is not a worse
opinion, it is a different one: the older generators rewrite what the newer
ones wrote and the repository goes backwards. It has happened — a stale build
reverted twenty-five lines of this repository's own `Makefile`, `AGENTS.md`
and `.golangci.yaml`, undoing a merged change, while a warning sat in the log
doing nothing.

Three things about it are easy to get wrong, and each has cost real time.

### Reading the mark and writing it are different questions

`Newer` decides whether a build should *become* the mark, and exempts
anything carrying build metadata: a `+dirty` binary is one nobody else can
install, so recording it would tell every other machine it is behind
something unobtainable.

`Outdated` decides whether a build is too old to *write*, and does not
exempt it: the commit a dirty build came from orders it perfectly well, and
semver ignores the metadata when comparing anyway.

Conflating the two is what let the stale build through the first time the
refusal was written. `Installable` is the third piece — a local build cannot
be replaced by self-update, so it is refused with the remedy that suits it.

### The refusal has a branch that carries on, by design

A mark naming a build nobody published — a local version that leaked into the
file — must not lock the repository on every machine. So when the update
reports there is nothing newer to fetch, the run warns and proceeds.

That branch is correct and it is also a trap for tests: a test that records a
fictional future version and expects a refusal is asserting against the
branch that carries on. One did, and passed only on a machine whose build
happened to be `+dirty` and took the other path.

### The mark is a passenger

A run that changed nothing leaves it alone. A newer build with no different
opinion about a repository has nothing to say about it, and recording it
anyway would produce a commit whose only content is the mark — which in *this*
repository publishes a version for the next run to record, and so on without
end.

"Changed anything" is a before-and-after of `HEAD` plus the paths that differ
from it, which covers the commit run (clean worktree, `HEAD` moves) and the
plain rebuild (dirty worktree, no commits) alike.

## What the toolchain stamps, and why it differs per machine

Everything above rests on a binary knowing what it is, and that is decided by
how it was built:

| built by | reports |
| --- | --- |
| `go install …@latest` | the module version it fetched |
| `go build`, worktree clean | `v0.0.0-<time>-<commit>`, synthesised from the VCS stamp |
| `go build`, worktree dirty | the same, plus `+dirty` |
| `go build`, no VCS stamp | `(devel)` — no commit, no time, nothing to order |

The last row is not a choice. The toolchain omits the stamp **silently**
wherever it cannot read git; `go build -buildvcs=true` names the reason
instead of staying quiet.

The consequence worth remembering: **a test that drives a built binary
inherits whichever row that machine lands on.** Three separate mistakes in
one afternoon came from assuming one of them. Assertions about version
behaviour belong in `internal/local` and `maintain`, where the version is a
parameter and the updater is injected; a test at the binary level should
assert only what holds on every row.
