package temp_repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
)

// also chdirs into the repo for the duration of the test.
func InitTempRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()

    // Ensure deterministic environment
    RunGit(t, dir, "init")
    RunGit(t, dir, "config", "user.name", "Test User")
    RunGit(t, dir, "config", "user.email", "test@example.com")
    RunGit(t, dir, "config", "commit.gpgsign", "false")
    RunGit(t, dir, "config", "tag.gpgsign", "false")

    // Create initial file and commit
    fname := filepath.Join(dir, "README.md")
    err := os.WriteFile(fname, []byte("# test repo\n"), 0o644)
    assert.NoError(t, err, "write initial file")
    RunGit(t, dir, "add", "README.md")
    RunGit(t, dir, "commit", "-m", "initial commit")

    // chdir into repo, restored on cleanup
    origDir, err := os.Getwd()
    if err != nil {
        t.Fatalf("getwd: %v", err)
    }
    if err := os.Chdir(dir); err != nil {
        t.Fatalf("chdir to temp repo: %v", err)
    }
    t.Cleanup(func() { _ = os.Chdir(origDir) })
    return dir
}

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

func CreateCommit(t *testing.T, dir, filename, content, message string) {
    t.Helper()
    fpath := filepath.Join(dir, filename)
    err := os.WriteFile(fpath, []byte(content), 0o644)
    assert.NoError(t, err, "write file for commit")
    RunGit(t, dir, "add", filename)
    RunGit(t, dir, "commit", "-m", message)
}

func CreateBranch(t *testing.T, dir, branch string) {
    t.Helper()
    RunGit(t, dir, "branch", branch)
}
