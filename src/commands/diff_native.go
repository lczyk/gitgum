package commands

import (
	"fmt"
	"io"
	"strings"
)

func (d *DiffCommand) renderCollected(w io.Writer, out string) error {
	for line := range strings.SplitSeq(out, "\n") {
		fmt.Fprintln(w, line)
	}
	return nil
}
