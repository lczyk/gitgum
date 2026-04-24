package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

func (c *CompletionCommand) Execute(args []string) error {
	shell := c.Args.Shell
	content, err := completions.Get(shell)
	if err != nil {
		return err
	}

	cmdName := c.cmdName
	if cmdName == "" {
		cmdName = filepath.Base(os.Args[0])
	}

	w := c.out
	if w == nil {
		w = os.Stdout
	}

	result := strings.ReplaceAll(content, "__GITGUM_CMD__", cmdName)
	fmt.Fprint(w, result)
	return nil
}
