package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func boolPtr(b bool) *bool { return &b }

func fileNotExists(t *testing.T, dir, filename string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, filename))
	assert.That(t, os.IsNotExist(err), filename+" should be removed")
}

func fileExists(t *testing.T, dir, filename string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, filename))
	assert.NoError(t, err, filename+" should still exist")
}

func fileContent(t *testing.T, dir, filename, expected string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, filename))
	assert.NoError(t, err, "read "+filename)
	assert.That(t, string(content) == expected, filename+" content mismatch")
}

func TestCleanCommand_Execute(t *testing.T) {

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
				fileContent(t, dir, "README.md", "# test repo\n")
				fileNotExists(t, dir, "untracked.txt")
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
				fileContent(t, dir, "README.md", "# test repo\n")
				fileExists(t, dir, "untracked.txt")
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
				fileContent(t, dir, "README.md", "modified\n")
				fileNotExists(t, dir, "untracked.txt")
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
				fileNotExists(t, dir, "untracked.txt")
				fileNotExists(t, dir, "test.log")
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
				fileNotExists(t, dir, "untracked.txt")
				fileExists(t, dir, "test.log")
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
				fileContent(t, dir, "README.md", "# test repo\n")
				fileNotExists(t, dir, "untracked.txt")
				fileNotExists(t, dir, "test.log")
			},
		},
		{
			name: "nothing to clean when both changes and untracked disabled",
			setup: func(t *testing.T, dir string) {
				err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644)
				assert.NoError(t, err, "modify tracked file")
				err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644)
				assert.NoError(t, err, "create untracked file")
			},
			cmd: &CleanCommand{Changes: boolPtr(false), Untracked: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				fileContent(t, dir, "README.md", "modified\n")
				fileExists(t, dir, "untracked.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := temp_repo.InitTempRepo(t)
			if tt.setup != nil {
				tt.setup(t, dir)
			}

			err := tt.cmd.Execute()
			assert.NoError(t, err, "command should succeed")

			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}
