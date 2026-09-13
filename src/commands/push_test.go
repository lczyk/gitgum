package commands

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
	"github.com/lczyk/gitgum/internal/ui"
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

// User declines the create-remote-branch confirmation and there is no other
// remote to offer: command says so and exits without pushing or setting
// upstream.
func TestPushCommand_DeclinesCreateRemoteBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute([]string{"origin"})
	require.NoError(t, err)
	assert.Equal(t, len(stub.selectCalls), 0)
	assert.ContainsString(t, buf.String(), "Nothing pushed")

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
// else to offer, so the command says nothing was pushed and exits without a
// picker.
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
	assert.ContainsString(t, buf.String(), "Nothing pushed")
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

// same stale-upstream setup with a single remote, user declines the recreate
// prompt: nothing else to offer, so push says nothing was pushed and does not
// resurrect the branch on the remote.
func TestPushCommand_UpstreamDeleted_DeclineDoesNotRecreate(t *testing.T) {
	t.Parallel()
	dir, bareDir := pushRepoWithStaleUpstream(t, "feat")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.confirmCalls), 1)
	assert.Equal(t, len(stub.selectCalls), 0)
	assert.ContainsString(t, buf.String(), "Nothing pushed")
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

// same diverged setup with a single remote, user declines the rebase: nothing
// else to offer, so push says nothing was pushed and leaves the remote alone.
func TestPushCommand_Diverged_DeclineDoesNotPush(t *testing.T) {
	t.Parallel()
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	assert.Equal(t, len(stub.confirmCalls), 1)
	assert.Equal(t, len(stub.selectCalls), 0)
	assert.ContainsString(t, buf.String(), "Nothing pushed")
	log := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(log, "feat: local commit"), "local commit must not reach remote on decline")
}

// remote diverged and a rebase would conflict (both sides touched the same
// file), with no other remote to offer: push errors before prompting, telling
// the user to integrate manually.
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

// addBareRemote creates an empty bare repo and adds it to dir as remote name,
// returning the bare repo's path.
func addBareRemote(t *testing.T, dir, name string) string {
	t.Helper()
	bare := t.TempDir()
	temp_repo.RunGit(t, bare, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", name, bare)
	return bare
}

// upstreamOf returns branch's configured upstream in dir, e.g. "origin/main".
func upstreamOf(t *testing.T, dir, branch string) string {
	t.Helper()
	return strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}"))
}

// remote diverged and the user declines the rebase because the commits belong
// elsewhere: push offers the other remotes rather than exiting, and pushing
// there moves the upstream.
func TestPushCommand_Diverged_DeclineOffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	mirrorDir := addBareRemote(t, dir, "mirror")

	// decline the rebase, pick mirror, confirm creating the branch there.
	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.confirmCalls), 2)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "diverged")
	require.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.selectCalls[0].Options), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "mirror")

	assert.ContainsString(t, temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch), "feat: local commit")
	assert.Equal(t, upstreamOf(t, dir, branch), "mirror/"+branch)
	originLog := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(originLog, "feat: local commit"), "declined upstream must not receive the push")
}

// local strictly behind, user declines the fast-forward: the other remotes are
// offered, and the local tip goes there as it is.
func TestPushCommand_Behind_DeclineOffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	mirrorDir := addBareRemote(t, dir, "mirror")

	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.confirmCalls), 2)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "fast-forward")
	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "mirror")

	local := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))
	pushed := strings.TrimSpace(temp_repo.RunGit(t, mirrorDir, "rev-parse", branch))
	assert.Equal(t, pushed, local)
	assert.Equal(t, upstreamOf(t, dir, branch), "mirror/"+branch)
}

