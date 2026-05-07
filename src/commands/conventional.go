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
	b.WriteString(sep)
	b.WriteString(rest)
	return b.String()
}
