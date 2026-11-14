package maintain

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"text/template"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
	"golang.org/x/mod/modfile"
)

// databasesDir holds one directory per declared database.
const databasesDir = "databases"

var (
	//go:embed migrate.go.tmpl
	migrateText string

	//go:embed open.go.tmpl
	openText string

	//go:embed migrate_postgres.go.tmpl
	migratePostgresText string

	//go:embed open_postgres.go.tmpl
	openPostgresText string

	//go:embed dbtest_postgres.go.tmpl
	dbtestPostgresText string

	//go:embed dbtest_server.go.tmpl
	dbtestServerText string

	migrateTmpl         = template.Must(template.New("migrate.go").Parse(migrateText))
	openTmpl            = template.Must(template.New("open.go").Parse(openText))
	migratePostgresTmpl = template.Must(template.New("migrate.go").Parse(migratePostgresText))
	openPostgresTmpl    = template.Must(template.New("open.go").Parse(openPostgresText))
	dbtestPostgresTmpl  = template.Must(template.New("dbtest.go").Parse(dbtestPostgresText))
	dbtestServerTmpl    = template.Must(template.New("server.go").Parse(dbtestServerText))
)

// The kinds of database devtool lays out.
const (
	sqlite   = "sqlite"
	postgres = "postgres"
)

// pgxModule is the driver a Postgres database is opened with.
const pgxModule = "github.com/jackc/pgx/v5"

// xzModule unpacks the Postgres builds a database's tests start.
const xzModule = "github.com/ulikunitz/xz"

// sqlcConfig is the sqlc configuration of one database, written beside it so
// that sqlc, run by go generate in that directory, finds it unasked.
const sqlcConfig = "# " + generatedBy + ` — edit devtool.json instead.
version: "2"
sql:
  - engine: sqlite
    schema: migrations
    queries: queries
    gen:
      go:
        package: %s
        out: .
        overrides:
          - db_type: "DATE"
            go_type:
              import: "cloud.google.com/go/civil"
              type: "Date"
          - db_type: "DATE"
            nullable: true
            go_type:
              import: "cloud.google.com/go/civil"
              type: "Date"
              pointer: true
`

// sqlcPostgresConfig is sqlc's configuration for a Postgres database.
const sqlcPostgresConfig = "# " + generatedBy + ` — edit devtool.json instead.
version: "2"
sql:
  - engine: postgresql
    schema: migrations
    queries: queries
    gen:
      go:
        package: %s
        out: .
        sql_package: pgx/v5
        emit_pointers_for_null_types: true
        emit_interface: true
        overrides:
          - db_type: "date"
            go_type:
              import: "cloud.google.com/go/civil"
              type: "Date"
          - db_type: "date"
            nullable: true
            go_type:
              import: "cloud.google.com/go/civil"
              type: "Date"
              pointer: true
          - db_type: "timestamptz"
            go_type:
              type: "time.Time"
          - db_type: "timestamptz"
            nullable: true
            go_type:
              type: "time.Time"
              pointer: true
          - db_type: "uuid"
            go_type:
              import: "uuid"
              type: "UUID"
          - db_type: "uuid"
            nullable: true
            go_type:
              import: "uuid"
              type: "UUID"
              pointer: true
`

// generateGo is the directive that runs sqlc, from the database's directory.
// Split, because go generate reads a directive at the start of any line of
// a source file, string literals included, and would run this one here.
const generateGo = goGenerated + "\n\npackage %s\n\n" +
	"//go:" + "generate sqlc generate\n"

// DatabasesTask writes the files that go with the databases a repository
// declares in devtool.json. A database named foo lives in databases/foo,
// package foo, from two things written by hand:
//
//	migrations/<version>_<title>.up.sql   the schema; the first one is its initial state
//	queries/*.sql                         sqlc queries
//
// Beside them it writes sqlc.yaml and generate.go, so go generate runs sqlc
// there; migrate.go, which embeds the migrations and applies them; and
// open.go, which opens a database file and migrates it. sqlc's own output
// comes from go generate, which make generate runs after this.
//
// A database declared with no migrations gets an example of each, so that
// declaring it is all it takes to generate and build: see [writeStarter].
// Where go.mod requires no driver to open it with, the task adds one and
// tidies the module, vendoring it where the repository vendors.
func DatabasesTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "databases",
		Short: "databases",
		Run: func(ctx context.Context) error {
			def, _, err := LoadDefinition(repo.Fs())
			if err != nil {
				return err
			}

			added := false

			for _, db := range def.Resources.Databases {
				got, err := writeDatabase(ctx, repo, db, len(def.Resources.Databases) == 1)
				if err != nil {
					return fmt.Errorf("database %q: %w", db.Name, err)
				}

				added = added || got
			}

			if !added {
				return nil
			}

			return settleModules(ctx, repo)
		},
	}
}

