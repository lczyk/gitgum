package commands

import (
	"errors"
	"fmt"

	"github.com/lczyk/gitgum/internal/git"
)

type EmptyCommand struct {
	cmdIO
}

func (e *EmptyCommand) Execute(args []string) error {
	if err := git.CheckInRepo(); err != nil {
		return err
	}

	currentBranch, err := git.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	upstream, err := git.GetCurrentBranchUpstream()
	if err != nil {
		return fmt.Errorf("getting current branch upstream: %w", err)
	}

	hasUpstream := upstream != ""
	if !hasUpstream {
		fmt.Fprintf(e.out(), "Current branch '%s' has no upstream tracking branch.\n", currentBranch)
	}

	if hasUpstream {
		ahead, err := git.IsBranchAheadOfRemote(currentBranch, upstream)
		if err != nil {
			return fmt.Errorf("checking if branch is ahead of remote: %w", err)
		}
		if ahead {
			return fmt.Errorf("refusing to create empty commit: branch '%s' is ahead of remote '%s'", currentBranch, upstream)
		}
	}

	cleanup, err := handleDirtyTree(&e.cmdIO, "empty")
	if err != nil {
		if errors.Is(err, errDirtyTreeAborted) {
			fmt.Fprintln(e.out(), "Aborted.")
			return nil
		}
		return err
	}
	defer cleanup()

	if err := git.CommitEmpty("chore: empty commit"); err != nil {
		return fmt.Errorf("creating empty commit: %w", err)
	}

	fmt.Fprintf(e.out(), "Created empty commit on branch '%s'.\n", currentBranch)

	if hasUpstream {
		confirmed, err := e.sel().Confirm("Do you want to push this commit to the remote?", true)
		if err != nil {
			return err
		}
		if confirmed {
			if err := git.Push(); err != nil {
				return fmt.Errorf("pushing: %w", err)
			}
			fmt.Fprintln(e.out(), "Pushed to remote.")
		} else {
			fmt.Fprintln(e.out(), "Not pushing.")
		}
	}

	return nil
}
