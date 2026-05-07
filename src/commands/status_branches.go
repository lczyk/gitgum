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
// it with the same color scheme as the tree view: bold cyan for HEAD/detached,
// bold green for local branch names, yellow for short hashes, bold red for
// upstream ref names, bold yellow for brackets and separators.
//
// Lines that don't match the expected shape pass through unchanged.
func renderBranchList(raw string) string {
	if !colorEnabled() {
		return strings.TrimRight(raw, "\n")
	}
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	for i, line := range lines {
		lines[i] = colorBranchLine(line)
	}
	return strings.Join(lines, "\n")
}

func colorBranchLine(line string) string {
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
	b.WriteString(colorCommitSubject(trail))
	return b.String()
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
