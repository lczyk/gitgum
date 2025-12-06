package commands

import (
	"fmt"

	"github.com/lczyk/gitgum/src/internal"
)

// CleanCommand handles discarding working tree changes and untracked files
type CleanCommand struct {
	Changes   *bool `long:"changes" description:"Discard staged and unstaged changes (default: true)"`
	Untracked *bool `long:"untracked" description:"Remove untracked files (default: true)"`
	Ignored   *bool `long:"ignored" description:"Remove ignored files (default: false)"`
	All       bool  `long:"all" description:"Enable all cleanup options"`
	Yes       bool  `short:"y" long:"yes" description:"Skip confirmation prompt"`
}

// Execute runs the clean command
func (c *CleanCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Apply defaults
	changes := true
	untracked := true
	ignored := false

	// Override defaults with explicit flags
	if c.Changes != nil {
		changes = *c.Changes
	}
	if c.Untracked != nil {
		untracked = *c.Untracked
	}
	if c.Ignored != nil {
		ignored = *c.Ignored
	}

	// If --all is specified, enable all cleanup options
	if c.All {
		changes = true
		untracked = true
		ignored = true
	}

	// If --ignored is specified, it implies --untracked
	if ignored {
		untracked = true
	}

	// If nothing is enabled, nothing to do
	if !changes && !untracked {
		fmt.Println("Nothing to clean (all options disabled)")
		return nil
	}

	// Build summary of what will be affected
	var affectedFiles []string
	
	if changes {
		// Get list of modified files
		stdout, _, err := internal.RunCommand("git", "diff", "--name-only")
		if err == nil && stdout != "" {
			affectedFiles = append(affectedFiles, internal.SplitLines(stdout)...)
		}
		// Get staged files
		stdout, _, err = internal.RunCommand("git", "diff", "--cached", "--name-only")
		if err == nil && stdout != "" {
			affectedFiles = append(affectedFiles, internal.SplitLines(stdout)...)
		}
	}
	
	if untracked {
		// Get list of untracked files
		cleanArgs := []string{"clean", "-fdn"}
		if ignored {
			cleanArgs = append(cleanArgs, "-x")
		}
		stdout, _, err := internal.RunCommand("git", cleanArgs...)
		if err == nil && stdout != "" {
			// Parse output like "Would remove file.txt"
			for _, line := range internal.SplitLines(stdout) {
				if len(line) > 13 && line[:13] == "Would remove " {
					affectedFiles = append(affectedFiles, line[13:])
				}
			}
		}
	}

	// Show summary
	if len(affectedFiles) == 0 {
		fmt.Println("Nothing to clean (working tree is clean)")
		return nil
	}

	fmt.Printf("Files to be discarded (%d):\n", len(affectedFiles))
	maxDisplay := 20
	for i, file := range affectedFiles {
		if i >= maxDisplay {
			fmt.Printf("  ... and %d more files\n", len(affectedFiles)-maxDisplay)
			break
		}
		fmt.Printf("  %s\n", file)
	}
	fmt.Println()

	if !c.Yes {
		confirmed, err := internal.FzfConfirm("Proceed with cleanup? This cannot be undone", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Cleanup cancelled")
			return nil
		}
	}

	// Execute git reset --hard if changes should be discarded
	if changes {
		fmt.Println("Discarding changes...")
		stdout, stderr, err := internal.RunCommand("git", "reset", "--hard")
		if err != nil {
			return fmt.Errorf("failed to reset changes: %v\nStdout: %s\nStderr: %s", err, stdout, stderr)
		}
		if stdout != "" {
			fmt.Println(stdout)
		}
	}

	// Execute git clean if untracked files should be removed
	if untracked {
		cleanArgs := []string{"clean", "-fd"}
		if ignored {
			cleanArgs = append(cleanArgs, "-x")
		}

		fmt.Println("Removing untracked files...")
		stdout, stderr, err := internal.RunCommand("git", cleanArgs...)
		if err != nil {
			return fmt.Errorf("failed to clean untracked files: %v\nStdout: %s\nStderr: %s", err, stdout, stderr)
		}
		if stdout != "" {
			fmt.Println(stdout)
		}
	}

	fmt.Println("Clean complete")
	return nil
}
