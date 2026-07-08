package commands

import (
	"regexp"
	"strings"
)

// branchLine captures the structure of a single `git branch -vv` line:
//
//	"* main               26c3916 [origin/main: ahead 1] subject"
//	"  feature            abc1234 subject"
//	"* (HEAD detached at abc1234) abc1234 subject"
//
// Groups: marker, name, hash, tracking (incl. brackets), subject.
var branchLineRe = regexp.MustCompile(`^([*+ ]) +(\([^)]*\)|\S+)( +)([0-9a-f]{7,40})(?: +(\[[^\]]*\]))?( +.*)?$`)

// renderBranchList parses the plain output of `git branch -vv` and re-renders
// it as a multi-row layout to avoid terminal overflow:
//
//	row 1: marker name hash
//	row 2: tracking info (indented, skipped when absent)
//	row 3: commit subject (indented)
func renderBranchList(raw string) string {
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) > 0 && len(lines[0]) > 0 && lines[0][0] != '*' && lines[0][0] != '+' && lines[0][0] != ' ' {
		lines[0] = "  " + lines[0]
	}
	color := colorEnabled()
	multiRow := !quirkEnabled("normal-branches")
	var out []string
	for _, line := range lines {
		if multiRow {
			out = append(out, formatBranchRows(line, color)...)
		} else {
			out = append(out, formatBranchSingleLine(line, color))
		}
	}
	return strings.Join(out, "\n")
}

func formatBranchSingleLine(line string, color bool) string {
	if !color {
		return line
	}
	m := branchLineRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	marker, name, gap1, hash, tracking, trail := m[1], m[2], m[3], m[4], m[5], m[6]

	var b strings.Builder
	switch marker {
	case "*":
		b.WriteString(ansiBoldCyan + "*" + ansiReset)
	case "+":
		b.WriteString(ansiBoldYellow + "+" + ansiReset)
	default:
		b.WriteByte(' ')
	}
	b.WriteByte(' ')
	if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") {
		b.WriteString(ansiBoldCyan + name + ansiReset)
	} else {
		b.WriteString(ansiBoldGreen + name + ansiReset)
	}
	b.WriteString(gap1)
	b.WriteString(ansiYellow + hash + ansiReset)
	if tracking != "" {
		b.WriteByte(' ')
		b.WriteString(colorBranchTracking(tracking))
	}
	b.WriteString(colorCommitSubject(trail, nil))
	return b.String()
}

func formatBranchRows(line string, color bool) []string {
	m := branchLineRe.FindStringSubmatch(line)
	if m == nil {
		return []string{line}
	}
	marker, name, hash, tracking, trail := m[1], m[2], m[4], m[5], m[6]
	subject := strings.TrimSpace(trail)

	// A same-name upstream collapses into the switch-style "(remote/)name",
	// leaving the bracket row for ahead/behind/gone notes only.
	nameDisplay := ""
	notesOnly := false
	if remote, notes, ok := sameNameUpstream(tracking, name); ok {
		nameDisplay = remoteSlashBranch(remote, name, color)
		tracking = ""
		if notes != "" {
			tracking = "[" + notes + "]"
			notesOnly = true
		}
	}

	var row1 strings.Builder
	if color {
		switch marker {
		case "*":
			row1.WriteString(ansiBoldCyan + "*" + ansiReset)
		case "+":
			row1.WriteString(ansiBoldYellow + "+" + ansiReset)
		default:
			row1.WriteByte(' ')
		}
		row1.WriteByte(' ')
		switch {
		case nameDisplay != "":
			row1.WriteString(nameDisplay)
		case strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")"):
			row1.WriteString(ansiBoldCyan + name + ansiReset)
		default:
			row1.WriteString(ansiBoldGreen + name + ansiReset)
		}
		row1.WriteByte(' ')
		row1.WriteString(ansiYellow + hash + ansiReset)
	} else {
		if nameDisplay == "" {
			nameDisplay = name
		}
		row1.WriteString(marker + " " + nameDisplay + " " + hash)
	}

	rows := []string{row1.String()}

	if tracking != "" {
		switch {
		case !color:
			rows = append(rows, "    "+tracking)
		case notesOnly:
			rows = append(rows, "    "+ansiBoldYellow+tracking+ansiReset)
		default:
			rows = append(rows, "    "+colorBranchTracking(tracking))
		}
	}

	if subject != "" {
		if color {
			rows = append(rows, "    "+colorCommitSubject(subject, nil))
		} else {
			rows = append(rows, "    "+subject)
		}
	}

	return rows
}

// sameNameUpstream reports whether a tracking string ("[origin/main]" or
// "[origin/main: ahead 1]") names an upstream whose branch part equals name.
// Returns the remote and the notes after ": " (empty when in sync).
func sameNameUpstream(tracking, name string) (remote, notes string, ok bool) {
	if len(tracking) < 2 || tracking[0] != '[' || tracking[len(tracking)-1] != ']' {
		return "", "", false
	}
	upstream := tracking[1 : len(tracking)-1]
	if colon := strings.Index(upstream, ": "); colon >= 0 {
		upstream, notes = upstream[:colon], upstream[colon+2:]
	}
	remote, branch, found := strings.Cut(upstream, "/")
	if !found || branch != name {
		return "", "", false
	}
	return remote, notes, true
}

// colorBranchTracking colors a "[upstream]" or "[upstream: ahead N, behind M]"
// or "[upstream: gone]" string. Brackets and separators are bold yellow,
// upstream ref is bold red, ahead/behind/gone notes are bold yellow.
func colorBranchTracking(s string) string {
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return s
	}
	inner := s[1 : len(s)-1]
	var b strings.Builder
	b.WriteString(ansiBoldYellow + "[" + ansiReset)
	if colon := strings.Index(inner, ": "); colon >= 0 {
		b.WriteString(ansiBoldRed + inner[:colon] + ansiReset)
		b.WriteString(ansiBoldYellow + ": " + inner[colon+2:] + ansiReset)
	} else {
		b.WriteString(ansiBoldRed + inner + ansiReset)
	}
	b.WriteString(ansiBoldYellow + "]" + ansiReset)
	return b.String()
}