// upstream branch deleted on the remote, user declines recreating it: the
// other remotes are offered instead, and the branch lands there.
func TestPushCommand_UpstreamDeleted_DeclineOffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir, bareDir := pushRepoWithStaleUpstream(t, "feat")
	mirrorDir := addBareRemote(t, dir, "mirror")

	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.confirmCalls), 2)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "Recreate remote branch")
	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "mirror")

	assert.NotEqual(t, strings.TrimSpace(temp_repo.RunGit(t, mirrorDir, "branch", "--list", "feat")), "")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, bareDir, "branch", "--list", "feat")), "",
		"declined recreate must leave origin alone")
	assert.Equal(t, upstreamOf(t, dir, "feat"), "mirror/feat")
}

// remote diverged and a rebase would conflict, but another remote exists: push
// says why it won't rebase, then offers that remote rather than erroring.
func TestPushCommand_Diverged_ConflictOffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir, _, branch := pushRepoWithRemoteAhead(t, "conflict.txt", "remote version\n")
	temp_repo.CreateCommit(t, dir, "conflict.txt", "local version\n", "feat: local commit")
	mirrorDir := addBareRemote(t, dir, "mirror")

	var errBuf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &errBuf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, errBuf.String(), "would conflict")
	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No remote branch")
	assert.ContainsString(t, temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch), "feat: local commit")
}

// Escape on the other-remotes picker cancels, like every other prompt, and
// leaves the upstream where it was.
func TestPushCommand_DeclineOffer_EscapeCancels(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	addBareRemote(t, dir, "mirror")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	stub := &stubSelector{confirmAnswers: []bool{false}, selectErrs: []error{ui.ErrCancelled}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled)
	assert.Equal(t, upstreamOf(t, dir, branch), "origin/"+branch)
}

// declining the remote picked from the offer rules it out too: the remaining
// remotes are offered next.
func TestPushCommand_DeclinePickedRemote_OffersTheRest(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	mirrorDir := addBareRemote(t, dir, "mirror")
	backupDir := addBareRemote(t, dir, "backup")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	// decline origin, pick mirror, decline creating it there, pick backup, create.
	stub := &stubSelector{confirmAnswers: []bool{false, false, true}, selectAnswers: []string{"mirror", "backup"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.selectCalls), 2)
	assert.Equal(t, len(stub.selectCalls[0].Options), 2)
	require.Equal(t, len(stub.selectCalls[1].Options), 1)
	assert.Equal(t, stub.selectCalls[1].Options[0], "backup")

	assert.ContainsString(t, temp_repo.RunGit(t, backupDir, "log", "--format=%s", branch), "feat: local commit")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, mirrorDir, "branch", "--list", branch)), "")
	assert.Equal(t, upstreamOf(t, dir, branch), "backup/"+branch)
}

// no upstream, the named remote lacks the branch and the user declines
// creating it there: the other remotes are offered.
func TestPushCommand_NoUpstream_DeclineCreateOffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	originDir := addBareRemote(t, dir, "origin")
	mirrorDir := addBareRemote(t, dir, "mirror")
	branch := currentBranchIn(t, dir)

	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"origin"}))

	require.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.selectCalls[0].Options), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "mirror")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, originDir, "branch", "--list", branch)), "")
	assert.NotEqual(t, strings.TrimSpace(temp_repo.RunGit(t, mirrorDir, "branch", "--list", branch)), "")
	assert.Equal(t, upstreamOf(t, dir, branch), "mirror/"+branch)
}

// no upstream, the named remote already has the branch at an older commit and
// the user declines pushing to it: the other remotes are offered.
func TestPushCommand_DeclineExistingRemoteBranch_OffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	originDir := addBareRemote(t, dir, "origin")
	mirrorDir := addBareRemote(t, dir, "mirror")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "mirror", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	// decline pushing to mirror's existing branch, pick origin, create it there.
	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"mirror"}))

	require.Equal(t, len(stub.confirmCalls), 2)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "already exists")
	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "origin")
	assert.ContainsString(t, temp_repo.RunGit(t, originDir, "log", "--format=%s", branch), "feat: local commit")
	mirrorLog := temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(mirrorLog, "feat: local commit"), "declined remote must not receive the push")
}

