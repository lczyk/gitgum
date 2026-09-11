package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/src/completions"
)

func TestCompletionCommand_Execute(t *testing.T) {
	cases := map[string]struct {
		cmdName string
		shell   string
		cd      bool
		wantErr string // non-empty: expected error pattern; empty: expect success
	}{
		"bash with gitgum":      {cmdName: "gitgum", shell: "bash"},
		"fish with custom-name": {cmdName: "custom-name", shell: "fish"},
		"zsh with gg":           {cmdName: "gg", shell: "zsh"},
		"invalid shell":         {shell: "invalid", wantErr: "invalid shell type 'invalid'"},
		"default cmd name":      {shell: "bash"},
		"cd wrapper":            {cmdName: "gg", shell: "fish", cd: true},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			cmd := &CompletionCommand{cmdIO: cmdIO{Out: &buf}, cmdName: tt.cmdName}
			cmd.Args.Shell = tt.shell
			cmd.Cd = tt.cd

			err := cmd.Execute(nil)
			output := buf.String()

			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.That(t, !strings.Contains(output, completions.Placeholder), "placeholder should be replaced")
				if tt.cmdName != "" {
					assert.ContainsString(t, output, tt.cmdName)
				} else {
					assert.ContainsString(t, output, filepath.Base(os.Args[0]))
				}
				// the wrapper is only in the --cd output.
				assert.Equal(t, strings.Contains(output, "GITGUM_CD_WRAPPER"), tt.cd)
			}
		})
	}
}
