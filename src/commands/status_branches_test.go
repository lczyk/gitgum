package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestRenderBranchList_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	in := "* main               26c3916 [origin/main] release: v0.17.0\n  feat              abc1234 subject\n"
	got := renderBranchList(in)
	want := "* main               26c3916 [origin/main] release: v0.17.0\n  feat              abc1234 subject"
	assert.Equal(t, got, want)
}

func TestColorBranchLine_CurrentWithUpstream(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorBranchLine("* main               26c3916 [origin/main] release: v0.17.0")
	// marker bold cyan
	assert.ContainsString(t, got, ansiBoldCyan+"*"+ansiReset)
	// branch name bold green
	assert.ContainsString(t, got, ansiBoldGreen+"main"+ansiReset)
	// hash yellow
	assert.ContainsString(t, got, ansiYellow+"26c3916"+ansiReset)
	// upstream ref bold red
	assert.ContainsString(t, got, ansiBoldRed+"origin/main"+ansiReset)
	// brackets bold yellow
	assert.ContainsString(t, got, ansiBoldYellow+"["+ansiReset)
	assert.ContainsString(t, got, ansiBoldYellow+"]"+ansiReset)
	// subject is colour-treated as a conventional commit
	assert.ContainsString(t, got, ansiBoldOrange+"release"+ansiReset)
	assert.ContainsString(t, got, "v0.17.0")
	// strip ansi -> identical to input
	assert.Equal(t, stripAnsi(got), "* main               26c3916 [origin/main] release: v0.17.0")
}

func TestColorBranchLine_AheadBehind(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorBranchLine("  feat               abc1234 [origin/feat: ahead 2, behind 1] some subject")
	assert.ContainsString(t, got, ansiBoldRed+"origin/feat"+ansiReset)
	assert.ContainsString(t, got, ansiBoldYellow+": ahead 2, behind 1"+ansiReset)
	assert.Equal(t, stripAnsi(got), "  feat               abc1234 [origin/feat: ahead 2, behind 1] some subject")
}

func TestColorBranchLine_NoUpstream(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorBranchLine("  hierarchy-aware-ff e4cee51 docs: mark hierarchy-aware ff implemented")
	assert.ContainsString(t, got, ansiBoldGreen+"hierarchy-aware-ff"+ansiReset)
	assert.ContainsString(t, got, ansiYellow+"e4cee51"+ansiReset)
	assert.Equal(t, stripAnsi(got), "  hierarchy-aware-ff e4cee51 docs: mark hierarchy-aware ff implemented")
}

func TestColorBranchLine_DetachedHEAD(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := colorBranchLine("* (HEAD detached at abc1234) abc1234 some subject")
	assert.ContainsString(t, got, ansiBoldCyan+"(HEAD detached at abc1234)"+ansiReset)
	assert.ContainsString(t, got, ansiYellow+"abc1234"+ansiReset)
	assert.Equal(t, stripAnsi(got), "* (HEAD detached at abc1234) abc1234 some subject")
}

func TestColorBranchLine_NoMatch(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := "warning: something weird"
	assert.Equal(t, colorBranchLine(in), in)
}

func TestRenderBranchList_RoundTripsContent(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := strings.Join([]string{
		"* main               26c3916 [origin/main] release: v0.17.0",
		"  hierarchy-aware-ff e4cee51 docs: mark hierarchy-aware ff implemented",
		"  some-feature       82348bb [origin/some-feature: ahead 87] subject",
		"",
	}, "\n")
	got := renderBranchList(in)
	assert.Equal(t, stripAnsi(got), strings.TrimRight(in, "\n"))
}
