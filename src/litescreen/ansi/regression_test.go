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

// xterm's sgr0 is "\x1b(B\x1b[m": a three-byte charset designation ahead of
// the reset. Regression: every non-CSI, non-OSC escape was assumed to be two
// bytes, so the 'B' leaked into the text of anything built with tput sgr0.
func TestRegressionCharsetDesignationConsumed(t *testing.T) {
	assert.Equal(t, ansi.Strip("a\x1b(B\x1b[mb"), "ab")
	assert.Equal(t, ansi.Strip("a\x1b)0b"), "ab")
	// The nF family allows several intermediates before the final byte.
	assert.Equal(t, ansi.Strip("a\x1b%@b"), "ab")
	assert.Equal(t, ansi.Strip("a\x1b#8b"), "ab")
}

// A bare ESC at the very end of the input is not an escape sequence, but it is
// not text either. Regression: walk only looked for escapes with a byte after
// them, so the ESC fell through as a rune and Strip returned it.
func TestRegressionTrailingEscDropped(t *testing.T) {
	assert.Equal(t, ansi.Strip("abc\x1b"), "abc")
	assert.Equal(t, len(ansi.Parse("abc\x1b", tcell.StyleDefault)), 3)
}

// ESC followed by a multibyte rune is a malformed escape, not an instruction to
// eat half the rune. Regression: the two-byte branch swallowed the rune's lead
// byte and the leftover continuation bytes decoded as U+FFFD.
func TestRegressionEscBeforeMultibyteRune(t *testing.T) {
	assert.Equal(t, ansi.Strip("a\x1b\u00e9b"), "a\u00e9b")
}

// ITU T.416 truecolour carries an optional colour-space id as the sub-param
// after the 2: "38:2::r:g:b". Regression: the empty id was read as the red
// channel, shifting the channels and leaving a stray 0 that reset the style.
func TestRegressionColonTrueColorWithColorSpaceID(t *testing.T) {
	got := ansi.Parse("\x1b[38:2::255:0:0mX", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	r, g, b := fg.RGB()
	assert.Equal(t, r, int32(255))
	assert.Equal(t, g, int32(0))
	assert.Equal(t, b, int32(0))

	// A non-empty id is skipped the same way.
	got = ansi.Parse("\x1b[48:2:1:10:20:30mX", tcell.StyleDefault)
	_, bg, _ := got[0].Style.Decompose()
	r, g, b = bg.RGB()
	assert.Equal(t, r, int32(10))
	assert.Equal(t, g, int32(20))
	assert.Equal(t, b, int32(30))
}

// Channel values above 255 are out of range; tcell packs them into a colour
// word where they corrupt the neighbouring channel. Clamp instead.
func TestRegressionTrueColorChannelsClamped(t *testing.T) {
	got := ansi.Parse("\x1b[38;2;300;0;999mX", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	r, g, b := fg.RGB()
	assert.Equal(t, r, int32(255))
	assert.Equal(t, g, int32(0))
	assert.Equal(t, b, int32(255))
}

// A CSI with a private-parameter marker ("\x1b[>4;2m", xterm's modifyOtherKeys
// setting, or "\x1b[?...") is not SGR even when it ends in 'm'. Regression:
// the marker byte was skipped as noise and the digits applied as attributes.
func TestRegressionPrivateMarkerCSINotSGR(t *testing.T) {
	for _, in := range []string{"\x1b[>4;2mX", "\x1b[?1;7mX", "\x1b[=5mX", "\x1b[<1mX"} {
		got := ansi.Parse(in, tcell.StyleDefault)
		assert.Equal(t, len(got), 1)
		assert.Equal(t, got[0].R, 'X')
		assert.Equal(t, got[0].Style, tcell.StyleDefault, "%q must not change the style", in)
	}
}
