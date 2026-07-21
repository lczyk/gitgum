package git

import (
	"context"
	"fmt"
	"strings"
)

// PullMode selects how fetched upstream commits are integrated into the
// current branch. It maps onto git's three integration strategies.
type PullMode int

const (
	PullFFOnly PullMode = iota // git merge --ff-only: refuse a merge commit
	PullRebase                 // git rebase: replay local commits onto upstream
	PullMerge                  // git merge: merge commit when the branches diverged
)

// Integrate applies upstream into the current branch using the chosen mode.
// The caller is expected to have Fetch'd first: Integrate works against the
// local remote-tracking ref (e.g. "origin/main") and does not touch the
// network. Progress and hook output are streamed live; the tail of stderr is
// folded into the error on failure. A shallow clone stays shallow -- nothing
// here deepens history.
func (r Repo) Integrate(mode PullMode, upstream string) error {
	var args []string
	switch mode {
	case PullFFOnly:
		// --no-stat: the caller renders its own compact-summary of what landed,
		// so git's plain diffstat would just be a duplicate.
		args = []string{"merge", "--no-stat", "--ff-only", upstream}
	case PullRebase:
		args = []string{"rebase", upstream}
	case PullMerge:
		args = []string{"merge", "--no-stat", upstream}
	default:
		return fmt.Errorf("unknown pull mode %d", mode)
	}
	if _, stderr, err := r.runWriteStreaming(context.Background(), args...); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr))
	}
	return nil
}

func (mode PullMode) String() string {
	switch mode {
	case PullFFOnly:
		return "fast-forward only"
	case PullRebase:
		return "rebase"
	case PullMerge:
		return "merge"
	default:
		return fmt.Sprintf("PullMode(%d)", int(mode))
	}
}
