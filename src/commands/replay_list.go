package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
)

// ReplayListCommand handles listing commits on branch A since divergence from trunk B
type ReplayListCommand struct {
	Args struct {
		BranchA string `positional-arg-name:"A" description:"Feature branch with commits to list"`
		BranchB string `positional-arg-name:"B" description:"Trunk/base branch"`
	} `positional-args:"yes" required:"yes"`
}

// listCommits returns the list of commits on branchA since divergence from branchB,
// in chronological order.
func listCommits(branchA, branchB string) ([]string, error) {
	// Compute merge base between A and B
	mergeBase, _, err := cmdrun.Run("git", "merge-base", branchA, branchB)
	if err != nil {
		return nil, fmt.Errorf("failed to find merge base between '%s' and '%s': %w", branchA, branchB, err)
	}

	// List commits from merge-base to A in reverse (chronological) order
	revRange := mergeBase + ".." + branchA
	output, _, err := cmdrun.Run("git", "rev-list", revRange, "--reverse")
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	if output == "" {
		return nil, nil
	}

	return strings.Split(output, "\n"), nil
}

// Execute runs the replay-list command
func (r *ReplayListCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := git.CheckInRepo(); err != nil {
		return err
	}

	commits, err := listCommits(r.Args.BranchA, r.Args.BranchB)
	if err != nil {
		return err
	}

	for _, commit := range commits {
		fmt.Println(commit)
	}

	return nil
}
