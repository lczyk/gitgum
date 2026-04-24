package commands

import (
	"os"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestPushCommand_NotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cmd := &PushCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error outside git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestPushCommand_NoRemotes(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &PushCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error without remotes")
	assert.ContainsString(t, err.Error(), "no remotes")
}
