package litescreen_test

// Black-box tests for the litescreen public API. Internal-touching tests
// (parser, framebuf, sequences, hooks) live in screen_internal_test.go and
// run as `package litescreen`.
//
// The public surface is intentionally tiny — `New(height int)` plus
// behavioral methods (Init/Fini/Size/Clear/SetContent/ShowCursor/Show/
// ChannelEvents) — and most behavior requires an actual /dev/tty to
// exercise meaningfully. End-to-end coverage of those methods lives in
// the internal test package via stubHooks.

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/src/litescreen"
)

// TestNew_OpensTty smoke-tests construction. Skips if /dev/tty isn't
// available (CI in fully detached envs).
func TestNew_OpensTty(t *testing.T) {
	if _, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0); err != nil {
		t.Skipf("no /dev/tty: %v", err)
	}
	s, err := litescreen.New(0)
	require.NoError(t, err)
	assert.That(t, s != nil, "expected non-nil Screen")
	// Don't call Init -- that would mess with our test's terminal state.
}

// fixedSize is a Size override that always reports the same dimensions.
func fixedSize(w, h int) func() (int, int) {
	return func() (int, int) { return w, h }
}

func TestNewWithOptions_HeadlessInline(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 5,
		Out:    &out,
		Size:   fixedSize(80, 24),
	})
	require.NoError(t, err)

	require.NoError(t, s.Init())
	got := out.String()
	assert.ContainsString(t, got, "\x1b[?25l", "init hides cursor; got %q", got)
	assert.ContainsString(t, got, "\x1b[20;1H\x1b[J", "init clears region at row 20; got %q", got)
	w, h := s.Size()
	assert.Equal(t, w, 80)
	assert.Equal(t, h, 5)

	out.Reset()
	s.Fini()
	final := out.String()
	assert.ContainsString(t, final, "\x1b[20;1H\x1b[J", "fini clears region; got %q", final)
}

func TestNewWithOptions_HeadlessFullscreen(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 0, // fullscreen
		Out:    &out,
		Size:   fixedSize(80, 24),
	})
	require.NoError(t, err)

	require.NoError(t, s.Init())
	got := out.String()
	assert.ContainsString(t, got, "\x1b[?1049h", "fullscreen enters alt-screen; got %q", got)

	out.Reset()
	s.Fini()
	final := out.String()
	assert.ContainsString(t, final, "\x1b[?1049l", "fini exits alt-screen; got %q", final)
}

func TestNewWithOptions_SetContentAndShow(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	out.Reset()

	s.SetContent(0, 0, 'X', nil, tcell.StyleDefault)
	s.Show()
	got := out.String()
	// First fb row maps to terminal row yOrigin+1 = (10-3)+1 = 8.
	assert.ContainsString(t, got, "\x1b[8;1H", "expected cursor move to row 8; got %q", got)
	assert.ContainsString(t, got, "X", "expected 'X' in output; got %q", got)

	s.Fini()
}

func TestNewWithOptions_SyncForcesFullRepaintAndClearsRegion(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())

	// Initial frame: write a cell and Show. Front buffer now matches back.
	s.SetContent(0, 0, 'X', nil, tcell.StyleDefault)
	s.Show()
	out.Reset()

	// Show again with no SetContent in between -- the diff is empty and the
	// cursor hasn't moved, so nothing at all reaches the terminal. An idle
	// picker redraws on a timer; emitting framing every time would toggle the
	// cursor at that rate.
	s.Show()
	noChange := out.String()
	assert.Equal(t, noChange, "")
	out.Reset()

	// Sync must re-emit every cell (including the unchanged 'X') without
	// an explicit clear-to-end-of-screen — the cell-by-cell repaint covers
	// the picker region on its own.
	s.Sync()
	got := out.String()
	assert.That(t, !strings.Contains(got, "\x1b[J"), "Sync should not clear-to-end; got %q", got)
	assert.ContainsString(t, got, "X", "Sync should re-emit 'X' even though back==front; got %q", got)

	s.Fini()
}

