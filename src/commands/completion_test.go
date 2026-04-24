package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestCompletionCommand_Execute(t *testing.T) {
	testCases := []struct {
		name    string
		cmdName string
		shell   string
	}{
		{"bash with gitgum", "gitgum", "bash"},
		{"fish with custom-name", "custom-name", "fish"},
		{"zsh with gg", "gg", "zsh"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			cmd := &CompletionCommand{out: &buf, cmdName: tc.cmdName}
			cmd.Args.Shell = tc.shell

			err := cmd.Execute(nil)

			assert.NoError(t, err)
			assert.ContainsString(t, buf.String(), tc.cmdName)
		})
	}
}

func TestCompletionCommand_InvalidShell(t *testing.T) {
	cmd := &CompletionCommand{}
	cmd.Args.Shell = "invalid"

	err := cmd.Execute(nil)
	assert.Error(t, err, "invalid shell type 'invalid'")
}
