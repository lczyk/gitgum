package completions

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		shell   string
		cmdName string
		wantErr bool
	}{
		{name: "bash", shell: "bash", cmdName: "myapp"},
		{name: "fish", shell: "fish", cmdName: "gg"},
		{name: "zsh", shell: "zsh", cmdName: "gitgum"},
		{name: "empty cmdname", shell: "bash", cmdName: ""},
		{name: "invalid shell", shell: "invalid", cmdName: "test", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Render(tt.shell, tt.cmdName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Render(%q, %q): expected error", tt.shell, tt.cmdName)
				}
				if result != "" {
					t.Errorf("Render(%q, %q): error case should return empty string", tt.shell, tt.cmdName)
				}
				return
			}
			if err != nil {
				t.Fatalf("Render(%q, %q): %v", tt.shell, tt.cmdName, err)
			}
			if tt.cmdName != "" && !strings.Contains(result, tt.cmdName) {
				t.Errorf("Render(%q, %q): result missing command name", tt.shell, tt.cmdName)
			}
			if strings.Contains(result, Placeholder) {
				t.Errorf("Render(%q, %q): placeholder not replaced", tt.shell, tt.cmdName)
			}
		})
	}
}
