package commands

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/lczyk/gitgum/src/internal"
)

// ReplayListCommand handles listing commits on branch A since divergence from trunk B
type ReplayListCommand struct {
	Args struct {
		BranchA string `positional-arg-name:"A" description:"Feature branch with commits to list"`
		BranchB string `positional-arg-name:"B" description:"Trunk/base branch"`
	} `positional-args:"yes" required:"yes"`
}

// Execute runs the replay-list command
func (r *ReplayListCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	branchA := r.Args.BranchA
	branchB := r.Args.BranchB

	// Compute merge base between A and B
	mergeBase, _, err := internal.RunCommand("git", "merge-base", branchA, branchB)
	if err != nil {
		return fmt.Errorf("failed to find merge base between '%s' and '%s': %v", branchA, branchB, err)
	}

	if mergeBase == "" {
		return fmt.Errorf("no merge base found between '%s' and '%s'", branchA, branchB)
	}

	// List commits from merge-base to A in reverse (chronological) order
	revRange := mergeBase + ".." + branchA
	cmd := exec.Command("git", "rev-list", revRange, "--reverse")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to list commits: %v", err)
	}

	return nil
}
