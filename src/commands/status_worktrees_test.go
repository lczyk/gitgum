package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
)

func TestFormatWorktreeRows_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	wt := git.Worktree{Path: "/repo", Head: "26c3916aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Branch: "feat/binutils-s390x-ppc64el"}
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
	wt := git.Worktree{Path: "/repo", Head: "26c3916aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Branch: "main"}
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
	wt := git.Worktree{Path: "/repo-wt", Head: "abc1234bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Branch: "feat"}
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
	wt := git.Worktree{Path: "/repo-wt", Head: "abc1234bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Detached: true}
	rows := formatWorktreeRows(wt, false, "", "subject", true)
	assert.ContainsString(t, rows[1], ansiBoldCyan+"(detached HEAD)"+ansiReset)
}

func TestFormatWorktreeRows_Bare(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	wt := git.Worktree{Path: "/repo.git", Bare: true}
	rows := formatWorktreeRows(wt, false, "", "", false)
	got := strings.Join(rows, "\n")
	want := strings.Join([]string{
		"  /repo.git",
		"    (bare)",
	}, "\n")
	assert.Equal(t, got, want)
}
