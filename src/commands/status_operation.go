package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

// operationRows renders an in-progress operation under the HEAD line. A zero
// Operation prints nothing: a checkout that is not mid-operation must not
// grow a blank row.
//
//	rebasing topic onto f9ba3c7 (1/2, editing a commit)
//	  done  edit f9ba3c7 chore: two
//	  next  pick 0cfb447 chore: three
//	        pick abc1234 chore: four
//	        ... 2 more
func operationRows(op git.Operation, color bool) []string {
	if op.Name == "" {
		return nil
	}
	rows := []string{"  " + formatOpSummary(op, color)}
	switch {
	case !op.Last.Empty():
		rows = append(rows, "    "+opLabel("done", color)+"  "+formatOpCommand(op.Last, color))
	case op.Subject != "":
		rows = append(rows, "    "+op.Subject)
	}
	const shown = 3
	n := min(len(op.Next), shown)
	for i := 0; i < n; i++ {
		cmd := formatOpCommand(op.Next[i], color)
		if i == 0 {
			rows = append(rows, "    "+opLabel("next", color)+"  "+cmd)
		} else {
			rows = append(rows, "          "+cmd)
		}
	}
	if more := len(op.Next) - n; more > 0 {
		rows = append(rows, "          "+paintIf(fmt.Sprintf("... %d more", more), ansiDim, color))
	}
	return rows
}

func formatOpSummary(op git.Operation, color bool) string {
	var b strings.Builder
	b.WriteString(paintIf(opGerund(op.Name), ansiBoldYellow, color))
	switch op.Name {
	case git.OpRebase:
		if op.Branch != "" {
			b.WriteByte(' ')
			b.WriteString(paintIf(op.Branch, ansiBoldGreen, color))
		}
		if op.Onto != "" {
			b.WriteString(" onto ")
			b.WriteString(paintIf(op.Onto, ansiYellow, color))
		}
	case git.OpCherryPick, git.OpRevert:
		if op.Head != "" {
			b.WriteByte(' ')
			b.WriteString(paintIf(op.Head, ansiYellow, color))
		}
	case git.OpBisect:
		if op.Branch != "" {
			b.WriteString(", from ")
			b.WriteString(paintIf(op.Branch, ansiBoldGreen, color))
		}
	}
	if note := opNote(op); note != "" {
		b.WriteByte(' ')
		noteCode := ansiBoldYellow
		if op.Stop == git.OpStopConflicts {
			noteCode = ansiBoldRed
		}
		b.WriteString(paintIf(note, noteCode, color))
	}
	return b.String()
}

func opGerund(name string) string {
	switch name {
	case git.OpRebase:
		return "rebasing"
	case git.OpMerge:
		return "merging"
	case git.OpCherryPick:
		return "cherry-picking"
	case git.OpRevert:
		return "reverting"
	case git.OpBisect:
		return "bisecting"
	case git.OpSequence:
		return "cherry-pick or revert"
	default:
		return name
	}
}

func opNote(op git.Operation) string {
	var parts []string
	if op.Step > 0 && op.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", op.Step, op.Total))
	}
	if op.Stop != "" {
		parts = append(parts, op.Stop)
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func formatOpCommand(c git.OpCommand, color bool) string {
	var b strings.Builder
	if c.Verb != "" {
		b.WriteString(c.Verb)
	}
	if c.Hash != "" {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(paintIf(c.Hash, ansiYellow, color))
	}
	if c.Subject != "" {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(c.Subject)
	}
	return b.String()
}

func opLabel(s string, color bool) string {
	return paintIf(s, ansiDim, color)
}

func paintIf(s, code string, color bool) string {
	if !color || s == "" {
		return s
	}
	return code + s + ansiReset
}
