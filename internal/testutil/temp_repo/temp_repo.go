package temp_repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert/require"
)

// NewRepo creates a temp git repo without chdir-ing into it and returns its
// path. Safe to call from t.Parallel() tests.
func NewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initRepoAt(t, dir)
	return dir
}

// NewEmptyRepo creates a temp git repo without any commits (no branches),
// without chdir-ing, and returns its path. Safe for t.Parallel().
func NewEmptyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initEmptyAt(t, dir)
	return dir
}

func initEmptyAt(t testing.TB, dir string) {
	t.Helper()
	RunGit(t, dir, "init", "-b", "main")
	RunGit(t, dir, "config", "user.name", "Test User")
	RunGit(t, dir, "config", "user.email", "test@example.com")
	RunGit(t, dir, "config", "commit.gpgsign", "false")
	RunGit(t, dir, "config", "tag.gpgsign", "false")
	// Use local .git/hooks so tests are not affected by global hooks
	// (e.g. agent-blocking pre-push guards set via core.hooksPath).
	RunGit(t, dir, "config", "core.hooksPath", ".git/hooks")
}

func initRepoAt(t testing.TB, dir string) {
	t.Helper()
	initEmptyAt(t, dir)
	WriteFile(t, dir, "README.md", "# test repo\n")
	RunGit(t, dir, "add", "README.md")
	RunGit(t, dir, "commit", "-m", "chore: init")
}

// RunGit runs a git command inside dir and returns its combined output.
func RunGit(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git ", args, " failed: ", string(out))
	return string(out)
}

// RunGitAllowFail is RunGit for commands expected to exit non-zero -- a merge
// that conflicts on purpose, say. Returns the combined output and the error.
func RunGitAllowFail(t testing.TB, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// WriteFile writes content to dir/filename, failing the test on error.
func WriteFile(t testing.TB, dir, filename, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644), "write ", filename)
}

// CreateCommit writes a file then stages and commits it.
func CreateCommit(t testing.TB, dir, filename, content, message string) {
	t.Helper()
	WriteFile(t, dir, filename, content)
	RunGit(t, dir, "add", filename)
	RunGit(t, dir, "commit", "-m", message)
}

// NewRepoWithRemote returns (local, remote) where local is a fresh repo with
// origin pointing at remote (a bare repo). The remote already has one commit
// on "main"; local's main tracks origin/main after the clone.
//
// Both dirs are t.TempDir-managed. Safe for parallel tests.
func NewRepoWithRemote(t *testing.T) (local, remote string) {
	t.Helper()
	upstream := t.TempDir()
	initRepoAt(t, upstream)
	remote = t.TempDir()
	RunGit(t, remote, "clone", "--bare", upstream, ".")

	local = t.TempDir()
	RunGit(t, local, "clone", remote, ".")
	RunGit(t, local, "config", "user.name", "Test User")
	RunGit(t, local, "config", "user.email", "test@example.com")
	RunGit(t, local, "config", "commit.gpgsign", "false")
	RunGit(t, local, "config", "tag.gpgsign", "false")
	RunGit(t, local, "config", "core.hooksPath", ".git/hooks")
	return local, remote
}
