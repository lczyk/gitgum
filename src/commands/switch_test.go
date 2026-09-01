package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/dirty"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
	"github.com/lczyk/gitgum/internal/ui"
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
	require.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
	assert.ContainsString(t, buf.String(), "Switched to branch 'feature'.")
}

// Selecting the branch you're already on (the HEAD row) pulls it: a branch
// behind its upstream fast-forwards.
func TestApplySelection_HEADFastForwards(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	remoteHead := advanceRemote(t, remote, "feature.txt", "x", "feat: upstream commit")

	var buf strings.Builder
	s := &SwitchCommand{cmdIO: cmdIO{Out: &buf, UI: &stubSelector{}, Repo: git.Repo{Dir: local}}}
	// any payload carrying the HEAD marker routes to the pull-current path.
	err := s.applySelection("local/remote: origin/main" + currentBranchMarker)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD")), remoteHead)
}

// Switching to a purely local branch (no upstream) still succeeds -- there's
// nothing to pull, and that's not an error.
func TestApplySelection_LocalNoUpstreamBenign(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	var buf strings.Builder
	s := &SwitchCommand{cmdIO: cmdIO{Out: &buf, UI: &stubSelector{}, Repo: git.Repo{Dir: dir}}}
	err := s.applySelection("local: feature")
	require.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
	assert.ContainsString(t, buf.String(), "Switched to branch 'feature'.")
	assert.ContainsString(t, buf.String(), "nothing to pull")
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
	err := s.applySelection("local/remote: origin/feature")
	require.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
}

// The remote prefix is stripped on the first '/', so a branch name that itself
// contains '/' round-trips intact.
func TestApplySelection_LocalRemoteSlashBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feat/x")

	var buf strings.Builder
	s := &SwitchCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	err := s.applySelection("local/remote: origin/feat/x")
	require.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feat/x")
}

// Regression (bug: missing choices): a local branch that tracks a remote must
// carry the remote name in its picker row so it's searchable by remote owner.
// Previously the row was "local/remote: <branch>" with the remote dropped, so
// typing the remote name filtered the branch out entirely.
func TestStreamBranches_LocalRemoteIncludesRemote(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "fetch", "origin")
	temp_repo.RunGit(t, local, "branch", "feature")
	temp_repo.RunGit(t, local, "branch", "--set-upstream-to=origin/main", "feature")

	r := git.Repo{Dir: local}
	remotes, err := r.GetRemotes()
	require.NoError(t, err)

	var errBuf bytes.Buffer
	src := streamBranches(context.Background(), r, &errBuf, currentBranchIn(t, local), "", remotes,
		branchStreamOpts{markCheckedOut: true})

	var feature string
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		for _, item := range src.Snapshot() {
			if strings.HasPrefix(item, "local/remote: ") && strings.Contains(item, "feature") {
				feature = item
			}
		}
		if feature != "" {
			break
		}
	}
	require.That(t, feature != "", "feature branch tracking origin should appear")
	assert.Equal(t, feature, "local/remote: origin/feature")
}

func TestResolveCurrentBranchContext_OnBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	currentBranch, trackingRemote, statusLine, err := resolveCurrentBranchContext(git.Repo{Dir: dir})
	require.NoError(t, err)
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
	require.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
	assert.ContainsString(t, buf.String(), "Switched to branch 'feature'.")
	assert.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Stream, true)
}

