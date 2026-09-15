// Package remote reads what a git remote says a repository is.
package remote

import (
	"context"
	"os/exec"
	"strings"
)

// Split pulls owner and name out of a git remote URL, in either of the two
// shapes GitHub hands out.
func Split(url string) (owner, name string, ok bool) {
	url = strings.TrimSuffix(strings.TrimSpace(url), ".git")

	if rest, found := strings.CutPrefix(url, "git@"); found {
		// git@github.com:owner/name
		_, path, found := strings.Cut(rest, ":")
		if !found {
			return "", "", false
		}

		url = path
	} else {
		// https://github.com/owner/name, and anything else with a host
		if _, after, found := strings.Cut(url, "://"); found {
			url = after
		}

		_, path, found := strings.Cut(url, "/")
		if !found {
			return "", "", false
		}

		url = path
	}

	owner, name, found := strings.Cut(url, "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}

	return owner, name, true
}

// Of names the GitHub repository whose working tree contains dir.
//
// Read from the git remote rather than from the directory name: a checkout can
// be called anything, and what matters is the repository the remote points at.
func Of(ctx context.Context, dir string) (owner, name string, ok bool) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", false
	}

	return Split(string(out))
}
