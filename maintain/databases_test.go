package maintain

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

// sqlcChildEnv makes this test binary stand in for "devtool sqlc": a run
// starts devtool, which a test cannot, and sqlc exits the process on an error,
// so it cannot run in the test's own.
const sqlcChildEnv = "DEVTOOL_TEST_SQLC"

func init() {
	name, ok := os.LookupEnv(sqlcChildEnv)
	if !ok {
		return
	}

	dir, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}

	os.Exit(GenerateQueries(dir, name))
}

// sqlcChild runs sqlc the way RunSQLC does: as a child, in the repository's
// directory, its output carried in the error.
func sqlcChild(dir string) SQLC {
	return func(ctx context.Context, _ engine.Repo, name string) error {
		cmd := exec.CommandContext(ctx, os.Args[0])
		cmd.Dir = dir

		cmd.Env = append(os.Environ(), sqlcChildEnv+"="+name)

		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}

		return nil
	}
}

// databaseRepo is a repository on disk declaring one database, user, with a
// table and two queries.
func databaseRepo(t *testing.T) (*fakeRepo, string) {
	t.Helper()

	fs, dir := realFs(t)

	for _, d := range []string{"databases/user/migrations", "databases/user/queries"} {
		if err := fs.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, fs, "go.mod", "module example.com/app\n\ngo 1.27\n\nrequire modernc.org/sqlite v1.0.0\n")
	writeFile(t, fs, DefinitionPath, `{"resources": {"databases": [{"name": "user"}]}}`+"\n")
	writeFile(t, fs, "databases/user/migrations/001_initial.up.sql",
		"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\n")
	writeFile(t, fs, "databases/user/migrations/001_initial.down.sql", "DROP TABLE users;\n")
	writeFile(t, fs, "databases/user/queries/users.sql",
		"-- name: GetUser :one\nSELECT * FROM users WHERE id = ?;\n")
	writeFile(t, fs, "databases/user/queries/names.sql",
		"-- name: ListNames :many\nSELECT name FROM users ORDER BY name;\n")

	return &fakeRepo{fs: fs}, dir
}

func TestDatabasesGeneratesTheCode(t *testing.T) {
	repo, dir := databaseRepo(t)

	if err := DatabasesTask(repo, sqlcChild(dir)).Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"db.go", "models.go", "users.sql.go", "names.sql.go", "migrate.go", "queries_test.go"} {
		if _, err := os.Stat(filepath.Join(dir, "databases/user", f)); err != nil {
			t.Errorf("no %s: %v", f, err)
		}
	}

	if got := readFile(t, repo.Fs(), "databases/user/migrate.go"); !strings.Contains(got, "package user\n") {
		t.Errorf("migrate.go is not in package user:\n%s", got)
	}

	test := readFile(t, repo.Fs(), "databases/user/queries_test.go")
	for _, want := range []string{
		`"getUser": getUser`, `"listNames": listNames`,
		`_ "modernc.org/sqlite"`, `sql.Open("sqlite"`,
	} {
		if !strings.Contains(test, want) {
			t.Errorf("queries_test.go lacks %q:\n%s", want, test)
		}
	}

	// The configuration is the run's own, never the repository's.
	matches, err := filepath.Glob(filepath.Join(dir, "**", "sqlc.*"))
	if err != nil {
		t.Fatal(err)
	}

	if root, _ := filepath.Glob(filepath.Join(dir, "sqlc.*")); len(matches)+len(root) > 0 {
		t.Errorf("a sqlc config was left in the repository: %v %v", matches, root)
	}
}

// TestDatabasesIsIdempotent: a second run with nothing changed writes the
// same bytes, or every maintained run would commit.
func TestDatabasesIsIdempotent(t *testing.T) {
	repo, dir := databaseRepo(t)
	task := DatabasesTask(repo, sqlcChild(dir))

	if err := task.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	before := snapshot(t, filepath.Join(dir, "databases/user"))

	if err := task.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	after := snapshot(t, filepath.Join(dir, "databases/user"))

	for f, b := range before {
		if after[f] != b {
			t.Errorf("%s changed on a second run", f)
		}
	}

	if len(after) != len(before) {
		t.Errorf("a second run left %d files, the first %d", len(after), len(before))
	}
}

