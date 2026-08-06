package git_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// Each subtest gets its own tmpdir Repo and runs t.Parallel(). The Repo type
// makes the dir explicit, removing the process-cwd dependency that previously
// serialised these tests.

func TestCheckInRepo(t *testing.T) {
	t.Parallel()
	r := git.Repo{Dir: temp_repo.NewRepo(t)}
	require.NoError(t, r.CheckInRepo())
}

func TestGetLocalBranches(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")
	branches, err := git.Repo{Dir: dir}.GetLocalBranches()
	require.NoError(t, err)
	assert.That(t, slices.Contains(branches, "feature"), "feature branch present")
}

func TestGetRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://example.com/repo.git")
	remotes, err := git.Repo{Dir: dir}.GetRemotes()
	require.NoError(t, err)
	assert.That(t, slices.Contains(remotes, "origin"), "origin remote present")
}

func TestBranchExists(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "dev")
	r := git.Repo{Dir: dir}
	assert.That(t, r.BranchExists("dev"), "dev branch exists")
	assert.That(t, !r.BranchExists("missing"), "missing branch absent")
}

func TestGetCommitHash(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	hash, err := git.Repo{Dir: dir}.GetCommitHash("HEAD")
	require.NoError(t, err)
	assert.That(t, len(hash) >= 7, "hash length plausible")
}

func TestGetFileStatus(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		setup func(t *testing.T, dir string) string // returns target file
		want  git.FileStatus
	}{
		"untracked": {
			setup: func(t *testing.T, dir string) string {
				temp_repo.WriteFile(t, dir, "untracked.txt", "content")
				return "untracked.txt"
			},
			want: git.FileUntracked,
		},
		"modified": {
			setup: func(t *testing.T, dir string) string {
				require.NoError(t, appendFile(dir+"/README.md", "\nmodified content"))
				return "README.md"
			},
			want: git.FileModified,
		},
		"staged": {
			setup: func(t *testing.T, dir string) string {
				temp_repo.WriteFile(t, dir, "staged.txt", "content")
				temp_repo.RunGit(t, dir, "add", "staged.txt")
				return "staged.txt"
			},
			want: git.FileStaged,
		},
		"deleted": {
			setup: func(t *testing.T, dir string) string {
				temp_repo.WriteFile(t, dir, "deleted.txt", "content")
				temp_repo.RunGit(t, dir, "add", "deleted.txt")
				temp_repo.RunGit(t, dir, "commit", "-m", "chore: add file")
				require.NoError(t, os.Remove(dir+"/deleted.txt"))
				temp_repo.RunGit(t, dir, "rm", "deleted.txt")
				return "deleted.txt"
			},
			want: git.FileDeleted,
		},
		"clean": {
			setup: func(t *testing.T, dir string) string {
				temp_repo.WriteFile(t, dir, "clean.txt", "content")
				temp_repo.RunGit(t, dir, "add", "clean.txt")
				temp_repo.RunGit(t, dir, "commit", "-m", "chore: add file")
				return "clean.txt"
			},
			want: git.FileUnknown,
		},
		"nonexistent": {
			setup: func(t *testing.T, dir string) string { return "nonexistent.txt" },
			want:  git.FileUnknown,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := temp_repo.NewRepo(t)
			file := c.setup(t, dir)
			status, err := git.Repo{Dir: dir}.GetFileStatus(file)
			require.NoError(t, err)
			assert.Equal(t, c.want, status)
		})
	}
}

func TestGetBranchUpstream(t *testing.T) {
	t.Parallel()

	t.Run("no upstream", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "branch", "no-upstream")
		remote, remoteBranch, err := git.Repo{Dir: dir}.GetBranchUpstream("no-upstream")
		require.NoError(t, err)
		assert.Equal(t, "", remote)
		assert.Equal(t, "", remoteBranch)
	})

	t.Run("tracked branch", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "origin", dir)
		temp_repo.RunGit(t, dir, "fetch", "origin")
		temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
		remote, remoteBranch, err := git.Repo{Dir: dir}.GetBranchUpstream("main")
		require.NoError(t, err)
		assert.Equal(t, "origin", remote)
		assert.Equal(t, "main", remoteBranch)
	})

	// Regression: when the remote-tracking ref is pruned but branch.<x>.remote
	// config remains (shown as "[origin/x: gone]" in `git branch -vv`),
	// `rev-parse --abbrev-ref x@{u}` exits non-zero. That used to break
	// `gg switch` with "getting tracking remote: exit status 128".
	t.Run("gone upstream", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "origin", dir)
		temp_repo.RunGit(t, dir, "fetch", "origin")
		temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
		temp_repo.RunGit(t, dir, "update-ref", "-d", "refs/remotes/origin/main")
		remote, remoteBranch, err := git.Repo{Dir: dir}.GetBranchUpstream("main")
		require.NoError(t, err)
		assert.Equal(t, "origin", remote)
		assert.Equal(t, "main", remoteBranch)
	})
}

