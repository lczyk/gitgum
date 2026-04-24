package git_test

import (
	"os"
	"slices"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestGitFunctions(t *testing.T) {
	t.Run("CheckInRepo succeeds", func(t *testing.T) {
		temp_repo.InitTempRepo(t)
		err := git.CheckInRepo()
		assert.NoError(t, err, "should be inside git repo")
	})

	t.Run("GetLocalBranches lists created branch", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.RunGit(t, repo, "branch", "feature")
		branches, err := git.GetLocalBranches()
		assert.NoError(t, err, "list branches")
		assert.That(t, slices.Contains(branches, "feature"), "feature branch present")
	})

	t.Run("GetRemotes lists origin", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.RunGit(t, repo, "remote", "add", "origin", "https://example.com/repo.git")
		remotes, err := git.GetRemotes()
		assert.NoError(t, err, "list remotes")
		assert.That(t, slices.Contains(remotes, "origin"), "origin remote present")
	})

	t.Run("BranchExists detects branch", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.RunGit(t, repo, "branch", "dev")
		assert.That(t, git.BranchExists("dev"), "dev branch exists")
		assert.That(t, !git.BranchExists("missing"), "missing branch not exists")
	})

	t.Run("GetCommitHash for HEAD returns hash", func(t *testing.T) {
		temp_repo.InitTempRepo(t)
		hash, err := git.GetCommitHash("HEAD")
		assert.NoError(t, err, "get hash")
		assert.That(t, len(hash) >= 7, "hash length plausible")
	})

	t.Run("IsDirty true after modification", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		assert.NoError(t, appendFile(repo+"/README.md", "extra"), "append to README")
		dirty, derr := git.IsDirty(repo)
		assert.NoError(t, derr, "check dirty state")
		assert.That(t, dirty, "repository should be dirty after modification")
	})

	t.Run("GetFileStatus detects untracked file", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.WriteFile(t, repo, "untracked.txt", "content")
		status, err := git.GetFileStatus("untracked.txt")
		assert.NoError(t, err, "get status")
		assert.Equal(t, git.FileUntracked, status)
	})

	t.Run("GetFileStatus detects modified file", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		assert.NoError(t, appendFile(repo+"/README.md", "\nmodified content"), "append to README")
		status, err := git.GetFileStatus("README.md")
		assert.NoError(t, err, "get status")
		assert.Equal(t, git.FileModified, status)
	})

	t.Run("GetFileStatus detects staged file", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.WriteFile(t, repo, "staged.txt", "content")
		cmdrun.Run("git", "add", "staged.txt")
		status, err := git.GetFileStatus("staged.txt")
		assert.NoError(t, err, "get status")
		assert.Equal(t, git.FileStaged, status)
	})

	t.Run("GetFileStatus detects deleted file", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.WriteFile(t, repo, "deleted.txt", "content")
		cmdrun.Run("git", "add", "deleted.txt")
		cmdrun.Run("git", "commit", "-m", "add file")
		os.Remove(repo + "/deleted.txt")
		cmdrun.Run("git", "rm", "deleted.txt")
		status, err := git.GetFileStatus("deleted.txt")
		assert.NoError(t, err, "get status")
		assert.Equal(t, git.FileDeleted, status)
	})

	t.Run("GetFileStatus returns unknown for clean file", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.WriteFile(t, repo, "clean.txt", "content")
		cmdrun.Run("git", "add", "clean.txt")
		cmdrun.Run("git", "commit", "-m", "add file")
		status, err := git.GetFileStatus("clean.txt")
		assert.NoError(t, err, "get status")
		assert.Equal(t, git.FileUnknown, status)
	})

	t.Run("GetFileStatus returns unknown for nonexistent file", func(t *testing.T) {
		temp_repo.InitTempRepo(t)
		status, _ := git.GetFileStatus("nonexistent.txt")
		assert.Equal(t, git.FileUnknown, status)
	})

	t.Run("GetBranchUpstream returns empty for branch with no upstream", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.RunGit(t, repo, "branch", "no-upstream")
		remote, remoteBranch, err := git.GetBranchUpstream("no-upstream")
		assert.NoError(t, err, "no upstream is not an error")
		assert.Equal(t, "", remote)
		assert.Equal(t, "", remoteBranch)
	})

	t.Run("GetBranchUpstream returns remote and branch for tracked branch", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.RunGit(t, repo, "remote", "add", "origin", repo)
		temp_repo.RunGit(t, repo, "fetch", "origin")
		temp_repo.RunGit(t, repo, "branch", "--set-upstream-to=origin/main", "main")
		remote, remoteBranch, err := git.GetBranchUpstream("main")
		assert.NoError(t, err, "get upstream")
		assert.Equal(t, "origin", remote)
		assert.Equal(t, "main", remoteBranch)
	})

	t.Run("GetCurrentBranchUpstream returns empty for branch with no upstream", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.RunGit(t, repo, "branch", "no-upstream")
		temp_repo.RunGit(t, repo, "checkout", "no-upstream")
		upstream, err := git.GetCurrentBranchUpstream()
		assert.NoError(t, err, "no upstream is not an error")
		assert.Equal(t, "", upstream)
	})

	t.Run("GetCurrentBranchUpstream returns remote tracking branch when set", func(t *testing.T) {
		repo := temp_repo.InitTempRepo(t)
		temp_repo.RunGit(t, repo, "remote", "add", "origin", repo)
		temp_repo.RunGit(t, repo, "fetch", "origin")
		temp_repo.RunGit(t, repo, "branch", "--set-upstream-to=origin/main", "main")
		upstream, err := git.GetCurrentBranchUpstream()
		assert.NoError(t, err, "get upstream")
		assert.Equal(t, "origin/main", upstream)
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