// TestNewWithOptions_FramesAreSynchronized locks the DEC 2026 wrapper on the
// public path: a full repaint is tens of KB, and without an atomic present the
// terminal shows the cursor-hidden intermediate state for as long as those
// bytes take to arrive.
func TestNewWithOptions_FramesAreSynchronized(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())

	for _, step := range []struct {
		name string
		emit func()
	}{
		{"show", func() { s.SetContent(0, 0, 'X', nil, tcell.StyleDefault); s.Show() }},
		{"sync", s.Sync},
	} {
		out.Reset()
		step.emit()
		got := out.String()
		require.That(t, got != "", "%s emitted nothing", step.name)
		assert.That(t, strings.HasPrefix(got, "\x1b[?2026h"), "%s must open with BSU; got %q", step.name, got)
		assert.That(t, strings.HasSuffix(got, "\x1b[?2026l"), "%s must close with ESU; got %q", step.name, got)
	}

	out.Reset()
	s.Fini()
	assert.That(t, strings.HasPrefix(out.String(), "\x1b[?2026l"), "fini must close any open update; got %q", out.String())
}

// Fini hands the terminal back to the shell. Anything still holding the Screen
// -- a redraw a consumer hasn't joined, a signal-driven teardown racing a draw
// -- must not be able to write after that, or its cursor moves and SGR land in
// the middle of whatever the shell prints next.
func TestNewWithOptions_NoWritesAfterFini(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	s.SetContent(0, 0, 'X', nil, tcell.StyleDefault)
	s.Show()

	s.Fini()
	out.Reset()

	// A frame that was already in flight when Fini landed.
	s.Clear()
	s.SetContent(0, 0, 'Y', nil, tcell.StyleDefault)
	s.ShowCursor(1, 1)
	s.Show()
	s.Sync()
	assert.Equal(t, out.String(), "")
}

// recvEvent reads an event with a deadline so tests don't hang if the
// parser never produces one.
func recvEvent(t *testing.T, ch <-chan tcell.Event) tcell.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for event")
		return nil
	}
}

