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
		"",
		"worktree /repo-gone",
		"HEAD def5678cccccccccccccccccccccccccccccccc",
		"branch refs/heads/gone",
		"prunable gitdir file points to non-existent location",
	}, "\n")
	wts := ParseWorktreePorcelain(raw)
	if len(wts) != 4 {
		t.Fatalf("got %d worktrees, want 4", len(wts))
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
	if wts[3].Path != "/repo-gone" || !wts[3].Prunable {
		t.Errorf("prunable entry = %+v", wts[3])
	}
}