// single remote, local strictly behind, user declines the fast-forward:
// nothing else to offer, so push says nothing was pushed and moves nothing.
func TestPushCommand_Behind_DeclineSingleRemote(t *testing.T) {
	t.Parallel()
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	before := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	assert.Equal(t, len(stub.selectCalls), 0)
	assert.ContainsString(t, buf.String(), "Nothing pushed")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch)), before)
}

// single remote that already has the branch at an older commit, no upstream,
// user declines pushing to it: nothing else to offer, so push says nothing was
// pushed and sets no upstream.
func TestPushCommand_DeclineExistingRemoteBranch_SingleRemote(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"origin"}))

	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "already exists")
	assert.Equal(t, len(stub.selectCalls), 0)
	assert.ContainsString(t, buf.String(), "Nothing pushed")
	_, err := temp_repo.RunGitAllowFail(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}")
	assert.Error(t, err, assert.AnyError, "upstream must not be set when user declines")
}

// the in-memory rebase check itself fails (not a conflict) with a single
// remote: push reports it couldn't check, before any prompt.
func TestPushCommand_Diverged_MergeTreeFailureErrors(t *testing.T) {
	dir, _, _ := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	failGitSubcommand(t, "merge-tree")

	stub := &stubSelector{}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "a failed rebase check with nowhere else to go should error")
	assert.ContainsString(t, err.Error(), "could not check")
	assert.Equal(t, len(stub.confirmCalls), 0)
}

// same failing check with another remote: push says it couldn't check, then
// offers that remote.
func TestPushCommand_Diverged_MergeTreeFailureOffersOtherRemotes(t *testing.T) {
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	mirrorDir := addBareRemote(t, dir, "mirror")
	failGitSubcommand(t, "merge-tree")

	var errBuf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &errBuf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, errBuf.String(), "could not check")
	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "mirror")
	assert.ContainsString(t, temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch), "feat: local commit")
}

// an upstream is set but the user names another remote: push goes there,
// skipping the upstream comparison, and moves the upstream.
func TestPushCommand_RemoteArg_OverridesUpstream(t *testing.T) {
	t.Parallel()
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	mirrorDir := addBareRemote(t, dir, "mirror")

	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"mirror"}))

	// straight to creating the branch on mirror: no rebase prompt, no picker.
	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No remote branch")
	assert.Equal(t, len(stub.selectCalls), 0)
	assert.ContainsString(t, temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch), "feat: local commit")
	assert.Equal(t, upstreamOf(t, dir, branch), "mirror/"+branch)
	originLog := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(originLog, "feat: local commit"), "origin must not receive the push")
}

// an argument resolving to the upstream's own branch -- via the picker here,
// so the argument is visibly read -- is a plain `gg push`: the upstream flow
// runs.
func TestPushCommand_RemoteArg_NamingUpstreamIsPlainPush(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	bareDir := addBareRemote(t, dir, "origin")
	addBareRemote(t, dir, "mirror")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"orig"}))

	assert.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "remote tracking branch")
	assert.ContainsString(t, buf.String(), "Pushed to remote tracking branch")
	assert.ContainsString(t, temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch), "feat: local commit")
}

// the upstream is a differently-named branch (a PR base, say) and the user
// names its remote: push creates the branch there under its own name rather
// than pushing onto the upstream.
func TestPushCommand_RemoteArg_UpstreamRemoteUnderOwnName(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	bareDir := addBareRemote(t, dir, "origin")
	base := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", base)
	temp_repo.RunGit(t, dir, "checkout", "-b", "fix")
	temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=origin/"+base, "fix")
	temp_repo.CreateCommit(t, dir, "fix.txt", "x\n", "fix: local commit")

	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"origin"}))

	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No remote branch 'origin/fix'")
	assert.ContainsString(t, temp_repo.RunGit(t, bareDir, "log", "--format=%s", "fix"), "fix: local commit")
	baseLog := temp_repo.RunGit(t, bareDir, "log", "--format=%s", base)
	assert.That(t, !strings.Contains(baseLog, "fix: local commit"), "the upstream branch must not receive the push")
	assert.Equal(t, upstreamOf(t, dir, "fix"), "origin/fix")
}

