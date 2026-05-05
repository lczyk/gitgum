package ansi_test

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
)

func TestParse_PlainText(t *testing.T) {
	got := ansi.Parse("abc", tcell.StyleDefault)
	assert.Equal(t, len(got), 3)
	for _, r := range got {
		assert.Equal(t, r.Style, tcell.StyleDefault)
	}
	assert.Equal(t, string([]rune{got[0].R, got[1].R, got[2].R}), "abc")
}

func TestParse_BasicForeground(t *testing.T) {
	got := ansi.Parse("\x1b[31mR\x1b[0mP", tcell.StyleDefault)
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].R, 'R')
	assert.Equal(t, got[1].R, 'P')
	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(1)) // red
	assert.Equal(t, got[1].Style, tcell.StyleDefault)
}

func TestParse_BrightForeground(t *testing.T) {
	got := ansi.Parse("\x1b[91mX", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(9)) // bright red
}

func TestParse_256Color(t *testing.T) {
	got := ansi.Parse("\x1b[38;5;200mY", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(200))
}

func TestParse_TrueColor(t *testing.T) {
	got := ansi.Parse("\x1b[38;2;10;20;30mZ", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	r, g, b := fg.RGB()
	assert.Equal(t, r, int32(10))
	assert.Equal(t, g, int32(20))
	assert.Equal(t, b, int32(30))
}

func TestParse_BoldThenReset(t *testing.T) {
	got := ansi.Parse("\x1b[1mB\x1b[0mP", tcell.StyleDefault)
	_, _, attr0 := got[0].Style.Decompose()
	assert.That(t, attr0&tcell.AttrBold != 0, "first rune should be bold")
	assert.Equal(t, got[1].Style, tcell.StyleDefault)
}

func TestParse_EmptySGRIsReset(t *testing.T) {
	// "\x1b[m" is implicit SGR 0
	got := ansi.Parse("\x1b[31mR\x1b[mP", tcell.StyleDefault)
	assert.Equal(t, got[1].Style, tcell.StyleDefault)
}

func TestParse_DimAndItalic(t *testing.T) {
	got := ansi.Parse("\x1b[2;3mDI", tcell.StyleDefault)
	_, _, attr := got[0].Style.Decompose()
	assert.That(t, attr&tcell.AttrDim != 0, "dim")
	assert.That(t, attr&tcell.AttrItalic != 0, "italic")
}

func TestParse_22ResetsBoldAndDim(t *testing.T) {
	got := ansi.Parse("\x1b[1;2mX\x1b[22mY", tcell.StyleDefault)
	_, _, a0 := got[0].Style.Decompose()
	assert.That(t, a0&tcell.AttrBold != 0, "X bold")
	assert.That(t, a0&tcell.AttrDim != 0, "X dim")
	_, _, a1 := got[1].Style.Decompose()
	assert.That(t, a1&tcell.AttrBold == 0, "Y not bold")
	assert.That(t, a1&tcell.AttrDim == 0, "Y not dim")
}

func TestParse_NonSGR_CSI_Ignored(t *testing.T) {
	// CSI A is cursor-up; should be consumed without producing runes or
	// changing style.
	got := ansi.Parse("a\x1b[5Ab", tcell.StyleDefault)
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].R, 'a')
	assert.Equal(t, got[1].R, 'b')
}

func TestParse_OSCConsumed(t *testing.T) {
	// OSC ... BEL should be skipped.
	got := ansi.Parse("a\x1b]0;title\x07b", tcell.StyleDefault)
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].R, 'a')
	assert.Equal(t, got[1].R, 'b')
}

func TestParse_OSCWithSTConsumed(t *testing.T) {
	// OSC ... ESC \ (ST) should be skipped.
	got := ansi.Parse("a\x1b]0;title\x1b\\b", tcell.StyleDefault)
	assert.Equal(t, len(got), 2)
}

func TestParse_NewlinePreserved(t *testing.T) {
	got := ansi.Parse("a\nb", tcell.StyleDefault)
	assert.Equal(t, len(got), 3)
	assert.Equal(t, got[1].R, '\n')
}

func TestParse_UTF8(t *testing.T) {
	got := ansi.Parse("\x1b[31mé✓", tcell.StyleDefault)
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].R, 'é')
	assert.Equal(t, got[1].R, '✓')
}

func TestParse_BaseStyleHonoured(t *testing.T) {
	base := tcell.StyleDefault.Background(tcell.PaletteColor(4))
	got := ansi.Parse("\x1b[31mR\x1b[0mP", base)
	// After reset, style returns to base (which has bg=4), not to
	// tcell.StyleDefault.
	_, bg, _ := got[1].Style.Decompose()
	assert.Equal(t, bg, tcell.PaletteColor(4))
}

func TestWriteToScreen_Layout(t *testing.T) {
	type cell struct {
		x, y int
		r    rune
	}
	var cells []cell
	set := func(x, y int, r rune, _ tcell.Style) {
		cells = append(cells, cell{x, y, r})
	}
	endX, endY := ansi.WriteToScreen(set, 2, 5, "ab\ncd", tcell.StyleDefault)
	assert.Equal(t, len(cells), 4)
	assert.Equal(t, cells[0], cell{2, 5, 'a'})
	assert.Equal(t, cells[1], cell{3, 5, 'b'})
	assert.Equal(t, cells[2], cell{2, 6, 'c'})
	assert.Equal(t, cells[3], cell{3, 6, 'd'})
	assert.Equal(t, endX, 4)
	assert.Equal(t, endY, 6)
}

