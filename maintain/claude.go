package maintain

import (
	"context"
	"fmt"
	"os"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const claudePath = "CLAUDE.md"

// realPather is what a filesystem has to offer for this task to work: the
// real path behind a name.
//
// afero's own SymlinkIfPossible resolves the target as well as the link, so
// asking it for "AGENTS.md" writes an absolute path into the link — which
// is machine-specific, and wrong the moment it is committed.
type realPather interface {
	RealPath(name string) (string, error)
}

// ClaudeTask returns a task that points CLAUDE.md at AGENTS.md, so a tool
// looking for the Claude-specific name reads the same file as everything
// else.
//
// A symlink rather than a copy: a copy is a second thing to keep true, and
// the whole point of AGENTS.md being generated is that there is one of it.
//
// Anything already at CLAUDE.md is left alone, whether it is a file
// somebody wrote or a link somewhere else of their choosing. Nor is a link
// made before AGENTS.md exists, a dangling one being worse than none.
// The filesystem it works on is the one a Repo embeds, not the Repo: a Repo
// is an afero.Fs only by embedding that interface, and Go promotes an
// embedded interface's own method set and nothing else. So a Repo reads and
// writes files perfectly well while carrying none of the methods the
// concrete filesystem underneath has — RealPath among them, which this task
// needs. Handing it the Repo asked every repository in the portfolio
// whether it could make a symlink and was told no.
func ClaudeTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "link CLAUDE.md to AGENTS.md",
		Short: "claude",
		Run: func(context.Context) error {
			return linkClaude(repo.Fs())
		},
	}
}

// linkClaude is [ClaudeTask]'s work, factored out so a test can hand it a
// filesystem of its own.
func linkClaude(fs afero.Fs) error {
	if exists, err := afero.Exists(fs, agentsPath); err != nil {
		return fmt.Errorf("checking for %s: %w", agentsPath, err)
	} else if !exists {
		return nil
	}

	if taken, err := claudeTaken(fs); err != nil {
		return err
	} else if taken {
		return nil
	}

	paths, ok := fs.(realPather)
	if !ok {
		// Naming the type, because the last time this fired it was not the
		// filesystem's fault: it was a wrapper that had dropped the method.
		return fmt.Errorf(
			"%s: %T cannot say where it keeps its files, so no symlink can be made",
			claudePath, fs,
		)
	}

	link, err := paths.RealPath(claudePath)
	if err != nil {
		return fmt.Errorf("%s: %w", claudePath, err)
	}

	// os.Symlink, not the filesystem's own, so the target stays the
	// relative "AGENTS.md" that gets committed.
	if err := os.Symlink(agentsPath, link); err != nil {
		return fmt.Errorf("linking %s to %s: %w", claudePath, agentsPath, err)
	}

	return nil
}

// claudeTaken reports whether something is at CLAUDE.md already.
//
// A link is read rather than followed, so one left dangling still counts as
// somebody's: afero.Exists follows it and would call it absent.
func claudeTaken(fs afero.Fs) (bool, error) {
	if links, ok := fs.(afero.LinkReader); ok {
		if _, err := links.ReadlinkIfPossible(claudePath); err == nil {
			return true, nil
		}
	}

	exists, err := afero.Exists(fs, claudePath)
	if err != nil {
		return false, fmt.Errorf("checking for %s: %w", claudePath, err)
	}

	return exists, nil
}
