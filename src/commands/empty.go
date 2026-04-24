package commands

import (
	"fmt"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type EmptyCommand struct{}

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
		return err
	}

	hasUpstream := upstream != ""
	if !hasUpstream {
		fmt.Printf("Current branch '%s' has no upstream tracking branch.\n", currentBranch)
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

	dirty, err := git.IsDirty(".")
	if err != nil {
		return fmt.Errorf("checking if working tree is dirty: %w", err)
	}
	if dirty {
		fmt.Println("Working tree has uncommitted changes. Stashing them before creating empty commit.")
		if err := cmdrun.RunWithOutput("git", "stash", "--include-untracked"); err != nil {
			return fmt.Errorf("stashing changes: %w", err)
		}
		defer func() {
			fmt.Println("Restoring stashed changes.")
			if err := cmdrun.RunWithOutput("git", "stash", "pop"); err != nil {
				fmt.Printf("Warning: failed to pop stash: %v\n", err)
			}
		}()
	}

	if err := cmdrun.RunWithOutput("git", "commit", "--allow-empty", "-m", "chore: empty commit"); err != nil {
		return fmt.Errorf("creating empty commit: %w", err)
	}

	fmt.Printf("Created empty commit on branch '%s'.\n", currentBranch)

	if hasUpstream {
		confirmed, err := ui.FzfConfirm("Do you want to push this commit to the remote?", true)
		if err != nil {
			return err
		}
		if confirmed {
			if err := cmdrun.RunWithOutput("git", "push"); err != nil {
				return fmt.Errorf("pushing: %w", err)
			}
			fmt.Printf("Pushed to remote.\n")
		} else {
			fmt.Println("Not pushing.")
		}
	}

	return nil
}
