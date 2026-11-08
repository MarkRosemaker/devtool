# Editing a generator's template

`maintain/*.tmpl` are Go templates, embedded and rendered against a small
data struct built by reading the repository. `AGENTS.md.tmpl` is the one
worth most care: it is read into nearly every agent session.

## Two conditionals in sequence reserve two blank lines, not one

`{{ if .X }}...{{ end }}` sandwiched between two literal newlines renders a
single blank line whether `X` is true or false — that is by design, and it
is what makes one optional paragraph look right either way. Put a second
`{{ if .Y }}...{{ end }}` after it the same way, each bracketed by its own
newlines, and the two reserved blank lines do not merge: a repository where
both are false gets two blank lines where it should get one, and every other
combination is off by one somewhere too.

The fix is not a trim marker on the conditionals themselves — that moves
which combination breaks rather than fixing all of them. It is to remove the
newline *between* the two conditionals in the source, so the closing `{{ end
}}` of the first touches the opening `{{ if }}` of the second directly. Each
conditional still owns exactly one newline before its content and one after;
chaining them with no gap is what lets those newlines compose correctly
across every combination of true and false.

Do not trust this by reasoning about it. Render every combination of the
conditions involved and read the output before writing a test — a
`{{ range .Generated }}` loop already precedes these conditionals, so
"three conditions" is eight renderings to check, not three.

## Verify a golden test actually pins what changed

A golden file `go test -update` writes for you proves nothing by itself: it
just repeats whatever the template currently produces, bug included. Before
trusting one, break the change on purpose — revert the fix, confirm the
existing goldens fail, restore it, confirm they pass again. Which goldens fail
shows how far the change reaches.

## Testing against the wrong binary

`make ready` and `make generate` shell out to the **installed** `devtool`,
not to the source tree you just edited. A template change — any of it, not
only the spacing kind above — is invisible to both until `go install .`
catches up, and this repository's own `AGENTS.md` silently stays as it was.
See [Versions](versions.md#before-running-it-on-anything) for the fuller
version of this trap.

Run `go install .` immediately after saving a `.tmpl` edit, before running
`make ready` even once — not after `make ready` looks wrong.

## A go:generate line in a string runs too

`go generate` reads every line of a source file that starts with
`//go:generate`, string literals included, and runs it. A generator that
writes a directive into another repository keeps it split in its own source
(`"//go:" + "generate …"`), or this repository's `make generate` runs it
here, where sqlc fails for want of a config. `generateGo` in
`maintain/databases.go` is split that way.

## Go source held in a string is linted as prose

golangci-lint's `dupword` reads string literals, and `make ready` runs it with
`--fix`: Go source kept in a string, such as `return nil, nil, nil`, loses its
repeated word without a word of output, and the code it generates stops
compiling. Write such source so that no word repeats, and test the
rendered code itself, not only its signature.
