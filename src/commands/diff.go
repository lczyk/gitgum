package commands

import (
	"fmt"
	"io"
	"os"
)

type DiffCommand struct {
	cmdIO
}

func (d *DiffCommand) Execute(args []string) error {
	if err := d.repo().CheckInRepo(); err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("diff takes no arguments")
	}
	return d.render(d.out())
}

func (d *DiffCommand) render(w io.Writer) error {
	if os.Getenv("GG_DIFF_NATIVE") == "1" {
		return d.renderNative(w)
	}
	return d.renderPassthrough(w)
}

func (d *DiffCommand) renderPassthrough(w io.Writer) error {
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
	fmt.Fprintln(w, stdout)
	return nil
}
