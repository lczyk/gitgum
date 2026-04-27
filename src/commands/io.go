package commands

import (
	"io"
	"os"

	"github.com/lczyk/gitgum/internal/ui"
)

// cmdIO is embedded by command structs so tests can capture stdout/stderr and
// drive selection prompts. Zero value falls through to the process's real
// stdio + real fuzzyfinder, so production wiring (go-flags reflection
// constructs zero-value structs) keeps working unchanged.
type cmdIO struct {
	Out io.Writer
	Err io.Writer
	UI  ui.Selector
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
