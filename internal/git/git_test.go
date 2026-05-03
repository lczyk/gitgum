package git_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// Each subtest gets its own tmpdir Repo and runs t.Parallel(). The Repo type
// makes the dir explicit, removing the process-cwd dependency that previously
// serialised these tests.

func TestCheckInRepo(t *testing.T) {
	t.Parallel()
	r := git.Repo{Dir: temp_repo.NewRepo(t)}
	assert.NoError(t, r.CheckInRepo())
}

func TestGetLocalBranches(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")
	branches, err := git.Repo{Dir: dir}.GetLocalBranches()
	assert.NoError(t, err)
	assert.That(t, slices.Contains(branches, "feature"), "feature branch present")
}

func TestGetRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://example.com/repo.git")
	remotes, err := git.Repo{Dir: dir}.GetRemotes()
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
				assert.NoError(t, appendFile(dir+"/README.md", "\nmodified content"))
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
				temp_repo.RunGit(t, dir, "commit", "-m", "add file")
				assert.NoError(t, os.Remove(dir+"/deleted.txt"))
				temp_repo.RunGit(t, dir, "rm", "deleted.txt")
				return "deleted.txt"
			},
			want: git.FileDeleted,
		},
		"clean": {
			setup: func(t *testing.T, dir string) string {
				temp_repo.WriteFile(t, dir, "clean.txt", "content")
				temp_repo.RunGit(t, dir, "add", "clean.txt")
				temp_repo.RunGit(t, dir, "commit", "-m", "add file")
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
			assert.NoError(t, err)
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
		assert.NoError(t, err)
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
		assert.NoError(t, err)
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
		assert.NoError(t, err)
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
		assert.NoError(t, err)
		assert.Equal(t, "", upstream)
	})

	t.Run("set", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "origin", dir)
		temp_repo.RunGit(t, dir, "fetch", "origin")
		temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
		upstream, err := git.Repo{Dir: dir}.GetCurrentBranchUpstream()
		assert.NoError(t, err)
		assert.Equal(t, "origin/main", upstream)
	})
}

func TestCheckedOutBranches(t *testing.T) {
	t.Parallel()

	t.Run("main worktree only", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		current := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))

		checked, err := git.Repo{Dir: dir}.CheckedOutBranches()
		assert.NoError(t, err)
		assert.That(t, checked[current], "current branch should be in set")
		assert.That(t, !checked["not-a-branch"], "absent branch should not be in set")
	})

	t.Run("with linked worktree", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		current := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
		temp_repo.RunGit(t, dir, "branch", "feature")
		wt := t.TempDir()
		temp_repo.RunGit(t, dir, "worktree", "add", wt, "feature")

		checked, err := git.Repo{Dir: dir}.CheckedOutBranches()
		assert.NoError(t, err)
		assert.That(t, checked[current], "main worktree branch in set")
		assert.That(t, checked["feature"], "linked worktree branch in set")
	})
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
