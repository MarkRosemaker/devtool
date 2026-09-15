// Command devtool brings Go repositories up to a standard and keeps them
// there.
//
// It is the opinionated half of the tool: the generators that write a README,
// a Makefile, a .gitignore and an AGENTS.md, and the order they run in. The
// machinery they plug into is github.com/MarkRosemaker/devtool-engine, which
// anybody's devtool can import unchanged.
//
// It reports what it did as JSON Lines on stdout rather than talking to a
// person, so a long-running front end — patchpal — can render a run it did not
// perform. Being a separate process is the point: the front end stays up for
// hours, and devtool is started fresh for every run, so a change to it takes
// effect on the next run rather than the next restart.
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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/MarkRosemaker/devtool-engine/selfupdate"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		slog.ErrorContext(ctx, name+" failed", "error", err)
		os.Exit(1)
	}
}

// run dispatches on the subcommand, if there is one, and otherwise parses
// flags. A subcommand is matched before flags are parsed so that
// "devtool self-update" does not have to be spelled with a leading dash.
func run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "self-update":
			return selfUpdate(ctx, args[1:])
		}
	}

	return runFlags(ctx, args)
}

// runFlags handles the flag-shaped invocations.
func runFlags(_ context.Context, args []string) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	showVersion := fs.Bool("version", false,
		"report which build this is and exit")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s [flags]\n       %s self-update\n\nflags:\n",
			name, name)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Println(buildVersion())

		return nil
	}

	// Maintaining repositories has not moved here yet: it is still portfolio's
	// main.go, along with the generators and the task sequence. Saying so is
	// better than a usage message that implies the tool simply took the
	// arguments badly.
	fs.Usage()

	return errNotYetMoved
}

// errNotYetMoved marks the half of this tool that is still in portfolio.
var errNotYetMoved = fmt.Errorf(
	"maintaining repositories is still portfolio's; only self-update and -version are here")

// selfUpdateFlags and selfUpdate: updating this binary, not any repository's
// dependencies. See the package comment.
func selfUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(name+" self-update", flag.ContinueOnError)
	direct := fs.Bool("direct", true,
		"ask the repository rather than the module proxy, whose answer to "+
			"\"what is the latest version\" is cached and can be minutes stale")

	if err := fs.Parse(args); err != nil {
		return err
	}

	up := &selfupdate.Updater{
		Module:  modulePath,
		Current: selfupdate.Version(),
		Direct:  *direct,
	}

	out, err := up.Update(ctx)
	if err != nil {
		return err
	}

	fmt.Println(out)

	return nil
}
