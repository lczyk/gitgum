package commands

import (
	"fmt"
	"os"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type DeleteCommand struct{}

// switchCurrentBranchIfNeeded handles the case where user tries to delete current branch.
// returns error if something goes wrong; nil if no switch was needed or switch succeeded.
// user cancellation at the confirmation prompt is non-fatal and returns nil.
func switchCurrentBranchIfNeeded(branch string, allBranches []string) error {
	currentBranch, err := git.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	if branch != currentBranch {
		return nil
	}

	confirmed, err := ui.FzfConfirm(
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
	for _, b := range allBranches {
		if b != branch {
			otherBranches = append(otherBranches, b)
		}
	}

	if len(otherBranches) == 0 {
		fmt.Fprintln(os.Stderr, "No other branches found to switch to. Aborting delete.")
		return fmt.Errorf("no other branches")
	}

	otherBranch, err := ui.FzfSelect("Select a branch to switch to", otherBranches)
	if err != nil {
		if err == ui.ErrFzfCancelled {
			fmt.Fprintln(os.Stderr, "No branch selected. Aborting delete.")
		}
		return err
	}

	if err := cmdrun.RunWithOutput("git", "checkout", otherBranch); err != nil {
		return fmt.Errorf("switching to branch '%s': %w", otherBranch, err)
	}
	fmt.Printf("Switched to branch '%s'.\n", otherBranch)
	return nil
}

func (d *DeleteCommand) Execute(args []string) error {
	if err := git.CheckInRepo(); err != nil {
		return err
	}

	branches, err := git.GetLocalBranches()
	if err != nil {
		return fmt.Errorf("getting local branches: %w", err)
	}

	if len(branches) == 0 {
		fmt.Fprintln(os.Stderr, "No local branches found.")
		return fmt.Errorf("no branches")
	}

	branch, err := ui.FzfSelect("Select a branch to delete", branches)
	if err != nil {
		if err == ui.ErrFzfCancelled {
			fmt.Fprintln(os.Stderr, "No branch selected. Aborting delete.")
		}
		return err
	}

	// main/master deletion is dangerous enough to warrant a confirmation
	if branch == "main" || branch == "master" {
		confirmed, err := ui.FzfConfirm(
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

	if err := switchCurrentBranchIfNeeded(branch, branches); err != nil {
		return err
	}

	// non-fatal: if we can't determine upstream, just skip remote deletion
	remoteName, remoteBranchName, err := git.GetBranchUpstream(branch)
	if err != nil {
		remoteName = ""
	}

	needsToDeleteRemote := false

	if remoteName != "" && remoteBranchName != "" {
		confirmed, err := ui.FzfConfirm(
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
		fmt.Fprintf(os.Stderr, "Could not delete branch '%s'. It may not be fully merged.\n", branch)

		var confirmMsg string
		if needsToDeleteRemote {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch and the remote branch?", branch)
		} else {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch?", branch)
		}

		confirmed, err := ui.FzfConfirm(confirmMsg, false)
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
			fmt.Fprintf(os.Stderr, "Error: Could not delete remote branch '%s/%s'.\n", remoteName, remoteBranchName)
			return fmt.Errorf("deleting remote branch: %w", err)
		}
		fmt.Printf("Deleted remote branch '%s/%s'.\n", remoteName, remoteBranchName)
	}

	return nil
}
