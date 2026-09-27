package maintain

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func recordAndLoad(t *testing.T, repo *fakeRepo, opts UpdateOptions) Definition {
	t.Helper()

	if err := recordDefinition(repo, opts); err != nil {
		t.Fatal(err)
	}

	def, found, err := LoadDefinition(repo.Fs())
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatal("nothing was recorded")
	}

	return def
}

func TestKindIsInferredOnce(t *testing.T) {
	for _, tc := range []struct {
		name  string
		owner string
		files []string
		want  Kind
	}{
		{"nothing", "", []string{"lib.go"}, KindLibrary},
		{"a main package at the root", "", []string{"main.go"}, KindCLI},
		{"a command", "", []string{"cmd/tool/main.go"}, KindCLI},
		// A web app has a command to serve it, so frontend/ is asked first.
		{"a frontend", "", []string{"cmd/serve/main.go", "frontend/index.html"}, KindWebapp},
		// And an API library may have a command of its own.
		{"under go-api-libs", "go-api-libs", []string{"cmd/tool/main.go"}, KindAPILib},
		// A file of that name is not the directory.
		{"a file named cmd", "", []string{"cmd"}, KindLibrary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{owner: tc.owner}
			for _, f := range tc.files {
				writeFile(t, repo.Fs(), f, packageFor(f))
			}

			if got := recordAndLoad(t, repo, UpdateOptions{}).Kind; got != tc.want {
				t.Errorf("kind = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRecordedKindWins: once recorded, the file is the answer, which is what
// lets a wrong guess be fixed by editing one line.
func TestRecordedKindWins(t *testing.T) {
	repo := &fakeRepo{}
	writeFile(t, repo.Fs(), DefinitionPath, `{"kind": "library"}`)
	writeFile(t, repo.Fs(), "cmd/tool/main.go", "")

	if got := recordAndLoad(t, repo, UpdateOptions{}).Kind; got != KindLibrary {
		t.Errorf("kind = %q, want the recorded %q", got, KindLibrary)
	}
}

// TestPrivacyIsRecordedFromGitHub: a maintained run knows, a local one does
// not, and not knowing must not overwrite what was known.
func TestPrivacyIsRecordedFromGitHub(t *testing.T) {
	for _, tc := range []struct {
		name     string
		recorded string
		asked    bool
		private  bool
		want     bool
	}{
		{"asked, private", `{}`, true, true, true},
		{"asked, made public", `{"private": true}`, true, false, false},
		{"not asked keeps the record", `{"private": true}`, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{private: tc.private}
			writeFile(t, repo.Fs(), DefinitionPath, tc.recorded)

			opts := UpdateOptions{RecordPrivate: tc.asked}
			if got := recordAndLoad(t, repo, opts).Private; got != tc.want {
				t.Errorf("private = %v, want %v", got, tc.want)
			}

			// Public is the default, and a file saying so on every public
			// repository is noise.
			b, err := afero.ReadFile(repo.Fs(), DefinitionPath)
			if err != nil {
				t.Fatal(err)
			}

			if !tc.want && strings.Contains(string(b), "private") {
				t.Errorf("a public repository's definition mentions privacy:\n%s", b)
			}
		})
	}
}

func TestUnknownKindIsRefused(t *testing.T) {
	repo := &fakeRepo{}
	writeFile(t, repo.Fs(), DefinitionPath, `{"kind": "libary"}`)

	_, _, err := LoadDefinition(repo.Fs())
	if err == nil {
		t.Fatal("a misspelt kind was accepted")
	}

	if !strings.Contains(err.Error(), "library, cli, webapp, apilib") {
		t.Errorf("the error does not name the kinds there are: %v", err)
	}
}

// packageFor gives a Go file the package clause its place implies: main at
// the root only where it is called main.go.
func packageFor(name string) string {
	switch {
	case name == "main.go" || strings.HasPrefix(name, "cmd/"):
		return "package main\n"
	case strings.HasSuffix(name, ".go"):
		return "package thing\n"
	default:
		return ""
	}
}
