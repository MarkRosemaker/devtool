// Command devtool brings Go repositories up to a standard and keeps them
// there.
//
// It is the opinionated half of the tool: the generators that write a README,
// a Makefile, a .gitignore and an AGENTS.md, and the order they run in. The
// machinery they plug into is github.com/MarkRosemaker/devtool-engine, which
// anybody's devtool can import unchanged.
//
// # Two ways to run it
//
// With no list, it rebuilds the generated files of the repository you are
// standing in and leaves them uncommitted for you to read. No GitHub, no
// network, no commits — a person at a terminal asking for their README to be
// brought up to date.
//
//	devtool
//	update
//
// With a list, it maintains every repository the list names, unattended:
// testing, committing each task separately, and pushing. The coverage it
// measures is written back into the list and committed to whichever repository
// holds it.
//
//	devtool update all --config=../portfolio/config.json
//	devtool update MarkRosemaker/openapi --config=../portfolio/config.json
//
// -jsonl on any of them writes the run as JSON Lines on stdout instead of
// leaving it to the log, which is how patchpal follows a run it did not
// perform.
//
// # Two different updates
//
// "devtool self-update" updates the devtool binary on this machine, and
// nothing else. It is about which build of this tool runs next.
//
// Updating the dependencies of a repository — its go.mod, go.sum and vendor
// directory, including its devtool-engine dependency if it has one — is a
// task within a run, applied to whatever repository is being maintained. The
// two are unrelated: a current binary can maintain a repository whose
// dependencies are stale, and a stale binary can bring a repository's
// dependencies right up to date.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/MarkRosemaker/devtool-engine/event"
	"github.com/MarkRosemaker/devtool-engine/selfupdate"
	"github.com/MarkRosemaker/devtool/internal/config"
	"github.com/MarkRosemaker/devtool/internal/local"
	"github.com/MarkRosemaker/devtool/internal/run"
	"github.com/MarkRosemaker/devtool/maintain"
)

//go:generate go run ./internal/lintgen

// allTarget is what to name instead of a repository to maintain every one the
// list holds.
const allTarget = "all"

// licenseHolder is who the copyright is asserted by: the legal person, with the
// GitHub handle after it so the notice connects to where the work lives. A
// notice naming only a pseudonym would leave the holder to establish that link
// later, if it ever mattered.
const licenseHolder = "Marco Rösler (MarkRosemaker)"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := dispatch(ctx, os.Args[1:]); err != nil {
		slog.ErrorContext(ctx, name+" failed", "error", err)
		os.Exit(1)
	}
}

// dispatch picks the subcommand. A subcommand is matched before flags are
// parsed so that "devtool update" does not have to be spelled with a dash, and
// an empty command line means "update", which is the common case.
func dispatch(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "self-update":
			return selfUpdate(ctx, args[1:])
		case "update":
			return update(ctx, args[1:])
		case "test":
			return runTests(ctx, args[1:])
		case "version":
			return printVersion(args[1:])
		}
	}

	// Flags with no subcommand: -version, or an empty line meaning "update".
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		return topLevelFlags(ctx, args)
	}

	if len(args) > 0 {
		return fmt.Errorf("unknown command %q", args[0])
	}

	return update(ctx, nil)
}

// printVersion reports which build this is.
//
// The same answer as the -version flag, which stays. A flag is what somebody
// reaches for beside other flags; a subcommand is what they reach for beside
// self-update, which is the question this usually follows — and it is what
// anybody who has used another tool will try first.
func printVersion(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("version takes no arguments, got %q", args[0])
	}

	fmt.Println(buildVersion())

	return nil
}

// topLevelFlags handles the flags that stand alone.
func topLevelFlags(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "report which build this is and exit")

	fs.Usage = func() { usage(fs.Output()) }

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Println(buildVersion())

		return nil
	}

	return update(ctx, args)
}

// usage says what the tool takes, in the order somebody is likely to want it.
func usage(w io.Writer) {
	//nolint:errcheck
	fmt.Fprintf(w, `usage:
  %[1]s                                rebuild this repository's generated files
  %[1]s update                         the same
  %[1]s update -commit                 the same, then test, commit and push
  %[1]s update all      -commit --config=PATH  maintain every repository listed
  %[1]s update OWNER/NAME -commit --config=PATH  maintain one of them
  %[1]s test                          run the tests and record the coverage
  %[1]s self-update                    update this binary
  %[1]s version                        report which build this is
  %[1]s -version                       the same

flags:
  -jsonl      write the run as JSON Lines on stdout
  -config     the list of repositories to maintain
  -commit     test, commit each change and push; off by default
  -verbose    report the outcome even when nothing changed
  -private    (no list) this repository is private
  -check-latest  ask the proxy whether this build is the latest, and fail if not
`, name)
}

// parseFlags parses args where a flag may follow a positional argument.
//
// Go's flag package stops at the first argument that is not a flag, so
// "update all -config=..." would take "all" and silently ignore everything
// after it — which is exactly how somebody would type it. Parsing is resumed
// past each positional instead, and the positionals are returned in order.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string

	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}

		if fs.NArg() == 0 {
			return positional, nil
		}

		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// update runs either shape, depending on whether a list was named.
