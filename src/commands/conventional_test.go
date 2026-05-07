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
		{"feat", ansiGreen},
		{"fix", ansiRed},
		{"revert", ansiRed},
		{"perf", ansiYellow},
		{"refactor", ansiYellow},
		{"bench", ansiYellow},
		{"docs", ansiCyan},
		{"test", ansiCyan},
		{"ci", ansiCyan},
		{"chore", ansiBlue},
		{"release", ansiBoldOrange},
	}
	for _, tc := range cases {
		in := tc.typ + ": something"
		got := colorCommitSubject(in, nil)
		assert.ContainsString(t, got, tc.color+tc.typ+ansiReset)
		assert.Equal(t, stripAnsi(got), in)
	}
}

func TestColorCommitSubject_Scope(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("feat(status): add thing", nil)
	assert.ContainsString(t, got, ansiGreen+"feat"+ansiReset)
	assert.ContainsString(t, got, ansiGreen+"("+ansiReset)
	assert.ContainsString(t, got, ansiDim+ansiItalic+"status"+ansiReset)
	assert.ContainsString(t, got, ansiGreen+")"+ansiReset)
	assert.Equal(t, stripAnsi(got), "feat(status): add thing")
}

func TestColorCommitSubject_BangSuffix(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("test!: failing on purpose", nil)
	assert.ContainsString(t, got, ansiBoldCyan+"test"+ansiReset)
	assert.ContainsString(t, got, ansiBoldRed+"!"+ansiReset)
	assert.Equal(t, stripAnsi(got), "test!: failing on purpose")
}

func TestColorCommitSubject_QuestionSuffix(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("fix?: best effort", nil)
	assert.ContainsString(t, got, ansiBoldRed+"fix"+ansiReset)
	assert.ContainsString(t, got, ansiBoldYellow+"?"+ansiReset)
	assert.Equal(t, stripAnsi(got), "fix?: best effort")
}

func TestColorCommitSubject_ScopeAndBang(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("refactor(api)!: drop old method", nil)
	assert.ContainsString(t, got, ansiBoldYellow+"refactor"+ansiReset)
	assert.ContainsString(t, got, ansiBoldYellow+"("+ansiReset)
	assert.ContainsString(t, got, ansiDim+ansiItalic+"api"+ansiReset)
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
		assert.Equal(t, colorCommitSubject(in, nil), in)
	}
}

func TestColorCommitSubject_LeadingWhitespace(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("   feat: a thing", nil)
	assert.That(t, strings.HasPrefix(got, "   "))
	assert.ContainsString(t, got, ansiGreen+"feat"+ansiReset)
	assert.Equal(t, stripAnsi(got), "   feat: a thing")
}

func TestColorCommitSubject_PermissiveScope(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("feat(weird scope!): subject", nil)
	assert.ContainsString(t, got, ansiGreen+"("+ansiReset)
	assert.ContainsString(t, got, ansiDim+ansiItalic+"weird scope!"+ansiReset)
	assert.ContainsString(t, got, ansiGreen+")"+ansiReset)
	assert.Equal(t, stripAnsi(got), "feat(weird scope!): subject")
}

func TestColorCommitSubject_SeparatorMatchesType(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("feat: thing", nil)
	assert.ContainsString(t, got, ansiGreen+": "+ansiReset)
	got = colorCommitSubject("fix(api): thing", nil)
	assert.ContainsString(t, got, ansiRed+": "+ansiReset)
}

func TestColorTreeLine_PlainSubjectAfterAnsi(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "\x1b[33m" + "abc1234" + "\x1b[m" + " feat: hello"
	got := colorTreeLine(in)
	assert.ContainsString(t, got, ansiGreen+"feat"+ansiReset)
	assert.ContainsString(t, got, ansiGreen+": "+ansiReset)
	assert.Equal(t, stripAnsi(got), "abc1234 feat: hello")
}

func TestColorTreeLine_WithDecoration(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "\x1b[31m" + "*" + "\x1b[m" + " \x1b[33m" + "abc1234" + "\x1b[m" +
		" \x1b[33m(\x1b[m\x1b[1;32m" + "main" + "\x1b[m\x1b[33m)\x1b[m" +
		" fix(x)!: ouch"
	got := colorTreeLine(in)
	assert.ContainsString(t, got, ansiBoldRed+"fix"+ansiReset)
	assert.ContainsString(t, got, ansiBoldRed+"("+ansiReset)
	assert.ContainsString(t, got, ansiDim+ansiItalic+"x"+ansiReset)
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

func TestColorCommitSubject_ReleaseTagMatch(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("release: v0.17.0", []string{"v0.17.0"})
	assert.ContainsString(t, got, ansiBoldOrange+"release"+ansiReset)
	assert.ContainsString(t, got, ansiBoldOrange+"v0.17.0"+ansiReset)
}

func TestColorCommitSubject_ReleaseNoTagMatch(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("release: v0.17.0", []string{"v0.16.0"})
	assert.ContainsString(t, got, ansiBoldOrange+"release"+ansiReset)
	// rest should not get the bold-orange highlight
	assert.Equal(t, strings.Contains(got, ansiBoldOrange+"v0.17.0"+ansiReset), false)
}

func TestColorCommitSubject_ReleaseNoTags(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorCommitSubject("release: v0.17.0", nil)
	assert.ContainsString(t, got, ansiBoldOrange+"release"+ansiReset)
	assert.Equal(t, strings.Contains(got, ansiBoldOrange+"v0.17.0"+ansiReset), false)
}

func TestExtractTags(t *testing.T) {
	cases := map[string][]string{
		"abc1234 (tag: v0.17.0, origin/main, origin/HEAD) release: v0.17.0": {"v0.17.0"},
		"abc1234 (HEAD -> main, tag: v1.0, tag: latest) release: v1.0":      {"v1.0", "latest"},
		"abc1234 (origin/main) feat: thing":                                 nil,
		"abc1234 feat: thing":                                               nil,
	}
	for in, want := range cases {
		got := extractTags(in)
		if want == nil {
			assert.Equal(t, len(got), 0)
		} else {
			assert.EqualArrays(t, got, want)
		}
	}
}

func TestColorTreeLine_ReleaseTagMatch(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "\x1b[33m" + "abc1234" + "\x1b[m" +
		" \x1b[33m(\x1b[m\x1b[1;33m" + "tag: v0.17.0" + "\x1b[m\x1b[33m, \x1b[m\x1b[1;31m" + "origin/main" + "\x1b[m\x1b[33m)\x1b[m" +
		" release: v0.17.0"
	got := colorTreeLine(in)
	assert.ContainsString(t, got, ansiBoldOrange+"release"+ansiReset)
	assert.ContainsString(t, got, ansiBoldOrange+"v0.17.0"+ansiReset)
	assert.Equal(t, stripAnsi(got), "abc1234 (tag: v0.17.0, origin/main) release: v0.17.0")
}

func TestColorCommitSubject_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	in := "feat(x): y"
	assert.Equal(t, colorCommitSubject(in, nil), in)
}
