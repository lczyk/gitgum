package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/lczyk/gitgum/src/internal"
)

// DeleteCommand handles deleting local and optionally remote branches
type DeleteCommand struct{}

// Execute runs the delete command
func (d *DeleteCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Get list of local branches
	branches, err := internal.GetLocalBranches()
	if err != nil {
		return fmt.Errorf("failed to get local branches: %v", err)
	}

	if len(branches) == 0 {
		fmt.Fprintln(os.Stderr, "No local branches found.")
		return fmt.Errorf("no branches")
	}

	// Let user select a branch to delete
	branch, err := internal.FzfSelect("Select a branch to delete", branches)
	if err != nil {
		if err == internal.ErrFzfCancelled {
			fmt.Fprintln(os.Stderr, "No branch selected. Aborting delete.")
		}
		return err
	}

	// Warn if deleting main or master
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

	// Check if the branch is the current branch
	currentBranch, _, err := internal.RunCommand("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current branch: %v", err)
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

		// Filter out the current branch from the list
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

		// Let user select a branch to switch to
		otherBranch, err := internal.FzfSelect("Select a branch to switch to", otherBranches)
		if err != nil {
			if err == internal.ErrFzfCancelled {
				fmt.Fprintln(os.Stderr, "No branch selected. Aborting delete.")
			}
			return err
		}

		// Switch to the other branch
		if err := internal.RunCommandWithOutput("git", "checkout", otherBranch); err != nil {
			return fmt.Errorf("failed to switch to branch '%s': %v", otherBranch, err)
		}
		fmt.Printf("Switched to branch '%s'.\n", otherBranch)
	}

	// Check if the branch has an upstream remote
	upstreamBranch, _, err := internal.RunCommand("git", "for-each-ref", "--format=%(upstream:short)", "refs/heads/"+branch)
	if err != nil {
		// Non-fatal error, continue without remote deletion
		upstreamBranch = ""
	}

	needsToDeleteRemote := false
	var remoteName, remoteBranchName string

	if upstreamBranch != "" {
		// Parse remote and branch name
		parts := strings.SplitN(upstreamBranch, "/", 2)
		if len(parts) == 2 {
			remoteName = parts[0]
			remoteBranchName = parts[1]

			confirmed, err := internal.FzfConfirm(
				fmt.Sprintf("Branch '%s' is tracking remote branch '%s'. Do you want to delete the remote branch as well?", branch, upstreamBranch),
				false,
			)
			if err != nil {
				return err
			}
			needsToDeleteRemote = confirmed
		}
	}

	// Try to delete the local branch (safe delete)
	_, _, err = internal.RunCommand("git", "branch", "-d", branch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not delete branch '%s'. It may not be fully merged.\n", branch)

		// Determine prompt based on whether there's a remote branch
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

		// Force delete the local branch
		if err := internal.RunCommandWithOutput("git", "branch", "-D", branch); err != nil {
			return fmt.Errorf("failed to force delete branch '%s': %v", branch, err)
		}
		fmt.Printf("Force deleted local branch '%s'.\n", branch)
	} else {
		fmt.Printf("Deleted local branch '%s'.\n", branch)
	}

	// Delete the remote branch if requested
	if needsToDeleteRemote && remoteName != "" && remoteBranchName != "" {
		if err := internal.RunCommandWithOutput("git", "push", "--delete", remoteName, remoteBranchName); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not delete remote branch '%s'.\n", upstreamBranch)
			return fmt.Errorf("failed to delete remote branch: %v", err)
		}
		fmt.Printf("Deleted remote branch '%s'.\n", upstreamBranch)
	}

	return nil
}
