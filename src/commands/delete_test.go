package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestDeleteCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &DeleteCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestDeleteCommand_NoBranches(t *testing.T) {
	temp_repo.InitEmptyTempRepo(t)

	cmd := &DeleteCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when no branches exist")
	assert.ContainsString(t, err.Error(), "no local branches")
}

// End-to-end test driven by a stub Selector: picks a non-current feature
// branch and deletes it without any further prompts. Demonstrates that
// commands can be exercised end-to-end without a TTY.
func TestDeleteCommand_DeletesPickedBranch(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	var buf strings.Builder
	stub := &stubSelector{selectAnswers: []string{"feature"}}
	cmd := &DeleteCommand{cmdIO: cmdIO{Out: &buf, UI: stub}}

	err := cmd.Execute(nil)
	assert.NoError(t, err)

	branches := temp_repo.RunGit(t, dir, "branch", "--list", "feature")
	assert.Equal(t, strings.TrimSpace(branches), "")
	assert.ContainsString(t, buf.String(), "Deleted local branch 'feature'.")
	assert.Equal(t, len(stub.confirmCalls), 0)
}
