package litescreen

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
)

// preloaded returns a buffered channel pre-filled with bs. Receivers don't
// block as long as they don't read past len(bs).
func preloaded(bs ...byte) chan byte {
	ch := make(chan byte, max(1, len(bs)))
	for _, b := range bs {
		ch <- b
	}
	return ch
}

func TestControlByteEvent(t *testing.T) {
	tests := []struct {
		name string
		b    byte
		key  tcell.Key
		mod  tcell.ModMask
	}{
		{"NUL/Ctrl-Space", 0x00, tcell.KeyCtrlSpace, tcell.ModCtrl},
		{"Ctrl-A", 0x01, tcell.KeyCtrlA, tcell.ModCtrl},
		{"Ctrl-C", 0x03, tcell.KeyCtrlC, tcell.ModCtrl},
		{"Ctrl-D", 0x04, tcell.KeyCtrlD, tcell.ModCtrl},
		{"Backspace", 0x08, tcell.KeyBackspace, tcell.ModNone},
		{"Tab", 0x09, tcell.KeyTab, tcell.ModNone},
		{"Ctrl-J", 0x0a, tcell.KeyCtrlJ, tcell.ModCtrl},
		{"Ctrl-K", 0x0b, tcell.KeyCtrlK, tcell.ModCtrl},
		{"Enter", 0x0d, tcell.KeyEnter, tcell.ModNone},
		{"Ctrl-N", 0x0e, tcell.KeyCtrlN, tcell.ModCtrl},
		{"Ctrl-P", 0x10, tcell.KeyCtrlP, tcell.ModCtrl},
		{"Ctrl-U", 0x15, tcell.KeyCtrlU, tcell.ModCtrl},
		{"Ctrl-W", 0x17, tcell.KeyCtrlW, tcell.ModCtrl},
		{"Ctrl-Z", 0x1a, tcell.KeyCtrlZ, tcell.ModCtrl},
		{"Esc", 0x1b, tcell.KeyEsc, tcell.ModNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := controlByteEvent(tc.b).(*tcell.EventKey)
			assert.Equal(t, ev.Key(), tc.key)
			assert.Equal(t, ev.Modifiers(), tc.mod)
		})
	}
}

func TestParseEvent_Runes(t *testing.T) {
	s := &Screen{}
	ev := s.parseEvent('a', preloaded()).(*tcell.EventKey)
	assert.Equal(t, ev.Key(), tcell.KeyRune)
	assert.Equal(t, ev.Rune(), 'a')

	// 0x7f is Backspace (DEL), not a control byte. tcell.NewEventKey
	// normalizes KeyBackspace2 → KeyBackspace at construction time, so the
	// returned event reports KeyBackspace.
	ev = s.parseEvent(0x7f, preloaded()).(*tcell.EventKey)
	assert.Equal(t, ev.Key(), tcell.KeyBackspace)
}

func TestParseEvent_CtrlC(t *testing.T) {
	// This is the regression test for the original bug: byte 0x03 must
	// produce KeyCtrlC, not Key(3) (which is KeyETX).
	s := &Screen{}
	ev := s.parseEvent(0x03, preloaded()).(*tcell.EventKey)
	assert.Equal(t, ev.Key(), tcell.KeyCtrlC)
}

func TestParseCSI(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte // bytes after "ESC ["
		key   tcell.Key
	}{
		{"Up", []byte{'A'}, tcell.KeyUp},
		{"Down", []byte{'B'}, tcell.KeyDown},
		{"Right", []byte{'C'}, tcell.KeyRight},
		{"Left", []byte{'D'}, tcell.KeyLeft},
		{"Home (CSI H)", []byte{'H'}, tcell.KeyHome},
		{"End (CSI F)", []byte{'F'}, tcell.KeyEnd},
		{"Home (CSI 1~)", []byte{'1', '~'}, tcell.KeyHome},
		{"End (CSI 4~)", []byte{'4', '~'}, tcell.KeyEnd},
		{"Insert", []byte{'2', '~'}, tcell.KeyInsert},
		{"Delete", []byte{'3', '~'}, tcell.KeyDelete},
		{"PgUp", []byte{'5', '~'}, tcell.KeyPgUp},
		{"PgDn", []byte{'6', '~'}, tcell.KeyPgDn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := parseCSI(preloaded(tc.bytes...))
			ek, ok := ev.(*tcell.EventKey)
			assert.That(t, ok, "expected *EventKey")
			assert.Equal(t, ek.Key(), tc.key)
		})
	}
}

