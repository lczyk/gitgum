package dirty

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// baseline files are committed in ONE commit before any divergence is applied.
// Committing per-kind instead sweeps an earlier kind's staged half into a later
// commit, because git commits the whole index rather than the paths named.
var baseline = map[string]string{
	// long enough for the rename similarity heuristic to fire
	"renamed.txt":     "the quick brown fox jumps over the lazy dog, repeatedly, for many lines\n",
	"mod.txt":         "base\n",
	"staged.txt":      "base\n",
	"both.txt":        "base\n",
	"del.txt":         "base\n",
	"del_staged.txt":  "base\n",
	"with space.txt":  "base\n",
	".gitignore":      "ignored_*\nignored_dir/\n",
	"keep/nested.txt": "base\n",
}

func newDirtyRepo(t *testing.T, dirty func(t *testing.T, dir string)) (git.Repo, string) {
	t.Helper()
	dir := temp_repo.NewRepo(t)
	for name, content := range baseline {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755), "mkdir")
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644), "write "+name)
	}
	temp_repo.RunGit(t, dir, "add", "-A")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: baseline")
	dirty(t, dir)
	return git.Repo{Dir: dir}, dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755), "mkdir")
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644), "write "+name)
}

// One repo per dirt kind, so a failure names exactly which kind broke -- plus a
// final case carrying all of them, which is the only place dedup across a
// staged rename, a staged delete and an MM file has to work together.
func TestScanAgainstRealRepos(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		dirty         func(t *testing.T, dir string)
		wantTracked   []string
		wantUntracked []string
		wantIgnored   []string
	}{
		"clean repo": {
			dirty: func(t *testing.T, dir string) {},
		},
		"modified unstaged": {
			dirty:       func(t *testing.T, dir string) { write(t, dir, "mod.txt", "changed\n") },
			wantTracked: []string{"mod.txt"},
		},
		"modified staged": {
			dirty: func(t *testing.T, dir string) {
				write(t, dir, "staged.txt", "changed\n")
				temp_repo.RunGit(t, dir, "add", "staged.txt")
			},
			wantTracked: []string{"staged.txt"},
		},
		"staged and unstaged is one file at risk": {
			dirty: func(t *testing.T, dir string) {
				write(t, dir, "both.txt", "staged\n")
				temp_repo.RunGit(t, dir, "add", "both.txt")
				write(t, dir, "both.txt", "then unstaged\n")
			},
			wantTracked: []string{"both.txt"},
		},
		"deleted unstaged": {
			dirty: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "del.txt")), "rm")
			},
			wantTracked: []string{"del.txt"},
		},
		"deleted staged": {
			dirty: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "del_staged.txt")), "rm")
				temp_repo.RunGit(t, dir, "add", "del_staged.txt")
			},
			wantTracked: []string{"del_staged.txt"},
		},
		// the default listing names only the destination, but a hard reset
		// restores the source too, so both are at risk
		"staged rename reports both halves": {
			dirty: func(t *testing.T, dir string) {
				temp_repo.RunGit(t, dir, "mv", "renamed.txt", "renamed2.txt")
			},
			wantTracked: []string{"renamed2.txt", "renamed.txt"},
		},
		"path containing a space": {
			dirty:       func(t *testing.T, dir string) { write(t, dir, "with space.txt", "changed\n") },
			wantTracked: []string{"with space.txt"},
		},
		"untracked file": {
			dirty:         func(t *testing.T, dir string) { write(t, dir, "loose.txt", "x\n") },
			wantUntracked: []string{"loose.txt"},
		},
		// the bug this package exists for: clean -n collapses this to one line
		"untracked directory is expanded to its files": {
			dirty: func(t *testing.T, dir string) {
				write(t, dir, "untracked_dir/deep/a.txt", "x\n")
				write(t, dir, "untracked_dir/deep/b.txt", "x\n")
			},
			wantUntracked: []string{"untracked_dir/deep/a.txt", "untracked_dir/deep/b.txt"},
		},
		"untracked file with a space in its name": {
			dirty:         func(t *testing.T, dir string) { write(t, dir, "loose file.txt", "x\n") },
			wantUntracked: []string{"loose file.txt"},
		},
		"ignored file": {
			dirty:       func(t *testing.T, dir string) { write(t, dir, "ignored_thing", "x\n") },
			wantIgnored: []string{"ignored_thing"},
		},
		"ignored directory is expanded too": {
			dirty: func(t *testing.T, dir string) {
				write(t, dir, "ignored_dir/x.txt", "x\n")
				write(t, dir, "ignored_dir/y.txt", "x\n")
			},
			wantIgnored: []string{"ignored_dir/x.txt", "ignored_dir/y.txt"},
		},
		"every kind at once": {
			dirty: func(t *testing.T, dir string) {
				write(t, dir, "mod.txt", "changed\n")
				write(t, dir, "staged.txt", "changed\n")
				temp_repo.RunGit(t, dir, "add", "staged.txt")
				write(t, dir, "both.txt", "staged\n")
				temp_repo.RunGit(t, dir, "add", "both.txt")
				write(t, dir, "both.txt", "then unstaged\n")
				require.NoError(t, os.Remove(filepath.Join(dir, "del.txt")), "rm")
				require.NoError(t, os.Remove(filepath.Join(dir, "del_staged.txt")), "rm")
				temp_repo.RunGit(t, dir, "add", "del_staged.txt")
				temp_repo.RunGit(t, dir, "mv", "renamed.txt", "renamed2.txt")
				write(t, dir, "untracked_dir/deep/a.txt", "x\n")
				write(t, dir, "loose.txt", "x\n")
				write(t, dir, "ignored_thing", "x\n")
			},
			wantTracked: []string{
				"both.txt", "del.txt", "mod.txt", "renamed2.txt", "del_staged.txt", "renamed.txt", "staged.txt",
			},
			wantUntracked: []string{"loose.txt", "untracked_dir/deep/a.txt"},
			wantIgnored:   []string{"ignored_thing"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo, _ := newDirtyRepo(t, tc.dirty)
			got, err := Scan(repo)
			require.NoError(t, err, "scan")
			assert.EqualArraysUnordered(t, got.Tracked, tc.wantTracked)
			assert.EqualArraysUnordered(t, got.Untracked, tc.wantUntracked)
			assert.EqualArraysUnordered(t, got.Ignored, tc.wantIgnored)
		})
	}
}

