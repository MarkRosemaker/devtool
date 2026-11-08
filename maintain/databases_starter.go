package maintain

import (
	"context"
	"fmt"
	"path"
	"strings"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

// The first migration and queries of a database declared with none, written
// so that the package generates and builds at once. They are an example, and
// say so: the tables are the database's own to write.
const (
	starterMigrationName = "001_initial.up.sql"
	starterQueriesName   = "notes.sql"

	starterMigrationHead = `-- The database's first state. This is an example, written by devtool when the
-- database was declared: replace the notes table with the database's own
-- tables, and queries/notes.sql with its own queries, before anything relies
-- on it. Once merged, a migration is never edited again: change the schema in
-- the next one, 002_<title>.up.sql.
`

	starterQueriesHead = `-- Queries in sqlc's syntax, one file per table. This is an example, written by
-- devtool with migrations/001_initial.up.sql: replace it with the database's
-- own queries. make generate turns each into a method on DB, named as the
-- comment above it says, returning :one row, :many rows, or nothing (:exec).
-- Keep these files ASCII, comments included.
`
)

var starterSQLite = [2]string{
	starterMigrationHead + `
CREATE TABLE notes (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
`,
	starterQueriesHead + `
-- name: CreateNote :one
INSERT INTO notes (title, body) VALUES (?, ?) RETURNING *;

-- name: GetNote :one
SELECT * FROM notes WHERE id = ?;

-- name: ListNotes :many
SELECT * FROM notes ORDER BY created_at DESC, id DESC;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = ?;
`,
}

var starterPostgres = [2]string{
	starterMigrationHead + `
CREATE TABLE notes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text NOT NULL,
    body text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
`,
	starterQueriesHead + `
-- name: CreateNote :one
INSERT INTO notes (title, body) VALUES ($1, $2) RETURNING *;

-- name: GetNote :one
SELECT * FROM notes WHERE id = $1;

-- name: ListNotes :many
SELECT * FROM notes ORDER BY created_at DESC, id DESC;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = $1;
`,
}

// writeStarter gives a database without migrations its example ones, and its
// example queries where it has none either.
func writeStarter(fs afero.Fs, dir, typ string) error {
	if up, err := afero.Glob(fs, path.Join(dir, "migrations", "*.up.sql")); err != nil || len(up) > 0 {
		return err
	}

	starter := starterSQLite
	if typ == postgres {
		starter = starterPostgres
	}

	if err := fs.MkdirAll(path.Join(dir, "migrations"), 0o755); err != nil {
		return err
	}

	if err := afero.WriteFile(fs, path.Join(dir, "migrations", starterMigrationName), []byte(starter[0]), 0o644); err != nil {
		return err
	}

	if queries, err := afero.Glob(fs, path.Join(dir, "queries", "*.sql")); err != nil || len(queries) > 0 {
		return err
	}

	if err := fs.MkdirAll(path.Join(dir, "queries"), 0o755); err != nil {
		return err
	}

	return afero.WriteFile(fs, path.Join(dir, "queries", starterQueriesName), []byte(starter[1]), 0o644)
}

// goGet adds module to go.mod, for the code devtool writes to import.
func goGet(ctx context.Context, repo engine.Repo, module string) error {
	if out, err := repo.ExecCommand(ctx, "go", "get", module); err != nil {
		return fmt.Errorf("go get %s: %w\n%s", module, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// settleModules tidies go.mod once the code importing what was added is
// written, and vendors it where the repository vendors.
func settleModules(ctx context.Context, repo engine.Repo) error {
	commands := [][]string{{"go", "mod", "tidy"}}

	if ok, err := afero.DirExists(repo.Fs(), "vendor"); err != nil {
		return err
	} else if ok {
		commands = append(commands, []string{"go", "mod", "vendor"})
	}

	for _, c := range commands {
		if out, err := repo.ExecCommand(ctx, c[0], c[1:]...); err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(c, " "), err, strings.TrimSpace(string(out)))
		}
	}

	return nil
}
