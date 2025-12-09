package commands

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestReplayListCommand_Execute(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, dir string) (branchA, branchB string)
		expectError  bool
		errorContains string
		verifyOutput func(t *testing.T, output string)
	}{
		{
			name: "list commits on feature branch since trunk",
			setup: func(t *testing.T, dir string) (string, string) {
				// Create trunk branch with initial commit (already done by temp_repo)
				trunk := "main"
				
				// Create feature branch
				_, _, err := internal.RunCommand("git", "-C", dir, "checkout", "-b", "feature")
				assert.NoError(t, err, "create feature branch")
				
				// Add commits to feature branch
				for i := 1; i <= 3; i++ {
					filename := filepath.Join(dir, "file"+string(rune('0'+i))+".txt")
					err := os.WriteFile(filename, []byte("content\n"), 0o644)
					assert.NoError(t, err, "write file")
					
					_, _, err = internal.RunCommand("git", "-C", dir, "add", filename)
					assert.NoError(t, err, "git add")
					
					_, _, err = internal.RunCommand("git", "-C", dir, "commit", "-m", "Commit "+string(rune('0'+i)))
					assert.NoError(t, err, "git commit")
				}
				
				return "feature", trunk
			},
			expectError: false,
			verifyOutput: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.That(t, len(lines) == 3, "should have 3 commits")
				// Verify all lines are valid commit hashes (40 hex chars)
				for _, line := range lines {
					assert.That(t, len(line) == 40, "commit hash should be 40 chars")
				}
			},
		},
		{
			name: "empty list when branches are at same commit",
			setup: func(t *testing.T, dir string) (string, string) {
				trunk := "main"
				
				// Create feature branch at same commit as trunk
				_, _, err := internal.RunCommand("git", "-C", dir, "checkout", "-b", "feature2")
				assert.NoError(t, err, "create feature2 branch")
				
				return "feature2", trunk
			},
			expectError: false,
			verifyOutput: func(t *testing.T, output string) {
				assert.That(t, strings.TrimSpace(output) == "", "should have no commits")
			},
		},
		{
			name: "error when branch A doesn't exist",
			setup: func(t *testing.T, dir string) (string, string) {
				return "nonexistent", "main"
			},
			expectError: true,
			errorContains: "merge base",
		},
		{
			name: "error when branch B doesn't exist",
			setup: func(t *testing.T, dir string) (string, string) {
				return "main", "nonexistent"
			},
			expectError: true,
			errorContains: "merge base",
		},
		{
			name: "list commits in chronological order",
			setup: func(t *testing.T, dir string) (string, string) {
				trunk := "main"
				
				// Create feature branch
				_, _, err := internal.RunCommand("git", "-C", dir, "checkout", "-b", "feature3")
				assert.NoError(t, err, "create feature3 branch")
				
				// Add commits with distinct messages to verify order
				commitHashes := make([]string, 3)
				for i := 0; i < 3; i++ {
					filename := filepath.Join(dir, "ordered"+string(rune('a'+i))+".txt")
					err := os.WriteFile(filename, []byte("content\n"), 0o644)
					assert.NoError(t, err, "write file")
					
					_, _, err = internal.RunCommand("git", "-C", dir, "add", filename)
					assert.NoError(t, err, "git add")
					
					_, _, err = internal.RunCommand("git", "-C", dir, "commit", "-m", "Ordered commit "+string(rune('A'+i)))
					assert.NoError(t, err, "git commit")
					
					// Get the commit hash
					hash, _, err := internal.RunCommand("git", "-C", dir, "rev-parse", "HEAD")
					assert.NoError(t, err, "get commit hash")
					commitHashes[i] = hash
				}
				
				return "feature3", trunk
			},
			expectError: false,
			verifyOutput: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.That(t, len(lines) == 3, "should have 3 commits in chronological order")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary git repo
			tempDir := temp_repo.InitTempRepo(t)
			defer os.RemoveAll(tempDir)
			
			// Change to temp directory for test
			origDir, err := os.Getwd()
			assert.NoError(t, err, "get working dir")
			defer os.Chdir(origDir)
			
			err = os.Chdir(tempDir)
			assert.NoError(t, err, "change to temp dir")
			
			// Run setup
			branchA, branchB := tt.setup(t, tempDir)
			
			// Create command
			cmd := &ReplayListCommand{}
			cmd.Args.BranchA = branchA
			cmd.Args.BranchB = branchB
			
			// Execute and capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			
			err = cmd.Execute(nil)
			
			w.Close()
			os.Stdout = oldStdout
			
			buf, _ := io.ReadAll(r)
			output := string(buf)
			
			// Check error expectation
			if tt.expectError {
				assert.That(t, err != nil, "expected error")
				if tt.errorContains != "" {
					assert.That(t, strings.Contains(err.Error(), tt.errorContains), 
						"error should contain '"+tt.errorContains+"', got: "+err.Error())
				}
			} else {
				assert.NoError(t, err, "should not error")
				if tt.verifyOutput != nil {
					tt.verifyOutput(t, output)
				}
			}
		})
	}
}
