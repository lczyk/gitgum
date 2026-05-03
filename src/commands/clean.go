package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/strutil"
)

const gitCleanDryRunPrefix = "Would remove "

// CleanCommand handles discarding working tree changes and untracked files
type CleanCommand struct {
	cmdIO
	Changes   *bool `long:"changes" description:"Discard staged and unstaged changes (default: true)"`
	Untracked *bool `long:"untracked" description:"Remove untracked files (default: true)"`
	Ignored   *bool `long:"ignored" description:"Remove ignored files (default: false)"`
	All       bool  `long:"all" description:"Enable all cleanup options"`
	Yes       bool  `short:"y" long:"yes" description:"Skip confirmation prompt"`
}

func (c *CleanCommand) Execute(args []string) error {
	r := c.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	changes := c.Changes == nil || *c.Changes
	untracked := c.Untracked == nil || *c.Untracked
	ignored := c.Ignored != nil && *c.Ignored

	if c.All {
		changes = true
		untracked = true
		ignored = true
	}

	// --ignored implies --untracked
	if ignored {
		untracked = true
	}

	if !changes && !untracked {
		fmt.Fprintln(c.out(), "Nothing to clean (all options disabled)")
		return nil
	}

	affectedFiles, err := getAffectedFiles(r, changes, untracked, ignored)
	if err != nil {
		return err
	}

	if len(affectedFiles) == 0 {
		fmt.Fprintln(c.out(), "Nothing to clean (working tree is clean)")
		return nil
	}

	fmt.Fprintf(c.out(), "Files to be discarded (%d):\n", len(affectedFiles))
	maxDisplay := 20
	for i, file := range affectedFiles {
		if i >= maxDisplay {
			fmt.Fprintf(c.out(), "  ... and %d more files\n", len(affectedFiles)-maxDisplay)
			break
		}
		fmt.Fprintf(c.out(), "  %s\n", file)
	}
	fmt.Fprintln(c.out())

	if !c.Yes {
		confirmed, err := c.sel().Confirm("Proceed with cleanup? This cannot be undone", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(c.out(), "Cleanup cancelled")
			return nil
		}
	}

	if changes {
		fmt.Fprintln(c.out(), "Discarding changes...")
		if _, stderr, err := r.RunWrite("reset", "--hard"); err != nil {
			return fmt.Errorf("failed to reset changes: %w: %s", err, strings.TrimSpace(stderr))
		}
	}

	if untracked {
		fmt.Fprintln(c.out(), "Removing untracked files...")
		if _, stderr, err := r.RunWrite(gitCleanArgs(false, ignored)...); err != nil {
			return fmt.Errorf("failed to clean untracked files: %w: %s", err, strings.TrimSpace(stderr))
		}
	}

	fmt.Fprintln(c.out(), "Clean complete")
	return nil
}

func getAffectedFiles(r git.Repo, changes, untracked, ignored bool) ([]string, error) {
	var affectedFiles []string

	if changes {
		stdout, _, err := r.Run("diff", "--name-only")
		if err != nil {
			return nil, fmt.Errorf("listing modified files: %w", err)
		}
		affectedFiles = append(affectedFiles, strutil.SplitLines(stdout)...)

		stdout, _, err = r.Run("diff", "--cached", "--name-only")
		if err != nil {
			return nil, fmt.Errorf("listing staged files: %w", err)
		}
		affectedFiles = append(affectedFiles, strutil.SplitLines(stdout)...)
	}

	if untracked {
		stdout, _, err := r.Run(gitCleanArgs(true, ignored)...)
		if err != nil {
			return nil, fmt.Errorf("listing untracked files: %w", err)
		}
		for _, line := range strutil.SplitLines(stdout) {
			if trimmed, ok := strings.CutPrefix(line, gitCleanDryRunPrefix); ok {
				affectedFiles = append(affectedFiles, trimmed)
			}
		}
	}

	return affectedFiles, nil
}

// gitCleanArgs builds git clean args. dry-run appends -n; ignored adds -x.
func gitCleanArgs(dryRun, ignored bool) []string {
	flags := "-fd"
	if dryRun {
		flags += "n"
	}
	args := []string{"clean", flags}
	if ignored {
		args = append(args, "-x")
	}
	return args
}
