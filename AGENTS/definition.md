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