// a name that isn't a remote seeds the picker, as it does with no upstream;
// the pick then overrides the upstream.
func TestPushCommand_RemoteArg_NotARemoteOpensPicker(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	mirrorDir := addBareRemote(t, dir, "mirror")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"mir"}))

	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, len(stub.selectCalls[0].Options), 2)
	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No remote branch")
	assert.ContainsString(t, temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch), "feat: local commit")
	assert.Equal(t, upstreamOf(t, dir, branch), "mirror/"+branch)
}

// declining the named remote offers the others, the upstream's remote included.
func TestPushCommand_RemoteArg_DeclineOffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	bareDir := addBareRemote(t, dir, "origin")
	mirrorDir := addBareRemote(t, dir, "mirror")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	// decline creating the branch on mirror, pick origin, push to its branch.
	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"mirror"}))

	require.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.selectCalls[0].Options), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "origin")
	require.Equal(t, len(stub.confirmCalls), 2)
	assert.ContainsString(t, stub.confirmCalls[1].Prompt, "already exists")
	assert.ContainsString(t, temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch), "feat: local commit")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, mirrorDir, "branch", "--list", branch)), "")
}

// Escape on that picker cancels and leaves the upstream alone.
func TestPushCommand_RemoteArg_PickerEscapeCancels(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)

	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	assert.ErrorIs(t, cmd.Execute([]string{"nope"}), ui.ErrCancelled)
	assert.Equal(t, upstreamOf(t, dir, branch), "origin/"+branch)
}

// pushRepoTrackingBase builds a local repo whose branch "fix" tracks
// origin/<base> -- another name, as a branch cut from a PR base does.
// origin/<base> has moved on since fix was cut, and fix has a commit of its
// own. Returns (localDir, bareDir, base).
func pushRepoTrackingBase(t *testing.T) (dir, bareDir, base string) {
	t.Helper()
	dir, bareDir, base = pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.RunGit(t, dir, "checkout", "-b", "fix")
	temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=origin/"+base, "fix")
	temp_repo.CreateCommit(t, dir, "fix.txt", "x\n", "fix: local commit")
	return dir, bareDir, base
}

// the branch tracks a differently-named upstream, so git has no push
// destination for it: push says so and asks where to go -- every remote
// offered, the upstream's included -- rather than comparing against the base.
func TestPushCommand_NoPushTarget_AsksWhereTo(t *testing.T) {
	t.Parallel()
	dir, bareDir, base := pushRepoTrackingBase(t)
	forkDir := addBareRemote(t, dir, "fork")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"fork"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, buf.String(), "no push destination")
	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, len(stub.selectCalls[0].Options), 2)
	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No remote branch 'fork/fix'")
	assert.ContainsString(t, temp_repo.RunGit(t, forkDir, "log", "--format=%s", "fix"), "fix: local commit")
	assert.Equal(t, upstreamOf(t, dir, "fix"), "fork/fix")
	baseLog := temp_repo.RunGit(t, bareDir, "log", "--format=%s", base)
	assert.That(t, !strings.Contains(baseLog, "fix: local commit"), "the base must not receive the push")
}

// same setup, picking the upstream's own remote: the branch goes there under
// its own name, not onto the base.
func TestPushCommand_NoPushTarget_UpstreamRemoteUnderOwnName(t *testing.T) {
	t.Parallel()
	dir, bareDir, base := pushRepoTrackingBase(t)

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.selectCalls), 1)
	assert.ContainsString(t, temp_repo.RunGit(t, bareDir, "log", "--format=%s", "fix"), "fix: local commit")
	baseLog := temp_repo.RunGit(t, bareDir, "log", "--format=%s", base)
	assert.That(t, !strings.Contains(baseLog, "fix: local commit"), "the base must not receive the push")
	assert.Equal(t, upstreamOf(t, dir, "fix"), "origin/fix")
}

