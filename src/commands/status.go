package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/src/internal"
)

type StatusCommand struct{}

func (s *StatusCommand) Execute() error {
	if err := internal.CheckInGitRepo(); err != nil {
		return fmt.Errorf("checking git repo: %w", err)
	}

	internal.PrintHeader("--- BRANCHES ---------------------------")
	if err := internal.RunCommandWithOutput("git", "--no-pager", "branch", "-vv"); err != nil {
		return fmt.Errorf("error getting branches: %w", err)
	}

	stdout, _, err := internal.RunCommand("git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error getting remotes: %w", err)
	}
	remotes := parseRemotes(stdout)
	if len(remotes) > 0 {
		internal.PrintHeader("--- REMOTES ----------------------------")
		for _, remote := range remotes {
			fmt.Println(remote)
		}
	}

	// single call gets both branch status (line 0) and change lines (rest)
	stdout, _, err = internal.RunCommand("git", "status", "--short", "--branch")
	if err != nil {
		return fmt.Errorf("error getting status: %w", err)
	}
	lines := strings.Split(stdout, "\n")

	if len(lines) > 1 {
		internal.PrintHeader("--- CHANGES ----------------------------")
		fmt.Println(strings.Join(lines[1:], "\n"))
	}

	internal.PrintHeader("--- STATUS -----------------------------")
	fmt.Println(lines[0])

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
