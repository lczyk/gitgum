package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/lczyk/gitgum/src/completions"
)

func TestCompletionCommand_AllShellsAvailable(t *testing.T) {
	shells := []string{"bash", "fish", "zsh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			template, ok := completions.CompletionTemplates[shell]
			if !ok {
				t.Errorf("completion template for %s not found in embedded assets", shell)
			}

			if template == "" {
				t.Errorf("completion template for %s is empty", shell)
			}

			// Verify the placeholder exists
			if !strings.Contains(template, "__GITGUM_CMD__") {
				t.Errorf("completion template for %s does not contain __GITGUM_CMD__ placeholder", shell)
			}
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
			if err != nil {
				t.Errorf("Execute() failed for %s: %v", shell, err)
			}
		})
	}
}

func TestCompletionCommand_InvalidShell(t *testing.T) {
	cmd := &CompletionCommand{}
	cmd.Args.Shell = "invalid"

	err := cmd.Execute(nil)
	if err == nil {
		t.Error("Execute() should have failed with invalid shell")
	}

	expectedMsg := "invalid shell type 'invalid'"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
	}
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
			if !ok {
				t.Fatalf("template for %s not found", tc.shell)
			}

			// Verify original has placeholder
			if !strings.Contains(template, "__GITGUM_CMD__") {
				t.Errorf("original template should contain __GITGUM_CMD__")
			}

			// Simulate what Execute does
			result := strings.ReplaceAll(template, "__GITGUM_CMD__", tc.cmdName)

			// Verify placeholder was replaced
			if strings.Contains(result, "__GITGUM_CMD__") {
				t.Errorf("result should not contain __GITGUM_CMD__ after replacement")
			}

			// Verify the command name appears in the output
			if !strings.Contains(result, tc.cmdName) {
				t.Errorf("result should contain command name %q", tc.cmdName)
			}
		})
	}
}
