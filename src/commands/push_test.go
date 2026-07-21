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
