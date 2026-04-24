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

			output := buf.String()
			assert.NoError(t, err)
			assert.ContainsString(t, output, tc.cmdName)
			assert.That(t, !strings.Contains(output, "__GITGUM_CMD__"), "placeholder should be replaced in output")
		})
	}
}

func TestCompletionCommand_InvalidShell(t *testing.T) {
	cmd := &CompletionCommand{}
	cmd.Args.Shell = "invalid"

	err := cmd.Execute(nil)
	assert.Error(t, err, "invalid shell type 'invalid'")
}

func TestCompletionCommand_DefaultCmdName(t *testing.T) {
	var buf strings.Builder
	cmd := &CompletionCommand{out: &buf}
	cmd.Args.Shell = "bash"
	// Don't set cmdName; should use default derived from os.Args[0]

	err := cmd.Execute(nil)

	output := buf.String()
	assert.NoError(t, err)
	// Output should contain some completion script content
	assert.That(t, len(output) > 0, "output should not be empty")
	assert.That(t, !strings.Contains(output, "__GITGUM_CMD__"), "placeholder should be replaced")
}