var databaseName = regexp.MustCompile(`^[a-z]+$`)

// pathNames are the names open.go's Path uses, which its argument cannot share.
var pathNames = []string{"err", "filepath", "home", "os"}

// writeDatabase writes one database's files, and reports whether it added a
// driver to go.mod or an example whose code imports more, which then needs
// tidying once they are written.
func writeDatabase(ctx context.Context, repo engine.Repo, db Database, only bool) (bool, error) {
	name := db.Name
	if !databaseName.MatchString(name) {
		return false, errors.New("a database's name has to be lower-case letters only, since it names the package")
	}

	if db.Per != "" && (!databaseName.MatchString(db.Per) || token.IsKeyword(db.Per) || slices.Contains(pathNames, db.Per)) {
		return false, fmt.Errorf("per %q cannot name Path's argument: it has to be lower-case letters, "+
			"and neither a Go keyword nor one of %s", db.Per, strings.Join(pathNames, ", "))
	}

	fs, dir := repo.Fs(), path.Join(databasesDir, name)

	switch db.Type {
	case sqlite, postgres:
	default:
		return false, fmt.Errorf("type %q is no kind of database devtool knows: %s or %s", db.Type, sqlite, postgres)
	}

	if db.Type == postgres {
		switch {
		case db.Per != "":
			return false, errors.New("a Postgres database is one server's, reached at its address: it has no copy per anything")
		case db.Encrypted:
			return false, errors.New("a Postgres database is encrypted by its server, not opened with a key")
		}
	}

	started, err := writeStarter(fs, dir, db.Type)
	if err != nil {
		return false, err
	}

	if err := removeOrphans(fs, dir); err != nil {
		return false, err
	}

	if db.Type == postgres {
		added := started

		// pgx opens it, and xz unpacks the Postgres its tests run against.
		for _, module := range []string{pgxModule, xzModule} {
			if ok, err := requires(fs, module); err != nil {
				return false, err
			} else if !ok {
				if err := goGet(ctx, repo, module); err != nil {
					return false, err
				}

				added = true
			}
		}

		return added, writePostgres(fs, dir, db, envURL(repo.Name(), db.Name, only))
	}

	driver, imports, err := sqliteDriver(fs)
	added := started

	switch {
	case errors.Is(err, errNoDriver):
		// The one that can encrypt where it has to, else the one in pure Go.
		module := "modernc.org/sqlite"
		if db.Encrypted {
			module = ncrucesModule
		}

		d := sqliteDrivers[slices.IndexFunc(sqliteDrivers, func(d sqliteDriverInfo) bool { return d.module == module })]

		if err := goGet(ctx, repo, d.module); err != nil {
			return false, err
		}

		driver, imports, added = d.driver, d.imports, true
	case err != nil:
		return false, err
	}

	if db.Encrypted {
		if driver != ncrucesDriver {
			return false, fmt.Errorf("an encrypted database needs %s, which can encrypt; "+
				"modernc.org/sqlite cannot", ncrucesModule)
		}

		imports = append(imports, ncrucesModule+"/vfs/adiantum")
	}

	files := []struct {
		name string
		body func() ([]byte, error)
	}{
		{"sqlc.yaml", func() ([]byte, error) { return fmt.Appendf(nil, sqlcConfig, name), nil }},
		{"generate.go", func() ([]byte, error) { return fmt.Appendf(nil, generateGo, name), nil }},
		{"migrate.go", func() ([]byte, error) {
			return render(migrateTmpl, struct{ Package string }{name})
		}},
		{"open.go", func() ([]byte, error) {
			return render(openTmpl, struct {
				Package, Repo, Driver, Per string
				Imports                    []string
				Encrypted                  bool
			}{name, repo.Name(), driver, db.Per, imports, db.Encrypted})
		}},
	}

	for _, f := range files {
		b, err := f.body()
		if err != nil {
			return false, fmt.Errorf("%s: %w", f.name, err)
		}

		if err := afero.WriteFile(fs, path.Join(dir, f.name), b, 0o644); err != nil {
			return false, err
		}
	}

	return added, nil
}

