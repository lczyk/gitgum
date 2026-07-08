package commands

import (
	"strings"
)

// worktreeInfo is one entry parsed out of `git worktree list --porcelain`.
type worktreeInfo struct {
	path     string
	head     string // full sha; "" for a bare entry
	branch   string // short branch name; "" when detached or bare
	detached bool
	bare     bool
}

// parseWorktreePorcelain parses `git worktree list --porcelain` output:
//
//	worktree /path/to/main
//	HEAD 26c3916...
//	branch refs/heads/main
//
//	worktree /path/to/other
//	HEAD abc1234...
//	detached
//
// Entries are separated by blank lines; unknown attributes (locked,
// prunable) are ignored.
func parseWorktreePorcelain(raw string) []worktreeInfo {
	var out []worktreeInfo
	var cur *worktreeInfo
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &worktreeInfo{path: strings.TrimPrefix(line, "worktree ")}
		case cur == nil:
			// stray line before the first worktree stanza
		case strings.HasPrefix(line, "HEAD "):
			cur.head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.detached = true
		case line == "bare":
			cur.bare = true
		}
	}
	flush()
	return out
}

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
func worktreeBranchDisplay(wt worktreeInfo, trackingRemote string, color bool) string {
	switch {
	case wt.bare:
		if color {
			return ansiBoldYellow + "(bare)" + ansiReset
		}
		return "(bare)"
	case wt.detached:
		if color {
			return ansiBoldCyan + "(detached HEAD)" + ansiReset
		}
		return "(detached HEAD)"
	case trackingRemote != "":
		return remoteSlashBranch(trackingRemote, wt.branch, color)
	default:
		if color {
			return ansiBoldGreen + wt.branch + ansiReset
		}
		return wt.branch
	}
}

// formatWorktreeRows renders one worktree in the multi-row layout of the
// BRANCHES section:
//
//	row 1: marker path hash
//	row 2: checked-out ref, switch-style (indented)
//	row 3: commit subject (indented, skipped when absent)
func formatWorktreeRows(wt worktreeInfo, current bool, trackingRemote, subject string, color bool) []string {
	hash := wt.head
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
		row1.WriteString(ansiBoldGreen + wt.path + ansiReset)
	} else {
		row1.WriteString(wt.path)
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