func TestParseCSI_Unknown(t *testing.T) {
	// CSI Z (Shift-Tab) — we don't map it. parseCSI should return nil
	// rather than crash.
	assert.That(t, parseCSI(preloaded('Z')) == nil, "unknown CSI returns nil")
}

func TestParseSS3(t *testing.T) {
	tests := []struct {
		name string
		b    byte
		key  tcell.Key
	}{
		// tmux/emacs emit arrows as SS3 (\x1bO[ABCD]); both forms must work.
		{"Up", 'A', tcell.KeyUp},
		{"Down", 'B', tcell.KeyDown},
		{"Right", 'C', tcell.KeyRight},
		{"Left", 'D', tcell.KeyLeft},
		{"Home", 'H', tcell.KeyHome},
		{"End", 'F', tcell.KeyEnd},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := parseSS3(preloaded(tc.b))
			ek := ev.(*tcell.EventKey)
			assert.Equal(t, ek.Key(), tc.key)
		})
	}
	assert.That(t, parseSS3(preloaded('Q')) == nil, "unknown SS3 final returns nil")
}

// TestParseCSI_Malformed makes sure the parser doesn't panic and returns nil
// on the kinds of inputs fzf treats as Invalid: bare CSI introducer, DSR
// response leakage, unrecognised final bytes, etc.
func TestParseCSI_Malformed(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte // bytes after ESC[
	}{
		{"DSR response (CSI R)", []byte{'1', ';', '1', 'R'}}, // \x1b[1;1R — terminal's reply to \e[6n
		{"Mouse intro (CSI <)", []byte{'<'}},
		{"Unknown final", []byte{'1', 'Z'}},
		{"Extra param after ~", []byte{'3', ';', '3', '~', '1'}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// All of these should return nil (silently ignored) and not panic.
			ev := parseCSI(preloaded(tc.bytes...))
			assert.That(t, ev == nil, "expected nil event for malformed CSI")
		})
	}
}

// TestControlByteEvent_RareControls covers the 0x1c-0x1f range that the
// finder doesn't currently use. Locking the passthrough so we notice if it
// changes.
func TestControlByteEvent_RareControls(t *testing.T) {
	tests := []struct {
		b   byte
		key tcell.Key
	}{
		{0x1c, tcell.KeyFS}, // Ctrl-\
		{0x1d, tcell.KeyGS}, // Ctrl-]
		{0x1e, tcell.KeyRS}, // Ctrl-^
		{0x1f, tcell.KeyUS}, // Ctrl-/ (or Ctrl-_)
	}
	for _, tc := range tests {
		ev := controlByteEvent(tc.b).(*tcell.EventKey)
		assert.Equal(t, ev.Key(), tc.key)
	}
}

// TestParseEscape_AltLetter: ESC followed by a printable byte. fzf treats
// these as Alt-<letter>; we don't support Alt and the finder doesn't use
// them, so we collapse to bare Esc and discard the follow-up. Locking that
// behavior here so a future change to support Alt is a deliberate choice.
func TestParseEscape_AltLetter(t *testing.T) {
	ev := parseEscape(preloaded('a'))
	ek := ev.(*tcell.EventKey)
	assert.Equal(t, ek.Key(), tcell.KeyEsc)
}

