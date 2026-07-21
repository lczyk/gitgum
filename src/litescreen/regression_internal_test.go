package litescreen

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// An inline picker anchored at the terminal's top row resolves to yOrigin 0,
// which is also the fullscreen sentinel. Regression: Fini inferred the render
// mode from yOrigin alone, so a top-row inline picker emitted the alt-screen
// leave sequence instead of clearing its region -- leftover picker rows on
// screen plus a spurious DECRST 1049 on the main buffer.
func TestRegressionTopRowInlineFiniClearsRegion(t *testing.T) {
	s, out, _ := newTestScreen(10, 80, 30, strings.NewReader(""))
	s.queryRow = func() (int, []byte) { return 1, nil } // cursor on top row

	require.NoError(t, s.Init())
	assert.Equal(t, 0, s.yOrigin) // inline anchored at top: the ambiguous case

	out.Reset()
	s.Fini()
	final := out.String()
	assert.That(t, !strings.Contains(final, "\x1b[?1049l"),
		"inline fini must not emit alt-screen leave, got %q", final)
	assert.ContainsString(t, final, "\x1b[1;1H\x1b[J", "fini clears the region at the top row")
}

// A picker that starts inline must stay inline when the terminal shrinks to or
// below its height mid-run. Regression: resize re-derived the mode from
// resolveHeight, flipping to "fullscreen" without ever entering the alt
// screen; the later Fini then followed the fullscreen path too.
func TestRegressionResizeKeepsInlineMode(t *testing.T) {
	termH := 30
	s, out, _ := newTestScreen(10, 80, 30, strings.NewReader(""))
	s.getSize = func() (int, int) { return 80, termH }
	s.queryRow = func() (int, []byte) { return 20, nil }
	require.NoError(t, s.Init())

	termH = 8 // shrink below the picker's 10 rows
	s.handleResize()

	out.Reset()
	s.Fini()
	final := out.String()
	assert.That(t, !strings.Contains(final, "\x1b[?1049l"),
		"picker that never entered the alt screen must not emit its leave sequence, got %q", final)
	assert.ContainsString(t, final, "\x1b[J", "fini clears the region")
}

// A picker that starts fullscreen (alt screen entered) must stay fullscreen
// when the terminal grows past its height policy mid-run. Regression: resize
// flipped to inline and shrank the drawn region inside the alt screen.
func TestRegressionResizeKeepsFullscreenMode(t *testing.T) {
	termH := 8
	s, out, _ := newTestScreen(10, 80, 8, strings.NewReader("")) // 10 >= 8: fullscreen
	s.getSize = func() (int, int) { return 80, termH }
	require.NoError(t, s.Init())
	assert.ContainsString(t, out.String(), "\x1b[?1049h", "init enters alt screen")

	termH = 30 // grow: resolveHeight alone would now say inline
	_, h := s.handleResize()
	assert.Equal(t, 30, h)

	out.Reset()
	s.Fini()
	assert.ContainsString(t, out.String(), "\x1b[?1049l",
		"picker that entered the alt screen must leave it on Fini")
}
