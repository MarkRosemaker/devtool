# Changing devtool.json's schema

`maintain.Definition` is encoded with `encoding/json/v2`, whose `omitempty`
drops only empty JSON — `""`, `null`, `[]`, `{}`. It does not drop `false` or
`0`. A boolean tagged `omitempty` wrote `"private": false` into every public
repository's file; use `omitzero` for anything that is not a string, slice or
map.

A new field also goes into `Definition.equal`, or a run that changes only that
field leaves the file as it was.

`kind` is inferred once and never again, so a change to `inferKind` reaches
only repositories that have no `kind` yet. Every repository maintained since
it existed has one.

`resources.databases` names the repository's SQLite databases, which are
laid out as `docs/architecture.md` describes. Each name is also its
package's name, so it is lower-case letters only. `"encrypted": true` gives
it an `Open` that takes a key, and needs the ncruces driver. `tools` pins the
Makefile's tools. A run moves the pins itself and only forward, so a pin
lowered by hand is raised again by the next run on a machine with a newer
copy installed.
