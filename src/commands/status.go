package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type StatusCommand struct{}

func (s *StatusCommand) Execute(args []string) error {
	if err := git.CheckInRepo(); err != nil {
		return fmt.Errorf("checking git repo: %w", err)
	}

	ui.PrintHeader("--- BRANCHES ---------------------------")
	if err := cmdrun.RunWithOutput("git", "--no-pager", "branch", "-vv"); err != nil {
		return fmt.Errorf("error getting branches: %w", err)
	}

	stdout, _, err := cmdrun.Run("git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error getting remotes: %w", err)
	}
	remotes := parseRemotes(stdout)
	if len(remotes) > 0 {
		ui.PrintHeader("--- REMOTES ----------------------------")
		for _, remote := range remotes {
			fmt.Println(remote)
		}
	}

	// single call gets both branch status (line 0) and change lines (rest)
	stdout, _, err = cmdrun.Run("git", "status", "--short", "--branch")
	if err != nil {
		return fmt.Errorf("error getting status: %w", err)
	}
	lines := strings.Split(stdout, "\n")

	if len(lines) > 1 {
		ui.PrintHeader("--- CHANGES ----------------------------")
		fmt.Println(strings.Join(lines[1:], "\n"))
	}

	ui.PrintHeader("--- STATUS -----------------------------")
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
