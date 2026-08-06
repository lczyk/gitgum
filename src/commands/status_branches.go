package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

// branchRow is one BRANCHES entry, held as the fields the layout uses rather
// than as the padded line `git branch -vv` would have printed. The padding is
// the reason: it is sized to the longest branch name, so reading a field back
// out of it means guessing where git put the column this time.
type branchRow struct {
	marker   byte // '*' checked out here, '+' checked out in another worktree
	name     string
	hash     string
	upstream string // empty when the branch tracks nothing
	notes    string // "ahead 2, behind 1", "gone"; no brackets, empty when in sync
	subject  string
	worktree string // the other worktree holding this branch, for the '+' rows
	detached bool   // the "(HEAD detached at abc1234)" pseudo-entry
}

// branchRows turns a ref listing into the rows to render, in git's own order:
// the detached-HEAD entry first when there is one, then branches by name.
//
// detachedAt is the short sha HEAD sits at, empty when HEAD is on a branch or
// has no commit yet -- an unborn HEAD gets no row at all, matching git, which
// has no ref to list.
func branchRows(locals []git.LocalBranch, detachedAt, detachedSubject string) []branchRow {
	var rows []branchRow
	if detachedAt != "" {
		rows = append(rows, branchRow{
			marker:   '*',
			name:     fmt.Sprintf("(HEAD detached at %s)", detachedAt),
			hash:     detachedAt,
			subject:  detachedSubject,
			detached: true,
		})
	}
	for _, b := range locals {
		row := branchRow{
			marker:   ' ',
			name:     b.Name,
			hash:     b.Hash,
			upstream: b.Upstream,
			notes:    strings.TrimSuffix(strings.TrimPrefix(b.Track, "["), "]"),
			subject:  b.Subject,
		}
		switch {
		case b.Head:
			row.marker = '*'
		case b.WorktreePath != "":
			row.marker, row.worktree = '+', b.WorktreePath
		}
		rows = append(rows, row)
	}
	return rows
}

// tracking renders the bracket git puts after the hash: "[origin/main]" when
// in sync, "[origin/main: ahead 2]" otherwise. Empty when nothing is tracked.
func (r branchRow) tracking() string {
	if r.upstream == "" {
		return ""
	}
	if r.notes == "" {
		return "[" + r.upstream + "]"
	}
	return "[" + r.upstream + ": " + r.notes + "]"
}

// sameNameUpstream reports whether the upstream is this branch on some remote,
// which is the case the switch-style "(remote/)name" collapses.
func (r branchRow) sameNameUpstream() (remote string, ok bool) {
	remote, branch, found := strings.Cut(r.upstream, "/")
	return remote, found && branch == r.name
}

// renderBranchList lays the rows out one branch per group of rows rather than
// one per line, so a long name, a divergence note and a subject do not have to
// share a terminal width:
//
//	row 1: marker name hash
//	row 2: tracking info (indented, skipped when absent)
//	row 3: commit subject (indented)
func renderBranchList(rows []branchRow) string {
	color := colorEnabled()
	if quirkEnabled("normal-branches") {
		return strings.Join(formatBranchLines(rows, color), "\n")
	}
	var out []string
	for _, row := range rows {
		out = append(out, formatBranchRows(row, color)...)
	}
	return strings.Join(out, "\n")
}

// formatBranchLines reproduces git's own layout, name column padded to the
// longest entry, for the normal-branches quirk.
func formatBranchLines(rows []branchRow, color bool) []string {
	width := 0
	for _, row := range rows {
		width = max(width, len(row.name))
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		var b strings.Builder
		b.WriteString(markerCell(row.marker, color))
		b.WriteByte(' ')
		b.WriteString(paintBranchName(row, color))
		b.WriteString(strings.Repeat(" ", width-len(row.name)+1))
		if color {
			b.WriteString(ansiYellow + row.hash + ansiReset)
		} else {
			b.WriteString(row.hash)
		}
		// git names the worktree holding a '+' branch between the hash and the
		// tracking bracket; the quirk exists to look like git, so it does too.
		if row.worktree != "" {
			b.WriteString(" (" + row.worktree + ")")
		}
		if t := row.tracking(); t != "" {
			b.WriteByte(' ')
			if color {
				b.WriteString(colorBranchTracking(row.upstream, row.notes))
			} else {
				b.WriteString(t)
			}
		}
		if row.subject != "" {
			b.WriteByte(' ')
			if color {
				b.WriteString(colorCommitSubject(row.subject, nil))
			} else {
				b.WriteString(row.subject)
			}
		}
		out = append(out, b.String())
	}
	return out
}

func formatBranchRows(row branchRow, color bool) []string {
	// A same-name upstream collapses into the switch-style "(remote/)name",
	// leaving the bracket row for ahead/behind/gone notes only.
	remote, collapsed := row.sameNameUpstream()

	var head strings.Builder
	head.WriteString(markerCell(row.marker, color))
	head.WriteByte(' ')
	if collapsed {
		head.WriteString(remoteSlashBranch(remote, row.name, color))
	} else {
		head.WriteString(paintBranchName(row, color))
	}
	head.WriteByte(' ')
	if color {
		head.WriteString(ansiYellow + row.hash + ansiReset)
	} else {
		head.WriteString(row.hash)
	}
	rows := []string{head.String()}

	switch {
	case collapsed && row.notes != "":
		note := "[" + row.notes + "]"
		if color {
			note = ansiBoldYellow + note + ansiReset
		}
		rows = append(rows, "    "+note)
	case !collapsed && row.upstream != "":
		if color {
			rows = append(rows, "    "+colorBranchTracking(row.upstream, row.notes))
		} else {
			rows = append(rows, "    "+row.tracking())
		}
	}

	if row.subject != "" {
		if color {
			rows = append(rows, "    "+colorCommitSubject(row.subject, nil))
		} else {
			rows = append(rows, "    "+row.subject)
		}
	}
	return rows
}

// markerCell renders the leading marker: '*' for the branch checked out here,
// '+' for one held by another worktree, a space otherwise.
func markerCell(marker byte, color bool) string {
	if !color {
		return string(marker)
	}
	switch marker {
	case '*':
		return ansiBoldCyan + "*" + ansiReset
	case '+':
		return ansiBoldYellow + "+" + ansiReset
	default:
		return " "
	}
}

func paintBranchName(row branchRow, color bool) string {
	if !color {
		return row.name
	}
	if row.detached {
		return ansiBoldCyan + row.name + ansiReset
	}
	return ansiBoldGreen + row.name + ansiReset
}

// colorBranchTracking colors the upstream bracket: brackets and separators
// bold yellow, the upstream ref bold red, the ahead/behind/gone notes bold
// yellow.
func colorBranchTracking(upstream, notes string) string {
	var b strings.Builder
	b.WriteString(ansiBoldYellow + "[" + ansiReset)
	b.WriteString(ansiBoldRed + upstream + ansiReset)
	if notes != "" {
		b.WriteString(ansiBoldYellow + ": " + notes + ansiReset)
	}
	b.WriteString(ansiBoldYellow + "]" + ansiReset)
	return b.String()
}
