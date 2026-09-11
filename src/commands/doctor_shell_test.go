package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/doctor"
)

func TestCheckCdWrapper(t *testing.T) {
	t.Parallel()
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}

	assert.Equal(t, len(checkCdWrapper(env(map[string]string{cdWrapperEnv: "1", "SHELL": "/bin/zsh"}))), 0)

	cases := map[string]string{
		"/usr/local/bin/fish":  "gg completion --cd fish | source",
		"/bin/zsh":             `eval "$(gg completion --cd zsh)"`,
		"/bin/bash":            `eval "$(gg completion --cd bash)"`,
		"/opt/homebrew/bin/nu": "gg completion --cd nu | save -f ~/.gitgum-cd.nu",
		"/bin/tcsh":            "gg completion --cd <shell>",
		"":                     "gg completion --cd <shell>",
	}
	for shell, wantFix := range cases {
		got := checkCdWrapper(env(map[string]string{"SHELL": shell}))
		require.Equal(t, len(got), 1, shell)
		assert.Equal(t, got[0].Check, "cd-wrapper")
		assert.Equal(t, got[0].Severity, doctor.SevFixable)
		assert.ContainsString(t, got[0].Fix, wantFix)
	}
}
