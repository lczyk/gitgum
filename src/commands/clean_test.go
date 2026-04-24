package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestCleanCommand_Execute(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name   string
		setup  func(t *testing.T, dir string)
		cmd    *CleanCommand
		verify func(t *testing.T, dir string)
	}{
		{
			name: "clean changes and untracked with --yes",
			setup: func(t *testing.T, dir string) {
				err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644)
				assert.NoError(t, err, "modify tracked file")
				err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644)
				assert.NoError(t, err, "create untracked file")
			},
			cmd: &CleanCommand{Yes: true},
			verify: func(t *testing.T, dir string) {
				content, err := os.ReadFile(filepath.Join(dir, "README.md"))
				assert.NoError(t, err, "read README.md")
				assert.That(t, string(content) == "# test repo\n", "README.md should be reset")
				_, err = os.Stat(filepath.Join(dir, "untracked.txt"))
				assert.That(t, os.IsNotExist(err), "untracked.txt should be removed")
			},
		},
		{
			name: "clean only changes with --no-untracked",
			setup: func(t *testing.T, dir string) {
				err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644)
				assert.NoError(t, err, "modify tracked file")
				err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644)
				assert.NoError(t, err, "create untracked file")
			},
			cmd: &CleanCommand{Untracked: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				content, err := os.ReadFile(filepath.Join(dir, "README.md"))
				assert.NoError(t, err, "read README.md")
				assert.That(t, string(content) == "# test repo\n", "README.md should be reset")
				_, err = os.Stat(filepath.Join(dir, "untracked.txt"))
				assert.NoError(t, err, "untracked.txt should still exist")
			},
		},
		{
			name: "clean only untracked with --no-changes",
			setup: func(t *testing.T, dir string) {
				err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644)
				assert.NoError(t, err, "modify tracked file")
				err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644)
				assert.NoError(t, err, "create untracked file")
			},
			cmd: &CleanCommand{Changes: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				content, err := os.ReadFile(filepath.Join(dir, "README.md"))
				assert.NoError(t, err, "read README.md")
				assert.That(t, string(content) == "modified\n", "README.md should still be modified")
				_, err = os.Stat(filepath.Join(dir, "untracked.txt"))
				assert.That(t, os.IsNotExist(err), "untracked.txt should be removed")
			},
		},
		{
			name: "clean with --ignored includes ignored files",
			setup: func(t *testing.T, dir string) {
				err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644)
				assert.NoError(t, err, "create .gitignore")
				temp_repo.RunGit(t, dir, "add", ".gitignore")
				temp_repo.RunGit(t, dir, "commit", "-m", "add gitignore")
				err = os.WriteFile(filepath.Join(dir, "test.log"), []byte("log\n"), 0o644)
				assert.NoError(t, err, "create ignored file")
				err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644)
				assert.NoError(t, err, "create untracked file")
			},
			cmd: &CleanCommand{Ignored: boolPtr(true), Changes: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				_, err := os.Stat(filepath.Join(dir, "untracked.txt"))
				assert.That(t, os.IsNotExist(err), "untracked.txt should be removed")
				_, err = os.Stat(filepath.Join(dir, "test.log"))
				assert.That(t, os.IsNotExist(err), "test.log should be removed")
			},
		},
		{
			name: "without --ignored, keep ignored files",
			setup: func(t *testing.T, dir string) {
				err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644)
				assert.NoError(t, err, "create .gitignore")
				temp_repo.RunGit(t, dir, "add", ".gitignore")
				temp_repo.RunGit(t, dir, "commit", "-m", "add gitignore")
				err = os.WriteFile(filepath.Join(dir, "test.log"), []byte("log\n"), 0o644)
				assert.NoError(t, err, "create ignored file")
				err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644)
				assert.NoError(t, err, "create untracked file")
			},
			cmd: &CleanCommand{Changes: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				_, err := os.Stat(filepath.Join(dir, "untracked.txt"))
				assert.That(t, os.IsNotExist(err), "untracked.txt should be removed")
				_, err = os.Stat(filepath.Join(dir, "test.log"))
				assert.NoError(t, err, "test.log should still exist")
			},
		},
		{
			name: "--all flag enables everything",
			setup: func(t *testing.T, dir string) {
				err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644)
				assert.NoError(t, err, "create .gitignore")
				temp_repo.RunGit(t, dir, "add", ".gitignore")
				temp_repo.RunGit(t, dir, "commit", "-m", "add gitignore")
				err = os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644)
				assert.NoError(t, err, "modify tracked file")
				err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644)
				assert.NoError(t, err, "create untracked file")
				err = os.WriteFile(filepath.Join(dir, "test.log"), []byte("log\n"), 0o644)
				assert.NoError(t, err, "create ignored file")
			},
			cmd: &CleanCommand{All: true, Yes: true},
			verify: func(t *testing.T, dir string) {
				content, err := os.ReadFile(filepath.Join(dir, "README.md"))
				assert.NoError(t, err, "read README.md")
				assert.That(t, string(content) == "# test repo\n", "README.md should be reset")
				_, err = os.Stat(filepath.Join(dir, "untracked.txt"))
				assert.That(t, os.IsNotExist(err), "untracked.txt should be removed")
				_, err = os.Stat(filepath.Join(dir, "test.log"))
				assert.That(t, os.IsNotExist(err), "test.log should be removed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := temp_repo.InitTempRepo(t)
			if tt.setup != nil {
				tt.setup(t, dir)
			}

			err := tt.cmd.Execute(nil)
			assert.NoError(t, err, "command should succeed")

			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}