func TestWriteToScreen_StripsAnsi(t *testing.T) {
	var sb strings.Builder
	set := func(_, _ int, r rune, _ tcell.Style) { sb.WriteRune(r) }
	ansi.WriteToScreen(set, 0, 0, "\x1b[31mhello\x1b[0m", tcell.StyleDefault)
	assert.Equal(t, sb.String(), "hello")
}

func TestParse_BasicBackground(t *testing.T) {
	got := ansi.Parse("\x1b[44mB", tcell.StyleDefault)
	_, bg, _ := got[0].Style.Decompose()
	assert.Equal(t, bg, tcell.PaletteColor(4)) // blue bg
}

func TestParse_BrightBackground(t *testing.T) {
	got := ansi.Parse("\x1b[104mB", tcell.StyleDefault)
	_, bg, _ := got[0].Style.Decompose()
	assert.Equal(t, bg, tcell.PaletteColor(12)) // bright blue bg
}

func TestParse_DefaultForeground(t *testing.T) {
	// 31 sets red, 39 resets to default fg.
	got := ansi.Parse("\x1b[31mR\x1b[39mD", tcell.StyleDefault)
	fg0, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg0, tcell.PaletteColor(1))
	fg1, _, _ := got[1].Style.Decompose()
	assert.Equal(t, fg1, tcell.ColorDefault)
}

func TestParse_DefaultBackground(t *testing.T) {
	got := ansi.Parse("\x1b[44mB\x1b[49mD", tcell.StyleDefault)
	_, bg0, _ := got[0].Style.Decompose()
	assert.Equal(t, bg0, tcell.PaletteColor(4))
	_, bg1, _ := got[1].Style.Decompose()
	assert.Equal(t, bg1, tcell.ColorDefault)
}

func TestParse_IndividualAttributeResets(t *testing.T) {
	cases := []struct {
		name     string
		set, off int
		attr     tcell.AttrMask
	}{
		{"italic 23", 3, 23, tcell.AttrItalic},
		{"underline 24", 4, 24, tcell.AttrUnderline},
		{"blink 25", 5, 25, tcell.AttrBlink},
		{"reverse 27", 7, 27, tcell.AttrReverse},
		{"strikethrough 29", 9, 29, tcell.AttrStrikeThrough},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := "\x1b[" + itoa(tc.set) + "mX\x1b[" + itoa(tc.off) + "mY"
			got := ansi.Parse(input, tcell.StyleDefault)
			_, _, a0 := got[0].Style.Decompose()
			assert.That(t, a0&tc.attr != 0, "X should have attr set")
			_, _, a1 := got[1].Style.Decompose()
			assert.That(t, a1&tc.attr == 0, "Y should have attr cleared")
		})
	}
}

func TestParse_MalformedTrueColor_Truncated(t *testing.T) {
	// "\x1b[38;2;1;2m" is missing the blue component; parser should not
	// emit garbage style or crash. Per applySGR, the 38 path bails out
	// when len(rest) < 4 -- subsequent 'X' should keep base style.
	got := ansi.Parse("\x1b[38;2;1;2mX", tcell.StyleDefault)
	assert.Equal(t, len(got), 1)
	// fg should remain default (truecolor parse failed silently).
	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.ColorDefault)
}

func TestParse_Malformed256_Truncated(t *testing.T) {
	got := ansi.Parse("\x1b[38;5mX", tcell.StyleDefault)
	assert.Equal(t, len(got), 1)
	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.ColorDefault)
}

func TestParse_OutOfRange256(t *testing.T) {
	// 256-color palette index 999 is invalid; parser should fall back to
	// default rather than emit garbage.
	got := ansi.Parse("\x1b[38;5;999mX", tcell.StyleDefault)
	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.ColorDefault)
}

func TestParse_MultipleOSCInRow(t *testing.T) {
	// Two OSC sequences back-to-back, each producing nothing.
	got := ansi.Parse("\x1b]0;a\x07\x1b]0;b\x07X", tcell.StyleDefault)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].R, 'X')
}

func TestParse_StackedSGRParams(t *testing.T) {
	// Multiple params in one CSI: bold + red fg + blue bg.
	got := ansi.Parse("\x1b[1;31;44mX", tcell.StyleDefault)
	fg, bg, attr := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(1))
	assert.Equal(t, bg, tcell.PaletteColor(4))
	assert.That(t, attr&tcell.AttrBold != 0, "bold")
}

// itoa is a tiny helper so the table-driven test can build SGR strings
// without pulling in strconv at the top level.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestParse_MalformedCSI_NoFinalByte(t *testing.T) {
	// No final byte; parser should consume to end without crashing.
	got := ansi.Parse("a\x1b[31", tcell.StyleDefault)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].R, 'a')
}
