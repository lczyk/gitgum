package commands

import (
	"fmt"
	"io"
	"strings"
)

func (d *DiffCommand) renderNative(w io.Writer) error {
	out, _, err := d.collectOutput()
	if err != nil {
		return err
	}
	if out == "" {
		return nil
	}
	// ingest line-by-line so future per-line transforms have a seam.
	// today: identity reprint -- byte-identical to renderPassthrough.
	for line := range strings.SplitSeq(out, "\n") {
		fmt.Fprintln(w, line)
	}
	return nil
}
