package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/completions"
)

func TestCompletionCommand_Execute(t *testing.T) {
	tests := []struct {
		name       string
		cmdName    string
		shell      string
		wantErr    bool
		errMessage string
	}{
		{
			name:    "bash with gitgum",
			cmdName: "gitgum",
			shell:   "bash",
		},
		{
			name:    "fish with custom-name",
			cmdName: "custom-name",
			shell:   "fish",
		},
		{
			name:    "zsh with gg",
			cmdName: "gg",
			shell:   "zsh",
		},
		{
			name:       "invalid shell",
			cmdName:    "",
			shell:      "invalid",
			wantErr:    true,
			errMessage: "invalid shell type 'invalid'",
		},
		{
			name:    "default cmd name",
			cmdName: "",
			shell:   "bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			cmd := &CompletionCommand{out: &buf, cmdName: tt.cmdName}
			cmd.Args.Shell = tt.shell

			err := cmd.Execute(nil)
			output := buf.String()

			if tt.wantErr {
				assert.Error(t, err, tt.errMessage)
			} else {
				assert.NoError(t, err)
				assert.That(t, len(output) > 0, "output should not be empty")
				assert.That(t, !strings.Contains(output, completions.Placeholder), "placeholder should be replaced")
				if tt.cmdName != "" {
					assert.ContainsString(t, output, tt.cmdName)
				}
			}
		})
	}
}
