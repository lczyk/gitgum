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

// collectOutput runs the cascade: working-tree diff first; if empty, the
// staged (--cached) diff; if still empty, the last commit (HEAD~1..HEAD).
// returns the trimmed git stdout (may be empty if all three are empty, e.g.
// a one-commit repo with a clean tree).
func (d *DiffCommand) collectOutput() (string, error) {
	colorFlag := "--color=never"
	if colorEnabled() {
		colorFlag = "--color=always"
	}

	// 1. working tree vs index (unstaged changes).
	out, _, err := d.repo().Run("diff", "--compact-summary", colorFlag)
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	if out != "" {
		return out, nil
	}

	// 2. index vs HEAD (staged-only).
	out, _, err = d.repo().Run("diff", "--cached", "--compact-summary", colorFlag)
	if err != nil {
		return "", fmt.Errorf("git diff --cached: %w", err)
	}
	if out != "" {
		return out, nil
	}

	// 3. clean tree -- show the last commit. on a single-commit repo HEAD~1
	// doesn't exist; swallow the error and return empty in that case.
	out, _, err = d.repo().Run("diff", "--compact-summary", colorFlag, "HEAD~1..HEAD")
	if err != nil {
		return "", nil
	}
	return out, nil
}

func (d *DiffCommand) renderPassthrough(w io.Writer) error {
	out, err := d.collectOutput()
	if err != nil {
		return err
	}
	if out == "" {
		return nil
	}
	fmt.Fprintln(w, out)
	return nil
}
