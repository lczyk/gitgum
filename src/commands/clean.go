package commands

import (
	"fmt"
	"path/filepath"

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
	affectedFiles, err := getAffectedFiles(changes, untracked, ignored)
	if err != nil {
		return err
	}

	// Show summary
	if len(affectedFiles) == 0 {
		fmt.Println("Nothing to clean (working tree is clean)")
		return nil
	}

	gitignoreFiles := []string{}
	for _, file := range affectedFiles {
		if filepath.Base(file) == ".gitignore" {
			gitignoreFiles = append(gitignoreFiles, file)
		}
	}

	// two-stage cleanup when .gitignore files are affected:
	// clean them first to get accurate file list, then restore for confirmation
	var gitignoreBackup []gitignoreState
	if len(gitignoreFiles) > 0 {
		fmt.Println("Detected changes to .gitignore files. Applying .gitignore changes first to get accurate cleanup preview...")

		var err error
		gitignoreBackup, err = cleanGitignoreFiles(gitignoreFiles, changes, untracked)
		if err != nil {
			return err
		}

		affectedFiles, err = getAffectedFiles(changes, untracked, ignored)
		if err != nil {
			return err
		}

		if len(affectedFiles) == 0 {
			fmt.Println("Clean complete")
			return nil
		}

		fmt.Println("Restoring .gitignore files for confirmation...")
		if err := restoreGitignoreFiles(gitignoreBackup); err != nil {
			return fmt.Errorf("failed to restore .gitignore files: %w", err)
		}

		affectedFiles = append(gitignoreFiles, affectedFiles...)
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

		if len(gitignoreFiles) > 0 {
			fmt.Println("Re-applying .gitignore cleanup...")
			if _, err := cleanGitignoreFiles(gitignoreFiles, changes, untracked); err != nil {
				return err
			}
		}
	}

	// Execute git reset --hard if changes should be discarded
	if changes {
		fmt.Println("Discarding changes...")
		stdout, stderr, err := internal.RunCommand("git", "reset", "--hard")
		if err != nil {
			return fmt.Errorf("failed to reset changes: %w\nStdout: %s\nStderr: %s", err, stdout, stderr)
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
			return fmt.Errorf("failed to clean untracked files: %w\nStdout: %s\nStderr: %s", err, stdout, stderr)
		}
		if stdout != "" {
			fmt.Println(stdout)
		}
	}

	fmt.Println("Clean complete")
	return nil
}

func getAffectedFiles(changes, untracked, ignored bool) ([]string, error) {
	var affectedFiles []string

	if changes {
		stdout, _, err := internal.RunCommand("git", "diff", "--name-only")
		if err != nil {
			return nil, fmt.Errorf("listing modified files: %w", err)
		}
		if stdout != "" {
			affectedFiles = append(affectedFiles, internal.SplitLines(stdout)...)
		}

		stdout, _, err = internal.RunCommand("git", "diff", "--cached", "--name-only")
		if err != nil {
			return nil, fmt.Errorf("listing staged files: %w", err)
		}
		if stdout != "" {
			affectedFiles = append(affectedFiles, internal.SplitLines(stdout)...)
		}
	}

	if untracked {
		cleanArgs := []string{"clean", "-fdn"}
		if ignored {
			cleanArgs = append(cleanArgs, "-x")
		}
		stdout, _, err := internal.RunCommand("git", cleanArgs...)
		if err != nil {
			return nil, fmt.Errorf("listing untracked files: %w", err)
		}
		if stdout != "" {
			for _, line := range internal.SplitLines(stdout) {
				if len(line) > 13 && line[:13] == "Would remove " {
					affectedFiles = append(affectedFiles, line[13:])
				}
			}
		}
	}

	return affectedFiles, nil
}

// gitignoreState tracks the state of a .gitignore file before cleaning
type gitignoreState struct {
	file       string
	fileStatus internal.GitFileStatus
	content    string // Content if untracked
}

// cleanGitignoreFiles handles cleaning only .gitignore files.
// returns the backup so the caller can pass it to restoreGitignoreFiles.
func cleanGitignoreFiles(gitignoreFiles []string, changes, untracked bool) ([]gitignoreState, error) {
	var backup []gitignoreState

	for _, file := range gitignoreFiles {
		status, err := internal.GetGitFileStatus(file)
		if err != nil || status == internal.GitFileUnknown {
			continue
		}

		state := gitignoreState{
			file:       file,
			fileStatus: status,
		}

		if status == internal.GitFileUntracked {
			if untracked {
				content, _, _ := internal.RunCommand("cat", file)
				state.content = content
				backup = append(backup, state)

				_, _, err := internal.RunCommand("rm", file)
				if err != nil {
					return nil, fmt.Errorf("failed to remove %s: %w", file, err)
				}
			}
		} else if changes {
			backup = append(backup, state)
			_, _, err := internal.RunCommand("git", "checkout", "HEAD", "--", file)
			if err != nil {
				return nil, fmt.Errorf("failed to reset %s: %w", file, err)
			}
		}
	}

	return backup, nil
}

func restoreGitignoreFiles(backup []gitignoreState) error {
	for _, state := range backup {
		if state.fileStatus == internal.GitFileUntracked {
			if err := internal.WriteFile(state.file, state.content); err != nil {
				return fmt.Errorf("failed to restore %s: %w", state.file, err)
			}
		} else {
			// stash round-trip restores the working-tree state
			_, _, err := internal.RunCommand("git", "stash", "push", "-m", "gitgum-restore", "--", state.file)
			if err != nil {
				continue
			}
			_, _, _ = internal.RunCommand("git", "stash", "pop")
		}
	}
	return nil
}
