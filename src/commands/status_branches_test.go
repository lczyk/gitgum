package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
)

func TestBranchRows(t *testing.T) {
	t.Parallel()
	locals := []git.LocalBranch{
		{Name: "main", Hash: "26c3916", Upstream: "origin/main", Head: true, Subject: "release: v0.17.0"},
		{Name: "feat", Hash: "abc1234", Upstream: "origin/feat", Track: "[ahead 2, behind 1]", Subject: "subject"},
		{Name: "held", Hash: "def5678", WorktreePath: "/wt/held", Subject: "elsewhere"},
	}
	rows := branchRows(locals, "", "")

	assert.Equal(t, len(rows), 3)
	assert.Equal(t, string(rows[0].marker), "*")
	assert.Equal(t, rows[0].tracking(), "[origin/main]")
	assert.Equal(t, string(rows[1].marker), " ")
	assert.Equal(t, rows[1].notes, "ahead 2, behind 1")
	assert.Equal(t, rows[1].tracking(), "[origin/feat: ahead 2, behind 1]")
	assert.Equal(t, string(rows[2].marker), "+")
	assert.Equal(t, rows[2].worktree, "/wt/held")
	assert.Equal(t, rows[2].tracking(), "")
}

// A detached HEAD gets git's pseudo-entry above the branches; an unborn one
// gets nothing, having no ref to list.
func TestBranchRows_Detached(t *testing.T) {
	t.Parallel()
	rows := branchRows([]git.LocalBranch{{Name: "main", Hash: "26c3916"}}, "abc1234", "some subject")
	assert.Equal(t, len(rows), 2)
	assert.Equal(t, rows[0].name, "(HEAD detached at abc1234)")
	assert.Equal(t, rows[0].detached, true)
	assert.Equal(t, string(rows[0].marker), "*")

	assert.Equal(t, len(branchRows(nil, "", "")), 0)
}

func TestRenderBranchList_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	got := renderBranchList(branchRows([]git.LocalBranch{
		{Name: "main", Hash: "26c3916", Upstream: "origin/main", Head: true, Subject: "release: v0.17.0"},
		{Name: "feat", Hash: "abc1234", Subject: "subject"},
	}, "", ""))
	want := strings.Join([]string{
		"* (origin/)main 26c3916",
		"    release: v0.17.0",
		"  feat abc1234",
		"    subject",
	}, "\n")
	assert.Equal(t, got, want)
}

func TestFormatBranchRows_CurrentWithUpstream(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows(branchRow{
		marker: '*', name: "main", hash: "26c3916",
		upstream: "origin/main", subject: "release: v0.17.0",
	}, true)
	assert.Equal(t, len(rows), 2)
	assert.ContainsString(t, rows[0], ansiBoldCyan+"*"+ansiReset)
	assert.ContainsString(t, rows[0], ansiBoldRed+"origin/"+ansiReset)
	assert.ContainsString(t, rows[0], ansiBoldGreen+"main"+ansiReset)
	assert.ContainsString(t, rows[0], ansiYellow+"26c3916"+ansiReset)
	assert.ContainsString(t, strings.Join(rows, "\n"), ansiBoldYellow+"release"+ansiReset)
}

func TestFormatBranchRows_AheadBehind(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows(branchRow{
		marker: ' ', name: "feat", hash: "abc1234",
		upstream: "origin/feat", notes: "ahead 2, behind 1", subject: "some subject",
	}, true)
	assert.Equal(t, len(rows), 3)
	assert.ContainsString(t, rows[0], ansiBoldRed+"origin/"+ansiReset)
	assert.ContainsString(t, rows[1], ansiBoldYellow+"[ahead 2, behind 1]"+ansiReset)
}

// An upstream named differently from the local branch keeps the full bracket.
func TestFormatBranchRows_DifferentUpstreamName(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows(branchRow{
		marker: ' ', name: "feat", hash: "abc1234",
		upstream: "origin/other", notes: "ahead 2", subject: "some subject",
	}, true)
	assert.Equal(t, len(rows), 3)
	assert.ContainsString(t, rows[0], ansiBoldGreen+"feat"+ansiReset)
	assert.ContainsString(t, rows[1], ansiBoldRed+"origin/other"+ansiReset)
	assert.ContainsString(t, rows[1], ansiBoldYellow+": ahead 2"+ansiReset)
}

func TestFormatBranchRows_NoUpstream(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows(branchRow{
		marker: ' ', name: "hierarchy-aware-ff", hash: "e4cee51",
		subject: "docs: mark hierarchy-aware ff implemented",
	}, true)
	assert.Equal(t, len(rows), 2)
	assert.ContainsString(t, rows[0], ansiBoldGreen+"hierarchy-aware-ff"+ansiReset)
	assert.ContainsString(t, rows[0], ansiYellow+"e4cee51"+ansiReset)
}

func TestFormatBranchRows_DetachedHEAD(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	rows := formatBranchRows(branchRow{
		marker: '*', name: "(HEAD detached at abc1234)", hash: "abc1234",
		detached: true, subject: "some subject",
	}, true)
	assert.Equal(t, len(rows), 2)
	assert.ContainsString(t, rows[0], ansiBoldCyan+"(HEAD detached at abc1234)"+ansiReset)
}

func TestRenderBranchList_MultiRow(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	got := renderBranchList(branchRows([]git.LocalBranch{
		{Name: "main", Hash: "26c3916", Upstream: "origin/main", Head: true, Subject: "release: v0.17.0"},
		{Name: "hierarchy-aware-ff", Hash: "e4cee51", Subject: "docs: mark hierarchy-aware ff implemented"},
		{Name: "some-feature", Hash: "82348bb", Upstream: "origin/some-feature", Track: "[ahead 87]", Subject: "subject"},
	}, "", ""))
	lines := strings.Split(stripAnsi(got), "\n")
	assert.Equal(t, lines[0], "* (origin/)main 26c3916")
	assert.Equal(t, lines[1], "    release: v0.17.0")
	assert.Equal(t, lines[2], "  hierarchy-aware-ff e4cee51")
	assert.Equal(t, lines[3], "    docs: mark hierarchy-aware ff implemented")
	assert.Equal(t, lines[4], "  (origin/)some-feature 82348bb")
	assert.Equal(t, lines[5], "    [ahead 87]")
	assert.Equal(t, lines[6], "    subject")
}

// The quirk reproduces git's own layout, name column padded to the longest.
func TestRenderBranchList_NormalBranchesQuirk(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("GG_QUIRKS", "normal-branches")
	got := renderBranchList(branchRows([]git.LocalBranch{
		{Name: "main", Hash: "26c3916", Upstream: "origin/main", Head: true, Subject: "release: v0.17.0"},
		{Name: "a-longer-name", Hash: "abc1234", Subject: "subject"},
		{Name: "held", Hash: "def5678", WorktreePath: "/wt/held", Subject: "elsewhere"},
	}, "", ""))
	lines := strings.Split(stripAnsi(got), "\n")
	assert.Equal(t, len(lines), 3)
	assert.Equal(t, lines[0], "* main          26c3916 [origin/main] release: v0.17.0")
	assert.Equal(t, lines[1], "  a-longer-name abc1234 subject")
	assert.Equal(t, lines[2], "+ held          def5678 (/wt/held) elsewhere")
}
