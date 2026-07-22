package commands

import (
	"fmt"
	"io"
	"sort"
)

// DoctorCommand diagnoses known inconsistencies in the repo's remote/worktree
// layout. It is diagnose-only: it reports findings and always exits 0. The
// rules and the Diagnose engine live in doctor.go; this file is just the CLI
// wiring and rendering.
type DoctorCommand struct {
	cmdIO
}

func (d *DoctorCommand) Execute(args []string) error {
	r := d.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}
	renderFindings(d.out(), Diagnose(r))
	return nil
}

// renderFindings prints findings grouped fixable-first, each with its check id,
// message, and (for fixables) an indented suggested command. A clean repo prints
// a single confirmation line.
func renderFindings(out io.Writer, findings []Finding) {
	if len(findings) == 0 {
		fmt.Fprintln(out, paint(ansiBoldGreen, "no issues found."))
		return
	}

	// stable sort keeps discovery order within each severity band.
	sort.SliceStable(findings, func(i, j int) bool {
		return findings[i].Severity < findings[j].Severity
	})

	var nFix, nWarn int
	for _, f := range findings {
		label, code := "fixable", ansiBoldYellow
		if f.Severity == SevWarning {
			label, code = "warning", ansiBoldRed
			nWarn++
		} else {
			nFix++
		}
		fmt.Fprintf(out, "%s [%s] %s\n", paint(code, label), f.Check, f.Message)
		if f.Severity == SevFixable && f.Fix != "" {
			fmt.Fprintf(out, "    %s %s\n", paint(ansiDim, "fix:"), f.Fix)
		}
	}

	fmt.Fprintf(out, "\n%d fixable, %d warning(s).\n", nFix, nWarn)
}
