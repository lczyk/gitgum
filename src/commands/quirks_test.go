package commands

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestParseQuirks_Empty(t *testing.T) {
	t.Setenv("GG_QUIRKS", "")
	q, err := parseQuirks()
	assert.NoError(t, err)
	assert.Equal(t, len(q), 0)
}

func TestParseQuirks_Single(t *testing.T) {
	t.Setenv("GG_QUIRKS", "normal-branches")
	q, err := parseQuirks()
	assert.NoError(t, err)
	assert.Equal(t, q["normal-branches"], true)
}

func TestParseQuirks_Negation(t *testing.T) {
	t.Setenv("GG_QUIRKS", "normal-branches,-normal-branches")
	q, err := parseQuirks()
	assert.NoError(t, err)
	assert.Equal(t, q["normal-branches"], false)
}

func TestParseQuirks_UnknownErrors(t *testing.T) {
	t.Setenv("GG_QUIRKS", "nonexistent")
	_, err := parseQuirks()
	assert.Error(t, err, assert.AnyError)
}

func TestQuirkEnabled(t *testing.T) {
	t.Setenv("GG_QUIRKS", "normal-branches")
	assert.Equal(t, quirkEnabled("normal-branches"), true)
}

func TestQuirkEnabled_Unset(t *testing.T) {
	t.Setenv("GG_QUIRKS", "")
	assert.Equal(t, quirkEnabled("normal-branches"), false)
}