// Backing out of the branch picker after choosing discard must leave the
// working tree alone -- the answer to the dirty prompt only takes effect once
// there is a branch to switch to.
func TestSwitchCommand_Execute_DiscardWaitsForSelection(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")
	temp_repo.WriteFile(t, dir, "README.md", "edited\n")

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers: []string{discardLabel(
			dirty.Plan{Tracked: []string{"README.md"}},
			dirty.Options{Tracked: true, Untracked: true})},
		// the dirty prompt answers, the branch picker is escaped
		selectErrs: []error{nil, ui.ErrCancelled},
	}
	cmd := &SwitchCommand{cmdIO: cmdIO{Out: &buf, Err: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.ErrorIs(t, err, ui.ErrCancelled)
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "status", "--porcelain")), "M README.md")
	assert.That(t, !strings.Contains(buf.String(), "Discarded"), "nothing was discarded")
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
	require.NoError(t, err)
	assert.Equal(t, trackingRemote, "other")

	remotes, err := r.GetRemotes()
	require.NoError(t, err)

	var errBuf bytes.Buffer
	src := streamBranches(context.Background(), r, &errBuf, "main", trackingRemote, remotes,
		branchStreamOpts{markCheckedOut: true})

	var items []string
	for range 50 {
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

// A branch checked out in another worktree must still appear in the picker,
// tagged with the worktree marker and reported unselectable -- previously it
// was silently dropped, which looked like a missing branch.
func TestStreamBranches_CheckedOutElsewhereIsMarkedUnselectable(t *testing.T) {
	t.Parallel()

	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")
	wt := t.TempDir()
	temp_repo.RunGit(t, dir, "worktree", "add", wt, "feature")

	r := git.Repo{Dir: dir}
	remotes, err := r.GetRemotes()
	require.NoError(t, err)

	var errBuf bytes.Buffer
	src := streamBranches(context.Background(), r, &errBuf, currentBranchIn(t, dir), "", remotes,
		branchStreamOpts{markCheckedOut: true})

	var feature string
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		for _, item := range src.Snapshot() {
			if strings.Contains(item, "feature") {
				feature = item
			}
		}
		if feature != "" {
			break
		}
	}

	require.That(t, feature != "", "feature branch should appear")
	assert.ContainsString(t, feature, "(checked out in")
	assert.That(t, isUnselectable(feature), "marked entry should be unselectable")
}

// The branch checked out in the current worktree reads "(checked out here.
// HEAD)" rather than naming the worktree -- the "HEAD" token lets you type HEAD
// to surface it. In switch/delete (markCheckedOut) it stays unselectable --
// you're already on it, so `git checkout` is a no-op.
func TestStreamBranches_CurrentBranchReadsHere(t *testing.T) {
	t.Parallel()

	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}
	remotes, err := r.GetRemotes()
	require.NoError(t, err)

	current := currentBranchIn(t, dir)

	var errBuf bytes.Buffer
	src := streamBranches(context.Background(), r, &errBuf, current, "", remotes,
		branchStreamOpts{markCheckedOut: true, includeCurrent: true})

	var entry string
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		for _, item := range src.Snapshot() {
			if strings.Contains(item, current) {
				entry = item
			}
		}
		if entry != "" {
			break
		}
	}

	require.That(t, entry != "", "current branch should appear")
	assert.ContainsString(t, entry, "(checked out here. HEAD)")
	assert.That(t, !strings.Contains(entry, "worktree"), "current branch should not name a worktree")
	assert.That(t, isUnselectable(entry), "current branch should be unselectable")
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
	require.NoError(t, err)
	assert.Equal(t, currentBranch, "")
	assert.Equal(t, trackingRemote, "")
	assert.ContainsString(t, statusLine, "detached HEAD")
}

// Regression: `git branch` lists a "(HEAD detached at abc1234)" pseudo-entry,
// which used to reach the picker as a selectable "local:" branch and fail on
// checkout. The picker now names HEAD itself, and refuses to switch to it.
func TestStreamBranches_DetachedHEAD(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a", "chore: second")
	temp_repo.RunGit(t, dir, "checkout", "--detach", "HEAD~1")
	r := git.Repo{Dir: dir}

	short := detachedShortSHA(r, "")
	require.That(t, short != "", "detached HEAD should have a short sha")

	ctx := t.Context()
	var errBuf bytes.Buffer
	src := streamBranches(ctx, r, &errBuf, "", "", nil,
		branchStreamOpts{includeCurrent: true, markCheckedOut: true, detachedAt: short})

	var items []string
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		if items = src.Snapshot(); len(items) == 2 {
			break
		}
	}
	assert.Equal(t, len(items), 2)
	assert.Equal(t, items[0], "local: HEAD (detached at "+short+")")
	assert.That(t, isUnselectable(items[0]), "detached HEAD should be unselectable")

	// the pseudo-entry no longer masquerades as a branch
	assert.Equal(t, items[1], "local: main")
	assert.That(t, !isUnselectable(items[1]), "main is not checked out anywhere")
}
