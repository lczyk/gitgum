package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lczyk/gitgum/src/completions"
)

type CompletionCommand struct {
	cmdIO
	Cd   bool `long:"cd" description:"Print the cd wrapper for worktree-switch instead of the completions"`
	Args struct {
		Shell string `positional-arg-name:"shell" description:"Shell type (bash, fish, zsh, or nu)"`
	} `positional-args:"yes" required:"yes"`

	cmdName string // injectable for testing; empty falls back to os.Args[0]
}

func (c *CompletionCommand) Execute(args []string) error {
	cmdName := c.cmdName
	if cmdName == "" {
		cmdName = filepath.Base(os.Args[0])
	}

	render := completions.Render
	if c.Cd {
		render = completions.RenderCd
	}
	result, err := render(c.Args.Shell, cmdName)
	if err != nil {
		return fmt.Errorf("rendering completion: %w", err)
	}

	fmt.Fprint(c.out(), result)
	return nil
}
