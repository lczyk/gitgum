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
	"github.com/lczyk/gitgum/src/fuzzyfinder/litescreen"
)

// TestNew_OpensTty smoke-tests construction. Skips if /dev/tty isn't
// available (CI in fully detached envs).
func TestNew_OpensTty(t *testing.T) {
	if _, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0); err != nil {
		t.Skipf("no /dev/tty: %v", err)
	}
	s, err := litescreen.New(0)
	assert.NoError(t, err)
	assert.That(t, s != nil, "expected non-nil Screen")
	// Don't call Init — that would mess with our test's terminal state.
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
	assert.NoError(t, err)

	assert.NoError(t, s.Init())
	got := out.String()
	assert.That(t, strings.Contains(got, "\x1b[?25l"), "init hides cursor; got %q", got)
	assert.That(t, strings.Contains(got, "\x1b[20;1H\x1b[J"), "init clears region at row 20; got %q", got)
	w, h := s.Size()
	assert.Equal(t, w, 80)
	assert.Equal(t, h, 5)

	out.Reset()
	s.Fini()
	final := out.String()
	assert.That(t, strings.Contains(final, "\x1b[20;1H\x1b[J"), "fini clears region; got %q", final)
}

func TestNewWithOptions_HeadlessFullscreen(t *testing.T) {
	var out bytes.Buffer
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Height: 0, // fullscreen
		Out:    &out,
		Size:   fixedSize(80, 24),
	})
	assert.NoError(t, err)

	assert.NoError(t, s.Init())
	got := out.String()
	assert.That(t, strings.Contains(got, "\x1b[?1049h"), "fullscreen enters alt-screen; got %q", got)

	out.Reset()
	s.Fini()
	final := out.String()
	assert.That(t, strings.Contains(final, "\x1b[?1049l"), "fini exits alt-screen; got %q", final)
}

func TestNewWithOptions_SetContentAndShow(t *testing.T) {
	var out bytes.Buffer
	s, _ := litescreen.NewWithOptions(litescreen.Options{
		Height: 3,
		Out:    &out,
		Size:   fixedSize(20, 10),
	})
	assert.NoError(t, s.Init())
	out.Reset()

	s.SetContent(0, 0, 'X', nil, tcell.StyleDefault)
	s.Show()
	got := out.String()
	// First fb row maps to terminal row yOrigin+1 = (10-3)+1 = 8.
	assert.That(t, strings.Contains(got, "\x1b[8;1H"), "expected cursor move to row 8; got %q", got)
	assert.That(t, strings.Contains(got, "X"), "expected 'X' in output; got %q", got)

	s.Fini()
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
	s, _ := litescreen.NewWithOptions(litescreen.Options{
		In:   strings.NewReader("ab"),
		Out:  &bytes.Buffer{},
		Size: fixedSize(80, 24),
	})
	assert.NoError(t, s.Init())
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
	s, _ := litescreen.NewWithOptions(litescreen.Options{
		In:   strings.NewReader("\x1b[A\x03"), // up arrow, Ctrl-C
		Out:  &bytes.Buffer{},
		Size: fixedSize(80, 24),
	})
	assert.NoError(t, s.Init())
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
	assert.NoError(t, s.Init())
	w, h := s.Size()
	assert.Equal(t, w, 80)
	assert.Equal(t, h, 24)
	s.Fini()
}