func TestParseUTF8(t *testing.T) {
	// 'あ' (Japanese hiragana) = U+3042 = 0xe3 0x81 0x82
	ev := parseUTF8(0xe3, preloaded(0x81, 0x82))
	ek := ev.(*tcell.EventKey)
	assert.Equal(t, ek.Key(), tcell.KeyRune)
	assert.Equal(t, ek.Rune(), 'あ')

	// Bulgarian 'З' = U+0417 = 0xd0 0x97
	ev = parseUTF8(0xd0, preloaded(0x97))
	ek = ev.(*tcell.EventKey)
	assert.Equal(t, ek.Rune(), 'З')

	// '🚀' = U+1F680 = 0xf0 0x9f 0x9a 0x80
	ev = parseUTF8(0xf0, preloaded(0x9f, 0x9a, 0x80))
	ek = ev.(*tcell.EventKey)
	assert.Equal(t, ek.Rune(), '🚀')
}

func TestParseUTF8_BadLead(t *testing.T) {
	// 0xff isn't a valid UTF-8 lead byte.
	assert.That(t, parseUTF8(0xff, preloaded()) == nil, "bad lead returns nil")
}

func TestParseEscape_BareEsc(t *testing.T) {
	// No follow-up bytes: parser waits escTimeout, then emits bare Esc.
	start := time.Now()
	ev := parseEscape(preloaded())
	elapsed := time.Since(start)

	ek := ev.(*tcell.EventKey)
	assert.Equal(t, ek.Key(), tcell.KeyEsc)
	// Confirm the timeout fired (and isn't absurdly long).
	assert.That(t, elapsed >= escTimeout, "should have waited for timeout")
	assert.That(t, elapsed < 5*escTimeout, "shouldn't be much longer than timeout")
}

func TestParseEscape_CSIArrow(t *testing.T) {
	// Full ESC [ A sequence consumed via parseEvent.
	s := &Screen{}
	ch := preloaded('[', 'A')
	ev := s.parseEvent(0x1b, ch).(*tcell.EventKey)
	assert.Equal(t, ev.Key(), tcell.KeyUp)
}

