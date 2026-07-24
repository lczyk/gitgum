package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// advanceRemote clones the bare remote into a throwaway worktree, commits a
// new file on main, and pushes it back. Returns the new remote head hash. Used
// to simulate upstream moving ahead between pulls.
func advanceRemote(t *testing.T, remoteBare, filename, content, msg string) string {
	t.Helper()
	other := t.TempDir()
	temp_repo.RunGit(t, other, "clone", remoteBare, ".")
	temp_repo.RunGit(t, other, "config", "user.name", "Other User")
	temp_repo.RunGit(t, other, "config", "user.email", "other@example.com")
	temp_repo.RunGit(t, other, "config", "commit.gpgsign", "false")
	temp_repo.RunGit(t, other, "config", "core.hooksPath", ".git/hooks")
	temp_repo.CreateCommit(t, other, filename, content, msg)
	temp_repo.RunGit(t, other, "push", "origin", "main")
	return strings.TrimSpace(temp_repo.RunGit(t, other, "rev-parse", "HEAD"))
}

// A checkout-pr branch has no upstream, but `gg pull` re-fetches the PR ref
// and fast-forwards to its latest head.
func TestPullCommand_PRBranchFastForward(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)
	temp_repo.RunGit(t, dir, "push", "origin", "HEAD")
	headSHA := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))
	temp_repo.RunGit(t, bareDir, "update-ref", "refs/pull/1/head", headSHA)

	// Land on the PR branch via checkout-pr (records the metadata pull reads).
	co := &CheckoutPRCommand{cmdIO: cmdIO{UI: &stubSelector{selectAnswers: []string{"origin", "PR #1 (head)"}}, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, co.Execute(nil))
	require.Equal(t, currentBranchIn(t, dir), "pr/origin/1")

	// Advance the PR head on the remote (a new commit fast-forward from it).
	other := t.TempDir()
	temp_repo.RunGit(t, other, "clone", bareDir, ".")
	temp_repo.RunGit(t, other, "config", "user.name", "Other User")
	temp_repo.RunGit(t, other, "config", "user.email", "other@example.com")
	temp_repo.RunGit(t, other, "config", "commit.gpgsign", "false")
	temp_repo.RunGit(t, other, "config", "core.hooksPath", ".git/hooks")
	temp_repo.CreateCommit(t, other, "prfile.txt", "y", "feat: new pr commit")
	newSHA := strings.TrimSpace(temp_repo.RunGit(t, other, "rev-parse", "HEAD"))
	temp_repo.RunGit(t, other, "push", "origin", "HEAD:refs/pull/1/head")

	var buf strings.Builder
	pull := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: &stubSelector{}, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, pull.Execute(nil))

	assert.ContainsString(t, buf.String(), "Updated 'pr/origin/1' to PR #1")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD")), newSHA)
	assert.Equal(t, currentBranchIn(t, dir), "pr/origin/1")
}

func TestPullCommand_NotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := &PullCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestPullCommand_NoUpstream(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cmd := &PullCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when no upstream")
	assert.ContainsString(t, err.Error(), "no upstream configured")
}

// local matches upstream after the clone: pull reports up to date without
// showing the strategy picker.
func TestPullCommand_AlreadyUpToDate(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)

	var buf strings.Builder
	stub := &stubSelector{}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)
	assert.ContainsString(t, buf.String(), "Already up to date")
	assert.Equal(t, len(stub.selectCalls), 0)
}

// upstream moved ahead, local is strictly behind: the strategy picker is
// pointless (all three modes fast-forward identically), so it is skipped and
// HEAD advances to the remote head with no merge commit.
func TestPullCommand_FastForward(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	remoteHead := advanceRemote(t, remote, "feature.txt", "x", "feat: upstream commit")

	var buf strings.Builder
	stub := &stubSelector{}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.selectCalls), 0, "ff-able pull must not show the picker")
	localHead := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	assert.Equal(t, localHead, remoteHead)
	merges := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-list", "--merges", "HEAD"))
	assert.Equal(t, merges, "", "fast-forward must not create a merge commit")
	assert.ContainsString(t, buf.String(), "Pulled")
}

