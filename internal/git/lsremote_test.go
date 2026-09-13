package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestRemoteBranchTip(t *testing.T) {
	t.Parallel()

	t.Run("exists", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		tip, exists, err := Repo{Dir: local}.RemoteBranchTip("origin", "main")
		require.NoError(t, err)
		assert.That(t, exists, "main is on the remote")
		assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main")), tip)
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		tip, exists, err := Repo{Dir: local}.RemoteBranchTip("origin", "nope")
		require.NoError(t, err)
		assert.That(t, !exists, "nope is not on the remote")
		assert.Equal(t, "", tip)
	})

	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "gone", filepath.Join(t.TempDir(), "missing.git"))
		_, exists, err := Repo{Dir: dir}.RemoteBranchTip("gone", "main")
		assert.Error(t, err, assert.AnyError, "an unreachable remote is an error, not an absent branch")
		assert.That(t, !exists, "nothing exists on a remote that can't be asked")
	})
}

// A remote that never answers is cut off at probeTimeout -- including when git
// has handed the connection to a child (ssh) that keeps its pipes open, which
// the shim's plain `sleep` stands in for.
func TestRemoteBranchTipTimesOut(t *testing.T) {
	realGit, err := exec.LookPath("git")
	require.NoError(t, err, "git must be on PATH")
	shim := t.TempDir()
	script := "#!/bin/sh\ncase \" $* \" in *\" ls-remote \"*) sleep 10 ;; esac\nexec " + realGit + " \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(shim, "git"), []byte(script), 0o755), "write git shim")
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))

	old := probeTimeout
	probeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { probeTimeout = old })

	local, _ := temp_repo.NewRepoWithRemote(t)
	start := time.Now()
	_, _, err = Repo{Dir: local}.RemoteBranchTip("origin", "main")
	assert.Error(t, err, assert.AnyError, "no answer is an error")
	assert.ContainsString(t, err.Error(), "no answer within")
	assert.That(t, time.Since(start) < 5*time.Second, "the probe must not wait out the hung child")
}

func TestSyncTrackingRef(t *testing.T) {
	t.Parallel()

	// the commit is already here: the ref moves without touching the remote,
	// which is pointed somewhere unreachable to prove it.
	t.Run("local commit", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.CreateCommit(t, local, "f.txt", "x\n", "feat: local")
		tip := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main"))
		temp_repo.RunGit(t, local, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))

		require.NoError(t, Repo{Dir: local}.SyncTrackingRef("origin", "main", tip))
		assert.Equal(t, tip, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "origin/main")))
	})

	// the commit only exists on the remote: that one branch is fetched.
	t.Run("remote commit", func(t *testing.T) {
		t.Parallel()
		local, remote := temp_repo.NewRepoWithRemote(t)
		other := t.TempDir()
		temp_repo.RunGit(t, other, "clone", remote, ".")
		temp_repo.RunGit(t, other, "config", "user.name", "Other User")
		temp_repo.RunGit(t, other, "config", "user.email", "other@example.com")
		temp_repo.RunGit(t, other, "config", "commit.gpgsign", "false")
		temp_repo.RunGit(t, other, "config", "core.hooksPath", ".git/hooks")
		temp_repo.CreateCommit(t, other, "g.txt", "y\n", "feat: remote")
		temp_repo.RunGit(t, other, "push", "origin", "main")

		r := Repo{Dir: local}
		tip, exists, err := r.RemoteBranchTip("origin", "main")
		require.NoError(t, err)
		require.That(t, exists, "main is on the remote")
		require.That(t, !r.HasObject(tip), "the new tip is not here yet")

		require.NoError(t, r.SyncTrackingRef("origin", "main", tip))
		assert.Equal(t, tip, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "origin/main")))
	})
}
