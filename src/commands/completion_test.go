package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/completions"
)

func TestCompletionCommand_AllShellsAvailable(t *testing.T) {
	shells := []string{"bash", "fish", "zsh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			template, ok := completions.CompletionTemplates[shell]
			assert.That(t, ok, "completion template for %s not found in embedded assets", shell)
			assert.That(t, template != "", "completion template for %s is empty", shell)
			assert.ContainsString(t, template, "__GITGUM_CMD__")
		})
	}
}

func TestCompletionCommand_Execute(t *testing.T) {
	// Save and restore os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	shells := []string{"bash", "fish", "zsh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			// Set test command name
			os.Args = []string{"test-gitgum"}

			cmd := &CompletionCommand{}
			cmd.Args.Shell = shell

			// Capture stdout by executing and checking for errors
			err := cmd.Execute(nil)
			assert.NoError(t, err, "Execute() failed for %s: %v", shell, err)
		})
	}
}

func TestCompletionCommand_InvalidShell(t *testing.T) {
	cmd := &CompletionCommand{}
	cmd.Args.Shell = "invalid"

	err := cmd.Execute(nil)
	assert.Error(t, err, "invalid shell type 'invalid'")

	expectedMsg := "invalid shell type 'invalid'"
	assert.ContainsString(t, err.Error(), expectedMsg)
}

func TestCompletionCommand_PlaceholderReplacement(t *testing.T) {
	// Save and restore os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	testCases := []struct {
		name        string
		cmdName     string
		shell       string
	}{
		{"bash with gitgum", "gitgum", "bash"},
		{"fish with custom-name", "custom-name", "fish"},
		{"zsh with gg", "gg", "zsh"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = []string{tc.cmdName}

			// Get the template directly
			template, ok := completions.CompletionTemplates[tc.shell]
			assert.That(t, ok, "template for %s not found", tc.shell)

			// Verify original has placeholder
			assert.ContainsString(t, template, "__GITGUM_CMD__")

			// Simulate what Execute does
			result := strings.ReplaceAll(template, "__GITGUM_CMD__", tc.cmdName)

			// Verify placeholder was replaced
			// Note: no NotContainsString in assert package yet
			assert.That(t, !strings.Contains(result, "__GITGUM_CMD__"), "result should not contain __GITGUM_CMD__ after replacement")


			// Verify the command name appears in the output
			assert.ContainsString(t, result, tc.cmdName)
		})
	}
}