// a fast-forward pull renders what landed as a compact-summary (like gg diff),
// not git's plain --stat: added files carry the "(new)" annotation.
func TestPullCommand_RendersCompactSummary(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, remote, "feature.txt", "x\ny\n", "feat: upstream commit")

	var buf strings.Builder
	stub := &stubSelector{}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	out := buf.String()
	assert.ContainsString(t, out, "feature.txt")
	assert.ContainsString(t, out, "(new)") // compact-summary marker, absent from plain --stat
	assert.ContainsString(t, out, "Pulled")
}

// local moved ahead while upstream stood still: nothing to pull, and the picker
// is skipped -- ff-only would only fail and rebase/merge are no-ops.
func TestPullCommand_LocalAhead(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	before := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	temp_repo.CreateCommit(t, local, "local.txt", "l", "feat: local commit")
	head := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))

	var buf strings.Builder
	stub := &stubSelector{}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.selectCalls), 0, "nothing-to-pull must not show the picker")
	assert.ContainsString(t, buf.String(), "Nothing to pull")
	after := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	assert.Equal(t, after, head, "HEAD must not move")
	assert.NotEqual(t, after, before, "local commit stays")
}

// local and upstream diverged (each has a unique commit): rebase replays the
// local commit onto upstream, keeping history linear.
func TestPullCommand_Rebase(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, remote, "remote.txt", "r", "feat: remote commit")
	temp_repo.CreateCommit(t, local, "local.txt", "l", "feat: local commit")

	stub := &stubSelector{selectAnswers: []string{git.PullRebase.String()}}
	cmd := &PullCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	merges := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-list", "--merges", "HEAD"))
	assert.Equal(t, merges, "", "rebase must not create a merge commit")
	// both the rebased-local and the upstream file are present.
	files := temp_repo.RunGit(t, local, "ls-files")
	assert.ContainsString(t, files, "local.txt")
	assert.ContainsString(t, files, "remote.txt")
}

// same diverged setup, merge strategy: a merge commit joins the two lines.
func TestPullCommand_Merge(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, remote, "remote.txt", "r", "feat: remote commit")
	temp_repo.CreateCommit(t, local, "local.txt", "l", "feat: local commit")

	stub := &stubSelector{selectAnswers: []string{git.PullMerge.String()}}
	cmd := &PullCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	merges := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-list", "--merges", "HEAD"))
	assert.NotEqual(t, merges, "", "merge must create a merge commit when diverged")
}

// dirty tracked change is stashed before the pull and popped after, so the
// uncommitted edit survives a fast-forward.
func TestPullCommand_DirtyTreeStashed(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, remote, "feature.txt", "x", "feat: upstream commit")

	// uncommitted edit to a tracked file (README.md from the init commit).
	temp_repo.WriteFile(t, local, "README.md", "# locally edited\n")

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers:  []string{git.PullFFOnly.String()},
		confirmAnswers: []bool{true}, // yes, stash and continue
	}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.confirmCalls), 1)
	// upstream file arrived and the dirty edit was restored.
	files := temp_repo.RunGit(t, local, "ls-files")
	assert.ContainsString(t, files, "feature.txt")
	assert.ContainsString(t, temp_repo.RunGit(t, local, "status", "--porcelain"), "README.md")
}

// declining the stash prompt aborts cleanly without integrating.
func TestPullCommand_DirtyTreeDeclined(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, remote, "feature.txt", "x", "feat: upstream commit")
	temp_repo.WriteFile(t, local, "README.md", "# locally edited\n")

	before := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	stub := &stubSelector{
		selectAnswers:  []string{git.PullFFOnly.String()},
		confirmAnswers: []bool{false}, // decline the stash
	}
	cmd := &PullCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.NoError(t, err, "declining is a clean abort")

	after := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	assert.Equal(t, after, before, "HEAD must not move when the stash is declined")
	files := temp_repo.RunGit(t, local, "ls-files")
	assert.That(t, !strings.Contains(files, "feature.txt"), "upstream commit must not be integrated")
}

