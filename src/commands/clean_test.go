package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func boolPtr(b bool) *bool { return &b }

func fileNotExists(t *testing.T, dir, filename string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, filename))
	assert.That(t, os.IsNotExist(err), "%s should not exist (stat err: %v)", filename, err)
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
	assert.That(t, string(content) == expected, "%s: got %q, want %q", filename, string(content), expected)
}

func TestCleanCommand_NotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := &CleanCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestCleanCommand_Execute(t *testing.T) {

	cases := map[string]struct {
		setup  func(t *testing.T, dir string)
		cmd    *CleanCommand
		verify func(t *testing.T, dir string)
	}{
		"clean changes and untracked with --yes": {
			setup: func(t *testing.T, dir string) {
				temp_repo.WriteFile(t, dir, "README.md", "modified\n")
				temp_repo.WriteFile(t, dir, "untracked.txt", "untracked\n")
			},
			cmd: &CleanCommand{Yes: true},
			verify: func(t *testing.T, dir string) {
				fileContent(t, dir, "README.md", "# test repo\n")
				fileNotExists(t, dir, "untracked.txt")
			},
		},
		"clean only changes with --no-untracked": {
			setup: func(t *testing.T, dir string) {
				temp_repo.WriteFile(t, dir, "README.md", "modified\n")
				temp_repo.WriteFile(t, dir, "untracked.txt", "untracked\n")
			},
			cmd: &CleanCommand{Untracked: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				fileContent(t, dir, "README.md", "# test repo\n")
				fileExists(t, dir, "untracked.txt")
			},
		},
		"clean only untracked with --no-changes": {
			setup: func(t *testing.T, dir string) {
				temp_repo.WriteFile(t, dir, "README.md", "modified\n")
				temp_repo.WriteFile(t, dir, "untracked.txt", "untracked\n")
			},
			cmd: &CleanCommand{Changes: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				fileContent(t, dir, "README.md", "modified\n")
				fileNotExists(t, dir, "untracked.txt")
			},
		},
		"clean with --ignored includes ignored files": {
			setup: func(t *testing.T, dir string) {
				temp_repo.WriteFile(t, dir, ".gitignore", "*.log\n")
				temp_repo.RunGit(t, dir, "add", ".gitignore")
				temp_repo.RunGit(t, dir, "commit", "-m", "chore: add gitignore")
				temp_repo.WriteFile(t, dir, "test.log", "log\n")
				temp_repo.WriteFile(t, dir, "untracked.txt", "untracked\n")
			},
			cmd: &CleanCommand{Ignored: boolPtr(true), Changes: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				fileNotExists(t, dir, "untracked.txt")
				fileNotExists(t, dir, "test.log")
			},
		},
		"without --ignored, keep ignored files": {
			setup: func(t *testing.T, dir string) {
				temp_repo.WriteFile(t, dir, ".gitignore", "*.log\n")
				temp_repo.RunGit(t, dir, "add", ".gitignore")
				temp_repo.RunGit(t, dir, "commit", "-m", "chore: add gitignore")
				temp_repo.WriteFile(t, dir, "test.log", "log\n")
				temp_repo.WriteFile(t, dir, "untracked.txt", "untracked\n")
			},
			cmd: &CleanCommand{Changes: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				fileNotExists(t, dir, "untracked.txt")
				fileExists(t, dir, "test.log")
			},
		},
		"--all flag enables everything": {
			setup: func(t *testing.T, dir string) {
				temp_repo.WriteFile(t, dir, ".gitignore", "*.log\n")
				temp_repo.RunGit(t, dir, "add", ".gitignore")
				temp_repo.RunGit(t, dir, "commit", "-m", "chore: add gitignore")
				temp_repo.WriteFile(t, dir, "README.md", "modified\n")
				temp_repo.WriteFile(t, dir, "untracked.txt", "untracked\n")
				temp_repo.WriteFile(t, dir, "test.log", "log\n")
			},
			cmd: &CleanCommand{All: true, Yes: true},
			verify: func(t *testing.T, dir string) {
				fileContent(t, dir, "README.md", "# test repo\n")
				fileNotExists(t, dir, "untracked.txt")
				fileNotExists(t, dir, "test.log")
			},
		},
		"nothing to clean when both changes and untracked disabled": {
			setup: func(t *testing.T, dir string) {
				temp_repo.WriteFile(t, dir, "README.md", "modified\n")
				temp_repo.WriteFile(t, dir, "untracked.txt", "untracked\n")
			},
			cmd: &CleanCommand{Changes: boolPtr(false), Untracked: boolPtr(false), Yes: true},
			verify: func(t *testing.T, dir string) {
				fileContent(t, dir, "README.md", "modified\n")
				fileExists(t, dir, "untracked.txt")
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := temp_repo.NewRepo(t)
			if tt.setup != nil {
				tt.setup(t, dir)
			}

			// Per-iter copy so parallel sub-tests don't race on tt.cmd.Repo.
			cmd := *tt.cmd
			cmd.Repo = git.Repo{Dir: dir}

			err := cmd.Execute(nil)
			assert.NoError(t, err, "command should succeed")

			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}
