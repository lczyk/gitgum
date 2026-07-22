package git

import (
	"strings"
	"testing"
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
	wts := ParseWorktreePorcelain(raw)
	if len(wts) != 3 {
		t.Fatalf("got %d worktrees, want 3", len(wts))
	}

	if wts[0].Path != "/repo" || wts[0].Branch != "main" || wts[0].Head[:7] != "26c3916" {
		t.Errorf("main entry = %+v", wts[0])
	}
	if wts[1].Path != "/repo-wt" || !wts[1].Detached || wts[1].Branch != "" {
		t.Errorf("detached entry = %+v", wts[1])
	}
	if wts[2].Path != "/repo-bare" || !wts[2].Bare {
		t.Errorf("bare entry = %+v", wts[2])
	}
}
