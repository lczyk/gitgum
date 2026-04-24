package internal_test

import (
	"os"
	"slices"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestGitFunctions(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "CheckInGitRepo succeeds",
			run: func(t *testing.T) {
				temp_repo.InitTempRepo(t)
				err := internal.CheckInGitRepo()
				assert.NoError(t, err, "should be inside git repo")
			},
		},
		{
			name: "GetLocalBranches lists created branch",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				temp_repo.CreateBranch(t, repo, "feature")
				branches, err := internal.GetLocalBranches()
				assert.NoError(t, err, "list branches")
				assert.That(t, slices.Contains(branches, "feature"), "feature branch present")
			},
		},
		{
			name: "GetRemotes lists origin",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				temp_repo.RunGit(t, repo, "remote", "add", "origin", "https://example.com/repo.git")
				remotes, err := internal.GetRemotes()
				assert.NoError(t, err, "list remotes")
				assert.That(t, slices.Contains(remotes, "origin"), "origin remote present")
			},
		},
		{
			name: "BranchExists detects branch",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				temp_repo.CreateBranch(t, repo, "dev")
				assert.That(t, internal.BranchExists("dev"), "dev branch exists")
				assert.That(t, !internal.BranchExists("missing"), "missing branch not exists")
			},
		},
		{
			name: "GetCommitHash for HEAD returns hash",
			run: func(t *testing.T) {
				temp_repo.InitTempRepo(t)
				hash, err := internal.GetCommitHash("HEAD")
				assert.NoError(t, err, "get hash")
				assert.That(t, len(hash) >= 7, "hash length plausible")
			},
		},
		{
			name: "SplitLines trims and splits",
			run: func(t *testing.T) {
				input := "a\n\n b \n c  "
				out := internal.SplitLines(input)
				assert.That(t, len(out) == 3, "three non-empty lines")
				assert.That(t, out[1] == "b", "second line trimmed")
			},
		},
		{
			name: "IsGitDirty true after modification",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				assert.NoError(t, appendFile(repo+"/README.md", "extra"), "append to README")
				dirty, derr := internal.IsGitDirty(repo)
				assert.NoError(t, derr, "check dirty state")
				assert.That(t, dirty, "repository should be dirty after modification")
			},
		},
		{
			name: "GetGitFileStatus detects untracked file",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				os.WriteFile(repo+"/untracked.txt", []byte("content"), 0644)
				status, err := internal.GetGitFileStatus("untracked.txt")
				assert.NoError(t, err, "get status")
				assert.Equal(t, internal.GitFileUntracked, status)
			},
		},
		{
			name: "GetGitFileStatus detects modified file",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				assert.NoError(t, appendFile(repo+"/README.md", "\nmodified content"), "append to README")
				status, err := internal.GetGitFileStatus("README.md")
				assert.NoError(t, err, "get status")
				assert.Equal(t, internal.GitFileModified, status)
			},
		},
		{
			name: "GetGitFileStatus detects staged file",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				os.WriteFile(repo+"/staged.txt", []byte("content"), 0644)
				internal.RunCommand("git", "add", "staged.txt")
				status, err := internal.GetGitFileStatus("staged.txt")
				assert.NoError(t, err, "get status")
				assert.Equal(t, internal.GitFileStaged, status)
			},
		},
		{
			name: "GetGitFileStatus detects deleted file",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				os.WriteFile(repo+"/deleted.txt", []byte("content"), 0644)
				internal.RunCommand("git", "add", "deleted.txt")
				internal.RunCommand("git", "commit", "-m", "add file")
				os.Remove(repo + "/deleted.txt")
				internal.RunCommand("git", "rm", "deleted.txt")
				status, err := internal.GetGitFileStatus("deleted.txt")
				assert.NoError(t, err, "get status")
				assert.Equal(t, internal.GitFileDeleted, status)
			},
		},
		{
			name: "GetGitFileStatus returns unknown for clean file",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				os.WriteFile(repo+"/clean.txt", []byte("content"), 0644)
				internal.RunCommand("git", "add", "clean.txt")
				internal.RunCommand("git", "commit", "-m", "add file")
				status, err := internal.GetGitFileStatus("clean.txt")
				assert.NoError(t, err, "get status")
				assert.Equal(t, internal.GitFileUnknown, status)
			},
		},
		{
			name: "GetGitFileStatus returns unknown for nonexistent file",
			run: func(t *testing.T) {
				temp_repo.InitTempRepo(t)
				status, _ := internal.GetGitFileStatus("nonexistent.txt")
				assert.Equal(t, internal.GitFileUnknown, status)
			},
		},
		{
			name: "GetBranchUpstream returns empty for branch with no upstream",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				temp_repo.CreateBranch(t, repo, "no-upstream")
				remote, remoteBranch, err := internal.GetBranchUpstream("no-upstream")
				assert.NoError(t, err, "no upstream is not an error")
				assert.Equal(t, "", remote)
				assert.Equal(t, "", remoteBranch)
			},
		},
		{
			name: "GetBranchUpstream returns remote and branch for tracked branch",
			run: func(t *testing.T) {
				repo := temp_repo.InitTempRepo(t)
				temp_repo.RunGit(t, repo, "remote", "add", "origin", repo) // point at self
				temp_repo.RunGit(t, repo, "fetch", "origin")
				temp_repo.RunGit(t, repo, "branch", "--set-upstream-to=origin/main", "main")
				remote, remoteBranch, err := internal.GetBranchUpstream("main")
				assert.NoError(t, err, "get upstream")
				assert.Equal(t, "origin", remote)
				assert.Equal(t, "main", remoteBranch)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
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
