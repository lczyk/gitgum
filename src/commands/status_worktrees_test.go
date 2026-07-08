package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestParseWorktreePorcelain(t *testing.T) {
	t.Parallel()
	raw := strings.Join([]string{
		"worktree /repo",
		"HEAD 26c3916aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"branch refs/heads/main",
		"",
		"worktree /repo-wt",
		"HEAD abc1234bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"detached",
		"",
		"worktree /repo-bare",
		"bare",
	}, "\n")
	wts := parseWorktreePorcelain(raw)
	assert.Equal(t, len(wts), 3)

	assert.Equal(t, wts[0].path, "/repo")
	assert.Equal(t, wts[0].branch, "main")
	assert.Equal(t, wts[0].head[:7], "26c3916")

	assert.Equal(t, wts[1].path, "/repo-wt")
	assert.Equal(t, wts[1].detached, true)
	assert.Equal(t, wts[1].branch, "")

	assert.Equal(t, wts[2].path, "/repo-bare")
	assert.Equal(t, wts[2].bare, true)
}

func TestFormatWorktreeRows_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	wt := worktreeInfo{path: "/repo", head: "26c3916aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", branch: "feat/binutils-s390x-ppc64el"}
	rows := formatWorktreeRows(wt, true, "alesancor1", "some subject", false)
	got := strings.Join(rows, "\n")
	want := strings.Join([]string{
		"* /repo 26c3916",
		"    (alesancor1/)feat/binutils-s390x-ppc64el",
		"    some subject",
	}, "\n")
	assert.Equal(t, got, want)
}

func TestFormatWorktreeRows_Color(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	wt := worktreeInfo{path: "/repo", head: "26c3916aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", branch: "main"}
	rows := formatWorktreeRows(wt, true, "origin", "release: v0.17.0", true)
	assert.Equal(t, len(rows), 3)
	assert.ContainsString(t, rows[0], ansiBoldCyan+"*"+ansiReset)
	assert.ContainsString(t, rows[0], ansiBoldGreen+"/repo"+ansiReset)
	assert.ContainsString(t, rows[0], ansiYellow+"26c3916"+ansiReset)
	assert.ContainsString(t, rows[1], ansiBoldRed+"origin/"+ansiReset)
	assert.ContainsString(t, rows[1], ansiBoldGreen+"main"+ansiReset)
	assert.ContainsString(t, rows[2], ansiBoldYellow+"release"+ansiReset)
}

func TestFormatWorktreeRows_NoUpstream(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	wt := worktreeInfo{path: "/repo-wt", head: "abc1234bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", branch: "feat"}
	rows := formatWorktreeRows(wt, false, "", "", false)
	got := strings.Join(rows, "\n")
	want := strings.Join([]string{
		"  /repo-wt abc1234",
		"    feat",
	}, "\n")
	assert.Equal(t, got, want)
}

func TestFormatWorktreeRows_Detached(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	wt := worktreeInfo{path: "/repo-wt", head: "abc1234bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", detached: true}
	rows := formatWorktreeRows(wt, false, "", "subject", true)
	assert.ContainsString(t, rows[1], ansiBoldCyan+"(detached HEAD)"+ansiReset)
}

func TestFormatWorktreeRows_Bare(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	wt := worktreeInfo{path: "/repo.git", bare: true}
	rows := formatWorktreeRows(wt, false, "", "", false)
	got := strings.Join(rows, "\n")
	want := strings.Join([]string{
		"  /repo.git",
		"    (bare)",
	}, "\n")
	assert.Equal(t, got, want)
}
