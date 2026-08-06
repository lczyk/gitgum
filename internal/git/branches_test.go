package git_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func find(t *testing.T, branches []git.LocalBranch, name string) git.LocalBranch {
	t.Helper()
	for _, b := range branches {
		if b.Name == name {
			return b
		}
	}
	t.Fatalf("branch %q not listed in %v", name, branches)
	return git.LocalBranch{}
}

func TestLocalBranches(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "fetch", "origin")
	temp_repo.RunGit(t, local, "branch", "tracked")
	temp_repo.RunGit(t, local, "branch", "--set-upstream-to=origin/main", "tracked")
	temp_repo.RunGit(t, local, "branch", "bare")

	branches, err := git.Repo{Dir: local}.LocalBranches()
	require.NoError(t, err, "listing local branches")

	tracked := find(t, branches, "tracked")
	assert.Equal(t, tracked.Upstream, "origin/main")
	assert.Equal(t, tracked.Remote(), "origin")
	assert.Equal(t, tracked.Gone, false)

	bare := find(t, branches, "bare")
	assert.Equal(t, bare.Upstream, "")
	assert.Equal(t, bare.Remote(), "")
}

// A branch tracking another *local* branch (branch.<name>.remote = ".") has an
// upstream with no remote half. It must not read as tracking a remote called
// after the upstream branch.
func TestLocalBranches_LocalUpstreamHasNoRemote(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feat")
	temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=main", "feat")

	branches, err := git.Repo{Dir: dir}.LocalBranches()
	require.NoError(t, err, "listing local branches")

	feat := find(t, branches, "feat")
	assert.Equal(t, feat.Upstream, "main")
	assert.Equal(t, feat.Remote(), "")
}

// An upstream that was pruned off the remote keeps its config, and git marks
// it "[gone]" -- the signal gg doctor reports on.
func TestLocalBranches_GoneUpstream(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "fetch", "origin")
	temp_repo.RunGit(t, local, "branch", "orphan")
	temp_repo.RunGit(t, local, "branch", "--set-upstream-to=origin/main", "orphan")
	temp_repo.RunGit(t, local, "branch", "plain")
	temp_repo.RunGit(t, local, "update-ref", "-d", "refs/remotes/origin/main")

	branches, err := git.Repo{Dir: local}.LocalBranches()
	require.NoError(t, err, "listing local branches")

	assert.Equal(t, find(t, branches, "orphan").Gone, true)
	// Tracking nothing is not the same as tracking something that vanished.
	assert.Equal(t, find(t, branches, "plain").Gone, false)
}

func TestRemoteBranches(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "remote", "add", "second", remote)
	temp_repo.RunGit(t, local, "fetch", "--all")

	byRemote, err := git.Repo{Dir: local}.RemoteBranches()
	require.NoError(t, err, "listing remote branches")

	assert.EqualArrays(t, byRemote["origin"], []string{"main"})
	assert.EqualArrays(t, byRemote["second"], []string{"main"})
	assert.Equal(t, len(byRemote["nope"]), 0)
}

// A branch name may contain '/'; a remote name may not, so only the first
// segment names the remote.
func TestRemoteBranches_SlashInBranchName(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "update-ref", "refs/remotes/origin/feat/x", "HEAD")

	byRemote, err := git.Repo{Dir: local}.RemoteBranches()
	require.NoError(t, err, "listing remote branches")

	assert.That(t, contains(byRemote["origin"], "feat/x"),
		"origin should list feat/x, got %v", byRemote["origin"])
}

// sameAsStatus is the whole contract of HeadLine: it must produce, from refs
// alone, the exact line `git status --branch` opens with.
func sameAsStatus(t *testing.T, dir string) {
	t.Helper()
	r := git.Repo{Dir: dir}
	want, _, err := r.Status(git.ScanOpts{Branch: true, Untracked: git.UntrackedNone})
	require.NoError(t, err, "reading status branch line")
	got, err := r.HeadLine()
	require.NoError(t, err, "reading head line")
	assert.Equal(t, got, want)
}

func TestHeadLine_MatchesStatus(t *testing.T) {
	t.Parallel()

	t.Run("no upstream", func(t *testing.T) {
		t.Parallel()
		sameAsStatus(t, temp_repo.NewRepo(t))
	})

	t.Run("unborn head", func(t *testing.T) {
		t.Parallel()
		sameAsStatus(t, temp_repo.NewEmptyRepo(t))
	})

	t.Run("detached head", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "checkout", "--detach")
		sameAsStatus(t, dir)
	})

	t.Run("local upstream", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "branch", "base")
		temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=base", "main")
		sameAsStatus(t, dir)
	})

	t.Run("in sync", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		sameAsStatus(t, local)
	})

	t.Run("ahead", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.CreateCommit(t, local, "a.txt", "a\n", "chore: a")
		temp_repo.CreateCommit(t, local, "b.txt", "b\n", "chore: b")
		sameAsStatus(t, local)
	})

	t.Run("behind", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.CreateCommit(t, local, "a.txt", "a\n", "chore: a")
		temp_repo.RunGit(t, local, "update-ref", "refs/remotes/origin/main", "HEAD")
		temp_repo.RunGit(t, local, "reset", "--hard", "HEAD~1")
		sameAsStatus(t, local)
	})

	t.Run("diverged", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.CreateCommit(t, local, "theirs.txt", "theirs\n", "chore: theirs")
		temp_repo.RunGit(t, local, "update-ref", "refs/remotes/origin/main", "HEAD")
		temp_repo.RunGit(t, local, "reset", "--hard", "HEAD~1")
		temp_repo.CreateCommit(t, local, "mine.txt", "mine\n", "chore: mine")
		temp_repo.CreateCommit(t, local, "mine2.txt", "mine2\n", "chore: mine two")
		sameAsStatus(t, local)
	})

	t.Run("gone upstream", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.RunGit(t, local, "update-ref", "-d", "refs/remotes/origin/main")
		sameAsStatus(t, local)
	})
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
