package maintain

import (
	"context"
	"fmt"

	engine "github.com/MarkRosemaker/devtool-engine/maintain"
	"github.com/spf13/afero"
)

const claudePath = "CLAUDE.md"

// ClaudeTask returns a task that removes the CLAUDE.md link this tool used to
// make to AGENTS.md. Claude reads AGENTS.md itself now, so the link is a
// second name for one file that nothing needs.
//
// Only that link goes. A CLAUDE.md somebody wrote, or a link of their own to
// somewhere else, is theirs, and is left where it is.
func ClaudeTask(repo engine.Repo) engine.Task {
	return engine.Task{
		Name:  "remove the CLAUDE.md link",
		Short: "claude",
		Run: func(context.Context) error {
			return unlinkClaude(repo.Fs())
		},
	}
}

// unlinkClaude is [ClaudeTask]'s work, factored out so a test can hand it a
// filesystem of its own.
func unlinkClaude(fs afero.Fs) error {
	links, ok := fs.(afero.LinkReader)
	if !ok {
		// A filesystem that cannot read a link cannot hold one either.
		return nil
	}

	target, err := links.ReadlinkIfPossible(claudePath)
	if err != nil || target != agentsPath {
		return nil //nolint:nilerr // not a link, or not ours: either way nothing to remove
	}

	if err := fs.Remove(claudePath); err != nil {
		return fmt.Errorf("removing %s: %w", claudePath, err)
	}

	return nil
}
