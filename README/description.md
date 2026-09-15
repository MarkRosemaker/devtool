Brings Go repositories up to a standard and keeps them there: it writes the
README, the Makefile, the `.gitignore` and the `AGENTS.md`, updates the
dependencies and the tooling, formats and lints, and commits each kind of
change on its own so the history stays readable.

Run it in a repository to rebuild that one's generated files, or give it a list
to maintain every repository on it unattended. The machinery underneath is
[devtool-engine](https://github.com/MarkRosemaker/devtool-engine); what is here
is one owner's taste.
