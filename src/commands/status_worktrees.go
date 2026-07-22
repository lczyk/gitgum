package commands

import (
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

// remoteSlashBranch renders a branch and its tracking remote in the shape
// `gg switch` uses: "(remote/)branch". Parens bold yellow, remote bold red,
// branch bold green.
func remoteSlashBranch(remote, branch string, color bool) string {
	if !color {
		return "(" + remote + "/)" + branch
	}
	var b strings.Builder
	b.WriteString(ansiBoldYellow + "(" + ansiReset)
	b.WriteString(ansiBoldRed + remote + "/" + ansiReset)
	b.WriteString(ansiBoldYellow + ")" + ansiReset)
	b.WriteString(ansiBoldGreen + branch + ansiReset)
	return b.String()
}

// worktreeBranchDisplay renders the checked-out ref of a worktree in the same
// shape `gg switch` uses for its status line: "(remote/)branch" when the
// branch tracks a remote, bare branch name otherwise, "(detached HEAD)" /
// "(bare)" for the special states.
func worktreeBranchDisplay(wt git.Worktree, trackingRemote string, color bool) string {
	switch {
	case wt.Bare:
		if color {
			return ansiBoldYellow + "(bare)" + ansiReset
		}
		return "(bare)"
	case wt.Detached:
		if color {
			return ansiBoldCyan + "(detached HEAD)" + ansiReset
		}
		return "(detached HEAD)"
	case trackingRemote != "":
		return remoteSlashBranch(trackingRemote, wt.Branch, color)
	default:
		if color {
			return ansiBoldGreen + wt.Branch + ansiReset
		}
		return wt.Branch
	}
}

// formatWorktreeRows renders one worktree in the multi-row layout of the
// BRANCHES section:
//
//	row 1: marker path hash
//	row 2: checked-out ref, switch-style (indented)
//	row 3: commit subject (indented, skipped when absent)
func formatWorktreeRows(wt git.Worktree, current bool, trackingRemote, subject string, color bool) []string {
	hash := wt.Head
	if len(hash) > 7 {
		hash = hash[:7]
	}

	var row1 strings.Builder
	if current {
		if color {
			row1.WriteString(ansiBoldCyan + "*" + ansiReset)
		} else {
			row1.WriteByte('*')
		}
	} else {
		row1.WriteByte(' ')
	}
	row1.WriteByte(' ')
	if color {
		row1.WriteString(ansiBoldGreen + wt.Path + ansiReset)
	} else {
		row1.WriteString(wt.Path)
	}
	if hash != "" {
		row1.WriteByte(' ')
		if color {
			row1.WriteString(ansiYellow + hash + ansiReset)
		} else {
			row1.WriteString(hash)
		}
	}

	rows := []string{row1.String(), "    " + worktreeBranchDisplay(wt, trackingRemote, color)}

	if subject != "" {
		if color {
			rows = append(rows, "    "+colorCommitSubject(subject, nil))
		} else {
			rows = append(rows, "    "+subject)
		}
	}
	return rows
}
