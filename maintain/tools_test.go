package maintain

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeTools is a machine's tools: what is installed, what the proxy calls
// latest, and every install asked of it.
type fakeTools struct {
	mu        sync.Mutex
	installed map[string]string
	latest    map[string]string
	installs  []string
	asked     int

	// shadowed keeps an install from reaching PATH, as an older copy
	// earlier on it would.
	shadowed bool
}

func (f *fakeTools) Installed(t Tool) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.installed[t.Name]
}

func (f *fakeTools) Install(_ context.Context, t Tool, version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.installs = append(f.installs, t.Name+"@"+version)

	if f.shadowed {
		return nil
	}

	if version == "latest" {
		version = f.latest[t.Name]
	}

	f.installed[t.Name] = version

	return nil
}

func (f *fakeTools) Latest(_ context.Context, t Tool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.asked++

	v, ok := f.latest[t.Name]
	if !ok {
		return "", errors.New("offline")
	}

	return v, nil
}

// every pins all house tools at v.
func every(v string) map[string]string {
	m := map[string]string{}
	for _, t := range houseTools {
		m[t.Name] = v
	}

	return m
}

func TestPinTools(t *testing.T) {
	const lint = "golangci-lint"

	for _, tc := range []struct {
		name         string
		pins         map[string]string
		installed    map[string]string
		latest       map[string]string
		checkLatest  bool
		wantPin      string
		wantInstalls []string
	}{
		{
			name: "equal costs nothing",
			pins: every("v1.0.0"), installed: every("v1.0.0"),
			wantPin: "v1.0.0",
		},
		{
			name: "an installed tool newer than its pin moves the pin",
			pins: every("v1.0.0"), installed: with(every("v1.0.0"), lint, "v1.2.0"),
			wantPin: "v1.2.0",
		},
		{
			name: "an installed tool behind its pin is replaced",
			pins: every("v1.0.0"), installed: with(every("v1.0.0"), lint, "v0.9.0"),
			wantPin: "v1.0.0", wantInstalls: []string{lint + "@v1.0.0"},
		},
		{
			name: "a missing tool is installed at its pin",
			pins: every("v1.0.0"), installed: without(every("v1.0.0"), lint),
			wantPin: "v1.0.0", wantInstalls: []string{lint + "@v1.0.0"},
		},
		{
			name: "an unpinned tool is pinned at what is installed",
			pins: without(every("v1.0.0"), lint), installed: every("v1.0.0"),
			wantPin: "v1.0.0",
		},
		{
			name: "neither pinned nor installed: latest",
			pins: without(every("v1.0.0"), lint), installed: without(every("v1.0.0"), lint),
			latest:  map[string]string{lint: "v2.0.0"},
			wantPin: "v2.0.0", wantInstalls: []string{lint + "@latest"},
		},
		{
			name: "a maintained run moves to a newer release",
			pins: every("v1.0.0"), installed: every("v1.0.0"),
			latest: with(every("v1.0.0"), lint, "v1.3.0"), checkLatest: true,
			wantPin: "v1.3.0", wantInstalls: []string{lint + "@v1.3.0"},
		},
		{
			name: "a pin never moves back, even to what the proxy calls latest",
			pins: every("v1.0.0"), installed: every("v1.0.0"),
			latest: with(every("v1.0.0"), lint, "v0.9.0"), checkLatest: true,
			wantPin: "v1.0.0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := &fakeTools{installed: maps.Clone(tc.installed), latest: tc.latest}

			pins, err := pinTools(t.Context(), ops, Definition{Tools: tc.pins}, tc.checkLatest)
			if err != nil {
				t.Fatal(err)
			}

			if pins[lint] != tc.wantPin {
				t.Errorf("pin = %q, want %q", pins[lint], tc.wantPin)
			}

			if !slices.Equal(ops.installs, tc.wantInstalls) {
				t.Errorf("installs = %v, want %v", ops.installs, tc.wantInstalls)
			}

			if !tc.checkLatest && tc.latest == nil && ops.asked > 0 {
				t.Errorf("asked the network %d times without being told to", ops.asked)
			}
		})
	}
}

// TestPinToolsRefusesAShadowedInstall: an install that does not reach PATH
// must not pin whatever older copy answers instead.
func TestPinToolsRefusesAShadowedInstall(t *testing.T) {
	ops := &fakeTools{installed: with(every("v1.0.0"), "golangci-lint", "v0.9.0"), shadowed: true}

	if _, err := pinTools(t.Context(), ops, Definition{Tools: every("v1.0.0")}, false); err == nil {
		t.Error("pinned a tool whose install never reached PATH")
	}
}

