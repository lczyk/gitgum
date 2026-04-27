package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func currentBranchIn(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
}

func TestApplySelection_InvalidFormat(t *testing.T) {
	s := &SwitchCommand{}
	err := s.applySelection("no-colon-separator")
	assert.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "invalid selection")
}

func TestApplySelection_UnknownType(t *testing.T) {
	s := &SwitchCommand{}
	err := s.applySelection("unknown: foo")
	assert.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "unknown branch type")
}

func TestApplySelection_Local(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	s := &SwitchCommand{}
	err := s.applySelection("local: feature")
	assert.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
}

func TestApplySelection_RemoteInvalidFormat(t *testing.T) {
	s := &SwitchCommand{}
	err := s.applySelection("remote: noslash")
	assert.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "invalid remote branch format")
}

// "local/remote" entries appear when a local branch already has a tracking
// remote — selecting such an entry must check out the local branch, not error.
func TestApplySelection_LocalRemote(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	s := &SwitchCommand{}
	err := s.applySelection("local/remote: feature")
	assert.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "feature")
}
