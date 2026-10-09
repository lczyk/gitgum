package litescreen

import (
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// resizableScreen is a test screen whose reported terminal size can change
// under it, with an input that never EOFs so ChannelEvents stays up.
func resizableScreen(t *testing.T, height, termW, termH int) (s *Screen, setSize func(w, h int)) {
	t.Helper()
	var mu sync.Mutex
	w, h := termW, termH
	s, _, _ = newTestScreen(height, termW, termH, strings.NewReader(""))
	s.input = &fakeInputSource{} // always errNoData: readLoop polls until quit
	s.getSize = func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		return w, h
	}
	require.NoError(t, s.Init())
	return s, func(nw, nh int) {
		mu.Lock()
		defer mu.Unlock()
		w, h = nw, nh
	}
}

func recvResize(t *testing.T, events <-chan tcell.Event) (int, int) {
	t.Helper()
	select {
	case ev := <-events:
		er, ok := ev.(*tcell.EventResize)
		require.That(t, ok, "expected *EventResize, got %T", ev)
		return er.Size()
	case <-time.After(2 * time.Second):
		t.Fatal("no resize event")
	}
	return 0, 0
}

// SIGWINCH on an inline picker reports the new width with the picker's own
// row count, not the terminal's, and keeps the region anchored where it was.
func TestScreen_WinchInlineKeepsRowsAndAnchor(t *testing.T) {
	s, setSize := resizableScreen(t, 5, 80, 24)
	defer s.Fini()
	events := make(chan tcell.Event, 4)
	quit := make(chan struct{})
	defer close(quit)
	go s.ChannelEvents(events, quit)

	out := s.out.(interface{ String() string })
	setSize(100, 30)
	s.winch <- syscall.SIGWINCH
	w, h := recvResize(t, events)
	assert.Equal(t, w, 100)
	assert.Equal(t, h, 5)
	sw, sh := s.Size()
	assert.Equal(t, sw, 100)
	assert.Equal(t, sh, 5)
	// Bottom-anchored at Init on a 24-row terminal: yOrigin 19, which still
	// fits in 30 rows, so the region does not move.
	s.mu.Lock()
	got := out.String()
	s.mu.Unlock()
	assert.ContainsString(t, got, "\x1b[20;1H\x1b[J", "region re-cleared at the kept anchor; got %q", got)
}

// A fullscreen picker follows the terminal's full new size.
func TestScreen_WinchFullscreenTracksTerminal(t *testing.T) {
	s, setSize := resizableScreen(t, 0, 80, 24)
	defer s.Fini()
	events := make(chan tcell.Event, 4)
	quit := make(chan struct{})
	defer close(quit)
	go s.ChannelEvents(events, quit)

	setSize(100, 30)
	s.winch <- syscall.SIGWINCH
	w, h := recvResize(t, events)
	assert.Equal(t, w, 100)
	assert.Equal(t, h, 30)
}

// After a resize the whole grid is unknown, so the next Show repaints every
// cell instead of trusting the pre-resize front buffer.
func TestScreen_ShowAfterWinchRepaintsEverything(t *testing.T) {
	s, setSize := resizableScreen(t, 2, 4, 24)
	defer s.Fini()
	events := make(chan tcell.Event, 4)
	quit := make(chan struct{})
	defer close(quit)
	go s.ChannelEvents(events, quit)

	s.SetContent(0, 0, 'x', nil, tcell.StyleDefault)
	s.Show()
	setSize(4, 30)
	s.winch <- syscall.SIGWINCH
	recvResize(t, events)

	out := s.out.(interface {
		String() string
		Reset()
	})
	s.mu.Lock()
	out.Reset()
	s.mu.Unlock()
	s.SetContent(0, 0, 'x', nil, tcell.StyleDefault)
	s.Show()
	s.mu.Lock()
	got := out.String()
	s.mu.Unlock()
	assert.Equal(t, strings.Count(got, "\x1b[m"), 1+1, "one SGR reset per emitted cell plus the frame's own; got %q", got)
	assert.Equal(t, strings.Count(got, "H"), 4*2, "every cell of a 4x2 grid gets a cursor move; got %q", got)
}
