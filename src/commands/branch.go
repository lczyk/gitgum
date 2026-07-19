package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/ui"
)

type BranchCommand struct {
	cmdIO
}

func (b *BranchCommand) Execute(args []string) error {
	r := b.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	// The picker can't open until these read-only queries return, so run them
	// concurrently rather than serialising the working-tree scan behind the
	// branch/remote lookups -- see runConcurrent.
	var (
		currentBranch, trackingRemote, statusLine string
		remotes                                   []string
		dirty                                     []string
	)
	if err := runConcurrent(
		func() (err error) {
			currentBranch, trackingRemote, statusLine, err = resolveCurrentBranchContext(r)
			return err
		},
		func() (err error) {
			remotes, err = r.GetRemotes()
			if err != nil {
				return fmt.Errorf("getting remotes: %w", err)
			}
			return nil
		},
		func() (err error) {
			dirty, err = r.DirtyTrackedLines()
			return err
		},
	); err != nil {
		return err
	}
	fmt.Fprintln(b.out(), statusLine)

	cleanup, err := handleDirtyLines(&b.cmdIO, "branch", dirty)
	if err != nil {
		if errors.Is(err, errDirtyTreeAborted) {
			fmt.Fprintln(b.out(), "Aborted.")
			return nil
		}
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// includeCurrent: branching off the branch you're on is the common case.
	// markCheckedOut: off, and no unselectable predicate -- unlike switch and
	// delete, a branch checked out in another worktree is a fine start point.
	// detachedAt: likewise a fine start point, and the only way to keep the
	// commits made on a detached HEAD.
	src := streamBranches(ctx, r, b.err(), currentBranch, trackingRemote, remotes,
		branchStreamOpts{includeCurrent: true, detachedAt: detachedShortSHA(r, currentBranch)})

	selected, err := b.sel().SelectStream(ctx, "Select a branch to branch off", src, nil)
	cancel()
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			fmt.Fprintln(b.out(), "Aborting branch.")
			return nil
		}
		return err
	}

	sp, err := startPointFor(selected)
	if err != nil {
		return err
	}

	// Same as switch: a remote-tracking ref is only as fresh as the last fetch,
	// so refresh it before branching. Done before the name prompt so a fetch
	// failure doesn't waste the user's typing.
	if sp.remote != "" {
		fmt.Fprintf(b.out(), "Fetching '%s/%s'...\n", sp.remote, sp.branch)
		if err := r.Fetch(sp.remote, sp.branch); err != nil {
			return fmt.Errorf("fetching '%s/%s': %w", sp.remote, sp.branch, err)
		}
	}

	name, err := b.promptName()
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			fmt.Fprintln(b.out(), "Aborting branch.")
			return nil
		}
		return err
	}

	if r.BranchExists(name) {
		return fmt.Errorf("branch '%s' already exists; use `gg switch` to switch to it", name)
	}

	create := r.CheckoutNewBranch
	if sp.remote != "" {
		create = r.CheckoutNewBranchNoTrack
	}
	if err := create(name, sp.ref); err != nil {
		return fmt.Errorf("creating branch '%s': %w", name, err)
	}

	fmt.Fprintf(b.out(), "Created and switched to branch '%s' off '%s'.\n", name, sp.ref)
	return nil
}

// startPoint is where a new branch gets cut from. remote is set only when ref
// names a remote-tracking ref ("origin/foo"), which has two consequences: the
// ref wants a fetch first (it's as stale as the last one), and the new branch
// must be created --no-track (it's someone else's branch, and git's
// branch.autoSetupMerge default would silently adopt it as upstream). Local
// start points carry no upstream to inherit, so neither applies.
type startPoint struct {
	ref    string
	remote string
	branch string
}

// startPointFor turns a picker payload into the branch's start point.
func startPointFor(selected string) (startPoint, error) {
	typ, name, err := parseBranchEntry(selected)
	if err != nil {
		return startPoint{}, err
	}
	switch typ {
	case "local":
		// a detached HEAD is not a branch, but its commit is a start point --
		// and cutting a branch off it is how those commits stop being orphans.
		if short, ok := detachedEntrySHA(name); ok {
			return startPoint{ref: short}, nil
		}
		return startPoint{ref: name}, nil
	case "local/remote":
		branch, ok := localRemoteBranch(name)
		if !ok {
			return startPoint{}, fmt.Errorf("invalid local/remote branch format: %s", name)
		}
		return startPoint{ref: branch}, nil
	case "remote":
		// A remote name never contains '/', so the first cut splits it off; the
		// branch may itself contain '/' (e.g. "feat/foo").
		remote, branch, ok := strings.Cut(name, "/")
		if !ok {
			return startPoint{}, fmt.Errorf("invalid remote branch format: %s", name)
		}
		return startPoint{ref: name, remote: remote, branch: branch}, nil
	default:
		return startPoint{}, fmt.Errorf("unknown branch type: %s", typ)
	}
}

// promptName reads the new branch name. The prompt hands back whatever was
// typed, so nothing-at-all (or pure whitespace) has to be rejected here rather
// than letting `git checkout -b ""` fail in git's own words.
func (b *BranchCommand) promptName() (string, error) {
	name, err := b.sel().Prompt("New branch name")
	if err != nil {
		return "", err
	}
	if name = strings.TrimSpace(name); name == "" {
		return "", fmt.Errorf("empty branch name")
	}
	return name, nil
}
