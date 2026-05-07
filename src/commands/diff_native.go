package commands

import (
	"fmt"
	"io"
	"strings"
)

func (d *DiffCommand) renderNative(w io.Writer) error {
	colorFlag := "--color=never"
	if colorEnabled() {
		colorFlag = "--color=always"
	}
	gitArgs := []string{"diff", "--compact-summary", colorFlag}
	stdout, _, err := d.repo().Run(gitArgs...)
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}
	if stdout == "" {
		return nil
	}
	// ingest line-by-line so future per-line transforms have a seam.
	// today: identity reprint -- byte-identical to renderPassthrough.
	for _, line := range strings.Split(stdout, "\n") {
		fmt.Fprintln(w, line)
	}
	return nil
}
