# What a maintained run commits

## Write inside a task, or it never lands

The engine commits each task's changes as it goes and pushes at the end.
Anything written into a repository after `runner.Update` returns is left in the
worktree — and the next run's `prepare` discards uncommitted changes before it
pulls. Nothing reports this. The file is written, the run succeeds, and the
write is gone six hours later.

`devtool.json` was written that way, by the service after the engine returned,
for every repository on every run. Not one of them ever landed; every
`devtool.json` in the portfolio was committed by hand. It is written inside
`maintain.UpdateTask` now. Anything new a run records about a repository goes
there too, or into a task of its own — never into `Service.maintain` after the
call.

## When a write starts landing, look at who reads it

Fixing the above nearly froze every coverage badge. `Service.spec` preferred
the definition's coverage over the portfolio list's, which was harmless while
the definition never landed. Once it did, the list's figure — the one a run
measures and writes back every time — would have been ignored for good.

A value that silently never persisted has readers that grew up around its
absence. Before making it persist, read every place that reads it.

## A test must be red on the code it is fixing

The first test written for this drove the local `-commit` path end to end and
passed. It would have passed before the fix too: that path already recorded
the version inside its own sequence. The lost write was the **portfolio**
path. A test that is green on the broken code proves nothing about the
break — check it goes red with the fix reverted, and that it runs through the
path that was actually broken. `internal/run/tasks_test.go` is the half that
does the second.
