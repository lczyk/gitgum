package commands

import (
	"regexp"
	"strings"
)

// conventionalSubjectRe matches a Conventional Commits subject prefix:
//
//	type(scope)!: rest
//	type(scope)?: rest
//	type!: rest
//	type: rest
//
// Scope is permissive (anything that isn't ')'), since rendering shouldn't
// reject what git happily stored. The canonical type list mirrors the
// commit-msg hook in dotfiles.
var conventionalSubjectRe = regexp.MustCompile(
	`^(feat|fix|docs|test|refactor|chore|bench|revert|ci|perf)(\([^)]+\))?(!|\?)?(: )(.*)$`,
)

// typeColor maps each conventional-commit type to its ANSI escape.
var typeColor = map[string]string{
	"feat":     ansiBoldGreen,
	"fix":      ansiBoldRed,
	"revert":   ansiBoldRed,
	"perf":     ansiBoldYellow,
	"refactor": ansiBoldYellow,
	"bench":    ansiBoldYellow,
	"docs":     ansiBoldCyan,
	"test":     ansiBoldCyan,
	"ci":       ansiBoldCyan,
	"chore":    ansiDim,
}

// colorCommitSubject colors the conventional-commit prefix of a subject
// line. Non-conventional subjects pass through unchanged. Leading whitespace
// is preserved so callers can pass a "<gap><subject>" trail directly.
func colorCommitSubject(s string) string {
	if !colorEnabled() {
		return s
	}
	lead := s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	body := s[len(lead):]
	m := conventionalSubjectRe.FindStringSubmatch(body)
	if m == nil {
		return s
	}
	typ, scope, suffix, sep, rest := m[1], m[2], m[3], m[4], m[5]

	var b strings.Builder
	b.WriteString(lead)
	b.WriteString(typeColor[typ] + typ + ansiReset)
	if scope != "" {
		b.WriteString(ansiDim + scope + ansiReset)
	}
	switch suffix {
	case "!":
		b.WriteString(ansiBoldRed + "!" + ansiReset)
	case "?":
		b.WriteString(ansiYellow + "?" + ansiReset)
	}
	b.WriteString(typeColor[typ] + sep + ansiReset)
	b.WriteString(rest)
	return b.String()
}

// colorTreeLine post-processes a single line of `git log --graph --oneline
// --decorate --color=always` output, applying conventional-commit coloring
// to the subject. The subject is the trailing plain-text portion after the
// last ANSI escape on the line (git colors the hash and decoration but not
// %s, so the subject contains no ANSI).
func colorTreeLine(line string) string {
	if !colorEnabled() {
		return line
	}
	last := lastAnsiEnd(line)
	if last < 0 {
		return line
	}
	return line[:last] + colorCommitSubject(line[last:])
}

// colorTreeLines applies colorTreeLine to every line in s, preserving the
// trailing newline shape.
func colorTreeLines(s string) string {
	if !colorEnabled() {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = colorTreeLine(line)
	}
	return strings.Join(lines, "\n")
}

// lastAnsiEnd returns the byte offset immediately after the last ANSI SGR
// escape (\x1b[...m) in s, or -1 if there is none.
func lastAnsiEnd(s string) int {
	i := strings.LastIndex(s, "\x1b[")
	if i < 0 {
		return -1
	}
	j := strings.IndexByte(s[i:], 'm')
	if j < 0 {
		return -1
	}
	return i + j + 1
}
