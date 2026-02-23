package commands

import (
	"fmt"
	"os"

	"github.com/lczyk/gitgum/src/internal"
)

// PushCommand handles pushing the current branch to a remote
type PushCommand struct{}

// Execute runs the push command
func (p *PushCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Check if this branch already has a remote tracking branch
	remoteBranch, err := internal.GetCurrentBranchUpstream()
	if err == nil && remoteBranch != "" {
		// The current branch already has a remote tracking branch
		fmt.Printf("Current branch already has a remote tracking branch: %s\n", remoteBranch)
		confirmed, err := internal.FzfConfirm("Do you want to push to the remote tracking branch?", true)
		if err != nil {
			return err
		}
		if confirmed {
			if err := internal.RunCommandWithOutput("git", "push"); err != nil {
				return fmt.Errorf("failed to push: %v", err)
			}
			fmt.Printf("Pushed to remote tracking branch '%s'.\n", remoteBranch)
			return nil
		}
		fmt.Println("Not pushing to remote tracking branch")
	}

	// Get current branch
	currentBranch, err := internal.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("error getting current branch: %v", err)
	}

	// Get list of remotes
	remotes, err := internal.GetRemotes()
	if err != nil {
		return fmt.Errorf("error getting remotes: %v", err)
	}

	if len(remotes) == 0 {
		fmt.Fprintln(os.Stderr, "No remotes found. Aborting push.")
		return fmt.Errorf("no remotes")
	}

	// Let user select a remote
	remote, err := internal.FzfSelect(fmt.Sprintf("Push '%s' to", currentBranch), remotes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No remote selected. Aborting push.")
		return err
	}

	expectedRemoteBranchName := remote + "/" + currentBranch

	// Check if the remote branch exists
	remoteBranchExists, err := internal.RemoteBranchExists(remote, currentBranch)
	if err != nil {
		return fmt.Errorf("error checking remote branch: %v", err)
	}

	if remoteBranchExists {
		// The remote branch already exists
		// Check if there are any changes to push
		localCommit, err := internal.GetCommitHash(currentBranch)
		if err != nil {
			return fmt.Errorf("error getting local commit: %v", err)
		}

		remoteCommit, err := internal.GetCommitHash(expectedRemoteBranchName)
		if err != nil {
			return fmt.Errorf("could not find remote branch '%s': %v", expectedRemoteBranchName, err)
		}

		if localCommit == remoteCommit {
			fmt.Printf("No changes to push. Local branch '%s' is up to date with remote branch '%s'.\n",
				currentBranch, expectedRemoteBranchName)
			// Set upstream since we're targeting this remote
			if err := internal.RunCommandQuiet("git", "branch", "--set-upstream-to="+expectedRemoteBranchName, currentBranch); err != nil {
				return fmt.Errorf("failed to set upstream: %v", err)
			}
			fmt.Printf("Updated upstream to '%s'.\n", expectedRemoteBranchName)
			return nil
		}

		// Confirm push to existing remote branch
		confirmed, err := internal.FzfConfirm(fmt.Sprintf("Remote branch '%s' already exists. Do you want to push to it?",
			expectedRemoteBranchName), true)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}

		if err := internal.RunCommandWithOutput("git", "push", remote, currentBranch); err != nil {
			return fmt.Errorf("failed to push: %v", err)
		}
	} else {
		// No remote branch found - create it
		confirmed, err := internal.FzfConfirm(fmt.Sprintf("No remote branch '%s' found. Do you want to create it?",
			expectedRemoteBranchName), false)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}

		if err := internal.RunCommandWithOutput("git", "push", "-u", remote, currentBranch); err != nil {
			return fmt.Errorf("failed to push: %v", err)
		}

		fmt.Printf("Created and set tracking reference for '%s' to '%s'.\n",
			currentBranch, expectedRemoteBranchName)
	}

	return nil
}
