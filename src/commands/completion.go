package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lczyk/gitgum/src/completions"
)

type CompletionCommand struct {
	Args struct {
		Shell string `positional-arg-name:"shell" description:"Shell type (fish, bash, or zsh)"`
	} `positional-args:"yes" required:"yes"`

	// injectable for testing; nil/zero falls back to os defaults
	out     io.Writer
	cmdName string
}

func (c *CompletionCommand) Execute() error {
	cmdName := c.cmdName
	if cmdName == "" {
		cmdName = filepath.Base(os.Args[0])
	}

	result, err := completions.Render(c.Args.Shell, cmdName)
	if err != nil {
		return fmt.Errorf("rendering completion: %w", err)
	}

	w := c.out
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprint(w, result)
	return nil
}
