# Architecture

How this repository is put together, and the parts of it that have caught
somebody out.

## What it is

`devtool` brings Go repositories up to a standard and keeps them there. It
writes the files an owner would otherwise write by hand — `LICENSE`,
`README.md`, `Makefile`, `.gitignore`, `AGENTS.md`, the golangci-lint config
— and, when asked to, tests, commits and pushes them.

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
parallel as that order allows.

Both emit the same event stream, so one repository reports exactly like one
row of a full run. Who reads it is the caller's choice. By default
`internal/console` renders it for a person — a line per change, push and
failure, with plain-text logs at warning and above. With `-json` it goes onto
stdout as JSON Lines and the logs to stderr as JSON, which is what `patchpal`
asks for. `-jsonl` is an alias for `-json`, and a caller can use either.

## The generators

`maintain` holds them, one per file devtool owns, and `UpdateTask` runs them
all as a single `engine.Task` named `devtool update` — `devtool.json` first,
then licence, README, Makefile, `.gitignore`, `AGENTS.md` and the lint config,
with the `CLAUDE.md` link to `AGENTS.md` it used to make removed. That one task is the whole of a local rebuild and the
first task of a maintained run, so the two paths cannot disagree about what
devtool owns, and a maintained run commits all of it as one commit.

`devtool.json` goes first because the generators read it. It says what kind of
repository this is — `library`, `cli` (a command, library or not), `webapp` or
`apilib` — inferred once from the owner `go-api-libs`, `frontend/`, `cmd/` or
a root `main` package where it does not say yet, and authoritative from then
on. It also says whether the repository is private: a maintained run records
GitHub's answer, and a local run, which has nobody to ask, reads it back. That
is why the generated Makefile runs plain `devtool update`; privacy once rode
on it as `-private`, and a Makefile generated without the flag rebuilt a
private repository's README as a public one's. The flag is still accepted, for
the Makefiles that pass it, and recorded.

One commit, but not one name. The task tells the runner which of its parts a
commit came from, by the files it changed, so a run's table reads
`definition, readme → pushed` rather than `update → pushed`. The map is
`owners` in `maintain/update.go`: a generator that writes a new file needs an
entry there, or its changes are reported only as `update`.

They are deterministic: running one twice produces the same bytes, which is
what lets an unchanged worktree mean "nothing to commit".

**Anything a run writes into a repository has to be written inside a task.**
The engine commits a task's changes as it goes and pushes at the end; a file
written after that is left in the worktree, and the next run's `prepare`
discards it before pulling. `devtool.json` was written that way — by the
service, after the engine returned — for every repository on every run, and
not one of them ever landed. It is written inside `UpdateTask` now.

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

Where the build is a published one, the refusal comes after an attempt to
put it right. The run installs the newer build and starts it as a child
with the same arguments and streams, and that child's exit status becomes
the run's. It is marked by `DEVTOOL_RESTARTED`, so a build still behind
after one handover refuses instead of updating again. `-check-latest` hands
over the same way. Only a local build, a failed update, or one that cannot
say where it installed ends in "run the command again".

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

"Changed anything" is the paths that differ from `HEAD`, read before and
after the generators run inside `UpdateTask`. The engine commits only after
the task returns, so within it `HEAD` never moves: a run that starts clean —
every maintained run, and a local one with `-commit` — is judged exactly. A
plain rebuild can start dirty, and there a generator rewriting a file it had
already left dirty would not register; that would mean the generators are not
deterministic, which is a fault of its own.

Coverage is recorded alongside, and there the portfolio's list wins over the
definition: the list holds what the last run measured, written back after
every run, while `devtool.json` holds a copy. Preferring the copy would freeze
every badge at the first figure recorded.

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
