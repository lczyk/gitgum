package completions

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		shell   string
		cmdName string
		wantErr bool
	}{
		{shell: "bash", cmdName: "myapp"},
		{shell: "fish", cmdName: "gg"},
		{shell: "zsh", cmdName: "gitgum"},
		{shell: "invalid", cmdName: "test", wantErr: true},
	}

	for _, tt := range tests {
		result, err := Render(tt.shell, tt.cmdName)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Render(%q, %q): expected error", tt.shell, tt.cmdName)
			}
			if result != "" {
				t.Errorf("Render(%q, %q): error case should return empty string", tt.shell, tt.cmdName)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Render(%q, %q): %v", tt.shell, tt.cmdName, err)
		}
		if !strings.Contains(result, tt.cmdName) {
			t.Errorf("Render(%q, %q): result missing command name", tt.shell, tt.cmdName)
		}
		if strings.Contains(result, Placeholder) {
			t.Errorf("Render(%q, %q): placeholder not replaced", tt.shell, tt.cmdName)
		}
	}
}

func TestRenderEmptyCommand(t *testing.T) {
	result, err := Render("bash", "")
	if err != nil {
		t.Fatalf("Render with empty command: %v", err)
	}
	if strings.Contains(result, Placeholder) {
		t.Error("Placeholder not replaced with empty string")
	}
}
