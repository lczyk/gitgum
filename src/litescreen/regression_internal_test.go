package litescreen

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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

// ChannelEvents must exit when the winch channel closes (Fini closes it while
// the event loop may still be selecting on it). Regression: the receive
// ignored the ok flag, and a closed channel is always ready -- the loop then
// ran handleResize against closed fds and emitted fabricated resize events,
// parking the goroutine forever on the send when nobody was draining.
func TestRegressionChannelEventsExitsOnWinchClose(t *testing.T) {
	s, _, _ := newTestScreen(10, 80, 24, strings.NewReader(""))
	s.fb = newFramebuf(80, 10)
	s.winch = make(chan os.Signal, 1)
	s.bytesCh = make(chan byte) // open and empty: winch is the only ready case
	close(s.winch)

	events := make(chan tcell.Event, 4)
	done := make(chan struct{})
	go func() { s.ChannelEvents(events, nil); close(done) }()

	select {
	case <-done:
		// clean exit, nothing emitted
	case ev := <-events:
		t.Fatalf("got fabricated event after winch close: %#v", ev)
	case <-time.After(2 * time.Second):
		t.Fatal("ChannelEvents did not exit after winch close")
	}
}

// Control runes stored in cells must not reach the output stream raw -- a
// stray ESC/TAB/CR desyncs the terminal and breaks the cell-grid model.
// Regression: flushTo wrote cell runes verbatim, so item strings carrying
// control bytes (filenames from find, a trailing bare ESC in ansi input)
// corrupted the terminal. tcell sanitises these; litescreen must too.
func TestRegressionFlushSanitisesControlRunes(t *testing.T) {
	fb := newFramebuf(4, 1)
	fb.set(0, 0, liteCell{mainc: 0x1b})
	fb.set(1, 0, liteCell{mainc: '\t'})
	fb.set(2, 0, liteCell{mainc: 'a', combc: []rune{0x08}})
	got := string(fb.flush(0, 0, 0, false))

	assert.That(t, !strings.Contains(got, "\t"), "raw TAB in output: %q", got)
	assert.That(t, !strings.Contains(got, "\x08"), "raw BS from combc in output: %q", got)
	// Every ESC in the stream must start one of our own CSI sequences.
	for i := 0; i < len(got); i++ {
		if got[i] == 0x1b {
			assert.That(t, i+1 < len(got) && got[i+1] == '[',
				"bare ESC leaked into output at byte %d: %q", i, got)
		}
	}
}

// A 2-column rune in the last column has no room for its second half.
// Regression: flushTo emitted it anyway; terminals disagree on what happens
// (clip, wrap-adjacent, shift) so the grid desyncs. fzf blanks that cell;
// litescreen must too.
func TestRegressionWideRuneLastColumnBlanked(t *testing.T) {
	fb := newFramebuf(4, 1)
	wide := rune(0x3042) // hiragana 'a', 2 columns
	fb.set(3, 0, liteCell{mainc: wide})
	got := string(fb.flush(0, 0, 0, false))

	assert.That(t, !strings.Contains(got, string(wide)),
		"wide rune emitted in last column: %q", got)
	assert.ContainsString(t, got, "\x1b[1;4H ", "last column blanked instead")
}

// A failed Init must close the tty fds it opened. Regression: the raw-mode
// error path returned early without closeIO, and callers that bail on the
// error (fuzzyfinder does) have no handle to close them -- two leaked
// /dev/tty fds per failed init.
func TestRegressionInitFailureClosesIO(t *testing.T) {
	s, _, _ := newTestScreen(10, 80, 24, strings.NewReader(""))
	closed := false
	s.closeIO = func() { closed = true }
	s.enterRaw = func() (func(), error) { return nil, errors.New("raw mode boom") }

	err := s.Init()
	assert.Error(t, err, assert.AnyError)
	assert.That(t, closed, "init failure must close the tty fds")
}

// A wide rune that moves one column right is never drawn. Regression: the
// diff skipped the cell because back and front agreed on it, but the narrow
// rune just emitted one column to the left had overwritten the wide rune's
// left half, which makes the terminal blank both halves. front kept claiming
// the rune was on screen, so no later Show re-emitted it either.
func TestRegressionWideRuneShiftedRightReemitted(t *testing.T) {
	wide := rune(0x65e5) // CJK 'day', 2 columns
	fb := newFramebuf(6, 1)
	_ = fb.flush(0, 0, 0, false)

	fb.set(0, 0, liteCell{mainc: wide})
	_ = fb.flush(0, 0, 0, false)

	fb.clear()
	fb.set(1, 0, liteCell{mainc: wide})
	got := string(fb.flush(0, 0, 0, false))
	assert.ContainsString(t, got, "\x1b[1;2H", "wide rune must be re-emitted at its new column; got %q", got)
	assert.ContainsString(t, got, string(wide), "wide rune missing from the frame; got %q", got)
}

// Two adjacent wide runes shifting left lose the second one. Regression:
// emitting the first at its new column overwrites the left half of what used
// to be there -- the first rune's old position -- and the terminal blanks that
// rune entirely, taking the column the second rune now needs with it. front
// still matched back there, so the second rune was skipped.
func TestRegressionAdjacentWideRunesShiftedLeftReemitted(t *testing.T) {
	wide := rune(0x65e5)
	fb := newFramebuf(6, 1)
	_ = fb.flush(0, 0, 0, false)

	fb.set(0, 0, liteCell{mainc: 'a'})
	fb.set(1, 0, liteCell{mainc: wide})
	fb.set(3, 0, liteCell{mainc: wide})
	_ = fb.flush(0, 0, 0, false)

	fb.clear()
	fb.set(0, 0, liteCell{mainc: wide})
	fb.set(2, 0, liteCell{mainc: wide})
	got := string(fb.flush(0, 0, 0, false))
	assert.ContainsString(t, got, "\x1b[1;1H", "first wide rune must be emitted at column 1; got %q", got)
	assert.ContainsString(t, got, "\x1b[1;3H", "second wide rune must be emitted at column 3; got %q", got)
	assert.Equal(t, strings.Count(got, string(wide)), 2)
}

// SetContent must own its combining runes. Regression: the caller's slice was
// stored as-is in back and, after a flush, shared with front, so a caller that
// reused the buffer for the next frame changed both sides at once and the diff
// never re-emitted the cell.
func TestRegressionCombiningRunesAreCopied(t *testing.T) {
	fb := newFramebuf(4, 1)
	comb := []rune{0x0301} // combining acute
	fb.set(0, 0, liteCell{mainc: 'e', combc: comb})
	fb.flush(0, 0, 0, false)

	comb[0] = 0x0308 // combining diaeresis, same buffer
	fb.set(0, 0, liteCell{mainc: 'e', combc: comb})
	out := string(fb.flush(0, 0, 0, false))
	assert.ContainsString(t, out, "e\u0308", "changed combining rune must be re-emitted, got %q", out)
	assert.That(t, !strings.Contains(out, "\u0301"), "the old combining rune must not survive, got %q", out)
}
