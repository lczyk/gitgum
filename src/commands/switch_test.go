package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func currentBranchIn(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
}

func TestApplySelection_InvalidFormat(t *testing.T) {
	t.Parallel()
	s := &SwitchCommand{}
	err := s.applySelection("no-colon-separator")
	assert.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "invalid selection")
}

func TestApplySelection_UnknownType(t *testing.T) {
	t.Parallel()
	s := &SwitchCommand{}
	err := s.applySelection("unknown: foo")
	assert.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "unknown branch type")
}

func TestApplySelection_Local(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	var buf strings.Builder
	s := &SwitchCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	err := s.applySelection("local: feature")
	assert.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
	assert.ContainsString(t, buf.String(), "Switched to branch 'feature'.")
}

func TestApplySelection_RemoteInvalidFormat(t *testing.T) {
	t.Parallel()
	s := &SwitchCommand{}
	err := s.applySelection("remote: noslash")
	assert.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "invalid remote branch format")
}

// "local/remote" entries appear when a local branch already has a tracking
// remote — selecting such an entry must check out the local branch, not error.
func TestApplySelection_LocalRemote(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	var buf strings.Builder
	s := &SwitchCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	err := s.applySelection("local/remote: feature")
	assert.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
}

func TestResolveCurrentBranchContext_OnBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	currentBranch, trackingRemote, statusLine, err := resolveCurrentBranchContext(git.Repo{Dir: dir})
	assert.NoError(t, err)
	assert.Equal(t, trackingRemote, "")
	assert.ContainsString(t, statusLine, "Current branch is:")
	assert.ContainsString(t, statusLine, currentBranch)
}

// End-to-end Execute test driven by a stub Selector. The stub bypasses the
// real fuzzyfinder, so the streaming branch producers don't need to settle
// before SelectStream returns.
func TestSwitchCommand_Execute_PicksLocalBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	var buf strings.Builder
	stub := &stubSelector{selectAnswers: []string{"local: feature"}}
	cmd := &SwitchCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
	assert.ContainsString(t, buf.String(), "Switched to branch 'feature'.")
	assert.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Stream, true)
}

// Same-name branch on a different remote must appear even when the current
// branch is checked out. Regression: checkedOut["main"] was true (current
// branch is checked out), but the checkedOut filter in streamRemoteBranches
// dropped origin/main when local main tracked other/main — the filter didn't
// distinguish "checked out in another worktree" from "checked out here".
func TestStreamBranches_SameNameOnOtherRemote(t *testing.T) {
	t.Parallel()

	local, remote1 := temp_repo.NewRepoWithRemote(t)

	remote2 := t.TempDir()
	temp_repo.RunGit(t, remote2, "clone", "--bare", remote1, ".")
	temp_repo.RunGit(t, local, "remote", "add", "other", remote2)
	temp_repo.RunGit(t, local, "fetch", "other")
	temp_repo.RunGit(t, local, "branch", "--set-upstream-to=other/main", "main")

	r := git.Repo{Dir: local}
	trackingRemote, err := r.GetBranchTrackingRemote("main")
	assert.NoError(t, err)
	assert.Equal(t, trackingRemote, "other")

	remotes, err := r.GetRemotes()
	assert.NoError(t, err)

	var errBuf bytes.Buffer
	src := streamBranches(context.Background(), r, &errBuf, "main", trackingRemote, remotes)

	var items []string
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		items = src.Snapshot()
		if len(items) >= 1 {
			break
		}
	}

	hasOriginMain := false
	hasOtherMain := false
	for _, item := range items {
		if strings.Contains(item, "origin/main") {
			hasOriginMain = true
		}
		if strings.Contains(item, "other/main") {
			hasOtherMain = true
		}
	}
	assert.That(t, hasOriginMain, "origin/main should appear when local main tracks other/main")
	assert.That(t, !hasOtherMain, "other/main should not appear (current branch tracks it)")
}

// Regression: in detached HEAD, rev-parse --abbrev-ref returns "HEAD" and
// HEAD@{u} fails with "HEAD does not point to a branch". Previously this
// propagated as a "getting tracking remote" error and broke `gg switch`.
func TestResolveCurrentBranchContext_DetachedHEAD(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a", "chore: second")
	temp_repo.RunGit(t, dir, "checkout", "--detach", "HEAD~1")

	currentBranch, trackingRemote, statusLine, err := resolveCurrentBranchContext(git.Repo{Dir: dir})
	assert.NoError(t, err)
	assert.Equal(t, currentBranch, "")
	assert.Equal(t, trackingRemote, "")
	assert.ContainsString(t, statusLine, "detached HEAD")
}
