package commands

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestListCommits(t *testing.T) {
	type testCase struct {
		setup         func(t *testing.T, dir string) (branchA, branchB string)
		expectError   bool
		errorContains string
		verifyCommits func(t *testing.T, commits []string)
	}

	cases := map[string]testCase{
		"list commits on feature branch since trunk": {
			setup: func(t *testing.T, dir string) (string, string) {
				temp_repo.RunGit(t, dir, "checkout", "-b", "feature")

				for i := 1; i <= 3; i++ {
					temp_repo.CreateCommit(t, dir,
						fmt.Sprintf("file%d.txt", i), "content\n",
						fmt.Sprintf("Commit %d", i))
				}

				return "feature", "main"
			},
			verifyCommits: func(t *testing.T, commits []string) {
				assert.Len(t, commits, 3, "should have 3 commits")
				for _, hash := range commits {
					assert.Len(t, hash, 40, "commit hash should be 40 chars (SHA-1)")
				}
			},
		},
		"empty list when branches are at same commit": {
			setup: func(t *testing.T, dir string) (string, string) {
				temp_repo.RunGit(t, dir, "checkout", "-b", "feature2")
				return "feature2", "main"
			},
			verifyCommits: func(t *testing.T, commits []string) {
				assert.Len(t, commits, 0, "should have no commits")
			},
		},
		"error when branch A doesn't exist": {
			setup: func(t *testing.T, dir string) (string, string) {
				return "nonexistent", "main"
			},
			expectError:   true,
			errorContains: "merge base",
		},
		"error when branch B doesn't exist": {
			setup: func(t *testing.T, dir string) (string, string) {
				return "main", "nonexistent"
			},
			expectError:   true,
			errorContains: "merge base",
		},
	}

	// needs a closure so setup can capture SHA order for verifyCommits to check
	var wantSHAs []string
	cases["list commits in chronological order"] = testCase{
		setup: func(t *testing.T, dir string) (string, string) {
			temp_repo.RunGit(t, dir, "checkout", "-b", "feature3")
			for i := range 3 {
				temp_repo.CreateCommit(t, dir,
					fmt.Sprintf("ordered%c.txt", 'a'+i), "content\n",
					fmt.Sprintf("Ordered commit %c", 'A'+i))
				sha := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))
				wantSHAs = append(wantSHAs, sha)
			}
			return "feature3", "main"
		},
		verifyCommits: func(t *testing.T, commits []string) {
			assert.EqualArrays(t, commits, wantSHAs)
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			dir := temp_repo.InitTempRepo(t)

			branchA, branchB := tt.setup(t, dir)

			commits, err := listCommits(branchA, branchB)

			if tt.expectError {
				assert.Error(t, err, assert.AnyError, "expected error")
				if tt.errorContains != "" {
					assert.ContainsString(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err, "should not error")
				if tt.verifyCommits != nil {
					tt.verifyCommits(t, commits)
				}
			}
		})
	}
}

func TestReplayListCommand_Execute(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)
	temp_repo.RunGit(t, dir, "checkout", "-b", "feature")
	temp_repo.CreateCommit(t, dir, "file.txt", "content\n", "Test commit")

	cmd := &ReplayListCommand{}
	cmd.Args.BranchA = "feature"
	cmd.Args.BranchB = "main"

	err := cmd.Execute(nil)
	assert.NoError(t, err, "Execute should not error")
}
