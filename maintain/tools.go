package maintain

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/mod/semver"
)

// Tool is a binary the Makefile shells out to, installed with go install
// at the version the repository's definition pins.
type Tool struct {
	// Name is the binary's, and the key it is pinned under.
	Name string

	// Package is what go install builds; Module is what it is versioned by.
	Package, Module string

	// NPM marks a package from npm instead, which make tools installs into
	// the repository's own directory: devtool pins its version, as Default
	// until a maintained run has asked npm for the latest.
	NPM     bool
	Default string
}

// houseTools are the tools every repository gets.
//
// Pinned rather than installed at whatever is latest, so every machine
// lints with the same rules and a new one arrives as a commit of its own —
// one that can be read, and reverted, rather than a failure nobody changed
// anything to cause. Maintained runs move the pins to the latest release.
//
// devtool is not among them: it pins itself, as devtoolVersion.
var houseTools = []Tool{
	{Name: "golangci-lint", Package: "github.com/golangci/golangci-lint/v2/cmd/golangci-lint", Module: "github.com/golangci/golangci-lint/v2"},
	{Name: "govulncheck", Package: "golang.org/x/vuln/cmd/govulncheck", Module: "golang.org/x/vuln"},
	{Name: "pkgsite", Package: "golang.org/x/pkgsite/cmd/pkgsite", Module: "golang.org/x/pkgsite"},
}

// sqlcTool generates the query code of a repository that declares databases.
var sqlcTool = Tool{Name: "sqlc", Package: "github.com/sqlc-dev/sqlc/cmd/sqlc", Module: "github.com/sqlc-dev/sqlc"}

// playwrightTool runs a web app's smoke test, from smoke/package.json.
var playwrightTool = Tool{Name: "playwright", Package: "playwright", NPM: true, Default: "1.63.0"}

// toolsFor are the tools a repository with this definition needs.
func toolsFor(def Definition) []Tool {
	tools := slices.Clone(houseTools)
	if len(def.Resources.Databases) > 0 {
		tools = append(tools, sqlcTool)
	}

	return tools
}

// npmToolsFor are the npm packages a repository with this definition pins.
func npmToolsFor(def Definition) []Tool {
	if hasSmoke(def) {
		return []Tool{playwrightTool}
	}

	return nil
}

// npmPin is the version of t the definition pins, or its default.
func npmPin(def Definition, t Tool) string {
	if v := def.Tools[t.Name]; v != "" {
		return v
	}

	return t.Default
}

// ToolOps is what pinning tools does on the machine: a test replaces it.
type ToolOps interface {
	// Installed is the version of the tool on PATH, or "" where there is
	// none or it was not built by go install.
	Installed(t Tool) string

	// Install builds the tool at version, which may be "latest".
	Install(ctx context.Context, t Tool, version string) error

	// Latest is the newest published version of the tool.
	Latest(ctx context.Context, t Tool) (string, error)
}

// pinTools brings the definition's pins and the installed tools into line,
// the way devtoolVersion and the running devtool are: a pin only moves
// forward, and an installed tool behind its pin is replaced.
//
//   - Installed newer than the pin: the pin moves up to it.
//   - Installed older, or missing: the pinned version is installed.
//   - checkLatest, on a maintained run: a newer release is installed, and so
//     pinned.
//
// Equal costs nothing and needs no network, so a sandboxed make ci passes
// where the tools are current. A pin for a tool no longer needed goes.
func pinTools(ctx context.Context, ops ToolOps, def Definition, checkLatest bool) (map[string]string, error) {
	pins := map[string]string{}

	for _, t := range toolsFor(def) {
		want := def.Tools[t.Name]

		if checkLatest {
			latest, err := ops.Latest(ctx, t)
			if err != nil {
				return nil, fmt.Errorf("asking for the latest %s: %w", t.Name, err)
			}

			want = newest(want, latest)
		}

		installed := ops.Installed(t)
		want = newest(want, installed)

		if installed != want || want == "" {
			version := want
			if version == "" {
				version = "latest"
			}

			if err := ops.Install(ctx, t, version); err != nil {
				return nil, fmt.Errorf("installing %s@%s: %w", t.Name, version, err)
			}

			// Another copy earlier on PATH would otherwise be pinned instead.
			got := ops.Installed(t)
			if got == "" || (version != "latest" && got != version) {
				return nil, fmt.Errorf("installed %s@%s, but the %s on PATH is not it",
					t.Name, version, t.Name)
			}

			want = got
		}

		pins[t.Name] = want
	}

	// An npm package is installed in the repository, by make tools, so only
	// its pin moves here: up, to the latest release on a maintained run.
	for _, t := range npmToolsFor(def) {
		want := npmPin(def, t)

		if checkLatest {
			latest, err := ops.Latest(ctx, t)
			if err != nil {
				return nil, fmt.Errorf("asking for the latest %s: %w", t.Name, err)
			}

			if newerNPM(latest, want) {
				want = latest
			}
		}

		pins[t.Name] = want
	}

	return pins, nil
}

