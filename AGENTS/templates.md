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
existing goldens fail, restore it, confirm they pass again. `TestAgentsGolden`
in `maintain/agents_test.go` failed on all three cases, not just the new one,
which is what made the fix's reach obvious.

## Testing against the wrong binary

`make ready` and `make generate` shell out to the **installed** `devtool`,
not to the source tree you just edited. A template change — any of it, not
only the spacing kind above — is invisible to both until `go install .`
catches up: this repository's own `AGENTS.md` silently failed to pick up a
brand new section twice in one pull request, once for each edit to the
template. See [Versions](versions.md#before-running-it-on-anything) for the
fuller version of this trap.

Run `go install .` immediately after saving a `.tmpl` edit, before running
`make ready` even once — not after `make ready` looks wrong.