// Escape on that picker cancels and leaves the upstream alone.
func TestPushCommand_NoPushTarget_EscapeCancels(t *testing.T) {
	t.Parallel()
	dir, _, base := pushRepoTrackingBase(t)

	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled)
	assert.Equal(t, upstreamOf(t, dir, "fix"), "origin/"+base)
}

// declining to create the branch on the picked remote offers the others, the
// upstream's included.
func TestPushCommand_NoPushTarget_DeclineOffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir, bareDir, _ := pushRepoTrackingBase(t)
	forkDir := addBareRemote(t, dir, "fork")

	// pick fork, decline creating the branch there, pick origin, create it there.
	stub := &stubSelector{confirmAnswers: []bool{false, true}, selectAnswers: []string{"fork", "origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.selectCalls), 2)
	require.Equal(t, len(stub.selectCalls[1].Options), 1)
	assert.Equal(t, stub.selectCalls[1].Options[0], "origin")
	assert.ContainsString(t, temp_repo.RunGit(t, bareDir, "log", "--format=%s", "fix"), "fix: local commit")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, forkDir, "branch", "--list", "fix")), "")
	assert.Equal(t, upstreamOf(t, dir, "fix"), "origin/fix")
}

// push.default=nothing leaves git no destination even for a same-name
// upstream: push asks where to go.
func TestPushCommand_NoPushTarget_PushDefaultNothing(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	bareDir := addBareRemote(t, dir, "origin")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.RunGit(t, dir, "config", "push.default", "nothing")
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, buf.String(), "no push destination")
	require.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "already exists")
	assert.ContainsString(t, temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch), "feat: local commit")
}

// a branch tracking a local branch has no remote destination either: push
// asks, and the local branch it tracks is left where it was.
func TestPushCommand_NoPushTarget_LocalUpstream(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	bareDir := addBareRemote(t, dir, "origin")
	base := currentBranchIn(t, dir)
	baseTip := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", base))
	temp_repo.RunGit(t, dir, "checkout", "-b", "feature", "--track", base)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.selectCalls), 1)
	assert.ContainsString(t, temp_repo.RunGit(t, bareDir, "log", "--format=%s", "feature"), "feat: local commit")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", base)), baseTip)
}

// the picker lists the remotes most local branches track first -- where a
// user usually pushes -- so Enter lands on that one.
func TestPushCommand_PickerListsMostTrackedRemoteFirst(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "alpha")
	addBareRemote(t, dir, "zeta")
	base := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "zeta", base)
	temp_repo.RunGit(t, dir, "checkout", "-b", "other")
	temp_repo.RunGit(t, dir, "push", "-u", "zeta", "other")
	temp_repo.RunGit(t, dir, "checkout", "-b", "feature")

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"zeta"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.selectCalls[0].Options), 2)
	assert.Equal(t, stub.selectCalls[0].Options[0], "zeta")
	assert.Equal(t, stub.selectCalls[0].Options[1], "alpha")
}

// the other-remotes offer is ranked the same way.
func TestPushCommand_OfferListsMostTrackedRemoteFirst(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	addBareRemote(t, dir, "alpha")
	addBareRemote(t, dir, "zeta")
	base := currentBranchIn(t, dir)
	for _, branch := range []string{"b1", "b2"} {
		temp_repo.RunGit(t, dir, "branch", branch)
		temp_repo.RunGit(t, dir, "push", "-u", "zeta", branch)
	}
	temp_repo.RunGit(t, dir, "push", "-u", "origin", base)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")

	// decline origin, then back out of the offer: only its order matters here.
	stub := &stubSelector{confirmAnswers: []bool{false}, selectErrs: []error{ui.ErrCancelled}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled)
	require.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.selectCalls[0].Options), 2)
	assert.Equal(t, stub.selectCalls[0].Options[0], "zeta")
	assert.Equal(t, stub.selectCalls[0].Options[1], "alpha")
}

