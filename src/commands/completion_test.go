package commands

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestCompletionCommand_Execute(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

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
			os.Args = []string{tc.cmdName}

			cmd := &CompletionCommand{}
			cmd.Args.Shell = tc.shell

			// capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := cmd.Execute(nil)

			w.Close()
			os.Stdout = oldStdout

			buf, _ := io.ReadAll(r)
			output := string(buf)

			assert.NoError(t, err)
			assert.That(t, !strings.Contains(output, "__GITGUM_CMD__"), "placeholder should be replaced in output")
			assert.ContainsString(t, output, tc.cmdName)
		})
	}
}

func TestCompletionCommand_InvalidShell(t *testing.T) {
	cmd := &CompletionCommand{}
	cmd.Args.Shell = "invalid"

	err := cmd.Execute(nil)
	assert.Error(t, err, "invalid shell type 'invalid'")
	assert.ContainsString(t, err.Error(), "invalid shell type 'invalid'")
}
