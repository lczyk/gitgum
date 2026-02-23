package commands

import (
	"fmt"

	"github.com/lczyk/gitgum/src/internal"
)

// EmptyCommand creates an empty commit and optionally pushes it
type EmptyCommand struct{}

// Execute runs the empty command
func (e *EmptyCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Get current branch
	currentBranch, err := internal.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("error getting current branch: %v", err)
	}

	// Get upstream tracking branch
	upstream, err := internal.GetCurrentBranchUpstream()
	if err != nil {
		return fmt.Errorf("error getting upstream branch: %v", err)
	}
	if upstream == "" {
		return fmt.Errorf("current branch '%s' has no upstream remote tracking branch", currentBranch)
	}

	// Check if the branch is ahead of the remote
	ahead, err := internal.IsBranchAheadOfRemote(currentBranch, upstream)
	if err != nil {
		return fmt.Errorf("error checking if branch is ahead of remote: %v", err)
	}
	if ahead {
		return fmt.Errorf("refusing to create empty commit: branch '%s' is ahead of remote '%s'", currentBranch, upstream)
	}

	// Create empty commit
	message := "chore: empty commit"
	if err := internal.RunCommandWithOutput("git", "commit", "--allow-empty", "-m", message); err != nil {
		return fmt.Errorf("failed to create empty commit: %v", err)
	}

	fmt.Printf("Created empty commit on branch '%s'.\n", currentBranch)

	// Ask whether to push
	confirmed, err := internal.FzfConfirm("Do you want to push this commit to the remote?", true)
	if err != nil {
		return err
	}
	if confirmed {
		if err := internal.RunCommandWithOutput("git", "push"); err != nil {
			return fmt.Errorf("failed to push: %v", err)
		}
		fmt.Printf("Pushed to remote.\n")
	} else {
		fmt.Println("Not pushing.")
	}

	return nil
}