func TestNewWithOptions_ChannelEventsRunes(t *testing.T) {
	// Plain ASCII runes from injected input.
	s, err := litescreen.NewWithOptions(litescreen.Options{
		In: strings.NewReader("ab"),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	defer s.Fini()

	events := make(chan tcell.Event, 4)
	go s.ChannelEvents(events, nil)

	for _, want := range []rune{'a', 'b'} {
		ev := recvEvent(t, events).(*tcell.EventKey)
		assert.Equal(t, ev.Key(), tcell.KeyRune)
		assert.Equal(t, ev.Rune(), want)
	}
}

func TestNewWithOptions_ChannelEventsEscape(t *testing.T) {
	// CSI sequence followed by Ctrl-C. Verifies the parser handles escape
	// sequences end-to-end via the public API.
	s, err := litescreen.NewWithOptions(litescreen.Options{
		In: strings.NewReader("\x1b[A\x03"), // up arrow, Ctrl-C
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	defer s.Fini()

	events := make(chan tcell.Event, 4)
	go s.ChannelEvents(events, nil)

	up := recvEvent(t, events).(*tcell.EventKey)
	assert.Equal(t, up.Key(), tcell.KeyUp)

	cc := recvEvent(t, events).(*tcell.EventKey)
	assert.Equal(t, cc.Key(), tcell.KeyCtrlC)
}

func TestNewWithOptions_DefaultSizeFallback(t *testing.T) {
	// Size unset → falls back to 80x24.
	var out bytes.Buffer
	s, _ := litescreen.NewWithOptions(litescreen.Options{Height: 0, Out: &out})
	require.NoError(t, s.Init())
	w, h := s.Size()
	assert.Equal(t, w, 80)
	assert.Equal(t, h, 24)
	s.Fini()
}

// ShowCursor outside the grid hides the cursor, as it does in tcell, where
// ShowCursor(-1, -1) is the idiom for "no cursor". Emitting the position as
// given would let the terminal clamp it and show the cursor somewhere else.
func TestNewWithOptions_ShowCursorOutsideGridHides(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	defer s.Fini()

	s.ShowCursor(1, 1)
	s.Show()
	assert.ContainsString(t, out.String(), "\x1b[?25h", "in-range cursor is shown")

	for _, xy := range [][2]int{{-1, -1}, {20, 0}, {0, 3}, {-1, 0}} {
		out.Reset()
		s.ShowCursor(xy[0], xy[1])
		s.Show()
		got := out.String()
		assert.That(t, !strings.Contains(got, "\x1b[?25h"),
			"cursor at %v must be hidden, got %q", xy, got)
		s.ShowCursor(1, 1)
		s.Show()
	}
}

func TestNewWithOptions_HideCursor(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	defer s.Fini()

	s.ShowCursor(1, 1)
	s.Show()
	out.Reset()

	s.HideCursor()
	s.Show()
	got := out.String()
	assert.That(t, got != "", "hiding a shown cursor is a change the terminal must hear about")
	assert.That(t, !strings.Contains(got, "\x1b[?25h"), "cursor must stay hidden, got %q", got)

	// Idempotent: nothing changed, nothing to say.
	out.Reset()
	s.HideCursor()
	s.Show()
	assert.Equal(t, out.String(), "")
}

// Before Init there is no grid. Drawing calls are dropped and Size reports
// nothing rather than dereferencing a buffer that does not exist yet.
func TestNewWithOptions_MethodsBeforeInitAreInert(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)

	w, h := s.Size()
	assert.Equal(t, w, 0)
	assert.Equal(t, h, 0)
	s.Clear()
	s.SetContent(0, 0, 'X', nil, tcell.StyleDefault)
	s.ShowCursor(0, 0)
	s.HideCursor()
	s.Show()
	s.Sync()
	assert.Equal(t, out.String(), "")
	s.Fini()
}

// Bracketed paste: with the mode on, the terminal wraps pasted text in
// CSI 200~ and CSI 201~. Those become paste events around the keys of the
// text, so a consumer can tell a pasted newline from a typed Enter.
func TestNewWithOptions_PasteEvents(t *testing.T) {
	s, err := litescreen.NewWithOptions(litescreen.Options{
		In: strings.NewReader("\x1b[200~a\r\x1b[201~"),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	defer s.Fini()

	events := make(chan tcell.Event, 8)
	go s.ChannelEvents(events, nil)

	start, ok := recvEvent(t, events).(*tcell.EventPaste)
	require.That(t, ok, "expected a paste event first")
	assert.That(t, start.Start(), "first event opens the paste")
	assert.Equal(t, recvEvent(t, events).(*tcell.EventKey).Rune(), 'a')
	assert.Equal(t, recvEvent(t, events).(*tcell.EventKey).Key(), tcell.KeyEnter)
	end, ok := recvEvent(t, events).(*tcell.EventPaste)
	require.That(t, ok, "expected a paste event last")
	assert.That(t, end.End(), "last event closes the paste")
}

// EnablePaste turns the terminal mode on, DisablePaste and Fini turn it off.
// Asked for before Init, the mode is switched on by Init.
func TestNewWithOptions_EnablePaste(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{Height: 3, Out: &out})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	assert.That(t, !strings.Contains(out.String(), "\x1b[?2004"), "paste mode is off by default")

	out.Reset()
	s.EnablePaste()
	assert.Equal(t, out.String(), "\x1b[?2004h")
	out.Reset()
	s.DisablePaste()
	assert.Equal(t, out.String(), "\x1b[?2004l")

	out.Reset()
	s.EnablePaste()
	s.Fini()
	assert.ContainsString(t, out.String(), "\x1b[?2004l", "fini turns paste mode off")

	out.Reset()
	s, err = litescreen.NewWithOptions(litescreen.Options{Height: 3, Out: &out})
	require.NoError(t, err)
	s.EnablePaste()
	assert.Equal(t, out.String(), "", "nothing to write before init")
	require.NoError(t, s.Init())
	assert.ContainsString(t, out.String(), "\x1b[?2004h", "init applies the mode asked for earlier")
	s.Fini()
}

// Suspend and Resume through the public surface, headless: the screen can
// be handed back and taken again without a real terminal.
func TestNewWithOptions_SuspendResume(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	require.NoError(t, err)
	require.NoError(t, s.Init())
	defer s.Fini()
	s.SetContent(0, 0, 'X', nil, tcell.StyleDefault)
	s.Show()

	out.Reset()
	require.NoError(t, s.Suspend())
	assert.ContainsString(t, out.String(), "\x1b[8;1H\x1b[J", "region cleared")
	out.Reset()
	s.Show()
	assert.Equal(t, out.String(), "")

	require.NoError(t, s.Resume())
	out.Reset()
	s.Show()
	assert.ContainsString(t, out.String(), "X", "repainted from the back buffer")
}
