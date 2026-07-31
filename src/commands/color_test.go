package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
)

func TestColorEnabled_NoColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("NO_COLOR", "1")
	assert.Equal(t, colorEnabled(), false)
}

func TestColorEnabled_ForceColorTrue(t *testing.T) {
	t.Setenv("NO_COLOR", "1") // FORCE_COLOR wins
	t.Setenv("FORCE_COLOR", "1")
	assert.Equal(t, colorEnabled(), true)
}

func TestColorEnabled_ForceColorFalse(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "0")
	assert.Equal(t, colorEnabled(), false)
}

func TestColorEnabled_ForceColorUnparsable(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "garbage")
	assert.Equal(t, colorEnabled(), true)
}

func TestColorEnabled_NonTtyUnderGoTest(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("NO_COLOR", "")
	// `go test` pipes stdout, so colorEnabled should be false.
	assert.Equal(t, colorEnabled(), false)
}

func TestPaint_Disabled(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	got := paint(ansiRed, "hello")
	assert.Equal(t, got, "hello")
}

func TestPaint_Enabled(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	got := paint(ansiRed, "hello")
	assert.Equal(t, got, ansiRed+"hello"+ansiReset)
}

func TestRenderTree_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	out := drawStatus([]git.Entry{entry(' ', 'M', "go.mod")}, nil)
	assert.Equal(t, strings.Contains(out, "\x1b"), false)
}
