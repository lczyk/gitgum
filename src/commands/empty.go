package commands

import (
	"fmt"
)

type EmptyCommand struct {
	cmdIO
}

func (e *EmptyCommand) Execute(args []string) error {
	r := e.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	currentBranch, err := r.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	upstream, err := r.GetCurrentBranchUpstream()
	if err != nil {
		return fmt.Errorf("getting current branch upstream: %w", err)
	}

	hasUpstream := upstream != ""
	if !hasUpstream {
		fmt.Fprintf(e.out(), "Current branch '%s' has no upstream tracking branch.\n", localRef(currentBranch))
	}

	if hasUpstream {
		ahead, err := r.IsBranchAheadOfRemote(currentBranch, upstream)
		if err != nil {
			return fmt.Errorf("checking if branch is ahead of remote: %w", err)
		}
		if ahead {
			return fmt.Errorf("refusing to create empty commit: '%s' is ahead of '%s'", localRef(currentBranch), upstream)
		}
	}

	applyDirty, err := decideDirtyTree(&e.cmdIO, "empty")
	if err != nil {
		return err
	}

	// Asked before the commit exists, not after. Cancelling here then has
	// nothing to undo -- where the old order created the commit first and left
	// a cancel reporting failure over work that had already succeeded.
	push := false
	if hasUpstream {
		if push, err = e.sel().Confirm("Push the commit to the remote once it exists?", true); err != nil {
			return err
		}
	}

	intr := catchInterrupts()
	defer intr.stop()
	cleanup, err := applyDirty()
	if err != nil {
		return intr.or(err)
	}
	defer cleanup()

	if err := r.CommitEmpty("chore: empty commit"); err != nil {
		return intr.or(fmt.Errorf("creating empty commit: %w", err))
	}
	fmt.Fprintf(e.out(), "Created empty commit on '%s'.\n", localRef(currentBranch))

	if !push {
		if hasUpstream {
			fmt.Fprintln(e.out(), "Not pushing.")
		}
		return nil
	}
	if err := r.Push(); err != nil {
		return intr.or(fmt.Errorf("pushing: %w", err))
	}
	fmt.Fprintln(e.out(), "Pushed to remote.")
	return nil
}
