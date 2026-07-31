package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/dirty"
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
	require.NoError(t, err, filename+" should still exist")
}

func fileContent(t *testing.T, dir, filename, expected string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, filename))
	require.NoError(t, err, "read "+filename)
	assert.Equal(t, string(content), expected, "%s mismatch", filename)
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
			require.NoError(t, err, "command should succeed")

			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}

// A file with both staged and unstaged changes (porcelain `MM`) shows up in
// `diff --name-only` and in `diff --cached --name-only`. The two listings used
// to be concatenated blind, so the path was printed twice and the "(N)" header
// overstated how much was at risk in the one prompt the user gets before an
// irreversible clean.
// An untracked directory used to appear as one entry, so the count said 1
// where three files were about to be destroyed.
func TestCleanCommand_UntrackedDirectoryCountsItsFiles(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "build", "deep"), 0o755), "mkdir")
	for _, f := range []string{"build/a.o", "build/deep/b.o", "build/deep/c.o"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644), "write "+f)
	}

	var out bytes.Buffer
	cmd := &CleanCommand{cmdIO: cmdIO{Out: &out, Repo: git.Repo{Dir: dir}}, Yes: true}
	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, out.String(), "Files to be discarded (3):")
	// The directory is named once and its files hang off it, so the count and
	// the listing agree about how many things are at risk.
	assert.ContainsString(t, out.String(), "build/")
	assert.ContainsString(t, out.String(), "[??] b.o")
	assert.ContainsString(t, out.String(), "[??] c.o")
}

func TestPrintPlan(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")

	plan := dirty.Plan{
		Tracked:   []string{"spread.yaml"},
		Untracked: []string{"a/b/c/one.pyc", "a/b/c/two.pyc"},
		Ignored:   []string{"build/out.o"},
		Codes: map[string]string{
			"spread.yaml":   " M",
			"a/b/c/one.pyc": "??",
			"a/b/c/two.pyc": "??",
			"build/out.o":   "!!",
		},
	}

	t.Run("mixed groups are named in the summary", func(t *testing.T) {
		var out strings.Builder
		printPlan(&out, plan, dirty.Options{Tracked: true, Untracked: true})
		assert.EqualLineByLine(t, out.String(),
			"Files to be discarded (3): 1 change, 2 untracked\n"+
				"a/b/c/\n"+
				"├─ [??] one.pyc\n"+
				"└─ [??] two.pyc\n"+
				"[ M] spread.yaml\n"+
				"  (1 ignored file(s) left alone; --ignored includes them)\n")
	})

	// "(2): 2 untracked" would repeat the count back at you.
	t.Run("one group says nothing the count did not", func(t *testing.T) {
		var out strings.Builder
		printPlan(&out, dirty.Plan{Untracked: plan.Untracked, Codes: plan.Codes}, dirty.Options{Untracked: true})
		assert.ContainsString(t, out.String(), "Files to be discarded (2):\n")
	})

	// Unselected groups are absent from the tree, not merely uncounted.
	t.Run("ignored stays out until asked for", func(t *testing.T) {
		var out strings.Builder
		printPlan(&out, plan, dirty.Options{Tracked: true, Untracked: true, Ignored: true})
		// Lone file in a directory of its own: the chain folds to one line.
		assert.ContainsString(t, out.String(), "[!!] build/out.o")
		assert.ContainsString(t, out.String(), "1 change, 2 untracked, 1 ignored")

		var without strings.Builder
		printPlan(&without, plan, dirty.Options{Tracked: true})
		assert.Equal(t, strings.Contains(without.String(), "out.o"), false)
	})
}

// Ignored files are always scanned, so the listing can say they survive
// instead of leaving it to be discovered afterwards.
func TestCleanCommand_ReportsIgnoredFilesLeftAlone(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	temp_repo.WriteFile(t, dir, ".gitignore", "junk_*\n")
	temp_repo.RunGit(t, dir, "add", ".gitignore")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: ignore junk")
	temp_repo.WriteFile(t, dir, "junk_1", "x")
	temp_repo.WriteFile(t, dir, "loose", "x")

	var out bytes.Buffer
	cmd := &CleanCommand{cmdIO: cmdIO{Out: &out, Repo: git.Repo{Dir: dir}}, Yes: true}
	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, out.String(), "1 ignored file(s) left alone")
	_, err := os.Stat(filepath.Join(dir, "junk_1"))
	assert.NoError(t, err, "ignored file should survive")
}

func TestCleanCommand_StagedAndUnstagedFileListedOnce(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	temp_repo.WriteFile(t, dir, "README.md", "staged\n")
	temp_repo.RunGit(t, dir, "add", "README.md")
	temp_repo.WriteFile(t, dir, "README.md", "staged, then modified again\n")
	require.ContainsString(t,
		temp_repo.RunGit(t, dir, "status", "--porcelain"), "MM README.md")

	var out bytes.Buffer
	cmd := &CleanCommand{cmdIO: cmdIO{Out: &out, Repo: git.Repo{Dir: dir}}, Yes: true}
	require.NoError(t, cmd.Execute(nil))

	assert.ContainsString(t, out.String(), "Files to be discarded (1):")
	assert.Equal(t, strings.Count(out.String(), "README.md"), 1)
}
