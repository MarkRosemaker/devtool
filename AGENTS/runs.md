# What a maintained run commits

## Write inside a task, or it never lands

The engine commits each task's changes as it goes and pushes at the end.
Anything written into a repository after `runner.Update` returns is left in the
worktree — and the next run's `prepare` discards uncommitted changes before it
pulls. Nothing reports this. The file is written, the run succeeds, and the
write is gone six hours later.

So `devtool.json` is written inside `maintain.UpdateTask`. Anything new a run
records about a repository goes there too, or into a task of its own — never
into `Service.maintain` after the call.

## Before making a write land, read who reads it

A value that never persisted may have readers that rely on its absence. Before
making it persist, read every place that reads it. `Service.spec` takes
coverage from the portfolio list rather than the definition for this reason:
the list holds what the last run measured, and preferring the definition's
copy would freeze every badge.

## Test the path that loses the write

A test of a lost write has to run through the path that loses it. The local
`-commit` path records within its own sequence and never lost one; the
portfolio path did, and `internal/run/tasks_test.go` covers it. Check the test
goes red with the fix reverted, or it proves nothing about the break.
