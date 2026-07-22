package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/doctor"
)

func TestRenderFindings_Clean(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	var buf bytes.Buffer
	renderFindings(&buf, nil)
	assert.Equal(t, buf.String(), "no issues found.\n")
}

func TestRenderFindings_Grouped(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	var buf bytes.Buffer
	renderFindings(&buf, []doctor.Finding{
		{Check: "upstream-consistency", Severity: doctor.SevWarning, Message: "two remotes"},
		{Check: "remote-naming", Severity: doctor.SevFixable, Message: "misnamed", Fix: "git remote rename a b"},
	})
	got := buf.String()
	// fixable sorts before warning regardless of input order.
	want := strings.Join([]string{
		"fixable [remote-naming] misnamed",
		"    fix: git remote rename a b",
		"warning [upstream-consistency] two remotes",
		"",
		"1 fixable, 1 warning(s).",
		"",
	}, "\n")
	assert.Equal(t, got, want)
}
