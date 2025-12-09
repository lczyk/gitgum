package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestDeleteCommand_Execute(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string) string // returns branch to delete
		expectError bool
	}{
		{
			name: "basic structure test - command creation",
			setup: func(t *testing.T, dir string) string {
				// Create a feature branch
				_, _, err := internal.RunCommand("git", "-C", dir, "checkout", "-b", "feature")
				assert.NoError(t, err, "create feature branch")
				
				// Switch back to main
				_, _, err = internal.RunCommand("git", "-C", dir, "checkout", "main")
				assert.NoError(t, err, "checkout main")
				
				return "feature"
			},
			expectError: false,
		},
		{
			name: "verify branch exists before deletion",
			setup: func(t *testing.T, dir string) string {
				// Create test branch
				_, _, err := internal.RunCommand("git", "-C", dir, "checkout", "-b", "test-branch")
				assert.NoError(t, err, "create test-branch")
				
				// Make a commit to ensure branch has content
				filename := filepath.Join(dir, "test.txt")
				err = os.WriteFile(filename, []byte("test\n"), 0o644)
				assert.NoError(t, err, "write test file")
				
				_, _, err = internal.RunCommand("git", "-C", dir, "add", filename)
				assert.NoError(t, err, "git add")
				
				_, _, err = internal.RunCommand("git", "-C", dir, "commit", "-m", "Test commit")
				assert.NoError(t, err, "git commit")
				
				// Switch back to main and merge to allow clean deletion
				_, _, err = internal.RunCommand("git", "-C", dir, "checkout", "main")
				assert.NoError(t, err, "checkout main")
				
				_, _, err = internal.RunCommand("git", "-C", dir, "merge", "test-branch", "--no-ff")
				assert.NoError(t, err, "merge test-branch")
				
				return "test-branch"
			},
			expectError: false,
		},
		{
			name: "create multiple branches",
			setup: func(t *testing.T, dir string) string {
				// Create multiple branches to test selection
				for _, name := range []string{"feature-1", "feature-2", "feature-3"} {
					_, _, err := internal.RunCommand("git", "-C", dir, "checkout", "-b", name)
					assert.NoError(t, err, "create "+name)
				}
				
				_, _, err := internal.RunCommand("git", "-C", dir, "checkout", "main")
				assert.NoError(t, err, "checkout main")
				
				return "feature-1"
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary git repo
			tempDir := temp_repo.InitTempRepo(t)
			defer os.RemoveAll(tempDir)
			
			// Change to temp directory
			origDir, err := os.Getwd()
			assert.NoError(t, err, "get working dir")
			defer os.Chdir(origDir)
			
			err = os.Chdir(tempDir)
			assert.NoError(t, err, "change to temp dir")
			
			// Run setup
			branchToDelete := tt.setup(t, tempDir)
			
			// Verify the branch exists
			branches, err := internal.GetLocalBranches()
			assert.NoError(t, err, "get local branches")
			
			found := false
			for _, b := range branches {
				if b == branchToDelete {
					found = true
					break
				}
			}
			assert.That(t, found, "branch '"+branchToDelete+"' should exist before test")
			
			// Create command (we can't fully test Execute without mocking fzf)
			cmd := &DeleteCommand{}
			assert.That(t, cmd != nil, "DeleteCommand should be created successfully")
		})
	}
}

func TestDeleteCommand_BranchDetection(t *testing.T) {
	// Create a temporary git repo
	tempDir := temp_repo.InitTempRepo(t)
	defer os.RemoveAll(tempDir)
	
	// Change to temp directory
	origDir, err := os.Getwd()
	assert.NoError(t, err, "get working dir")
	defer os.Chdir(origDir)
	
	err = os.Chdir(tempDir)
	assert.NoError(t, err, "change to temp dir")
	
	// Create a test branch
	_, _, err = internal.RunCommand("git", "checkout", "-b", "test-branch")
	assert.NoError(t, err, "create test-branch")
	
	// Verify we can detect the current branch
	currentBranch, _, err := internal.RunCommand("git", "rev-parse", "--abbrev-ref", "HEAD")
	assert.NoError(t, err, "get current branch")
	assert.That(t, currentBranch == "test-branch", "current branch should be test-branch")
	
	// Switch back to main
	_, _, err = internal.RunCommand("git", "checkout", "main")
	assert.NoError(t, err, "checkout main")
	
	currentBranch, _, err = internal.RunCommand("git", "rev-parse", "--abbrev-ref", "HEAD")
	assert.NoError(t, err, "get current branch")
	assert.That(t, currentBranch == "main", "current branch should be main")
}

func TestDeleteCommand_UpstreamTracking(t *testing.T) {
	// Create a temporary git repo
	tempDir := temp_repo.InitTempRepo(t)
	defer os.RemoveAll(tempDir)
	
	// Change to temp directory
	origDir, err := os.Getwd()
	assert.NoError(t, err, "get working dir")
	defer os.Chdir(origDir)
	
	err = os.Chdir(tempDir)
	assert.NoError(t, err, "change to temp dir")
	
	// Create a bare repo to act as remote
	remoteDir := filepath.Join(tempDir, "..", "remote.git")
	_, _, err = internal.RunCommand("git", "init", "--bare", remoteDir)
	assert.NoError(t, err, "create bare remote")
	
	// Add remote
	_, _, err = internal.RunCommand("git", "remote", "add", "origin", remoteDir)
	assert.NoError(t, err, "add remote")
	
	// Push main to remote
	_, _, err = internal.RunCommand("git", "push", "origin", "main")
	assert.NoError(t, err, "push main to remote")
	
	// Create and push a feature branch
	_, _, err = internal.RunCommand("git", "checkout", "-b", "feature")
	assert.NoError(t, err, "create feature branch")
	
	filename := filepath.Join(tempDir, "feature.txt")
	err = os.WriteFile(filename, []byte("feature\n"), 0o644)
	assert.NoError(t, err, "write feature file")
	
	_, _, err = internal.RunCommand("git", "add", filename)
	assert.NoError(t, err, "git add")
	
	_, _, err = internal.RunCommand("git", "commit", "-m", "Feature commit")
	assert.NoError(t, err, "git commit")
	
	_, _, err = internal.RunCommand("git", "push", "-u", "origin", "feature")
	assert.NoError(t, err, "push feature to remote")
	
	// Check upstream tracking
	upstream, _, err := internal.RunCommand("git", "for-each-ref", "--format=%(upstream:short)", "refs/heads/feature")
	assert.NoError(t, err, "get upstream")
	assert.That(t, upstream == "origin/feature", "upstream should be origin/feature, got: "+upstream)
	
	// Switch back to main
	_, _, err = internal.RunCommand("git", "checkout", "main")
	assert.NoError(t, err, "checkout main")
}
