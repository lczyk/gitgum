package completions

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		shell   string
		cmdName string
		ok      bool
		check   func(t *testing.T, result string)
	}{
		{
			shell:   "bash",
			cmdName: "myapp",
			ok:      true,
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "myapp") {
					t.Error("result missing command name substitution")
				}
				if strings.Contains(result, placeholder) {
					t.Error("result still contains placeholder")
				}
			},
		},
		{
			shell:   "fish",
			cmdName: "gg",
			ok:      true,
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "gg") {
					t.Error("result missing command name substitution")
				}
				if strings.Contains(result, placeholder) {
					t.Error("result still contains placeholder")
				}
			},
		},
		{
			shell:   "zsh",
			cmdName: "gitgum",
			ok:      true,
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "gitgum") {
					t.Error("result missing command name substitution")
				}
				if strings.Contains(result, placeholder) {
					t.Error("result still contains placeholder")
				}
			},
		},
		{
			shell:   "invalid",
			cmdName: "test",
			ok:      false,
			check: func(t *testing.T, result string) {
				if result != "" {
					t.Error("error case should return empty string")
				}
			},
		},
	}

	for _, test := range tests {
		result, err := Render(test.shell, test.cmdName)
		if test.ok && err != nil {
			t.Errorf("Render(%q, %q): unexpected error: %v", test.shell, test.cmdName, err)
			continue
		}
		if !test.ok && err == nil {
			t.Errorf("Render(%q, %q): expected error but got none", test.shell, test.cmdName)
			continue
		}
		test.check(t, result)
	}
}

func TestRenderEmptyCommand(t *testing.T) {
	result, err := Render("bash", "")
	if err != nil {
		t.Fatalf("Render with empty command: %v", err)
	}
	// should replace placeholder with empty string, leaving valid bash but degenerate
	if strings.Contains(result, placeholder) {
		t.Error("placeholder not replaced with empty string")
	}
}
