package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestColorCommitSubject_Types(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	cases := []struct {
		typ   string
		color string
	}{
		{"feat", ansiBoldGreen},
		{"fix", ansiBoldRed},
		{"revert", ansiBoldRed},
		{"perf", ansiBoldYellow},
		{"refactor", ansiBoldYellow},
		{"bench", ansiBoldYellow},
		{"docs", ansiBoldCyan},
		{"test", ansiBoldCyan},
		{"ci", ansiBoldCyan},
		{"chore", ansiDim},
	}
	for _, tc := range cases {
		in := tc.typ + ": something"
		got := colorCommitSubject(in)
		assert.ContainsString(t, got, tc.color+tc.typ+ansiReset)
		assert.Equal(t, stripAnsi(got), in)
	}
}

func TestColorCommitSubject_Scope(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("feat(status): add thing")
	assert.ContainsString(t, got, ansiBoldGreen+"feat"+ansiReset)
	assert.ContainsString(t, got, ansiDim+"(status)"+ansiReset)
	assert.Equal(t, stripAnsi(got), "feat(status): add thing")
}

func TestColorCommitSubject_BangSuffix(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("test!: failing on purpose")
	assert.ContainsString(t, got, ansiBoldCyan+"test"+ansiReset)
	assert.ContainsString(t, got, ansiBoldRed+"!"+ansiReset)
	assert.Equal(t, stripAnsi(got), "test!: failing on purpose")
}

func TestColorCommitSubject_QuestionSuffix(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("fix?: best effort")
	assert.ContainsString(t, got, ansiBoldRed+"fix"+ansiReset)
	assert.ContainsString(t, got, ansiYellow+"?"+ansiReset)
	assert.Equal(t, stripAnsi(got), "fix?: best effort")
}

func TestColorCommitSubject_ScopeAndBang(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("refactor(api)!: drop old method")
	assert.ContainsString(t, got, ansiBoldYellow+"refactor"+ansiReset)
	assert.ContainsString(t, got, ansiDim+"(api)"+ansiReset)
	assert.ContainsString(t, got, ansiBoldRed+"!"+ansiReset)
	assert.Equal(t, stripAnsi(got), "refactor(api)!: drop old method")
}

func TestColorCommitSubject_NonConventional(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	cases := []string{
		"just a subject",
		"unknown: not a real type",
		"feat without colon",
		"",
	}
	for _, in := range cases {
		assert.Equal(t, colorCommitSubject(in), in)
	}
}

func TestColorCommitSubject_LeadingWhitespace(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("   feat: a thing")
	assert.That(t, strings.HasPrefix(got, "   "))
	assert.ContainsString(t, got, ansiBoldGreen+"feat"+ansiReset)
	assert.Equal(t, stripAnsi(got), "   feat: a thing")
}

func TestColorCommitSubject_PermissiveScope(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("feat(weird scope!): subject")
	assert.ContainsString(t, got, ansiDim+"(weird scope!)"+ansiReset)
	assert.Equal(t, stripAnsi(got), "feat(weird scope!): subject")
}

func TestColorCommitSubject_SeparatorMatchesType(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("feat: thing")
	assert.ContainsString(t, got, ansiBoldGreen+": "+ansiReset)
	got = colorCommitSubject("fix(api): thing")
	assert.ContainsString(t, got, ansiBoldRed+": "+ansiReset)
}

func TestColorTreeLine_PlainSubjectAfterAnsi(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "\x1b[33m" + "abc1234" + "\x1b[m" + " feat: hello"
	got := colorTreeLine(in)
	assert.ContainsString(t, got, ansiBoldGreen+"feat"+ansiReset)
	assert.ContainsString(t, got, ansiBoldGreen+": "+ansiReset)
	assert.Equal(t, stripAnsi(got), "abc1234 feat: hello")
}

func TestColorTreeLine_WithDecoration(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "\x1b[31m" + "*" + "\x1b[m" + " \x1b[33m" + "abc1234" + "\x1b[m" +
		" \x1b[33m(\x1b[m\x1b[1;32m" + "main" + "\x1b[m\x1b[33m)\x1b[m" +
		" fix(x)!: ouch"
	got := colorTreeLine(in)
	assert.ContainsString(t, got, ansiBoldRed+"fix"+ansiReset)
	assert.ContainsString(t, got, ansiDim+"(x)"+ansiReset)
	assert.ContainsString(t, got, ansiBoldRed+"!"+ansiReset)
	assert.Equal(t, stripAnsi(got), "* abc1234 (main) fix(x)!: ouch")
}

func TestColorTreeLine_NonConventional(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "\x1b[33m" + "abc1234" + "\x1b[m" + " just a plain subject"
	got := colorTreeLine(in)
	assert.Equal(t, got, in)
}

func TestColorTreeLine_NoAnsi(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "abc1234 feat: thing"
	got := colorTreeLine(in)
	assert.Equal(t, got, in)
}

func TestColorCommitSubject_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	in := "feat(x): y"
	assert.Equal(t, colorCommitSubject(in), in)
}
