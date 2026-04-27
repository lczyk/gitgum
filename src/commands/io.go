package commands

import (
	"io"
	"os"
)

// cmdIO is embedded by command structs so tests can capture stdout/stderr.
// Zero value falls through to the process's real stdio, so production wiring
// (go-flags reflection constructs zero-value structs) keeps working unchanged.
type cmdIO struct {
	Out io.Writer
	Err io.Writer
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
