package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

type StatusCommand struct {
	cmdIO
	Flat bool `long:"flat" description:"show changes as flat porcelain list instead of tree"`
}

func (s *StatusCommand) Execute(args []string) error {
	out := s.out()

	printHeader := func(msg string) {
		fmt.Fprintf(out, "\033[0;30m%s\033[0m\n", msg)
	}

	if err := git.CheckInRepo(); err != nil {
		return err
	}

	printHeader("--- BRANCHES ---------------------------")
	stdout, _, err := git.Run("branch", "-vv")
	if err != nil {
		return fmt.Errorf("getting branches: %w", err)
	}
	fmt.Fprintln(out, stdout)

	stdout, _, err = git.Run("remote", "-v")
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}
	remotes := parseRemotes(stdout)
	if len(remotes) > 0 {
		printHeader("--- REMOTES ----------------------------")
		for _, remote := range remotes {
			fmt.Fprintln(out, remote)
		}
	}

	// single call gets both branch status (line 0) and change lines (rest)
	stdout, _, err = git.Run("status", "--short", "--branch")
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}
	lines := strings.Split(stdout, "\n")

	changeLines := lines[1:]
	hasChanges := false
	for _, l := range changeLines {
		if l != "" {
			hasChanges = true
			break
		}
	}
	if hasChanges {
		printHeader("--- CHANGES ----------------------------")
		if s.Flat {
			fmt.Fprintln(out, strings.Join(changeLines, "\n"))
		} else {
			renderTree(buildTree(parseChangeLines(changeLines)), out)
		}
	}

	printHeader("--- STATUS -----------------------------")
	fmt.Fprintln(out, lines[0])

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
