package commands

import (
	"regexp"
	"slices"
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
	`^(feat|fix|docs|test|refactor|chore|bench|revert|ci|perf|release)(\([^)]+\))?(!|\?)?(: )(.*)$`,
)

// typeColor maps each conventional-commit type to its non-bold ANSI escape.
// The bold variant from typeBoldColor is used when the subject carries a
// `!` (intentional breakage) or `?` (unverified) suffix.
var typeColor = map[string]string{
	"feat":     ansiGreen,
	"fix":      ansiRed,
	"revert":   ansiRed,
	"perf":     ansiYellow,
	"refactor": ansiYellow,
	"bench":    ansiYellow,
	"docs":     ansiCyan,
	"test":     ansiCyan,
	"ci":       ansiCyan,
	"chore":    ansiBlue,
	"release":  ansiBoldOrange,
}

var typeBoldColor = map[string]string{
	"feat":     ansiBoldGreen,
	"fix":      ansiBoldRed,
	"revert":   ansiBoldRed,
	"perf":     ansiBoldYellow,
	"refactor": ansiBoldYellow,
	"bench":    ansiBoldYellow,
	"docs":     ansiBoldCyan,
	"test":     ansiBoldCyan,
	"ci":       ansiBoldCyan,
	"chore":    ansiBoldBlue,
	"release":  ansiBoldOrange,
}

// colorCommitSubject colors the conventional-commit prefix of a subject
// line. Non-conventional subjects pass through unchanged. Leading whitespace
// is preserved so callers can pass a "<gap><subject>" trail directly.
//
// For `release:` commits, if the subject text after `release: ` matches one
// of the supplied tag names, that text is also rendered in bold orange.
// Pass nil tags when no tag info is available.
func colorCommitSubject(s string, tags []string) string {
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

	col := typeColor[typ]
	if suffix == "!" || suffix == "?" {
		col = typeBoldColor[typ]
	}

	var b strings.Builder
	b.WriteString(lead)
	b.WriteString(col + typ + ansiReset)
	if scope != "" {
		// parens in the type colour, inner text dim (matches the
		// `gg tree --follow` timer styling).
		inner := scope[1 : len(scope)-1]
		b.WriteString(col + "(" + ansiReset)
		b.WriteString(ansiDim + ansiItalic + inner + ansiReset)
		b.WriteString(col + ")" + ansiReset)
	}
	switch suffix {
	case "!":
		b.WriteString(ansiBoldRed + "!" + ansiReset)
	case "?":
		b.WriteString(ansiBoldYellow + "?" + ansiReset)
	}
	b.WriteString(col + sep + ansiReset)
	if typ == "release" && matchesAnyTag(rest, tags) {
		b.WriteString(ansiBoldOrange + rest + ansiReset)
	} else {
		b.WriteString(rest)
	}
	return b.String()
}

// matchesAnyTag reports whether rest (trimmed) equals any of the supplied
// tag names. Used to highlight `release:` subjects whose text matches one
// of the tags decorating the same commit.
func matchesAnyTag(rest string, tags []string) bool {
	r := strings.TrimSpace(rest)
	if r == "" {
		return false
	}
	return slices.Contains(tags, r)
}

// extractTags pulls "tag: <name>" entries out of the first parenthesised
// decoration block of a `git log --decorate` line. ANSI escapes are stripped
// before parsing, so this works on colored and uncolored input alike.
func extractTags(line string) []string {
	plain := ansiSeq.ReplaceAllString(line, "")
	open := strings.Index(plain, "(")
	if open < 0 {
		return nil
	}
	rel := strings.Index(plain[open:], ")")
	if rel < 0 {
		return nil
	}
	inner := plain[open+1 : open+rel]
	var tags []string
	for p := range strings.SplitSeq(inner, ", ") {
		p = strings.TrimSpace(p)
		if t, ok := strings.CutPrefix(p, "tag: "); ok {
			tags = append(tags, t)
		}
	}
	return tags
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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
	return line[:last] + colorCommitSubject(line[last:], extractTags(line[:last]))
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
