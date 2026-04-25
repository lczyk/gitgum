package commands

import (
	"errors"
	"fmt"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type DeleteCommand struct{}

func (d *DeleteCommand) Execute(args []string) error {
	if err := git.CheckInRepo(); err != nil {
		return err
	}

	branches, err := git.GetLocalBranches()
	if err != nil {
		return fmt.Errorf("getting local branches: %w", err)
	}

	if len(branches) == 0 {
		return fmt.Errorf("no local branches found")
	}

	branch, err := ui.Select("Select a branch to delete", branches)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			fmt.Println("Aborting delete.")
			return nil
		}
		return err
	}

	// main/master deletion is dangerous enough to warrant a confirmation
	if branch == "main" || branch == "master" {
		confirmed, err := ui.Confirm(
			fmt.Sprintf("You are about to delete the '%s' branch. This is usually the main branch of the repository. Are you sure you want to proceed?", branch),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Aborting delete.")
			return nil
		}
	}

	// if user wants to delete their current branch, offer to switch first
	currentBranch, err := git.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	if branch == currentBranch {
		confirmed, err := ui.Confirm(
			fmt.Sprintf("You are currently on branch '%s'. Do you want to switch to another branch before deleting it?", branch),
			true,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Aborting delete.")
			return nil
		}

		var otherBranches []string
		for _, b := range branches {
			if b != branch {
				otherBranches = append(otherBranches, b)
			}
		}

		if len(otherBranches) == 0 {
			return fmt.Errorf("no other branches to switch to")
		}

		otherBranch, err := ui.Select("Select a branch to switch to", otherBranches)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				fmt.Println("Aborting delete.")
				return nil
			}
			return err
		}

		if err := cmdrun.RunWithOutput("git", "checkout", otherBranch); err != nil {
			return fmt.Errorf("switching to branch '%s': %w", otherBranch, err)
		}
		fmt.Printf("Switched to branch '%s'.\n", otherBranch)
	}

	// non-fatal: if we can't determine upstream, just skip remote deletion
	remoteName, remoteBranchName, err := git.GetBranchUpstream(branch)
	if err != nil {
		remoteName = ""
	}

	needsToDeleteRemote := false

	if remoteName != "" && remoteBranchName != "" {
		confirmed, err := ui.Confirm(
			fmt.Sprintf("Branch '%s' is tracking remote branch '%s/%s'. Do you want to delete the remote branch as well?", branch, remoteName, remoteBranchName),
			false,
		)
		if err != nil {
			return err
		}
		needsToDeleteRemote = confirmed
	}

	// try safe delete first, fall back to force delete with confirmation
	_, _, err = cmdrun.Run("git", "branch", "-d", branch)
	if err != nil {
		var confirmMsg string
		if needsToDeleteRemote {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch and the remote branch?", branch)
		} else {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch?", branch)
		}

		confirmed, err := ui.Confirm(confirmMsg, false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Aborting delete.")
			return nil
		}

		if err := cmdrun.RunWithOutput("git", "branch", "-D", branch); err != nil {
			return fmt.Errorf("force deleting branch '%s': %w", branch, err)
		}
		fmt.Printf("Force deleted local branch '%s'.\n", branch)
	} else {
		fmt.Printf("Deleted local branch '%s'.\n", branch)
	}

	if needsToDeleteRemote {
		if err := cmdrun.RunWithOutput("git", "push", "--delete", remoteName, remoteBranchName); err != nil {
			return fmt.Errorf("deleting remote branch: %w", err)
		}
		fmt.Printf("Deleted remote branch '%s/%s'.\n", remoteName, remoteBranchName)
	}

	return nil
}
