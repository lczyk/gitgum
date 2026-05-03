package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

type ReplayListCommand struct {
	cmdIO
	Args struct {
		BranchA string `positional-arg-name:"A" description:"Feature branch with commits to list"`
		BranchB string `positional-arg-name:"B" description:"Trunk/base branch"`
	} `positional-args:"yes" required:"yes"`
}

// returns commits on branchA since divergence from branchB, oldest-first
func listCommits(repo git.Repo, branchA, branchB string) ([]string, error) {
	mergeBase, _, err := repo.Run("merge-base", branchA, branchB)
	if err != nil {
		return nil, fmt.Errorf("failed to find merge base between '%s' and '%s': %w", branchA, branchB, err)
	}

	// --reverse flips git rev-list from newest-first to chronological order
	output, _, err := repo.Run("rev-list", mergeBase+".."+branchA, "--reverse")
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	if output == "" {
		return nil, nil
	}

	return strings.Split(output, "\n"), nil
}

func (r *ReplayListCommand) Execute(args []string) error {
	repo := r.repo()
	if err := repo.CheckInRepo(); err != nil {
		return err
	}

	commits, err := listCommits(repo, r.Args.BranchA, r.Args.BranchB)
	if err != nil {
		return err
	}

	for _, commit := range commits {
		fmt.Fprintln(r.out(), commit)
	}

	return nil
}
