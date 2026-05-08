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
	want := strings.Join([]string{
		"* main 26c3916",
		"    [origin/main]",
		"    release: v0.17.0",
		"  feat abc1234",
		"    subject",
	}, "\n")
	assert.Equal(t, got, want)
}

func TestFormatBranchRows_CurrentWithUpstream(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows("* main               26c3916 [origin/main] release: v0.17.0", true)
	assert.Equal(t, len(rows), 3)
	got := strings.Join(rows, "\n")
	assert.ContainsString(t, rows[0], ansiBoldCyan+"*"+ansiReset)
	assert.ContainsString(t, rows[0], ansiBoldGreen+"main"+ansiReset)
	assert.ContainsString(t, rows[0], ansiYellow+"26c3916"+ansiReset)
	assert.ContainsString(t, rows[1], ansiBoldRed+"origin/main"+ansiReset)
	assert.ContainsString(t, got, ansiBoldYellow+"release"+ansiReset)
}

func TestFormatBranchRows_AheadBehind(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows("  feat               abc1234 [origin/feat: ahead 2, behind 1] some subject", true)
	assert.Equal(t, len(rows), 3)
	assert.ContainsString(t, rows[1], ansiBoldRed+"origin/feat"+ansiReset)
	assert.ContainsString(t, rows[1], ansiBoldYellow+": ahead 2, behind 1"+ansiReset)
}

func TestFormatBranchRows_NoUpstream(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows("  hierarchy-aware-ff e4cee51 docs: mark hierarchy-aware ff implemented", true)
	assert.Equal(t, len(rows), 2)
	assert.ContainsString(t, rows[0], ansiBoldGreen+"hierarchy-aware-ff"+ansiReset)
	assert.ContainsString(t, rows[0], ansiYellow+"e4cee51"+ansiReset)
}

func TestFormatBranchRows_DetachedHEAD(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows("* (HEAD detached at abc1234) abc1234 some subject", true)
	assert.Equal(t, len(rows), 2)
	assert.ContainsString(t, rows[0], ansiBoldCyan+"(HEAD detached at abc1234)"+ansiReset)
}

func TestFormatBranchRows_NoMatch(t *testing.T) {
	in := "warning: something weird"
	rows := formatBranchRows(in, true)
	assert.Equal(t, len(rows), 1)
	assert.Equal(t, rows[0], in)
}

func TestRenderBranchList_MultiRow(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	in := strings.Join([]string{
		"* main               26c3916 [origin/main] release: v0.17.0",
		"  hierarchy-aware-ff e4cee51 docs: mark hierarchy-aware ff implemented",
		"  some-feature       82348bb [origin/some-feature: ahead 87] subject",
		"",
	}, "\n")
	got := renderBranchList(in)
	plain := stripAnsi(got)
	lines := strings.Split(plain, "\n")
	assert.Equal(t, lines[0], "* main 26c3916")
	assert.Equal(t, lines[1], "    [origin/main]")
	assert.Equal(t, lines[2], "    release: v0.17.0")
	assert.Equal(t, lines[3], "  hierarchy-aware-ff e4cee51")
	assert.Equal(t, lines[4], "    docs: mark hierarchy-aware ff implemented")
	assert.Equal(t, lines[5], "  some-feature 82348bb")
	assert.Equal(t, lines[6], "    [origin/some-feature: ahead 87]")
	assert.Equal(t, lines[7], "    subject")
}

func TestRenderBranchList_NormalBranchesQuirk(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("GG_QUIRKS", "normal-branches")
	in := "* main               26c3916 [origin/main] release: v0.17.0\n  feat              abc1234 subject\n"
	got := renderBranchList(in)
	plain := stripAnsi(got)
	lines := strings.Split(plain, "\n")
	assert.Equal(t, len(lines), 2)
	assert.Equal(t, lines[0], "* main               26c3916 [origin/main] release: v0.17.0")
	assert.Equal(t, lines[1], "  feat              abc1234 subject")
}
