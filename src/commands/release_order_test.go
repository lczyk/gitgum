package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// A release that git can refuse cheaply -- the tag already exists -- refuses
// before the working tree is asked about or touched: the dirty-tree prompt is
// never shown and the uncommitted change is still there afterwards.
func TestReleaseCommand_Execute_RefusesBeforeTouchingTheTree(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "VERSION", "1.0.0\n")
	temp_repo.RunGit(t, dir, "add", "VERSION")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: version")
	temp_repo.RunGit(t, dir, "tag", "v1.0.1")
	temp_repo.WriteFile(t, dir, "README.md", "dirty\n")

	stub := &stubSelector{}
	cmd := &ReleaseCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: dir}}}
	cmd.Args.Bump = "patch"

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "tag v1.0.1 already exists")
	assert.Equal(t, len(stub.selectCalls), 0, "the dirty-tree prompt must not have been asked")
	assert.ContainsString(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "status", "--porcelain")), "README.md")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "stash", "list")), "")
}
