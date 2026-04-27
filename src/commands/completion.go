package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lczyk/gitgum/src/completions"
)

type CompletionCommand struct {
	cmdIO
	Args struct {
		Shell string `positional-arg-name:"shell" description:"Shell type (fish, bash, or zsh)"`
	} `positional-args:"yes" required:"yes"`

	cmdName string // injectable for testing; empty falls back to os.Args[0]
}

func (c *CompletionCommand) Execute(args []string) error {
	cmdName := c.cmdName
	if cmdName == "" {
		cmdName = filepath.Base(os.Args[0])
	}

	result, err := completions.Render(c.Args.Shell, cmdName)
	if err != nil {
		return fmt.Errorf("rendering completion: %w", err)
	}

	fmt.Fprint(c.out(), result)
	return nil
}
