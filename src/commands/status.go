package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
)

type StatusCommand struct {
	// injectable for testing; nil falls back to os.Stdout
	out io.Writer
}

func (s *StatusCommand) Execute(args []string) error {
	out := s.out
	if out == nil {
		out = os.Stdout
	}

	if err := git.CheckInRepo(); err != nil {
		return fmt.Errorf("checking git repo: %w", err)
	}

	printHeader(out, "--- BRANCHES ---------------------------")
	stdout, _, err := cmdrun.Run("git", "--no-pager", "branch", "-vv")
	if err != nil {
		return fmt.Errorf("error getting branches: %w", err)
	}
	fmt.Fprintln(out, stdout)

	stdout, _, err = cmdrun.Run("git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error getting remotes: %w", err)
	}
	remotes := parseRemotes(stdout)
	if len(remotes) > 0 {
		printHeader(out, "--- REMOTES ----------------------------")
		for _, remote := range remotes {
			fmt.Fprintln(out, remote)
		}
	}

	// single call gets both branch status (line 0) and change lines (rest)
	stdout, _, err = cmdrun.Run("git", "status", "--short", "--branch")
	if err != nil {
		return fmt.Errorf("error getting status: %w", err)
	}
	lines := strings.Split(stdout, "\n")

	if len(lines) > 1 {
		printHeader(out, "--- CHANGES ----------------------------")
		fmt.Fprintln(out, strings.Join(lines[1:], "\n"))
	}

	printHeader(out, "--- STATUS -----------------------------")
	fmt.Fprintln(out, lines[0])

	return nil
}

func printHeader(w io.Writer, msg string) {
	fmt.Fprintf(w, "\033[0;30m%s\033[0m\n", msg)
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
