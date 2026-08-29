package commands

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestPushCommand_NotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := &PushCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestPushCommand_NoRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cmd := &PushCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when no remotes")
	assert.ContainsString(t, err.Error(), "no remotes")
}

// New remote branch flow: no upstream, no matching remote branch, user
// confirms creation. Stub answers the create-remote-branch prompt with yes.
func TestPushCommand_CreatesRemoteBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute([]string{"origin"})
	require.NoError(t, err)

	upstream := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}"))
	assert.Equal(t, upstream, "origin/"+branch)
	assert.ContainsString(t, buf.String(), "Created and set tracking reference")
	assert.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No remote branch")
}

// User declines the create-remote-branch confirmation: command exits cleanly
// without pushing or setting upstream.
func TestPushCommand_DeclinesCreateRemoteBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)

	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute([]string{"origin"})
	require.NoError(t, err)

	upstreamCmd := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{u}")
	upstreamCmd.Dir = dir
	assert.Error(t, upstreamCmd.Run(), assert.AnyError, "upstream must not be set when user declines")
}

// when upstream is configured and local matches remote, push exits without
// prompting.
func TestPushCommand_UpstreamSet_AlreadyUpToDate(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)

	var buf strings.Builder
	stub := &stubSelector{}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)
	assert.ContainsString(t, buf.String(), "No changes to push")
	assert.Equal(t, len(stub.confirmCalls), 0)
}

// local is ahead of an existing upstream: push renders a compact-summary of
// what's being sent (like gg diff) before the confirm, then pushes.
func TestPushCommand_UpstreamSet_ShowsDelta(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	// a new local commit puts local ahead of the upstream.
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\ny\n", "feat: local commit")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	out := buf.String()
	assert.ContainsString(t, out, "feature.yml")
	assert.ContainsString(t, out, "(new)") // compact-summary marker
	assert.ContainsString(t, out, "Pushed to remote tracking branch")
}

// user declines pushing to the upstream and there is only one remote: nothing
// else to offer, so the command exits cleanly without a picker.
func TestPushCommand_DeclineUpstream_SingleRemoteBreaks(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.selectCalls), 0)
	log := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(log, "feat: local commit"), "decline must not push")
}

// user declines pushing to the upstream but a second remote exists: push offers
// the other remotes, pushes there, and moves the upstream to the chosen remote.
func TestPushCommand_DeclineUpstream_OffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)
	mirrorDir := t.TempDir()
	temp_repo.RunGit(t, mirrorDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "mirror", mirrorDir)

	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	var buf strings.Builder
	// decline the upstream push, then confirm creating the branch on mirror.
	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	// picker offered only the non-upstream remote.
	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, len(stub.selectCalls[0].Options), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "mirror")

	// pushed to mirror, upstream moved to mirror, origin untouched.
	log := temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch)
	assert.ContainsString(t, log, "feat: local commit")
	upstream := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}"))
	assert.Equal(t, upstream, "mirror/"+branch)
	originLog := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(originLog, "feat: local commit"), "declined upstream must not receive the push")
}

// upstream configured and local matches the (stale) remote-tracking ref, but
// the branch was deleted on the remote: push must notice via a live check and
// offer to recreate it. Stub confirms the recreate prompt.
func TestPushCommand_UpstreamDeleted_RecreatesOnConfirm(t *testing.T) {
	t.Parallel()
	dir, bareDir := pushRepoWithStaleUpstream(t, "feat")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.ContainsString(t, buf.String(), "no longer exists on remote")
	assert.ContainsString(t, buf.String(), "Recreated remote branch")
	assert.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "Recreate remote branch")

	// branch is back on the remote after recreate.
	listed := strings.TrimSpace(temp_repo.RunGit(t, bareDir, "branch", "--list", "feat"))
	assert.NotEqual(t, listed, "", "feat must exist on remote after recreate")
}

// same stale-upstream setup, but the user declines the recreate prompt: push
// exits cleanly and does not resurrect the branch on the remote.
func TestPushCommand_UpstreamDeleted_DeclineDoesNotRecreate(t *testing.T) {
	t.Parallel()
	dir, bareDir := pushRepoWithStaleUpstream(t, "feat")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.confirmCalls), 1)
	listed := strings.TrimSpace(temp_repo.RunGit(t, bareDir, "branch", "--list", "feat"))
	assert.Equal(t, listed, "", "feat must not be recreated when user declines")
}

// pushRepoWithStaleUpstream builds a local repo tracking origin/<branch> whose
// remote-tracking ref is stale: the branch is pushed with -u, then deleted on
// the bare remote directly (no fetch/prune), so the local cache still points at
// it. Returns (localDir, bareDir) with <branch> checked out locally.
func pushRepoWithStaleUpstream(t *testing.T, branch string) (dir, bareDir string) {
	t.Helper()
	dir = temp_repo.NewRepo(t)

	bareDir = t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	// dedicated branch so the bare repo's HEAD (default branch) never points at
	// it -- otherwise deleting it server-side would be refused.
	temp_repo.RunGit(t, dir, "checkout", "-b", branch)
	temp_repo.CreateCommit(t, dir, branch+".txt", "x", "on "+branch)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)

	// delete on the remote without touching the local remote-tracking ref.
	temp_repo.RunGit(t, bareDir, "branch", "-D", branch)
	return dir, bareDir
}

