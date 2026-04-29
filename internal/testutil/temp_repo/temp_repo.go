package temp_repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
)

// ChdirTempDir changes to a fresh temp dir for the duration of the test.
//
// CAUTION: process cwd is shared, so a test that calls this cannot run in
// parallel with another test that also calls it (or runs git commands relying
// on cwd). Prefer passing dir explicitly where possible.
func ChdirTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origDir, err := os.Getwd()
	assert.NoError(t, err, "getwd")
	assert.NoError(t, os.Chdir(dir), "chdir")
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(origDir), "restore working dir")
	})
	return dir
}

// InitTempRepo creates a temp git repo, changes into it, makes an initial
// commit, and returns the repo path.
//
// Uses ChdirTempDir under the hood, so tests using InitTempRepo cannot run in
// parallel. Use NewRepo for parallel-safe tests that pass dir explicitly.
func InitTempRepo(t *testing.T) string {
	t.Helper()
	dir := ChdirTempDir(t)
	initRepoAt(t, dir)
	return dir
}

// NewRepo creates a temp git repo without chdir-ing into it and returns its
// path. Safe to call from t.Parallel() tests.
func NewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initRepoAt(t, dir)
	return dir
}

// InitEmptyTempRepo creates a temp git repo without any commits (no branches),
// changes into it, and returns the path. Like InitTempRepo but stops before the
// initial commit — useful for testing "no branches" error paths.
//
// Same cwd-sharing caveat as InitTempRepo; cannot run in parallel.
func InitEmptyTempRepo(t *testing.T) string {
	t.Helper()
	dir := ChdirTempDir(t)
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
}

func initRepoAt(t testing.TB, dir string) {
	t.Helper()
	initEmptyAt(t, dir)
	WriteFile(t, dir, "README.md", "# test repo\n")
	RunGit(t, dir, "add", "README.md")
	RunGit(t, dir, "commit", "-m", "initial commit")
}

// RunGit runs a git command inside dir and returns its combined output.
func RunGit(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "git ", args, " failed: ", string(out))
	return string(out)
}

// WriteFile writes content to dir/filename, failing the test on error.
func WriteFile(t testing.TB, dir, filename, content string) {
	t.Helper()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644), "write ", filename)
}

// CreateCommit writes a file then stages and commits it.
func CreateCommit(t testing.TB, dir, filename, content, message string) {
	t.Helper()
	WriteFile(t, dir, filename, content)
	RunGit(t, dir, "add", filename)
	RunGit(t, dir, "commit", "-m", message)
}