// newerNPM reports whether npm version a is later than b. npm writes them
// without semver's leading "v"; anything unrecognised is not newer.
func newerNPM(a, b string) bool {
	va, vb := "v"+a, "v"+b

	return semver.IsValid(va) && (!semver.IsValid(vb) || semver.Compare(va, vb) > 0)
}

// newest is the later of two versions, either of which may be unknown.
func newest(a, b string) string {
	if !usable(b) {
		return a
	}

	if !usable(a) || Newer(b, a) {
		return b
	}

	return a
}

// SystemTools pins tools on this machine with the go command.
type SystemTools struct{}

// Installed reads the version go install stamped into the binary on PATH.
func (SystemTools) Installed(t Tool) string {
	path, err := exec.LookPath(t.Name)
	if err != nil {
		return ""
	}

	info, err := buildinfo.ReadFile(path)
	if err != nil || info.Main.Path != t.Module {
		return ""
	}

	return info.Main.Version
}

// Install runs go install with this machine's Go as the toolchain: a tool
// built with an older Go than the module it analyses targets refuses to run.
func (SystemTools) Install(ctx context.Context, t Tool, version string) error {
	goVersion, err := goEnv(ctx, "GOVERSION")
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "go", "install", t.Package+"@"+version)

	cmd.Env = append(os.Environ(), "GOTOOLCHAIN="+goVersion)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(out))
	}

	return nil
}

// Latest asks the module proxy, from outside any module: inside one with a
// vendor directory, go list refuses to query versions at all. An npm package
// it asks the npm registry, which needs no npm on the machine.
func (SystemTools) Latest(ctx context.Context, t Tool) (string, error) {
	if t.NPM {
		return latestNPM(ctx, t.Package)
	}

	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Version}}", t.Module+"@latest")
	cmd.Dir = os.TempDir()

	cmd.Env = append(os.Environ(), "GO111MODULE=on", "GOFLAGS=")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}

	return strings.TrimSpace(string(out)), nil
}

// UseToolBin puts the directory go install writes to at the front of PATH,
// for this process and everything it starts, so the tools it pins are the
// ones that run — the Makefile does the same for itself.
func UseToolBin(ctx context.Context) error {
	dir, err := goEnv(ctx, "GOBIN")
	if err != nil {
		return err
	}

	if dir == "" {
		gopath, err := goEnv(ctx, "GOPATH")
		if err != nil {
			return err
		}

		dir = filepath.Join(filepath.SplitList(gopath)[0], "bin")
	}

	return os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func goEnv(ctx context.Context, name string) (string, error) {
	out, err := exec.CommandContext(ctx, "go", "env", name).Output()
	if err != nil {
		return "", fmt.Errorf("go env %s: %w", name, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// SharedTools is one run's [ToolOps], for repositories maintained side by
// side: each latest version is asked for once, and installs take turns, so
// two repositories never write the same binary at the same time.
type SharedTools struct {
	ops ToolOps

	mu     sync.Mutex
	latest map[string]string
}

// NewSharedTools shares ops among the repositories of one run.
func NewSharedTools(ops ToolOps) *SharedTools {
	return &SharedTools{ops: ops, latest: map[string]string{}}
}

// Installed is ops' answer, read under the lock an install holds.
func (s *SharedTools) Installed(t Tool) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.ops.Installed(t)
}

// Install installs once: a repository that asks for what another has just
// installed finds it there.
func (s *SharedTools) Install(ctx context.Context, t Tool, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if version != "latest" && s.ops.Installed(t) == version {
		return nil
	}

	return s.ops.Install(ctx, t, version)
}

// Latest asks once per tool.
func (s *SharedTools) Latest(ctx context.Context, t Tool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.latest[t.Name]; ok {
		return v, nil
	}

	v, err := s.ops.Latest(ctx, t)
	if err != nil {
		return "", err
	}

	s.latest[t.Name] = v

	return v, nil
}

// npmRegistry is where npm's packages are published; a test points it at a
// server of its own.
var npmRegistry = "https://registry.npmjs.org"

// latestNPM is the version npm's "latest" tag names for pkg.
func latestNPM(ctx context.Context, pkg string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, npmRegistry+"/"+pkg+"/latest", nil)
	if err != nil {
		return "", err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the npm registry answered %s for %s", res.Status, pkg)
	}

	var latest struct {
		Version string `json:"version"`
	}

	if err := json.UnmarshalRead(res.Body, &latest); err != nil {
		return "", fmt.Errorf("reading the npm registry's answer for %s: %w", pkg, err)
	}

	if !semver.IsValid("v" + latest.Version) {
		return "", fmt.Errorf("the npm registry gave %s the version %q", pkg, latest.Version)
	}

	return latest.Version, nil
}