func TestGetCurrentBranchUpstream(t *testing.T) {
	t.Parallel()

	t.Run("no upstream", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "branch", "no-upstream")
		temp_repo.RunGit(t, dir, "checkout", "no-upstream")
		upstream, err := git.Repo{Dir: dir}.GetCurrentBranchUpstream()
		require.NoError(t, err)
		assert.Equal(t, "", upstream)
	})

	t.Run("set", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "origin", dir)
		temp_repo.RunGit(t, dir, "fetch", "origin")
		temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
		upstream, err := git.Repo{Dir: dir}.GetCurrentBranchUpstream()
		require.NoError(t, err)
		assert.Equal(t, "origin/main", upstream)
	})

	// A --single-branch clone maps only one branch in its fetch refspec, so any
	// other branch's upstream has no remote-tracking ref and `rev-parse @{u}`
	// fatals. The configured name is still the right answer.
	t.Run("configured but not stored as a remote-tracking branch", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.RunGit(t, local, "checkout", "-b", "feature")
		temp_repo.RunGit(t, local, "config", "branch.feature.remote", "origin")
		temp_repo.RunGit(t, local, "config", "branch.feature.merge", "refs/heads/feature")
		temp_repo.RunGit(t, local, "config", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")

		upstream, err := git.Repo{Dir: local}.GetCurrentBranchUpstream()
		require.NoError(t, err)
		assert.Equal(t, "origin/feature", upstream)
	})

	// Same missing ref, wider refspec: git maps the branch but has never fetched
	// it (or it was pruned), so @{u} fatals with "unknown revision" instead. Still
	// an upstream.
	t.Run("configured but tracking ref absent", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "origin", dir)
		temp_repo.RunGit(t, dir, "fetch", "origin")
		temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
		temp_repo.RunGit(t, dir, "update-ref", "-d", "refs/remotes/origin/main")

		upstream, err := git.Repo{Dir: dir}.GetCurrentBranchUpstream()
		require.NoError(t, err)
		assert.Equal(t, "origin/main", upstream)
	})
}

func TestRefExists(t *testing.T) {
	t.Parallel()

	dir := temp_repo.NewRepo(t)
	assert.That(t, git.Repo{Dir: dir}.RefExists("HEAD"), "HEAD should resolve")
	assert.That(t, !git.Repo{Dir: dir}.RefExists("origin/nope"), "missing ref should not resolve")
}

// git refuses to check a branch out twice, so which worktree holds one is what
// the pickers need to mark it unselectable.
func TestLocalBranches_WorktreePath(t *testing.T) {
	t.Parallel()

	t.Run("main worktree only", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		current := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))

		branches, err := git.Repo{Dir: dir}.LocalBranches()
		require.NoError(t, err)
		held := heldBy(branches)
		assert.That(t, held[current] != "", "current branch should name a worktree")
		assert.That(t, held["not-a-branch"] == "", "absent branch should name none")
	})

	t.Run("with linked worktree", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		current := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
		temp_repo.RunGit(t, dir, "branch", "feature")
		temp_repo.RunGit(t, dir, "worktree", "add", t.TempDir(), "feature")

		branches, err := git.Repo{Dir: dir}.LocalBranches()
		require.NoError(t, err)
		held := heldBy(branches)
		assert.That(t, held[current] != "", "main worktree branch named")
		assert.That(t, held["feature"] != "", "linked worktree branch maps to its path")
	})

	t.Run("branch nothing has checked out", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "branch", "idle")

		branches, err := git.Repo{Dir: dir}.LocalBranches()
		require.NoError(t, err)
		assert.That(t, heldBy(branches)["idle"] == "", "an unchecked-out branch names no worktree")
	})
}

func heldBy(branches []git.LocalBranch) map[string]string {
	out := make(map[string]string, len(branches))
	for _, b := range branches {
		out[b.Name] = b.WorktreePath
	}
	return out
}

func appendFile(path, s string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(s)
	return err
}