// a shallow clone stays shallow across a pull: new commits arrive, old history
// is not backfilled.
func TestPullCommand_ShallowStaysShallow(t *testing.T) {
	t.Parallel()
	_, remote := temp_repo.NewRepoWithRemote(t)
	// give the remote some depth so an accidental unshallow would be visible.
	advanceRemote(t, remote, "a.txt", "a", "feat: a")
	advanceRemote(t, remote, "b.txt", "b", "feat: b")

	shallow := t.TempDir()
	// --no-local: git otherwise hardlinks a local-path clone and ignores --depth,
	// so the clone would not actually be shallow.
	temp_repo.RunGit(t, shallow, "clone", "--no-local", "--depth=1", remote, ".")
	temp_repo.RunGit(t, shallow, "config", "user.name", "Test User")
	temp_repo.RunGit(t, shallow, "config", "user.email", "test@example.com")
	temp_repo.RunGit(t, shallow, "config", "commit.gpgsign", "false")
	temp_repo.RunGit(t, shallow, "config", "core.hooksPath", ".git/hooks")

	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, shallow, "rev-parse", "--is-shallow-repository")), "true")

	// upstream moves ahead after the shallow clone.
	remoteHead := advanceRemote(t, remote, "c.txt", "c", "feat: c")

	stub := &stubSelector{selectAnswers: []string{git.PullFFOnly.String()}}
	cmd := &PullCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: shallow}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	// new commit arrived...
	localHead := strings.TrimSpace(temp_repo.RunGit(t, shallow, "rev-parse", "HEAD"))
	assert.Equal(t, localHead, remoteHead)
	// ...but the clone is still shallow (old history not backfilled).
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, shallow, "rev-parse", "--is-shallow-repository")), "true")
}

// A --single-branch clone (which `git clone --depth` implies) maps one branch in
// its fetch refspec, leaving every other branch with an upstream in config and
// no remote-tracking ref. That used to surface as a bare
// "getting upstream: exit status 128"; pull now fetches the ref explicitly,
// warns about the refspec, and integrates as normal.
func TestPullCommand_UpstreamNotCoveredByRefspec(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)

	// publish "feature" on the remote, one commit ahead of the clone.
	other := t.TempDir()
	temp_repo.RunGit(t, other, "clone", remote, ".")
	temp_repo.RunGit(t, other, "config", "user.name", "Other User")
	temp_repo.RunGit(t, other, "config", "user.email", "other@example.com")
	temp_repo.RunGit(t, other, "config", "commit.gpgsign", "false")
	temp_repo.RunGit(t, other, "config", "core.hooksPath", ".git/hooks")
	temp_repo.RunGit(t, other, "checkout", "-b", "feature")
	temp_repo.CreateCommit(t, other, "feature.txt", "x\n", "feat: on feature")
	temp_repo.RunGit(t, other, "push", "origin", "feature")
	remoteHead := strings.TrimSpace(temp_repo.RunGit(t, other, "rev-parse", "HEAD"))

	// reproduce the single-branch clone's config: an upstream git can't resolve.
	temp_repo.RunGit(t, local, "checkout", "-b", "feature")
	temp_repo.RunGit(t, local, "config", "branch.feature.remote", "origin")
	temp_repo.RunGit(t, local, "config", "branch.feature.merge", "refs/heads/feature")
	temp_repo.RunGit(t, local, "config", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")

	var out, errBuf strings.Builder
	stub := &stubSelector{}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &out, Err: &errBuf, UI: stub, Repo: git.Repo{Dir: local}}}

	require.NoError(t, cmd.Execute(nil))

	localHead := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	assert.Equal(t, localHead, remoteHead)
	assert.ContainsString(t, errBuf.String(), "narrow fetch refspec")
	assert.ContainsString(t, errBuf.String(), "git config remote.origin.fetch")
	assert.Equal(t, len(stub.selectCalls), 0, "a fast-forward must not show the picker")
}

// Same missing tracking ref, opposite cause: the branch was never pushed (or was
// deleted upstream), so no refspec widening would help. Say that, rather than
// blaming the refspec and letting git's "couldn't find remote ref" fatal through.
func TestPullCommand_UpstreamGoneFromRemote(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)

	temp_repo.RunGit(t, local, "checkout", "-b", "never-pushed")
	temp_repo.RunGit(t, local, "config", "branch.never-pushed.remote", "origin")
	temp_repo.RunGit(t, local, "config", "branch.never-pushed.merge", "refs/heads/never-pushed")

	var out, errBuf strings.Builder
	cmd := &PullCommand{cmdIO: cmdIO{Out: &out, Err: &errBuf, UI: &stubSelector{}, Repo: git.Repo{Dir: local}}}

	err := cmd.Execute(nil)
	require.Error(t, err, "does not exist on remote 'origin'")
	assert.That(t, !strings.Contains(errBuf.String(), "narrow fetch refspec"),
		"must not blame the refspec: ", errBuf.String())
}
