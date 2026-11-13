package maintain

import (
	"cmp"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"text/template"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const storesDir = "stores"

var (
	//go:embed store.go.tmpl
	storeText string

	//go:embed storetest.go.tmpl
	storetestText string

	storeTmpl     = template.Must(template.New("store.go").Parse(storeText))
	storetestTmpl = template.Must(template.New("storetest.go").Parse(storetestText))
)

// storeModules are what a store's code imports beyond the standard library:
// the S3 client and its credentials, whose own requirements bring the rest.
var storeModules = []string{"github.com/aws/aws-sdk-go-v2/service/s3", "github.com/aws/aws-sdk-go-v2/credentials"}

// StoresTask returns a task that writes the package each store devtool.json
// declares is kept in: stores/<name>/store.go, which opens the bucket from the
// environment and keeps files in it, and the package beside it a test opens
// one with, on a bucket of the test's own. Both are rewritten every run, so
// code of the repository's own goes in a file of its own beside them. Where
// go.mod requires no S3 client, the task adds one and tidies the module,
// vendoring it where the repository vendors.
func StoresTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "stores",
		Short: "stores",
		Run: func(ctx context.Context) error {
			def, _, err := LoadDefinition(repo.Fs())
			if err != nil {
				return err
			}

			added := false

			for _, st := range def.Resources.Stores {
				got, err := writeStore(ctx, repo, st, len(def.Resources.Stores) == 1)
				if err != nil {
					return fmt.Errorf("store %q: %w", st.Name, err)
				}

				added = added || got
			}

			if !added {
				return nil
			}

			return tidyModules(ctx, repo)
		},
	}
}

// bucketName is a bucket's name as S3 allows it.
var bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// writeStore writes the files of one store, reporting whether it added a
// module to go.mod.
func writeStore(ctx context.Context, repo engine.Repo, st Store, only bool) (bool, error) {
	if !databaseName.MatchString(st.Name) {
		return false, errors.New("a store's name has to be lower-case letters only, since it names the package")
	}

	bucket := cmp.Or(st.Bucket, st.Name)
	if !bucketName.MatchString(bucket) {
		return false, fmt.Errorf("bucket %q is no name S3 allows: 3 to 63 lower-case letters, digits, dots and hyphens", bucket)
	}

	fs := repo.Fs()

	module, err := modulePath(fs)
	if err != nil {
		return false, err
	}

	added := false

	for _, m := range storeModules {
		if ok, err := requires(fs, m); err != nil {
			return false, err
		} else if !ok {
			if err := goGet(ctx, repo, m); err != nil {
				return false, err
			}

			added = true
		}
	}

	dir, testPkg := path.Join(storesDir, st.Name), st.Name+"test"

	files := []struct {
		name string
		tmpl *template.Template
	}{
		{"store.go", storeTmpl},
		{path.Join(testPkg, testPkg+".go"), storetestTmpl},
	}

	data := struct{ Package, Module, Bucket, Env string }{st.Name, module, bucket, envStorage(repo.Name(), st.Name, only)}

	for _, f := range files {
		b, err := render(f.tmpl, data)
		if err != nil {
			return false, fmt.Errorf("%s: %w", f.name, err)
		}

		if err := fs.MkdirAll(path.Dir(path.Join(dir, f.name)), 0o755); err != nil {
			return false, err
		}

		if err := afero.WriteFile(fs, path.Join(dir, f.name), b, 0o644); err != nil {
			return false, err
		}
	}

	return added, nil
}

// envStorage begins the names of the environment variables a store is
// reached with: the repository's name, then the store's where it has more
// than one, as ATELIER_STORAGE or ATELIER_PICTURES_STORAGE.
func envStorage(repo, name string, only bool) string {
	return envKey(repo, name, only) + "_STORAGE"
}

// tidyModules tidies go.mod once what was added is imported, vendoring it
// where the repository vendors.
func tidyModules(ctx context.Context, repo engine.Repo) error {
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
