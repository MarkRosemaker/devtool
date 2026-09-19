# Versions

`devtool` refuses to run where the repository records a newer build than the
one you are running, because the older generators would rewrite the newer
ones' output and the repository would go backwards. If you are told that,
you cannot work around it — install what it asks for.

## If a run refuses

Read which remedy it named. They are not interchangeable.

- **"run devtool self-update"** — a published build, which self-update can
  replace.
- **"it is a local build, so rebuild it from a current checkout"** —
  self-update declines to overwrite something nobody published, so
  `go install .` from an up-to-date checkout is the only way out.

## Before running it on anything

`devtool update` shells out from `make generate`, so an out-of-date binary
quietly reverts generated files as part of a target meant to keep them right.
The refusal above catches that now, but only where the build carries a
version to compare — see below. After pulling, `go install .` costs a second
and removes the question.

## What your binary reports is a property of your machine

| built by | reports |
| --- | --- |
| `go install …@latest` | the module version it fetched |
| `go build`, worktree clean | `v0.0.0-<time>-<commit>` from the VCS stamp |
| `go build`, worktree dirty | the same, plus `+dirty` |
| `go build`, no VCS stamp | `(devel)` — nothing to order against anything |

`devtool version` says which you have. The last row happens where the
toolchain cannot read git, and it says nothing about it;
`go build -buildvcs=true` names the reason.

**So the same commit behaves differently on different machines**, and a test
that drives a built binary inherits whichever row it lands on. Three
mistakes in one afternoon came from assuming one of them, including a test
that passed here and failed on macOS for a reason neither the test nor its
failure message could name.

Put assertions about version behaviour in `internal/local` or `maintain`,
where the version is a parameter and the updater is injected. A test at the
binary level should assert only what holds on every row — that a build behind
the mark does not lower it, for instance, rather than that it refuses.

## Two traps in the check itself

**It has a branch that carries on.** Where the update reports nothing newer
to fetch, the mark names a build nobody published, and refusing would lock
the repository on every machine — so it warns and proceeds. A test recording
a fictional future version is testing *that* branch, not the refusal.

**`selfupdate.Updater` needs `Current`.** Without it, it declines every
update saying it has nothing to compare, which makes the refusal unreachable
for exactly the builds that could have been replaced. Build one through
`selfUpdater` in `main.go` rather than by hand.

More detail, and why each of these is shaped the way it is, in
[docs/architecture.md](../docs/architecture.md).
