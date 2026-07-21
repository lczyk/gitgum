package ansi_test

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
)

// ITU T.416 colon-separated extended colour (kitty, newer tools emit
// "\x1b[38:5:196m") must parse the same as the semicolon form. Regression:
// colons were skipped as noise, so the digits on both sides fused into one
// giant parameter and the whole sequence was silently ignored.
func TestRegressionColonSeparated256Color(t *testing.T) {
	got := ansi.Parse("\x1b[38:5:196mY", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(196))
}

func TestRegressionColonSeparatedTrueColor(t *testing.T) {
	got := ansi.Parse("\x1b[38:2:10:20:30mZ", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	r, g, b := fg.RGB()
	assert.Equal(t, r, int32(10))
	assert.Equal(t, g, int32(20))
	assert.Equal(t, b, int32(30))
}