// TestPinToolsDropsAPinNoLongerNeeded keeps devtool.json to what is used.
func TestPinToolsDropsAPinNoLongerNeeded(t *testing.T) {
	ops := &fakeTools{installed: every("v1.0.0")}

	pins, err := pinTools(t.Context(), ops,
		Definition{Tools: with(every("v1.0.0"), "retired", "v1.0.0")}, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := pins["retired"]; ok {
		t.Errorf("kept the pin of a tool nothing uses: %v", pins)
	}
}

// TestSharedToolsAsksAndInstallsOnce: repositories maintained side by side
// ask for each latest version once, and do not install the same binary twice.
func TestSharedToolsAsksAndInstallsOnce(t *testing.T) {
	ops := &fakeTools{installed: every("v1.0.0"), latest: with(every("v1.0.0"), "golangci-lint", "v1.3.0")}
	shared := NewSharedTools(ops)

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			if _, err := pinTools(t.Context(), shared, Definition{Tools: every("v1.0.0")}, true); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	if ops.asked != len(houseTools) {
		t.Errorf("asked for the latest versions %d times, want once per tool (%d)", ops.asked, len(houseTools))
	}

	if len(ops.installs) != 1 {
		t.Errorf("installs = %v, want golangci-lint once", ops.installs)
	}
}

// TestUpdateRecordsThePins: the pins land in devtool.json, inside the task.
func TestUpdateRecordsThePins(t *testing.T) {
	// Private, so the licence does not ask git for its first year.
	repo := &fakeRepo{private: true}
	writeFile(t, repo.Fs(), "go.mod", "module example.com/m\n\ngo 1.27\n")

	ops := &fakeTools{installed: every("v1.0.0")}
	if err := UpdateTask(repo, UpdateOptions{Tools: ops}).Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	def, _, err := LoadDefinition(repo.Fs())
	if err != nil {
		t.Fatal(err)
	}

	if !maps.Equal(def.Tools, every("v1.0.0")) {
		t.Errorf("devtool.json pins %v", def.Tools)
	}

	// make tools installs the latest rather than the pin, so it never takes a
	// machine back; the next update moves the pin up to it.
	if mk := readFile(t, repo.Fs(), makefilePath); !strings.Contains(mk, "golangci-lint@latest") {
		t.Errorf("the tools target does not install the latest:\n%s", mk)
	}
}

func with(m map[string]string, k, v string) map[string]string {
	m = maps.Clone(m)
	m[k] = v

	return m
}

func without(m map[string]string, k string) map[string]string {
	m = maps.Clone(m)
	delete(m, k)

	return m
}

// A web app pins Playwright beside its Go tools: at the default until a
// maintained run asks npm, then at the latest, never back, and never
// installed here, since make tools installs it into smoke/.
func TestPinToolsPlaywright(t *testing.T) {
	webapp := func(pins map[string]string) Definition {
		return Definition{Kind: KindWebapp, Webapp: &Webapp{Port: 8080}, Tools: pins}
	}

	installed := every("v1.0.0")

	for _, tc := range []struct {
		name        string
		def         Definition
		latest      string
		checkLatest bool
		want        string
	}{
		{name: "unpinned, without asking: the default", def: webapp(every("v1.0.0")), want: playwrightTool.Default},
		{name: "pinned, without asking: the pin", def: webapp(with(every("v1.0.0"), "playwright", "1.64.0")), want: "1.64.0"},
		{
			name: "a maintained run moves to the latest", def: webapp(with(every("v1.0.0"), "playwright", "1.63.0")),
			latest: "1.65.1", checkLatest: true, want: "1.65.1",
		},
		{
			name: "and never back", def: webapp(with(every("v1.0.0"), "playwright", "1.65.1")),
			latest: "1.64.0", checkLatest: true, want: "1.65.1",
		},
		{
			name: "compared as versions, not text", def: webapp(with(every("v1.0.0"), "playwright", "1.9.0")),
			latest: "1.10.0", checkLatest: true, want: "1.10.0",
		},
		{name: "not a web app: no pin", def: Definition{Tools: with(every("v1.0.0"), "playwright", "1.63.0")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			latest := with(every("v1.0.0"), "playwright", tc.latest)
			ops := &fakeTools{installed: maps.Clone(installed), latest: latest}

			pins, err := pinTools(t.Context(), ops, tc.def, tc.checkLatest)
			if err != nil {
				t.Fatal(err)
			}

			if got := pins["playwright"]; got != tc.want {
				t.Errorf("playwright pinned at %q, want %q", got, tc.want)
			}

			for _, i := range ops.installs {
				if strings.HasPrefix(i, "playwright@") {
					t.Errorf("installed %s with go install", i)
				}
			}
		})
	}
}

// The pin is what smoke/package.json is written with.
func TestSmokeUsesThePin(t *testing.T) {
	repo := webappRepo(t, strings.Replace(gardenDefinition, `"kind": "webapp",`, `"kind": "webapp", "tools": {"playwright": "1.65.1"},`, 1))

	if err := SmokeTask(repo).Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if pkg := readFile(t, repo.Fs(), "smoke/package.json"); !strings.Contains(pkg, `"playwright": "1.65.1"`) {
		t.Errorf("package.json does not have the pinned Playwright:\n%s", pkg)
	}
}

// Latest asks the npm registry what its latest tag names, and refuses an
// answer that is no version.
func TestLatestNPM(t *testing.T) {
	answer := `{"name": "playwright", "version": "1.65.1"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playwright/latest" {
			http.NotFound(w, r)

			return
		}

		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)

	was := npmRegistry
	npmRegistry = srv.URL
	t.Cleanup(func() { npmRegistry = was })

	if v, err := (SystemTools{}).Latest(t.Context(), playwrightTool); err != nil || v != "1.65.1" {
		t.Errorf("Latest = %q, %v, want 1.65.1", v, err)
	}

	answer = `{"version": "next"}`

	if _, err := (SystemTools{}).Latest(t.Context(), playwrightTool); err == nil {
		t.Error("took \"next\" for a version")
	}

	if _, err := (SystemTools{}).Latest(t.Context(), Tool{Name: "gone", Package: "gone", NPM: true}); err == nil {
		t.Error("took a 404 for a version")
	}
}
