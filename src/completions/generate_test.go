package completions

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestRender(t *testing.T) {
	cases := map[string]struct {
		shell   string
		cmdName string
		wantErr bool
	}{
		"bash":          {shell: "bash", cmdName: "myapp"},
		"fish":          {shell: "fish", cmdName: "gg"},
		"zsh":           {shell: "zsh", cmdName: "gitgum"},
		"empty cmdname": {shell: "bash", cmdName: ""},
		"invalid shell": {shell: "invalid", cmdName: "test", wantErr: true},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := Render(tt.shell, tt.cmdName)

			if tt.wantErr {
				assert.Error(t, err, assert.AnyError)
				assert.Equal(t, "", result)
				return
			}

			assert.NoError(t, err)
			assert.That(t, !strings.Contains(result, Placeholder), "placeholder should be replaced")
			if tt.cmdName != "" {
				assert.ContainsString(t, result, tt.cmdName)
			}
		})
	}
}
