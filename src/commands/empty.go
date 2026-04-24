package commands

import (
	"fmt"

	"github.com/lczyk/gitgum/src/internal"
)

type EmptyCommand struct{}

func (e *EmptyCommand) Execute(args []string) error {
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	currentBranch, err := internal.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("error getting current branch: %w", err)
	}

	upstream, err := internal.GetCurrentBranchUpstream()
	if err != nil {
		return fmt.Errorf("error getting upstream branch: %w", err)
	}

	ahead, err := internal.IsBranchAheadOfRemote(currentBranch, upstream)
	if err != nil {
		return fmt.Errorf("error checking if branch is ahead of remote: %w", err)
	}
	if ahead {
		return fmt.Errorf("refusing to create empty commit: branch '%s' is ahead of remote '%s'", currentBranch, upstream)
	}

	message := "chore: empty commit"
	if err := internal.RunCommandWithOutput("git", "commit", "--allow-empty", "-m", message); err != nil {
		return fmt.Errorf("failed to create empty commit: %w", err)
	}

	fmt.Printf("Created empty commit on branch '%s'.\n", currentBranch)

	confirmed, err := internal.FzfConfirm("Do you want to push this commit to the remote?", true)
	if err != nil {
		return err
	}
	if confirmed {
		if err := internal.RunCommandWithOutput("git", "push"); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
		fmt.Printf("Pushed to remote.\n")
	} else {
		fmt.Println("Not pushing.")
	}

	return nil
}