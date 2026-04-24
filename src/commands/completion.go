package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lczyk/gitgum/src/completions"
)

type CompletionCommand struct {
	Args struct {
		Shell string `positional-arg-name:"shell" description:"Shell type (fish, bash, or zsh)"`
	} `positional-args:"yes" required:"yes"`
}

func (c *CompletionCommand) Execute(args []string) error {
	shell := c.Args.Shell
	content, ok := completions.CompletionTemplates[shell]
	if !ok {
		return fmt.Errorf("invalid shell type '%s'. Must be one of: bash, fish, zsh", shell)
	}

	cmdName := filepath.Base(os.Args[0])
	output := strings.ReplaceAll(content, "__GITGUM_CMD__", cmdName)
	fmt.Print(output)
	return nil
}