func update(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(name+" update", flag.ContinueOnError)
	cfgPath := fs.String("config", "",
		"the list of repositories to maintain; without it, this repository alone")
	jsonl := fs.Bool("jsonl", false, "write the run as JSON Lines on stdout")
	verbose := fs.Bool("verbose", false, "report the outcome even when nothing changed")
	private := fs.Bool("private", false, "this repository is private (no list only)")
	commit := fs.Bool("commit", false,
		"test, commit each task that changed something, and push; without it "+
			"nothing is committed and the worktree is left to read")
	checkLatest := fs.Bool("check-latest", false,
		"ask the module proxy whether this is the latest build and fail if it "+
			"is not; a run already says so without the network where a "+
			"repository has been maintained by a later one")

	fs.Usage = func() { usage(fs.Output()) }

	positional, err := parseFlags(fs, args)
	if err != nil {
		return err
	}

	if len(positional) > 1 {
		return fmt.Errorf("%s update takes one repository or %q, got %q",
			name, allTarget, positional)
	}

	if *checkLatest {
		if err := requireLatest(ctx); err != nil {
			return err
		}
	}

	events := emitter(*jsonl)

	var target string
	if len(positional) == 1 {
		target = positional[0]
	}

	if *cfgPath == "" {
		if target != "" {
			return errors.New("naming a repository needs -config; without one, " +
				"the repository you are standing in is the only one there is")
		}

		// "devtool update" with nothing after it is "devtool": rebuild the
		// generated files of the repository you are standing in.
		return local.Update(ctx, ".", local.Options{
			Holder:  licenseHolder,
			Private: *private,
			Commit:  *commit,
			Version: selfupdate.Version(),
			// Direct, for the same reason requireLatest is: the proxy's
			// answer is cached for minutes after a push, and a repository
			// recording a build published in that window would otherwise be
			// told there is nothing to fetch.
			SelfUpdate: (&selfupdate.Updater{Module: modulePath, Direct: true}).Update,
		}, events)
	}

	// With a list, say which. Maintaining thirty repositories unattended is
	// not what somebody who typed two words and forgot the third meant.
	if target == "" {
		return errors.New(`with -config, name what to maintain: "all", or one "owner/name"`)
	}

	return maintained(ctx, *cfgPath, target, *commit, *verbose, events)
}

// requireLatest fails the run where this is not the latest published build.
//
// The one place devtool asks the network about itself, and only when asked to:
// a run already notices it is behind by reading what the repository records,
// which costs nothing and needs nothing. This covers the gap that leaves — a
// build published since the last run anywhere, which nothing has recorded yet.
//
// Direct, because the proxy's answer to "what is the latest" is cached for
// minutes after a push, and a check that reports yesterday is worse than none.
//
// Only a definite answer is a failure. Where the question could not be put —
// no toolchain, no network, no credentials for a private module — the run
// carries on, since that is a fact about this machine rather than about this
// build.
func requireLatest(ctx context.Context) error {
	current := selfupdate.Version()

	latest, err := (&selfupdate.Updater{Module: modulePath, Direct: true}).Latest(ctx)
	if err != nil {
		slog.WarnContext(ctx, "could not ask which build is the latest",
			"error", err, "running", current)

		return nil
	}

	if !maintain.Newer(latest, current) {
		return nil
	}

	return fmt.Errorf("%s %s is behind %s: run %s self-update",
		name, current, latest, name)
}

// maintained is the unattended shape: a list, and everything in it or one of
// them.
func maintained(
	ctx context.Context, cfgPath, target string, commit, verbose bool, events event.Emitter,
) error {
	// A maintained run commits and pushes to every repository on the list, so
	// it says so out loud rather than being what happens by default.
	if !commit {
		return errors.New("maintaining a list means committing and pushing to " +
			"every repository on it, so say -commit")
	}

	// What was asked for is checked before anything is opened or cloned: a
	// mistyped repository should not first cost a token check and a pass over
	// the list.
	var owner, repoName string

	if target != allTarget {
		var ok bool

		owner, repoName, ok = strings.Cut(target, "/")
		if !ok || owner == "" || repoName == "" || strings.Contains(repoName, "/") {
			return fmt.Errorf(`invalid repository %q, want "owner/name" or %q`,
				target, allTarget)
		}
	}

	cfg, err := run.OpenConfigFile(ctx, cfgPath, config.StateName)
	if err != nil {
		return err
	}

	svc, err := run.New(ctx, cfg, events, verbose, selfupdate.Version())
	if err != nil {
		return err
	}

	if target == allTarget {
		return svc.Run(ctx)
	}

	return svc.RunOne(ctx, owner, repoName)
}

// emitter is where a run's events go: onto stdout as JSON Lines for something
// parsing them, and otherwise nowhere, the log having said it already.
func emitter(jsonl bool) event.Emitter {
	if jsonl {
		return event.Write(os.Stdout)
	}

	return event.EmitterFunc(func(event.Event) {})
}

// selfUpdate updates this binary, not any repository's dependencies. See the
// package comment.
func selfUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(name+" self-update", flag.ContinueOnError)
	direct := fs.Bool("direct", true,
		"ask the repository rather than the module proxy, whose answer to "+
			"\"what is the latest version\" is cached and can be minutes stale")

	if err := fs.Parse(args); err != nil {
		return err
	}

	out, err := (&selfupdate.Updater{
		Module:  modulePath,
		Current: selfupdate.Version(),
		Direct:  *direct,
	}).Update(ctx)
	if err != nil {
		return err
	}

	fmt.Println(out)

	return nil
}

// runTests measures this repository's coverage and records it where the README
// reads it from. Named around the subcommand rather than "test", which in a
// package with tests of its own would read as one.
func runTests(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(name+" test", flag.ContinueOnError)
	jsonl := fs.Bool("jsonl", false, "write the run as JSON Lines on stdout")
	private := fs.Bool("private", false, "this repository is private")

	fs.Usage = func() { usage(fs.Output()) }

	positional, err := parseFlags(fs, args)
	if err != nil {
		return err
	}

	if len(positional) > 0 {
		return fmt.Errorf("%s test takes no arguments, got %q", name, positional[0])
	}

	return local.Test(ctx, ".", local.Options{
		Holder:  licenseHolder,
		Private: *private,
	}, emitter(*jsonl))
}
