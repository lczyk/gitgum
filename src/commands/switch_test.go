package commands

import (
	"strings"
	"testing"

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

// Regression: in detached HEAD, rev-parse --abbrev-ref returns "HEAD" and
// HEAD@{u} fails with "HEAD does not point to a branch". Previously this
// propagated as a "getting tracking remote" error and broke `gg switch`.
func TestResolveCurrentBranchContext_DetachedHEAD(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a", "second")
	temp_repo.RunGit(t, dir, "checkout", "--detach", "HEAD~1")

	currentBranch, trackingRemote, statusLine, err := resolveCurrentBranchContext(git.Repo{Dir: dir})
	assert.NoError(t, err)
	assert.Equal(t, currentBranch, "")
	assert.Equal(t, trackingRemote, "")
	assert.ContainsString(t, statusLine, "detached HEAD")
}
