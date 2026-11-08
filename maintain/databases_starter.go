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
// say so: the tables are the database's own to write. The example table has
// a column of every type the generated code maps cleanly to Go, each saying
// which type that is.
const (
	starterMigrationName = "001_initial.up.sql"
	starterQueriesName   = "notes.sql"

	starterMigrationHead = `-- The database's first state. This is an example, written by devtool when the
-- database was declared: replace the notes table with the database's own
-- tables, and queries/notes.sql with its own queries, before anything relies
-- on it. Once merged, a migration is never edited again: change the schema in
-- the next one, 002_<title>.up.sql.
--
-- It has a column of each type that maps cleanly to Go, with the Go type
-- make generate gives it. Use these types and no others.
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
    id INTEGER PRIMARY KEY,                               -- int64
    title TEXT NOT NULL,                                  -- string
    body TEXT,                                            -- sql.NullString: NULL allowed
    pinned BOOLEAN NOT NULL DEFAULT FALSE,                -- bool
    score REAL,                                           -- sql.NullFloat64
    due DATE,                                             -- *civil.Date; civil.Date where NOT NULL
    remind_at DATETIME,                                   -- sql.NullTime
    attachment BLOB,                                      -- []byte, nil for NULL
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP -- time.Time, in UTC
);
`,
	starterQueriesHead + `
-- name: CreateNote :one
INSERT INTO notes (title, body, pinned, score, due, remind_at, attachment)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

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
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- int64
    public_id uuid NOT NULL DEFAULT gen_random_uuid(),  -- uuid.UUID; *uuid.UUID where NULL is allowed
    title text NOT NULL,                                -- string
    body text,                                          -- *string: NULL allowed, nil for NULL
    pinned boolean NOT NULL DEFAULT false,              -- bool
    priority integer NOT NULL DEFAULT 0,                -- int32; bigint for int64
    score double precision,                             -- *float64
    due date,                                           -- *civil.Date; civil.Date where NOT NULL
    remind_at timestamptz,                              -- *time.Time
    attachment bytea,                                   -- []byte
    created_at timestamptz NOT NULL DEFAULT now()       -- time.Time
);
`,
	starterQueriesHead + `
-- name: CreateNote :one
INSERT INTO notes (title, body, pinned, priority, score, due, remind_at, attachment)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetNote :one
SELECT * FROM notes WHERE id = $1;

-- name: ListNotes :many
SELECT * FROM notes ORDER BY created_at DESC, id DESC;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = $1;
`,
}

// writeStarter gives a database without migrations its example ones, and its
// example queries where it has none either, and reports whether it did.
func writeStarter(fs afero.Fs, dir, typ string) (bool, error) {
	if up, err := afero.Glob(fs, path.Join(dir, "migrations", "*.up.sql")); err != nil || len(up) > 0 {
		return false, err
	}

	starter := starterSQLite
	if typ == postgres {
		starter = starterPostgres
	}

	if err := fs.MkdirAll(path.Join(dir, "migrations"), 0o755); err != nil {
		return false, err
	}

	if err := afero.WriteFile(fs, path.Join(dir, "migrations", starterMigrationName), []byte(starter[0]), 0o644); err != nil {
		return false, err
	}

	if queries, err := afero.Glob(fs, path.Join(dir, "queries", "*.sql")); err != nil || len(queries) > 0 {
		return true, err
	}

	if err := fs.MkdirAll(path.Join(dir, "queries"), 0o755); err != nil {
		return false, err
	}

	return true, afero.WriteFile(fs, path.Join(dir, "queries", starterQueriesName), []byte(starter[1]), 0o644)
}

// goGet adds module to go.mod, for the code devtool writes to import.
func goGet(ctx context.Context, repo engine.Repo, module string) error {
	if out, err := repo.ExecCommand(ctx, "go", "get", module); err != nil {
		return fmt.Errorf("go get %s: %w\n%s", module, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// settleModules has sqlc generate the code a new database's queries become,
// whose imports go.mod then needs, such as civil for a date, and tidies
// go.mod once all of it is written, vendoring it where the repository
// vendors. Ahead of make generate, which runs sqlc too, as tidying after it
// would need the network every time.
func settleModules(ctx context.Context, repo engine.Repo) error {
	// Not with vendor/, which has yet to hold what was just added.
	commands := [][]string{{"go", "generate", "-mod=mod", "./" + databasesDir + "/..."}, {"go", "mod", "tidy"}}

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
