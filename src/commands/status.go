package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/src/internal"
)

type StatusCommand struct{}

func (s *StatusCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Show the remote branches and their status
	internal.PrintHeader("--- BRANCHES ---------------------------")
	if err := internal.RunCommandWithOutput("git", "--no-pager", "branch", "-vv"); err != nil {
		return fmt.Errorf("error getting branches: %w", err)
	}

	// Show remotes
	stdout, _, err := internal.RunCommand("git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error getting remotes: %w", err)
	}

	if stdout != "" {
		// Process remotes to get unique entries (remove duplicates from fetch/push)
		remotes := parseRemotes(stdout)
		if len(remotes) > 0 {
			internal.PrintHeader("--- REMOTES ----------------------------")
			for _, remote := range remotes {
				fmt.Println(remote)
			}
		}
	}

	// Check whether there are any changes in the working directory
	changes, _, err := internal.RunCommand("git", "status", "--short")
	if err != nil {
		return fmt.Errorf("error getting status: %w", err)
	}

	if changes != "" {
		internal.PrintHeader("--- CHANGES ----------------------------")
		fmt.Println(changes)
	}

	// Show the status of the repository at the very end
	internal.PrintHeader("--- STATUS -----------------------------")

	stdout, _, err = internal.RunCommand("git", "status", "--short", "--branch")
	if err != nil {
		return fmt.Errorf("error getting status: %w", err)
	}

	lines := strings.Split(stdout, "\n")
	if len(lines) > 0 {
		fmt.Println(lines[0])
	}

	return nil
}

func parseRemotes(remoteOutput string) []string {
	lines := strings.Split(remoteOutput, "\n")
	seen := make(map[string]bool)
	var remotes []string

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Split by whitespace and take first two fields (name and URL)
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			entry := fields[0] + " " + fields[1]
			if !seen[entry] {
				seen[entry] = true
				remotes = append(remotes, entry)
			}
		}
	}

	return remotes
}
