package commands

import (
	"io"
	"os"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

// cmdIO is embedded by command structs so tests can capture stdout/stderr,
// drive selection prompts, and target a specific repo. Zero value falls
// through to the process's real stdio, real fuzzyfinder, and the cwd-bound
// Repo, so production wiring (go-flags reflection constructs zero-value
// structs) keeps working unchanged.
type cmdIO struct {
	Out  io.Writer
	Err  io.Writer
	UI   ui.Selector
	Repo git.Repo
}

func (c *cmdIO) out() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	return os.Stdout
}

func (c *cmdIO) err() io.Writer {
	if c.Err != nil {
		return c.Err
	}
	return os.Stderr
}

func (c *cmdIO) sel() ui.Selector {
	if c.UI != nil {
		return c.UI
	}
	return ui.RealSelector{}
}

// repo returns the cmdIO's Repo. Zero value (Repo{Dir: ""}) targets the
// process cwd, matching the prior free-function git.X() behaviour.
func (c *cmdIO) repo() git.Repo {
	return c.Repo
}
