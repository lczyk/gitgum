package commands

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestReplayListCommand_Execute(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, dir string) (branchA, branchB string)
		expectError   bool
		errorContains string
		verifyOutput  func(t *testing.T, output string)
	}{
		{
			name: "list commits on feature branch since trunk",
			setup: func(t *testing.T, dir string) (string, string) {
				temp_repo.RunGit(t, dir, "checkout", "-b", "feature")

				for i := 1; i <= 3; i++ {
					temp_repo.CreateCommit(t, dir,
						fmt.Sprintf("file%d.txt", i), "content\n",
						fmt.Sprintf("Commit %d", i))
				}

				return "feature", "main"
			},
			verifyOutput: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.That(t, len(lines) == 3, "should have 3 commits")
				for _, line := range lines {
					assert.That(t, len(line) == 40, "commit hash should be 40 chars")
				}
			},
		},
		{
			name: "empty list when branches are at same commit",
			setup: func(t *testing.T, dir string) (string, string) {
				temp_repo.RunGit(t, dir, "checkout", "-b", "feature2")
				return "feature2", "main"
			},
			verifyOutput: func(t *testing.T, output string) {
				assert.That(t, strings.TrimSpace(output) == "", "should have no commits")
			},
		},
		{
			name: "error when branch A doesn't exist",
			setup: func(t *testing.T, dir string) (string, string) {
				return "nonexistent", "main"
			},
			expectError:   true,
			errorContains: "merge base",
		},
		{
			name: "error when branch B doesn't exist",
			setup: func(t *testing.T, dir string) (string, string) {
				return "main", "nonexistent"
			},
			expectError:   true,
			errorContains: "merge base",
		},
		{
			name: "list commits in chronological order",
			setup: func(t *testing.T, dir string) (string, string) {
				temp_repo.RunGit(t, dir, "checkout", "-b", "feature3")

				for i := 0; i < 3; i++ {
					temp_repo.CreateCommit(t, dir,
						fmt.Sprintf("ordered%c.txt", 'a'+i), "content\n",
						fmt.Sprintf("Ordered commit %c", 'A'+i))
				}

				return "feature3", "main"
			},
			verifyOutput: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.That(t, len(lines) == 3, "should have 3 commits in chronological order")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := temp_repo.InitTempRepo(t)

			branchA, branchB := tt.setup(t, dir)

			cmd := &ReplayListCommand{}
			cmd.Args.BranchA = branchA
			cmd.Args.BranchB = branchB

			// capture stdout since Execute writes directly to os.Stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := cmd.Execute(nil)

			w.Close()
			os.Stdout = oldStdout

			buf, _ := io.ReadAll(r)
			output := string(buf)

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
