package commands

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/lczyk/gitgum/src/internal"
)

// StatusCommand handles showing the status of the git repository
type StatusCommand struct{}

// Execute runs the status command
func (s *StatusCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Show the remote branches and their status
	internal.PrintBlue("--- BRANCHES ---------------------------")
	if err := internal.RunCommandWithOutput("git", "--no-pager", "branch", "-vv"); err != nil {
		return fmt.Errorf("error getting branches: %v", err)
	}

	// Show remotes
	stdout, _, err := internal.RunCommand("git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error getting remotes: %v", err)
	}

	if stdout != "" {
		// Process remotes to get unique entries (remove duplicates from fetch/push)
		remotes := parseRemotes(stdout)
		if len(remotes) > 0 {
			internal.PrintBlue("--- REMOTES ----------------------------")
			for _, remote := range remotes {
				fmt.Println(remote)
			}
		}
	}

	// Check whether there are any changes in the working directory
	changes, _, err := internal.RunCommand("git", "status", "--short")
	if err != nil {
		return fmt.Errorf("error getting status: %v", err)
	}

	if changes != "" {
		internal.PrintBlue("--- CHANGES ----------------------------")
		fmt.Println(changes)
	}

	// Show the status of the repository at the very end
	internal.PrintBlue("--- STATUS -----------------------------")

	// Try to use unbuffer to preserve color output
	if isCommandAvailable("unbuffer") {
		stdout, _, err := internal.RunCommand("sh", "-c", "unbuffer git status --short --branch | head -n1")
		if err == nil {
			fmt.Println(stdout)
			return nil
		}
	}

	// Fallback: just show the first line of the status
	stdout, _, err = internal.RunCommand("git", "status", "--short", "--branch")
	if err != nil {
		return fmt.Errorf("error getting status: %v", err)
	}

	lines := strings.Split(stdout, "\n")
	if len(lines) > 0 {
		fmt.Println(lines[0])
	}

	return nil
}

// parseRemotes parses the output of 'git remote -v' and returns unique remote entries
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

// isCommandAvailable checks if a command is available in PATH
func isCommandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