// TestDatabasesDropsTheCodeOfAGoneQueryFile: sqlc never deletes what it
// wrote, so devtool does, and only what sqlc wrote.
func TestDatabasesDropsTheCodeOfAGoneQueryFile(t *testing.T) {
	repo, dir := databaseRepo(t)
	task := DatabasesTask(repo, sqlcChild(dir))

	if err := task.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	writeFile(t, repo.Fs(), "databases/user/user.go", "package user\n\n// Mine.\n")

	if err := repo.Fs().Remove("databases/user/queries/names.sql"); err != nil {
		t.Fatal(err)
	}

	if err := task.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "databases/user/names.sql.go")); !os.IsNotExist(err) {
		t.Errorf("names.sql.go outlived names.sql: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "databases/user/user.go")); err != nil {
		t.Errorf("a file of the repository's own went with it: %v", err)
	}

	if test := readFile(t, repo.Fs(), "databases/user/queries_test.go"); strings.Contains(test, "listNames") {
		t.Errorf("the test still prepares a query that has gone:\n%s", test)
	}
}

func TestDatabasesFailsOnABadQuery(t *testing.T) {
	repo, dir := databaseRepo(t)
	writeFile(t, repo.Fs(), "databases/user/queries/users.sql",
		"-- name: GetUser :one\nSELECT * FROM nonexistent WHERE id = ?;\n")

	err := DatabasesTask(repo, sqlcChild(dir)).Run(t.Context())
	if err == nil {
		t.Fatal("a query against a table that does not exist was accepted")
	}

	// Both what failed and why: sqlc's own message, not only its exit code.
	for _, want := range []string{`database "user"`, `relation "nonexistent" does not exist`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure does not say %s: %v", want, err)
		}
	}
}

func TestDatabasesRefusesANameThatIsNoPackage(t *testing.T) {
	repo, _ := databaseRepo(t)
	writeFile(t, repo.Fs(), DefinitionPath, `{"resources": {"databases": [{"name": "User-DB"}]}}`+"\n")

	err := DatabasesTask(repo, func(context.Context, engine.Repo, string) error {
		t.Error("sqlc ran for a name that cannot be a package")

		return nil
	}).Run(t.Context())
	if err == nil {
		t.Error("a name that cannot be a package was accepted")
	}
}

func TestDatabasesNeedsADriverForTheTest(t *testing.T) {
	repo, dir := databaseRepo(t)
	writeFile(t, repo.Fs(), "go.mod", "module example.com/app\n\ngo 1.27\n")

	err := DatabasesTask(repo, sqlcChild(dir)).Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "modernc.org/sqlite") {
		t.Errorf("want a failure naming a driver to add, got %v", err)
	}
}

func TestSQLiteDriverFollowsGoMod(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeFile(t, fs, "go.mod",
		"module example.com/app\n\ngo 1.27\n\nrequire github.com/ncruces/go-sqlite3 v0.35.5\n")

	driver, imports, err := sqliteDriver(fs)
	if err != nil {
		t.Fatal(err)
	}

	if driver != "sqlite3" || len(imports) == 0 || imports[0] != "github.com/ncruces/go-sqlite3/driver" {
		t.Errorf("ncruces gave %q %v", driver, imports)
	}
}

// TestDatabasesIsOwned: a run reports regenerated database code by name.
func TestDatabasesIsOwned(t *testing.T) {
	if got := describeUpdate([]string{"databases/user/users.sql.go"}); len(got) != 1 || got[0] != "databases" {
		t.Errorf("describeUpdate = %v, want [databases]", got)
	}
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()

	files := map[string]string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}

		files[e.Name()] = string(b)
	}

	return files
}

func TestDatabasesNeedsAnInitialMigration(t *testing.T) {
	repo, dir := databaseRepo(t)

	if err := repo.Fs().Remove("databases/user/migrations/001_initial.up.sql"); err != nil {
		t.Fatal(err)
	}

	err := DatabasesTask(repo, sqlcChild(dir)).Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "001_initial.up.sql") {
		t.Errorf("want a failure saying which migration to add, got %v", err)
	}
}
