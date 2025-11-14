package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CompletionCommand handles shell completion generation
type CompletionCommand struct {
	Args struct {
		Shell string `positional-arg-name:"shell" description:"Shell type (fish, bash, or zsh)"`
	} `positional-args:"yes" required:"yes"`
}

// Execute runs the completion command
func (c *CompletionCommand) Execute(args []string) error {
	// Validate shell type
	shell := c.Args.Shell
	validShells := map[string]bool{
		"fish": true,
		"bash": true,
		"zsh":  true,
	}

	if !validShells[shell] {
		return fmt.Errorf("invalid shell type '%s'. Must be one of: fish, bash, zsh", shell)
	}

	// Get the actual command name (handles symlinks)
	cmdName := filepath.Base(os.Args[0])

	// Find the completion template file
	// The templates are in bash/src/completions relative to the project root
	// We'll look relative to the executable or use a fallback
	completionFile, err := findCompletionFile(shell)
	if err != nil {
		return fmt.Errorf("completion file not found: %v", err)
	}

	// Read the template
	content, err := os.ReadFile(completionFile)
	if err != nil {
		return fmt.Errorf("error reading completion file: %v", err)
	}

	// Substitute the command name placeholder
	output := strings.ReplaceAll(string(content), "__GITGUM_CMD__", cmdName)

	// Output to stdout
	fmt.Print(output)

	return nil
}

// findCompletionFile locates the completion template file
func findCompletionFile(shell string) (string, error) {
	filename := fmt.Sprintf("gitgum.%s", shell)

	// Try relative to executable (for development)
	exePath, err := os.Executable()
	if err == nil {
		// From go/bin/gitgum -> bash/src/completions
		candidatePaths := []string{
			filepath.Join(filepath.Dir(exePath), "..", "..", "bash", "src", "completions", filename),
			filepath.Join(filepath.Dir(exePath), "..", "bash", "src", "completions", filename),
		}

		for _, path := range candidatePaths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	// Try relative to working directory
	cwd, err := os.Getwd()
	if err == nil {
		candidatePaths := []string{
			filepath.Join(cwd, "bash", "src", "completions", filename),
			filepath.Join(cwd, "..", "bash", "src", "completions", filename),
			filepath.Join(cwd, "..", "..", "bash", "src", "completions", filename),
		}

		for _, path := range candidatePaths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("completion file not found for shell: %s", shell)
}
