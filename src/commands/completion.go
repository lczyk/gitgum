package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lczyk/gitgum/src/completions"
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

	// Get the embedded template
	content, ok := completions.CompletionTemplates[shell]
	if !ok {
		return fmt.Errorf("completion template not found for shell: %s", shell)
	}

	// Substitute the command name placeholder
	output := strings.ReplaceAll(content, "__GITGUM_CMD__", cmdName)

	// Output to stdout
	fmt.Print(output)

	return nil
}
