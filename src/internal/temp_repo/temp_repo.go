package temp_repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
)

// InitTempRepo creates a temporary git repository and returns its path.
// It configures user identity, makes an initial commit, then changes the
// process working directory to that repo for the duration of the test.
func InitTempRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()

    // Ensure deterministic environment
    RunGit(t, dir, "init")
    RunGit(t, dir, "config", "user.name", "Test User")
    RunGit(t, dir, "config", "user.email", "test@example.com")

    // Create initial file and commit
    fname := filepath.Join(dir, "README.md")
    err := os.WriteFile(fname, []byte("# test repo\n"), 0o644)
    assert.NoError(t, err, "write initial file")
    RunGit(t, dir, "add", "README.md")
    RunGit(t, dir, "commit", "-m", "initial commit")

    // Change working directory so internal git commands operate in this repo
    origDir, _ := os.Getwd()
    _ = os.Chdir(dir)
    t.Cleanup(func() { _ = os.Chdir(origDir) })
    return dir
}

// runGit executes a git command inside the given repository path and fails the test on error.
func RunGit(t *testing.T, dir string, args ...string) string {
    t.Helper()
    cmd := exec.Command("git", args...)
    cmd.Dir = dir
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
    }
    return string(out)
}

// createCommit creates or updates a file and commits it with the provided message.
func CreateCommit(t *testing.T, dir, filename, content, message string) {
    t.Helper()
    fpath := filepath.Join(dir, filename)
    err := os.WriteFile(fpath, []byte(content), 0o644)
    assert.NoError(t, err, "write file for commit")
    RunGit(t, dir, "add", filename)
    RunGit(t, dir, "commit", "-m", message)
}

// createBranch creates a new branch from current HEAD.
func CreateBranch(t *testing.T, dir, branch string) {
    t.Helper()
    RunGit(t, dir, "branch", branch)
}

// addRemote adds a remote pointing to the given URL.
func AddRemote(t *testing.T, dir, name, url string) {
    t.Helper()
    RunGit(t, dir, "remote", "add", name, url)
}

// createTag creates a lightweight tag.
func CreateTag(t *testing.T, dir, tag string) {
    t.Helper()
    RunGit(t, dir, "tag", tag)
}
