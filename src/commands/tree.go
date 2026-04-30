package commands

import (
	"fmt"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
)

type TreeCommand struct {
	cmdIO
	Since string `long:"since" default:"2 weeks ago" description:"git log --since value; empty disables the time filter"`
}

// TODO: replace the `git log --graph` shell-out with an in-process renderer.
// Plan: fetch raw commit/parent/ref data from git (e.g. `git log --format=...`
// + `git for-each-ref`), build the graph ourselves, and render it. Initial
// output should match this format byte-for-byte; later it gains interactive
// fuzzyfinder features. Keep tests structural (no exact-format pins) so the
// swap is local to this function.
func (t *TreeCommand) Execute(args []string) error {
	if err := git.CheckInRepo(); err != nil {
		return err
	}

	gitArgs := []string{"log", "--graph", "--oneline", "--all", "--decorate", "--color=always"}
	if t.Since != "" {
		gitArgs = append(gitArgs, "--since", t.Since)
	}

	stdout, _, err := cmdrun.Run("git", gitArgs...)
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}

	fmt.Fprintln(t.out(), stdout)
	return nil
}