func TestStyleToSGR(t *testing.T) {
	tests := []struct {
		name  string
		style tcell.Style
		want  string
	}{
		{"default", tcell.StyleDefault, ""},
		{"bold", tcell.StyleDefault.Bold(true), "\x1b[1m"},
		// ColorBlue's palette index in tcell is 12 (the 16-color enum is
		// in VGA order: black, maroon, green, olive, navy, ..., red=9,
		// ..., blue=12). Don't assume index = ANSI base color.
		{"fg blue (palette)", tcell.StyleDefault.Foreground(tcell.ColorBlue), "\x1b[38;5;12m"},
		{"fg+bg", tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorBlack),
			"\x1b[38;5;9;48;5;0m"},
		{"bold+reverse", tcell.StyleDefault.Bold(true).Reverse(true), "\x1b[1;7m"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := styleToSGR(tc.style)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestStyleToSGR_TrueColor(t *testing.T) {
	// 24-bit color path: tcell.NewRGBColor returns a Color > Color255.
	st := tcell.StyleDefault.Foreground(tcell.NewRGBColor(255, 128, 0))
	got := styleToSGR(st)
	assert.That(t, strings.Contains(got, "38;2;255;128;0"), "expected 24-bit fg sequence, got %q", got)
}

func TestCellEqual(t *testing.T) {
	a := liteCell{mainc: 'x', style: tcell.StyleDefault}
	b := liteCell{mainc: 'x', style: tcell.StyleDefault}
	assert.That(t, cellEqual(a, b), "identical cells equal")

	b.mainc = 'y'
	assert.That(t, !cellEqual(a, b), "different rune unequal")

	b = liteCell{mainc: 'x', style: tcell.StyleDefault.Bold(true)}
	assert.That(t, !cellEqual(a, b), "different style unequal")

	a = liteCell{mainc: 'x', combc: []rune{0x301}}
	b = liteCell{mainc: 'x', combc: []rune{0x301}}
	assert.That(t, cellEqual(a, b), "matching combining marks equal")

	b.combc = []rune{0x302}
	assert.That(t, !cellEqual(a, b), "different combining marks unequal")
}

// --- framebuf tests --------------------------------------------------------
//
// Lock the rendering compositor's behavior. These exercise the diff-and-emit
// logic that previously lived inline in Screen.Show, hidden behind the
// /dev/tty plumbing.

func TestFramebuf_FirstFlushEmitsBlanks(t *testing.T) {
	// 3x2 buffer, no SetContent calls. First flush should still emit every
	// cell as a space because front is sentinel-initialized.
	fb := newFramebuf(3, 2)
	out := string(fb.flush(0, 0, 0, false))
	// 3*2 = 6 cells, all spaces. Cursor stays hidden (cursorVisible=false).
	spaces := strings.Count(out, " ")
	assert.Equal(t, spaces, 6)
	assert.That(t, !strings.Contains(out, "\x1b[?25h"), "cursor must stay hidden when !cursorVisible")
}

func TestFramebuf_NoChangeNoOutput(t *testing.T) {
	// Second flush with no SetContent calls should emit only framing
	// (cursor-hide + reset SGR + optional cursor-show), no cell repositioning.
	fb := newFramebuf(3, 2)
	_ = fb.flush(0, 0, 0, false)
	out := string(fb.flush(0, 0, 0, false))
	assert.That(t, !strings.Contains(out, "\x1b[1;1H"), "should not reposition cursor when no cells changed")
}

func TestFramebuf_SetContentEmitsCell(t *testing.T) {
	fb := newFramebuf(5, 1)
	_ = fb.flush(0, 0, 0, false) // sync front

	fb.set(2, 0, liteCell{mainc: 'X'})
	out := string(fb.flush(0, 0, 0, false))
	// Should reposition to row 1, col 3 (1-indexed) and emit 'X'.
	assert.That(t, strings.Contains(out, "\x1b[1;3H"), "expected cursor move to (1,3); got %q", out)
	assert.That(t, strings.Contains(out, "X"), "expected 'X' in output; got %q", out)
}

func TestFramebuf_OutOfRangeIgnored(t *testing.T) {
	fb := newFramebuf(3, 2)
	// Should not panic.
	fb.set(-1, 0, liteCell{mainc: 'X'})
	fb.set(0, -1, liteCell{mainc: 'X'})
	fb.set(99, 0, liteCell{mainc: 'X'})
	fb.set(0, 99, liteCell{mainc: 'X'})
}

func TestFramebuf_StyleTransition(t *testing.T) {
	fb := newFramebuf(4, 1)
	_ = fb.flush(0, 0, 0, false)

	fb.set(0, 0, liteCell{mainc: 'A', style: tcell.StyleDefault.Bold(true)})
	fb.set(1, 0, liteCell{mainc: 'B', style: tcell.StyleDefault.Bold(true)})
	fb.set(2, 0, liteCell{mainc: 'C', style: tcell.StyleDefault}) // style change
	out := string(fb.flush(0, 0, 0, false))

	// Bold SGR appears (it's `\x1b[1m`).
	assert.That(t, strings.Contains(out, "\x1b[1m"), "expected bold SGR; got %q", out)
	// Style reset between bold and default appears at least twice (initial + change).
	resets := strings.Count(out, "\x1b[m")
	assert.That(t, resets >= 2, "expected ≥2 SGR resets, got %d in %q", resets, out)
}

func TestFramebuf_WideRunePhantomSlot(t *testing.T) {
	// Wide runes occupy two cells. After drawing a wide rune at x=0, the
	// next flush with no changes should NOT re-emit the phantom at x=1.
	fb := newFramebuf(4, 1)
	_ = fb.flush(0, 0, 0, false)

	fb.set(0, 0, liteCell{mainc: '日'}) // width 2
	out1 := string(fb.flush(0, 0, 0, false))
	assert.That(t, strings.Contains(out1, "日"), "wide rune should be emitted")

	// Second flush, nothing changed. Should not contain another '日' or
	// reposition the cursor over the phantom slot.
	out2 := string(fb.flush(0, 0, 0, false))
	assert.That(t, !strings.Contains(out2, "日"), "wide rune should not re-emit on no-change flush; got %q", out2)
}

func TestFramebuf_CursorVisibility(t *testing.T) {
	fb := newFramebuf(3, 1)
	out := string(fb.flush(0, 2, 0, true))
	assert.That(t, strings.Contains(out, "\x1b[?25h"), "cursor-show expected when visible; got %q", out)
	assert.That(t, strings.Contains(out, "\x1b[1;3H"), "expected cursor positioned at 1;3; got %q", out)

	out = string(fb.flush(0, 0, 0, false))
	assert.That(t, !strings.Contains(out, "\x1b[?25h"), "no cursor-show when invisible")
}

func TestFramebuf_Clear(t *testing.T) {
	fb := newFramebuf(3, 1)
	_ = fb.flush(0, 0, 0, false)
	fb.set(0, 0, liteCell{mainc: 'X'})
	_ = fb.flush(0, 0, 0, false)

	// After clear, the next flush should overwrite 'X' with a space.
	fb.clear()
	out := string(fb.flush(0, 0, 0, false))
	// Cursor should have moved to col 1 to overwrite 'X'.
	assert.That(t, strings.Contains(out, "\x1b[1;1H"), "expected cursor move to (1,1) after clear; got %q", out)
}

func TestFramebuf_Resize(t *testing.T) {
	fb := newFramebuf(3, 2)
	_ = fb.flush(0, 0, 0, false)
	fb.set(0, 0, liteCell{mainc: 'X'})
	_ = fb.flush(0, 0, 0, false)

	// resize wipes both buffers; next flush re-emits everything as blanks.
	fb.resize(5, 1)
	assert.Equal(t, fb.width, 5)
	assert.Equal(t, fb.height, 1)
	out := string(fb.flush(0, 0, 0, false))
	// After resize, front is sentinel; back is blanks; flush emits 5 spaces.
	assert.Equal(t, strings.Count(out, " "), 5)
}

// --- height resolution -----------------------------------------------------

func TestResolveHeight(t *testing.T) {
	tests := []struct {
		name     string
		h        int
		termH    int
		wantRows int
		wantFull bool
	}{
		{"0 → fullscreen", 0, 24, 24, true},
		{"absolute fits", 10, 24, 10, false},
		{"absolute equals → full", 24, 24, 24, true},
		{"absolute exceeds → full", 100, 24, 24, true},
		{"negative", -2, 24, 22, false},
		{"negative leaves 1", -23, 24, 1, false},
		{"negative cap at 1", -100, 24, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, full := resolveHeight(tt.h, tt.termH)
			assert.Equal(t, rows, tt.wantRows)
			assert.Equal(t, full, tt.wantFull)
		})
	}
}

// stubHooks builds a hooks struct that doesn't touch /dev/tty or signals.
// getSize returns a fixed size; restore/closeIO/notify*/raise are no-ops.
// Tests that need to verify cleanup ran can pass a *bool for restoreCalled.
func stubHooks(termW, termH int, restoreCalled *bool, raised *os.Signal) hooks {
	return hooks{
		enterRaw: func() (func(), error) {
			return func() {
				if restoreCalled != nil {
					*restoreCalled = true
				}
			}, nil
		},
		getSize:     func() (int, int) { return termW, termH },
		closeIO:     func() {},
		notifyWinch: func(chan<- os.Signal) {},
		notifySig:   func(chan<- os.Signal) {},
		stopSignal:  func(chan<- os.Signal) {},
		raiseSignal: func(sig os.Signal) {
			if raised != nil {
				*raised = sig
			}
		},
	}
}

// newTestScreen builds a Screen wired up with stubHooks + bytes.Buffer for
// out + the given input reader. The buffer is returned so tests can assert
// on the output byte stream.
func newTestScreen(height, termW, termH int, in io.Reader) (*Screen, *bytes.Buffer, *bool) {
	var out bytes.Buffer
	var restoreCalled bool
	s := &Screen{
		hooks:  stubHooks(termW, termH, &restoreCalled, nil),
		in:     in,
		out:    &out,
		height: height,
	}
	return s, &out, &restoreCalled
}

func TestScreen_InitFini_Inline(t *testing.T) {
	s, out, restoreCalled := newTestScreen(5, 80, 24, strings.NewReader(""))

	err := s.Init()
	assert.NoError(t, err)
	got := out.String()
	assert.That(t, strings.Contains(got, "\x1b[?25l"), "init hides cursor")
	assert.That(t, strings.Contains(got, "\x1b[20;1H\x1b[J"), "init clears region at row 20")
	assert.Equal(t, s.yOrigin, 19)
	assert.Equal(t, s.fb.height, 5)
	assert.That(t, !*restoreCalled, "restore must not run before Fini")

	out.Reset()
	s.Fini()
	final := out.String()
	assert.That(t, strings.Contains(final, "\x1b[20;1H\x1b[J"), "fini clears region")
	assert.That(t, strings.Contains(final, "\x1b[?25h"), "fini shows cursor")
	assert.That(t, *restoreCalled, "fini must restore raw mode")
}

func TestScreen_InitFini_Fullscreen(t *testing.T) {
	s, out, restoreCalled := newTestScreen(0, 80, 24, strings.NewReader(""))

	assert.NoError(t, s.Init())
	got := out.String()
	assert.That(t, strings.Contains(got, "\x1b[?1049h"), "fullscreen enters alt-screen")
	assert.Equal(t, s.yOrigin, 0)

	out.Reset()
	s.Fini()
	final := out.String()
	assert.That(t, strings.Contains(final, "\x1b[?1049l"), "fullscreen exits alt-screen on Fini")
	assert.That(t, *restoreCalled, "fini restores raw mode")
}

func TestScreen_HandleResize_Narrow(t *testing.T) {
	s, out, _ := newTestScreen(5, 100, 24, strings.NewReader(""))
	assert.NoError(t, s.Init())

	// Simulate terminal narrowing: swap getSize to report new width.
	s.getSize = func() (int, int) { return 60, 24 }
	out.Reset()

	w, rows := s.handleResize()
	assert.Equal(t, w, 60)
	assert.Equal(t, rows, 5)
	got := out.String()
	assert.That(t, strings.Contains(got, "\x1b[2J"), "narrow triggers full-clear")

	s.Fini()
}

// --- init/fini/resize byte sequences ---------------------------------------
//
// These exercise the pure byte composition extracted from Init/Fini/
// handleResize so the actual emission can be verified without /dev/tty,
// raw mode, or signal handlers.

func TestInitSequence_Fullscreen(t *testing.T) {
	out, yOrigin, fb := initSequence(0, 80, 24)
	got := string(out)
	assert.Equal(t, yOrigin, 0)
	assert.Equal(t, fb.width, 80)
	assert.Equal(t, fb.height, 24)
	assert.That(t, strings.Contains(got, "\x1b[?1049h"), "expected smcup; got %q", got)
	assert.That(t, strings.Contains(got, "\x1b[2J"), "expected screen clear; got %q", got)
	assert.That(t, strings.Contains(got, "\x1b[H"), "expected cursor home; got %q", got)
	assert.That(t, strings.Contains(got, "\x1b[?25l"), "expected hide cursor; got %q", got)
}

func TestInitSequence_Inline(t *testing.T) {
	out, yOrigin, fb := initSequence(5, 80, 24)
	got := string(out)
	assert.Equal(t, yOrigin, 19) // 24-5
	assert.Equal(t, fb.width, 80)
	assert.Equal(t, fb.height, 5)
	assert.Equal(t, strings.Count(got, "\n"), 4) // rows-1 newlines
	assert.That(t, strings.Contains(got, "\x1b[?25l"), "expected hide cursor; got %q", got)
	assert.That(t, strings.Contains(got, "\x1b[20;1H\x1b[J"), "expected region clear at row 20; got %q", got)
	// Inline must NOT enter alt-screen — that would clobber prior output.
	assert.That(t, !strings.Contains(got, "\x1b[?1049h"), "inline must not enter alt-screen")
}

func TestInitSequence_NegativeHeight(t *testing.T) {
	out, yOrigin, fb := initSequence(-2, 80, 24)
	got := string(out)
	assert.Equal(t, fb.height, 22)         // termH + height = 24 + (-2)
	assert.Equal(t, yOrigin, 2)            // termH - rows = 24 - 22
	assert.That(t, strings.Contains(got, "\x1b[3;1H\x1b[J"), "expected region clear at row 3; got %q", got)
}

func TestFiniSequence(t *testing.T) {
	full := string(finiSequence(0))
	assert.That(t, strings.Contains(full, "\x1b[?1049l"), "fullscreen fini emits rmcup; got %q", full)
	assert.That(t, strings.Contains(full, "\x1b[?25h"), "fullscreen fini shows cursor; got %q", full)

	inline := string(finiSequence(19))
	assert.That(t, strings.Contains(inline, "\x1b[20;1H\x1b[J"), "inline fini clears region at yOrigin+1; got %q", inline)
	assert.That(t, strings.Contains(inline, "\x1b[?25h"), "inline fini shows cursor; got %q", inline)
	// Inline must NOT leave alt-screen — we never entered it.
	assert.That(t, !strings.Contains(inline, "\x1b[?1049l"), "inline fini must not emit rmcup")
}

func TestResizeSequence_Fullscreen(t *testing.T) {
	out, yOrigin, rows := resizeSequence(0, 100, 30, 80)
	got := string(out)
	assert.Equal(t, yOrigin, 0)
	assert.Equal(t, rows, 30)
	assert.That(t, strings.Contains(got, "\x1b[2J"), "fullscreen resize clears screen")
	assert.That(t, strings.Contains(got, "\x1b[H"), "fullscreen resize homes cursor")
}

func TestResizeSequence_InlineExpand(t *testing.T) {
	// width grew (60 → 80). No \x1b[2J — preserve content above.
	out, yOrigin, rows := resizeSequence(5, 80, 24, 60)
	got := string(out)
	assert.Equal(t, yOrigin, 19)
	assert.Equal(t, rows, 5)
	assert.That(t, !strings.Contains(got, "\x1b[2J"), "expand must not full-clear")
	assert.That(t, strings.Contains(got, "\x1b[20;1H\x1b[J"), "expand clears region")
}

func TestResizeSequence_InlineNarrow(t *testing.T) {
	// width shrank (100 → 60) → wipe viewport.
	out, yOrigin, rows := resizeSequence(5, 60, 24, 100)
	got := string(out)
	assert.Equal(t, yOrigin, 19)
	assert.Equal(t, rows, 5)
	assert.That(t, strings.Contains(got, "\x1b[2J"), "narrow must full-clear viewport")
	assert.That(t, strings.Contains(got, "\x1b[20;1H\x1b[J"), "narrow also clears region")
}

func TestResizeSequence_InlineSameWidth(t *testing.T) {
	// Width unchanged (80 == 80) → minimal clear.
	out, yOrigin, rows := resizeSequence(5, 80, 24, 80)
	got := string(out)
	assert.Equal(t, yOrigin, 19)
	assert.Equal(t, rows, 5)
	assert.That(t, !strings.Contains(got, "\x1b[2J"), "same-width must not full-clear")
}

// --- framebuf yOrigin ------------------------------------------------------

func TestFramebuf_FlushWithYOrigin(t *testing.T) {
	// In inline mode the framebuf row 0 maps to absolute terminal row
	// yOrigin. Cell at (0,0) with yOrigin=20 must emit cursor positioning
	// to row 21 (1-indexed).
	fb := newFramebuf(3, 1)
	_ = fb.flush(20, 0, 0, false) // sync front

	fb.set(0, 0, liteCell{mainc: 'X'})
	out := string(fb.flush(20, 0, 0, false))
	assert.That(t, strings.Contains(out, "\x1b[21;1H"), "expected cursor at row 21; got %q", out)
}

func TestUTF8ContinuationCount(t *testing.T) {
	tests := []struct {
		b    byte
		want int
	}{
		{0x7f, 0}, // 1-byte ASCII (no continuation needed; lead invalid for multibyte)
		{0xc2, 1}, // 2-byte lead
		{0xe3, 2}, // 3-byte lead
		{0xf0, 3}, // 4-byte lead
		{0xff, 0}, // invalid
		{0x80, 0}, // continuation byte, not a lead
	}
	for _, tc := range tests {
		assert.Equal(t, utf8ContinuationCount(tc.b), tc.want)
	}
}