// the upstream's remote can't be reached: push says so and offers the other
// remotes rather than failing outright.
func TestPushCommand_UnreachableUpstream_OffersOtherRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	mirrorDir := addBareRemote(t, dir, "mirror")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")
	temp_repo.RunGit(t, dir, "remote", "set-url", "origin", t.TempDir()+"/gone.git")

	var errBuf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"mirror"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &errBuf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, errBuf.String(), "could not reach remote 'origin'")
	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "mirror")
	assert.ContainsString(t, temp_repo.RunGit(t, mirrorDir, "log", "--format=%s", branch), "feat: local commit")
}

// the same with no other remote: the unreachable upstream is the error.
func TestPushCommand_UnreachableUpstream_SingleRemoteErrors(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.RunGit(t, dir, "remote", "set-url", "origin", t.TempDir()+"/gone.git")

	stub := &stubSelector{}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "an unreachable upstream with nowhere else to go is an error")
	assert.ContainsString(t, err.Error(), "could not reach remote 'origin'")
	assert.Equal(t, len(stub.confirmCalls), 0)
}

// a named remote that can't be reached rules itself out: the rest are offered.
func TestPushCommand_UnreachablePickedRemote_OffersTheRest(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	originDir := addBareRemote(t, dir, "origin")
	temp_repo.RunGit(t, dir, "remote", "add", "gone", t.TempDir()+"/gone.git")
	branch := currentBranchIn(t, dir)

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{"origin"}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute([]string{"gone"}))

	require.Equal(t, len(stub.selectCalls), 1)
	require.Equal(t, len(stub.selectCalls[0].Options), 1)
	assert.Equal(t, stub.selectCalls[0].Options[0], "origin")
	assert.NotEqual(t, strings.TrimSpace(temp_repo.RunGit(t, originDir, "branch", "--list", branch)), "")
}

// the remote has the branch but this repo never fetched it: push reads the tip
// from the remote rather than failing on the missing tracking ref.
func TestPushCommand_UnfetchedRemoteBranch(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "branch", "--unset-upstream")
	temp_repo.RunGit(t, local, "update-ref", "-d", "refs/remotes/origin/main")

	var buf strings.Builder
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: &stubSelector{}, Repo: git.Repo{Dir: local}}}

	require.NoError(t, cmd.Execute([]string{"origin"}))
	assert.ContainsString(t, buf.String(), "No changes to push")
	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
}

// with the remote unchanged, a push asks it one question and fetches nothing.
func TestPushCommand_ProbesOnlyTheBranch(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)
	temp_repo.CreateCommit(t, dir, "feature.yml", "x\n", "feat: local commit")
	calls := gitShim(t)

	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, countCalls(calls(), "ls-remote"), 1)
	assert.Equal(t, countCalls(calls(), "fetch"), 0)
	assert.Equal(t, countCalls(calls(), "push"), 1)
}

// the remote moved on: that one branch is fetched, by its own refspec rather
// than the whole remote.
func TestPushCommand_FetchesOnlyTheBranch(t *testing.T) {
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	calls := gitShim(t)

	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, countCalls(calls(), "ls-remote"), 1)
	assert.Equal(t, countCalls(calls(), "fetch"), 1)
	assert.Equal(t, countExact(calls(), "fetch", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch), 1)
}

// a detached HEAD has no branch to push: push says so before asking anything.
func TestPushCommand_DetachedHeadErrors(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	addBareRemote(t, dir, "origin")
	temp_repo.RunGit(t, dir, "checkout", "-q", "--detach")

	stub := &stubSelector{}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "a detached HEAD has nothing to push")
	assert.ContainsString(t, err.Error(), "detached")
	assert.Equal(t, len(stub.selectCalls), 0)
	assert.Equal(t, len(stub.confirmCalls), 0)
}

