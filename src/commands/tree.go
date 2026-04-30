package commands

import (
	"fmt"
	"slices"
	"strings"

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
// swap is local to this function. The reverse-and-flip dance below also goes
// away once we render natively — we'll just emit oldest-first directly.
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

	fmt.Fprintln(t.out(), reverseGraph(stdout))
	return nil
}

// reverseGraph flips git's --graph output so the tip lands at the bottom
// (right above the next prompt). git refuses --graph + --reverse, so we
// reverse line order and swap '/' <-> '\' in each line's graph prefix to
// keep diagonal connectors pointing the right way after the y-axis flip.
func reverseGraph(out string) string {
	lines := strings.Split(out, "\n")
	slices.Reverse(lines)
	for i, line := range lines {
		lines[i] = swapGraphSlashes(line)
	}
	return strings.Join(lines, "\n")
}

// swapGraphSlashes swaps '/' and '\' in the graph-drawing prefix of a line.
// The prefix is everything before the first letter or digit (which would
// belong to the hash or subject), skipping over ANSI SGR escapes since
// `--color=always` wraps the graph chars themselves in color codes.
func swapGraphSlashes(line string) string {
	end := 0
	for end < len(line) {
		c := line[end]
		if c == 0x1b && end+1 < len(line) && line[end+1] == '[' {
			// skip ANSI SGR: ESC [ ... m
			end += 2
			for end < len(line) && line[end] != 'm' {
				end++
			}
			if end < len(line) {
				end++ // consume the 'm'
			}
			continue
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			break
		}
		end++
	}
	if end == 0 {
		return line
	}
	swapped := strings.Map(func(r rune) rune {
		switch r {
		case '/':
			return '\\'
		case '\\':
			return '/'
		}
		return r
	}, line[:end])
	return swapped + line[end:]
}