// writePostgres writes the files of a Postgres database: it is reached at an
// address given when it is opened, so it has no path, nor a copy per
// anything, and the server encrypts what it keeps. Beside them is the package
// a test opens it with.
func writePostgres(fs afero.Fs, dir string, db Database, env string) error {
	module, err := modulePath(fs)
	if err != nil {
		return err
	}

	testPkg := db.Name + "test"

	files := map[string]func() ([]byte, error){
		path.Join(testPkg, testPkg+".go"): func() ([]byte, error) {
			return render(dbtestPostgresTmpl, struct{ Package, Module string }{db.Name, module})
		},
		path.Join(testPkg, "server.go"): func() ([]byte, error) {
			b, err := render(dbtestServerTmpl, struct {
				Package, PostgresVersion string
				Builds                   []postgresBuild
			}{db.Name, testPostgresVersion, testPostgresBuilds})
			if err != nil {
				return nil, err
			}

			// gofmt aligns the builds by the longest platform.
			return format.Source(b)
		},
		"sqlc.yaml":   func() ([]byte, error) { return fmt.Appendf(nil, sqlcPostgresConfig, db.Name), nil },
		"generate.go": func() ([]byte, error) { return fmt.Appendf(nil, generateGo, db.Name), nil },
		"migrate.go":  func() ([]byte, error) { return render(migratePostgresTmpl, struct{ Package string }{db.Name}) },
		"open.go": func() ([]byte, error) {
			return render(openPostgresTmpl, struct{ Package, Env string }{db.Name, env})
		},
	}

	for _, name := range slices.Sorted(maps.Keys(files)) {
		b, err := files[name]()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		if err := fs.MkdirAll(path.Dir(path.Join(dir, name)), 0o755); err != nil {
			return err
		}

		if err := afero.WriteFile(fs, path.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}

	return nil
}

// modulePath is the module go.mod declares.
func modulePath(fs afero.Fs) (string, error) {
	b, err := afero.ReadFile(fs, "go.mod")
	if err != nil {
		return "", err
	}

	mod, err := modfile.ParseLax("go.mod", b, nil)
	if err != nil {
		return "", err
	}

	if mod.Module == nil {
		return "", errors.New("go.mod declares no module")
	}

	return mod.Module.Mod.Path, nil
}

// testPostgresVersion is the Postgres a test runs against: the major version
// the live databases run (Neon's default, 17), as zonky builds it.
const testPostgresVersion = "17.11.0"

// postgresBuild is zonky's build of testPostgresVersion for one platform, on
// Maven Central, and the SHA-256 of its archive.
type postgresBuild struct{ Platform, Artifact, Sum string }

// testPostgresBuilds are the platforms a test can start Postgres on. Windows
// is not among them: see docs/roadmap.md.
var testPostgresBuilds = []postgresBuild{
	{"darwin/amd64", "darwin-amd64", "d464ff178e9860ba204662ac23fa547504b7fd392392ff2fb92e3fd73b5bdb64"},
	{"darwin/arm64", "darwin-arm64v8", "a1c2786acb0c398f9b2d76806fc52f5dc8b222cbc8e9383a9b9702084daaf3a5"},
	{"linux/amd64", "linux-amd64", "0dd7b72b6f335b8ecfb355fa24c5781e8a93edd09880bb77eb52ebbf29b3e96d"},
	{"linux/arm64", "linux-arm64v8", "8b042e0ea418b1927d95399207950da0734d27fe01e29503ade3bdc464ad4c2b"},
}

// envURL is the environment variable a Postgres database's URL is read from:
// the repository's name, then the database's where it has more than one, as
// LIFEFLOW_DATABASE_URL or LIFEFLOW_TASKS_DATABASE_URL.
func envURL(repo, name string, only bool) string {
	return envKey(repo, name, only) + "_DATABASE_URL"
}

// envKey begins the name of an environment variable a resource is reached
// with: the repository's name, and the resource's where there is more than one
// of its kind, upper-cased, with anything but a letter or digit an underscore.
func envKey(repo, name string, only bool) string {
	parts := []string{repo}
	if !only {
		parts = append(parts, name)
	}

	return strings.ToUpper(strings.Map(func(r rune) rune {
		if 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' {
			return r
		}

		return '_'
	}, strings.Join(parts, "_")))
}

