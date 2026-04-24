package commands

import (
	"testing"

	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

// DeleteCommand tests validate branch selection and deletion flow.
// Full E2E testing requires mocking fzf interactions (user input).
// Basic structure tests ensure refactored helpers are callable.

func TestDeleteCommand_Instantiate(t *testing.T) {
	_ = temp_repo.InitTempRepo(t)
	cmd := &DeleteCommand{}
	_ = cmd // verify command is instantiable
}