// the rebase applied, but the push after it failed and the remote still has
// what it had: the rebase is undone, leaving the branch where it started.
func TestPushCommand_Diverged_PushFailsUndoesRebase(t *testing.T) {
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	before := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))
	failGitSubcommand(t, "push")

	var errBuf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &errBuf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "the push failed")
	assert.ContainsString(t, errBuf.String(), "Undid the rebase")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch)), before)
}

// the push reported an error after the remote took it: nothing is undone.
func TestPushCommand_Diverged_PushLandedDespiteError(t *testing.T) {
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	shimGitSubcommand(t, "push", "\"$git\" \"$@\"; exit 1")

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	assert.ContainsString(t, buf.String(), "the remote has it")
	local := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, bareDir, "rev-parse", branch)), local)
}

// the push failed and the remote can't be asked whether it landed: the rebase
// stays -- right either way -- and push says how to undo it.
func TestPushCommand_Diverged_PushOutcomeUnknownKeepsRebase(t *testing.T) {
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	before := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))
	shimGitSubcommand(t, "push", "rm -rf '"+bareDir+"'; exit 128")

	var errBuf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &errBuf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "the push failed")
	assert.ContainsString(t, errBuf.String(), "git reset --keep "+before)
	assert.NotEqual(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch)), before)
}

// the in-memory check passed but the rebase stops part-way anyway -- an
// intermediate commit clashes though the tips merge cleanly: it's aborted,
// leaving the branch as it was rather than mid-rebase.
func TestPushCommand_Diverged_RebaseStopsIsAborted(t *testing.T) {
	t.Parallel()
	dir, _, branch := pushRepoWithRemoteAhead(t, "clash.txt", "remote version\n")
	temp_repo.CreateCommit(t, dir, "clash.txt", "local version\n", "feat: add clash")
	temp_repo.RunGit(t, dir, "rm", "-q", "clash.txt")
	temp_repo.RunGit(t, dir, "commit", "-q", "-m", "feat: drop clash")
	before := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))

	var errBuf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &errBuf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "the rebase stopped")
	assert.ContainsString(t, errBuf.String(), "aborted it")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch)), before)
	_, inProgress := git.Repo{Dir: dir}.InProgress()
	assert.That(t, !inProgress, "no rebase left in progress")
}

// Ctrl-C arriving once the rebase is done but before the push: the rebase is
// undone and push exits as cancelled.
func TestPushCommand_Diverged_InterruptAfterRebaseUndoesIt(t *testing.T) {
	dir, bareDir, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	before := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))
	shimGitSubcommand(t, "rebase", "\"$git\" \"$@\"; kill -INT $PPID; sleep 0.2; exit 0")

	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled)
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch)), before)
	originLog := temp_repo.RunGit(t, bareDir, "log", "--format=%s", branch)
	assert.That(t, !strings.Contains(originLog, "feat: local commit"), "nothing may reach the remote")
}

// Ctrl-C during the push itself: git stops, the remote is unchanged, so the
// rebase is undone and push exits as cancelled.
func TestPushCommand_Diverged_InterruptDuringPushUndoesRebase(t *testing.T) {
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	temp_repo.CreateCommit(t, dir, "local.txt", "local change\n", "feat: local commit")
	before := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))
	shimGitSubcommand(t, "push", "kill -INT $PPID; sleep 0.2; exit 130")

	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled)
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch)), before)
}

// Ctrl-C during the fast-forward of a branch that is behind, with changes
// stashed for it: push exits as cancelled and the changes come back.
func TestPushCommand_Behind_InterruptRestoresStash(t *testing.T) {
	dir, _, branch := pushRepoWithRemoteAhead(t, "remote.txt", "remote change\n")
	before := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch))
	temp_repo.WriteFile(t, dir, "README.md", "# test repo\nedited\n")
	shimGitSubcommand(t, "merge", "kill -INT $PPID; sleep 0.2; exit 130")

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{dirtyStashOption("push")}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Err: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled)
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", branch)), before)
	assert.ContainsString(t, temp_repo.RunGit(t, dir, "diff", "README.md"), "+edited")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "stash", "list")), "")
}