// Mutating tests get their own repos by construction, since discarding
// destroys the state the read assertions depend on.
func TestDiscardAgainstRealRepo(t *testing.T) {
	t.Parallel()

	dirtyEverything := func(t *testing.T, dir string) {
		write(t, dir, "mod.txt", "changed\n")
		write(t, dir, "loose.txt", "x\n")
		write(t, dir, "ignored_thing", "x\n")
	}

	t.Run("tracked and untracked leaves ignored alone", func(t *testing.T) {
		t.Parallel()
		repo, dir := newDirtyRepo(t, dirtyEverything)
		plan, err := Scan(repo)
		require.NoError(t, err, "scan")
		require.NoError(t, plan.Discard(repo, Options{Tracked: true, Untracked: true}), "discard")

		after, err := Scan(repo)
		require.NoError(t, err, "rescan")
		assert.Equal(t, len(after.Tracked), 0)
		assert.Equal(t, len(after.Untracked), 0)
		assert.EqualArrays(t, after.Ignored, []string{"ignored_thing"})
		_, statErr := os.Stat(filepath.Join(dir, "ignored_thing"))
		assert.NoError(t, statErr, "ignored file should survive")
	})

	t.Run("ignored sweeps everything", func(t *testing.T) {
		t.Parallel()
		repo, dir := newDirtyRepo(t, dirtyEverything)
		plan, err := Scan(repo)
		require.NoError(t, err, "scan")
		require.NoError(t, plan.Discard(repo, Options{Tracked: true, Untracked: true, Ignored: true}), "discard")

		after, err := Scan(repo)
		require.NoError(t, err, "rescan")
		assert.That(t, after.Empty(), "nothing should remain")
		_, statErr := os.Stat(filepath.Join(dir, "ignored_thing"))
		assert.That(t, statErr != nil, "ignored file should be gone")
	})
}

// The guard exists because a hard reset mid-merge ends the merge rather than
// merely discarding edits.
func TestDiscardRefusesDuringRealMerge(t *testing.T) {
	t.Parallel()
	repo, dir := newDirtyRepo(t, func(t *testing.T, dir string) {})

	temp_repo.RunGit(t, dir, "checkout", "-b", "other")
	write(t, dir, "mod.txt", "from other\n")
	temp_repo.RunGit(t, dir, "add", "mod.txt")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: other side")

	temp_repo.RunGit(t, dir, "checkout", "main")
	write(t, dir, "mod.txt", "from main\n")
	temp_repo.RunGit(t, dir, "add", "mod.txt")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: main side")

	// conflicts on purpose, so the merge stops and leaves MERGE_HEAD behind
	out, mergeErr := temp_repo.RunGitAllowFail(t, dir, "merge", "other")
	require.That(t, mergeErr != nil, "the merge should have conflicted: %s", out)

	operation, inProgress := repo.InProgress()
	require.That(t, inProgress, "a merge should be detected as in progress")
	assert.ContainsString(t, operation, "merge")

	plan, err := Scan(repo)
	require.NoError(t, err, "scan")
	err = plan.Discard(repo, Options{Tracked: true})
	assert.Error(t, err, assert.AnyError, "discard should refuse mid-merge")
	assert.ContainsString(t, err.Error(), "in progress")

	// and the merge is still there, untouched
	_, stillInProgress := repo.InProgress()
	assert.That(t, stillInProgress, "the merge must survive the refusal")
}
