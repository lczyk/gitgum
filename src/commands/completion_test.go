package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func executeCompletion(t *testing.T, cmdName, shell string) (string, error) {
	t.Helper()
	var buf strings.Builder
	cmd := &CompletionCommand{out: &buf, cmdName: cmdName}
	cmd.Args.Shell = shell
	err := cmd.Execute(nil)
	return buf.String(), err
}

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
			output, err := executeCompletion(t, tc.cmdName, tc.shell)

			assert.NoError(t, err)
			assert.ContainsString(t, output, tc.cmdName)
			assert.That(t, !strings.Contains(output, "__GITGUM_CMD__"), "placeholder should be replaced in output")
		})
	}
}

func TestCompletionCommand_InvalidShell(t *testing.T) {
	_, err := executeCompletion(t, "", "invalid")
	assert.Error(t, err, "invalid shell type 'invalid'")
}

func TestCompletionCommand_DefaultCmdName(t *testing.T) {
	output, err := executeCompletion(t, "", "bash")

	assert.NoError(t, err)
	assert.That(t, len(output) > 0, "output should not be empty")
	assert.That(t, !strings.Contains(output, "__GITGUM_CMD__"), "placeholder should be replaced")
}
