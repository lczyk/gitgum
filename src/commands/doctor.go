package commands

import (
	"fmt"
	"io"
	"sort"

	"github.com/lczyk/gitgum/internal/git"
)

// Severity classifies a doctor finding.
//
//   - Fixable: a known, deterministic inconsistency with a suggested remediation
//     (e.g. a misnamed github remote). doctor prints the fix; it does not run it.
//   - Warning: something doctor can't reason about (custom remote url, divergent
//     upstreams). informational only; never blocks gg.
type Severity int

const (
	SevFixable Severity = iota
	SevWarning
)

// Finding is a single issue reported by a check.
type Finding struct {
	Check    string   // check id, e.g. "remote-naming"
	Severity Severity // Fixable | Warning
	Message  string   // human description of what's wrong
	Fix      string   // suggested remediation command; only meaningful for SevFixable
}

// DoctorCommand diagnoses known inconsistencies in the repo's remote/worktree
// layout. It is diagnose-only: it reports findings and always exits 0.
type DoctorCommand struct {
	cmdIO
}

// doctorChecks is the ordered set of diagnostics doctor runs. Each returns its
// findings; internal git errors surface as warnings rather than aborting the run.
var doctorChecks = []func(git.Repo) []Finding{
	checkRemoteNaming,
	checkUpstreams,
	checkLayout,
}

func (d *DoctorCommand) Execute(args []string) error {
	r := d.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	var findings []Finding
	for _, check := range doctorChecks {
		findings = append(findings, check(r)...)
	}

	renderFindings(d.out(), findings)
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