// requires reports whether go.mod requires module. Indirect counts: there is
// one Postgres driver to choose, and go get marks it indirect until the code
// devtool writes imports it.
func requires(fs afero.Fs, module string) (bool, error) {
	b, err := afero.ReadFile(fs, "go.mod")
	if err != nil {
		return false, err
	}

	mod, err := modfile.ParseLax("go.mod", b, nil)
	if err != nil {
		return false, err
	}

	return slices.ContainsFunc(mod.Require, func(r *modfile.Require) bool { return r.Mod.Path == module }), nil
}

// sqlcSource finds the query file a file sqlc generated was made from.
var sqlcSource = regexp.MustCompile(`(?m)^// source: (\S+\.sql)$`)

// removeOrphans deletes the code sqlc generated from a query file that has
// since gone, which sqlc never does itself. Only that: what it generated
// from a file still there is its own to rewrite, and devtool may run where
// sqlc does not follow, which must not leave the package without it.
func removeOrphans(fs afero.Fs, dir string) error {
	files, err := afero.Glob(fs, path.Join(dir, "*.sql.go"))
	if err != nil {
		return err
	}

	for _, f := range files {
		b, err := afero.ReadFile(fs, f)
		if err != nil {
			return err
		}

		if !bytes.HasPrefix(b, []byte("// Code generated by sqlc.")) {
			continue
		}

		m := sqlcSource.FindSubmatch(b)
		if m == nil {
			continue
		}

		if _, err := fs.Stat(path.Join(dir, "queries", string(m[1]))); errors.Is(err, os.ErrNotExist) {
			if err := fs.Remove(f); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	return nil
}

// ncrucesModule is the one driver that can encrypt, and ncrucesDriver the
// name it registers.
const (
	ncrucesModule = "github.com/ncruces/go-sqlite3"
	ncrucesDriver = "sqlite3"
)

// sqliteDriverInfo is one driver: its module, the name it registers, and what
// has to be imported for it.
type sqliteDriverInfo struct {
	module, driver string
	imports        []string
}

// sqliteDrivers are the database/sql drivers open.go knows, by module: the
// name each registers, and what has to be imported for it.
var sqliteDrivers = []sqliteDriverInfo{
	{"modernc.org/sqlite", "sqlite", []string{"modernc.org/sqlite"}},
	{ncrucesModule, ncrucesDriver, []string{ncrucesModule + "/driver"}},
}

// sqliteDriver picks the driver the repository requires directly, so open.go
// adds no dependency of its own.
func sqliteDriver(fs afero.Fs) (string, []string, error) {
	b, err := afero.ReadFile(fs, "go.mod")
	if err != nil {
		return "", nil, err
	}

	mod, err := modfile.ParseLax("go.mod", b, nil)
	if err != nil {
		return "", nil, err
	}

	for _, d := range sqliteDrivers {
		for _, r := range mod.Require {
			// Indirect is what some dependency uses, not this repository.
			if r.Mod.Path == d.module && !r.Indirect {
				return d.driver, d.imports, nil
			}
		}
	}

	names := make([]string, len(sqliteDrivers))
	for i, d := range sqliteDrivers {
		names[i] = d.module
	}

	return "", nil, fmt.Errorf("%w: %s", errNoDriver, strings.Join(names, " or "))
}

// errNoDriver is go.mod requiring none of the SQLite drivers open.go knows.
var errNoDriver = errors.New("go.mod requires no SQLite driver to open it with")

func render(tmpl *template.Template, data any) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// databaseNames are the names of the databases def declares of type typ,
// or of any type where typ is empty.
func databaseNames(def Definition, typ string) []string {
	var names []string
	for _, db := range def.Resources.Databases {
		if typ == "" || db.Type == typ {
			names = append(names, db.Name)
		}
	}

	return names
}
