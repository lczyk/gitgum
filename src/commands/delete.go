package commands

import (
	"fmt"
	"os"

	"github.com/lczyk/gitgum/src/internal"
)

type DeleteCommand struct{}

func (d *DeleteCommand) Execute(args []string) error {
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	branches, err := internal.GetLocalBranches()
	if err != nil {
		return fmt.Errorf("getting local branches: %w", err)
	}

	if len(branches) == 0 {
		fmt.Fprintln(os.Stderr, "No local branches found.")
		return fmt.Errorf("no branches")
	}

	branch, err := internal.FzfSelect("Select a branch to delete", branches)
	if err != nil {
		if err == internal.ErrFzfCancelled {
			fmt.Fprintln(os.Stderr, "No branch selected. Aborting delete.")
		}
		return err
	}

	// main/master deletion is dangerous enough to warrant a confirmation
	if branch == "main" || branch == "master" {
		confirmed, err := internal.FzfConfirm(
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

	currentBranch, err := internal.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	if branch == currentBranch {
		confirmed, err := internal.FzfConfirm(
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
			fmt.Fprintln(os.Stderr, "No other branches found to switch to. Aborting delete.")
			return fmt.Errorf("no other branches")
		}

		otherBranch, err := internal.FzfSelect("Select a branch to switch to", otherBranches)
		if err != nil {
			if err == internal.ErrFzfCancelled {
				fmt.Fprintln(os.Stderr, "No branch selected. Aborting delete.")
			}
			return err
		}

		if err := internal.RunCommandWithOutput("git", "checkout", otherBranch); err != nil {
			return fmt.Errorf("switching to branch '%s': %w", otherBranch, err)
		}
		fmt.Printf("Switched to branch '%s'.\n", otherBranch)
	}

	// non-fatal: if we can't determine upstream, just skip remote deletion
	remoteName, remoteBranchName, err := internal.GetBranchUpstream(branch)
	if err != nil {
		remoteName = ""
	}

	needsToDeleteRemote := false

	if remoteName != "" && remoteBranchName != "" {
		confirmed, err := internal.FzfConfirm(
			fmt.Sprintf("Branch '%s' is tracking remote branch '%s/%s'. Do you want to delete the remote branch as well?", branch, remoteName, remoteBranchName),
			false,
		)
		if err != nil {
			return err
		}
		needsToDeleteRemote = confirmed
	}

	// try safe delete first, fall back to force delete with confirmation
	_, _, err = internal.RunCommand("git", "branch", "-d", branch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not delete branch '%s'. It may not be fully merged.\n", branch)

		var confirmMsg string
		if needsToDeleteRemote {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch and the remote branch?", branch)
		} else {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch?", branch)
		}

		confirmed, err := internal.FzfConfirm(confirmMsg, false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Aborting delete.")
			return nil
		}

		if err := internal.RunCommandWithOutput("git", "branch", "-D", branch); err != nil {
			return fmt.Errorf("force deleting branch '%s': %w", branch, err)
		}
		fmt.Printf("Force deleted local branch '%s'.\n", branch)
	} else {
		fmt.Printf("Deleted local branch '%s'.\n", branch)
	}

	if needsToDeleteRemote && remoteName != "" && remoteBranchName != "" {
		if err := internal.RunCommandWithOutput("git", "push", "--delete", remoteName, remoteBranchName); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not delete remote branch '%s/%s'.\n", remoteName, remoteBranchName)
			return fmt.Errorf("deleting remote branch: %w", err)
		}
		fmt.Printf("Deleted remote branch '%s/%s'.\n", remoteName, remoteBranchName)
	}

	return nil
}