// remote diverged from local (each side has a commit the other lacks) and a
// rebase would apply cleanly (different files): push offers pull --rebase, and
// on confirm rebases + pushes the result.
func TestPushCommand_Diverged_OffersRebaseAndPushes(t *testing.T) {
	t.Parallel()
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	// local commit on a different file -> clean rebase.
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "diverged")
	assert.ContainsString(t, buf.String(), "Rebased")

	// origin now carries both the remote and the (rebased) local commit.
	log := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.ContainsString(t, log, "feat: local commit")
	assert.ContainsString(t, log, "remote commit")
}

// same diverged setup, user declines the rebase: push exits cleanly and does
// not touch the remote.
func TestPushCommand_Diverged_DeclineDoesNotPush(t *testing.T) {
	t.Parallel()
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")

	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.confirmCalls), 1)
	log := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(log, "feat: local commit"), "local commit must not reach remote on decline")
}

// remote diverged and a rebase would conflict (both sides touched the same
// file): push errors before prompting, telling the user to integrate manually.
func TestPushCommand_Diverged_ConflictErrors(t *testing.T) {
	t.Parallel()
	dir, _, _ := pushRepoWithRemoteAhead(t, "conflict.txt", "remote version\n")
	// local commit on the SAME file with different content -> rebase conflict.
	temp_repo.CreateCommit(t, dir, "conflict.txt", "local version\n", "feat: local commit")

	stub := &stubSelector{}
	cmd := &PushCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "conflicting divergence should error")
	assert.ContainsString(t, err.Error(), "would conflict")
	assert.Equal(t, len(stub.confirmCalls), 0)
}

// local strictly behind the remote (no local commits): a plain push is
// rejected, so push offers a fast-forward pull --rebase; there is nothing to
// push afterwards.
func TestPushCommand_Behind_OffersFastForward(t *testing.T) {
	t.Parallel()
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.ContainsString(t, buf.String(), "Fast-forwarded")
	assert.ContainsString(t, buf.String(), "Nothing to push")
	// local now carries the remote commit.
	log := temp_repo.RunGit(t, dir, "log", "--format=%s", branch)
	assert.ContainsString(t, log, "remote commit")
}

// pushRepoWithRemoteAhead builds a local repo tracking origin/<branch> whose
// remote has advanced by one commit (touching `file`) that local hasn't fetched
// yet: origin is strictly ahead and local's remote-tracking ref is stale. The
// extra commit is pushed via a throwaway clone so the local working copy is
// untouched. Returns (localDir, bareDir, branch).
func pushRepoWithRemoteAhead(t *testing.T, file, content string) (dir, bareDir, branch string) {
	t.Helper()
	dir = temp_repo.NewRepo(t)
	bareDir = t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch = currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)

	// advance origin from a separate clone; local stays put with a stale ref.
	other := t.TempDir()
	temp_repo.RunGit(t, other, "clone", bareDir, ".")
	temp_repo.RunGit(t, other, "config", "user.name", "Other User")
	temp_repo.RunGit(t, other, "config", "user.email", "other@example.com")
	temp_repo.RunGit(t, other, "config", "commit.gpgsign", "false")
	// pin hooks to the local dir so a global commit-msg hook (e.g. a
	// conventional-commits guard) doesn't reject this fixture commit.
	temp_repo.RunGit(t, other, "config", "core.hooksPath", ".git/hooks")
	temp_repo.CreateCommit(t, other, file, content, "feat: remote commit")
	temp_repo.RunGit(t, other, "push", "origin", branch)
	return dir, bareDir, branch
}

// when local matches remote but no tracking is configured, push sets the
// upstream without prompting (no UI interaction needed).
func TestPushCommand_AlreadyUpToDate_SetsUpstream(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "origin", branch)

	cmd := &PushCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute([]string{"origin"})
	require.NoError(t, err, "should succeed when already up to date")

	upstream := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}"))
	assert.Equal(t, upstream, "origin/"+branch)
}

// A genuine non-fast-forward reaches the push error path: the remote branch
// exists and has moved on, local has not, and tracking is not configured -- so
// gg offers to push to the existing branch and git refuses it. No broken remote
// or artificial hook, just a situation people land in.
func TestPushCommand_RejectedPushSurfacesTheFailure(t *testing.T) {
	t.Parallel()
	dir, _, _ := pushRepoWithRemoteAhead(t, "upstream.txt", "from elsewhere")

	// drop tracking so push takes the "remote branch exists" path, and commit
	// locally so the push is a genuine non-fast-forward rather than a no-op
	temp_repo.RunGit(t, dir, "branch", "--unset-upstream")
	temp_repo.CreateCommit(t, dir, "local.txt", "mine", "chore: local work")

	// pick the remote, then confirm the push git will refuse
	stub := &stubSelector{selectAnswers: []string{"origin"}, confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "git should refuse a non-fast-forward")
	assert.ContainsString(t, err.Error(), "push")
}
