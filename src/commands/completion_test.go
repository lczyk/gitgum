package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/completions"
)

func TestCompletionCommand_Execute(t *testing.T) {
	cases := map[string]struct {
		cmdName    string
		shell      string
		wantErr    bool
		errMessage string
	}{
		"bash with gitgum":      {cmdName: "gitgum", shell: "bash"},
		"fish with custom-name": {cmdName: "custom-name", shell: "fish"},
		"zsh with gg":           {cmdName: "gg", shell: "zsh"},
		"invalid shell":         {shell: "invalid", wantErr: true, errMessage: "invalid shell type 'invalid'"},
		"default cmd name":      {shell: "bash"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			cmd := &CompletionCommand{out: &buf, cmdName: tt.cmdName}
			cmd.Args.Shell = tt.shell

			err := cmd.Execute(nil)
			output := buf.String()

			if tt.wantErr {
				assert.Error(t, err, tt.errMessage)
			} else {
				assert.NoError(t, err)
				assert.That(t, !strings.Contains(output, completions.Placeholder), "placeholder should be replaced")
				if tt.cmdName != "" {
					assert.ContainsString(t, output, tt.cmdName)
				} else {
					assert.ContainsString(t, output, filepath.Base(os.Args[0]))
				}
			}
		})
	}
}
