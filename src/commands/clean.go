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
	affectedFiles := getAffectedFiles(changes, untracked, ignored)

	// Show summary
	if len(affectedFiles) == 0 {
		fmt.Println("Nothing to clean (working tree is clean)")
		return nil
	}

	// Check if any .gitignore files are affected
	gitignoreFiles := []string{}
	for _, file := range affectedFiles {
		if file == ".gitignore" || len(file) > 11 && file[len(file)-11:] == "/.gitignore" {
			gitignoreFiles = append(gitignoreFiles, file)
		}
	}

	// If .gitignore files are affected, we need a two-stage cleanup
	if len(gitignoreFiles) > 0 {
		// First, clean .gitignore files to get accurate list of what else will be affected
		fmt.Println("Detected changes to .gitignore files. Applying .gitignore changes first to get accurate cleanup preview...")
		
		// Clean .gitignore files
		if err := cleanGitignoreFiles(gitignoreFiles, changes, untracked); err != nil {
			return err
		}

		// Re-evaluate affected files after .gitignore changes
		affectedFiles = getAffectedFiles(changes, untracked, ignored)

		// Check if there's anything left to clean
		if len(affectedFiles) == 0 {
			fmt.Println("Clean complete")
			return nil
		}

		// Restore .gitignore files before prompting
		fmt.Println("Restoring .gitignore files for confirmation...")
		if err := restoreGitignoreFiles(gitignoreFiles); err != nil {
			return fmt.Errorf("failed to restore .gitignore files: %v", err)
		}

		// Add gitignore files back to the affected files list for display
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

		// User confirmed - clean .gitignore files again if needed
		if len(gitignoreFiles) > 0 {
			fmt.Println("Re-applying .gitignore cleanup...")
			if err := cleanGitignoreFiles(gitignoreFiles, changes, untracked); err != nil {
				return err
			}
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

// getAffectedFiles returns a list of files that will be affected by the clean operation
func getAffectedFiles(changes, untracked, ignored bool) []string {
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
	
	return affectedFiles
}

// gitignoreState tracks the state of a .gitignore file before cleaning
type gitignoreState struct {
	file       string
	fileStatus internal.GitFileStatus
	content    string // Content if untracked
}

var gitignoreBackup []gitignoreState

// cleanGitignoreFiles handles cleaning only .gitignore files
func cleanGitignoreFiles(gitignoreFiles []string, changes, untracked bool) error {
	gitignoreBackup = []gitignoreState{}
	
	// For each .gitignore file, check if it's modified or untracked
	for _, file := range gitignoreFiles {
		status, err := internal.GetGitFileStatus(file)
		if err != nil || status == internal.GitFileUnknown {
			continue // File doesn't exist or no changes
		}
		
		// Backup the state
		state := gitignoreState{
			file:       file,
			fileStatus: status,
		}
		
		// Untracked file
		if status == internal.GitFileUntracked {
			if untracked {
				// Read content before removing
				content, _, _ := internal.RunCommand("cat", file)
				state.content = content
				gitignoreBackup = append(gitignoreBackup, state)
				
				_, _, err := internal.RunCommand("rm", file)
				if err != nil {
					return fmt.Errorf("failed to remove %s: %v", file, err)
				}
			}
		} else if changes {
			// Modified, staged, or deleted file - save state and reset it
			gitignoreBackup = append(gitignoreBackup, state)
			_, _, err := internal.RunCommand("git", "checkout", "HEAD", "--", file)
			if err != nil {
				return fmt.Errorf("failed to reset %s: %v", file, err)
			}
		}
	}
	
	return nil
}

// restoreGitignoreFiles restores .gitignore files to their state before cleaning
func restoreGitignoreFiles(gitignoreFiles []string) error {
	for _, state := range gitignoreBackup {
		// Restore untracked files
		if state.fileStatus == internal.GitFileUntracked {
			// Write content back
			if err := internal.WriteFile(state.file, state.content); err != nil {
				return fmt.Errorf("failed to restore %s: %v", state.file, err)
			}
		} else {
			// For modified files, we need to restore them from the index/working tree
			// This is tricky - we'll use git stash to restore
			_, _, err := internal.RunCommand("git", "stash", "push", "-m", "gitgum-restore", "--", state.file)
			if err != nil {
				// If stash fails, the file might already be in the right state
				continue
			}
			_, _, _ = internal.RunCommand("git", "stash", "pop")
		}
	}
	gitignoreBackup = nil
	return nil
}
