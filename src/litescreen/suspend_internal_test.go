package litescreen

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// gatedInput is an inputSource the test feeds by hand. It counts setup and
// teardown so a test can see the reader stop for a suspend and come back on
// resume.
type gatedInput struct {
	mu         sync.Mutex
	queue      []byte
	setupN     int
	teardownN  int
	onTeardown func() // runs inside teardown, on the reader goroutine
}

func (g *gatedInput) push(b ...byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.queue = append(g.queue, b...)
}

func (g *gatedInput) setup() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.setupN++
	return nil
}

func (g *gatedInput) teardown() {
	g.mu.Lock()
	g.teardownN++
	hook := g.onTeardown
	g.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (g *gatedInput) read(buf []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.queue) == 0 {
		return 0, errNoData
	}
	n := copy(buf, g.queue)
	g.queue = g.queue[n:]
	return n, nil
}

func (g *gatedInput) counts() (int, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.setupN, g.teardownN
}

func recvKey(t *testing.T, events <-chan tcell.Event) *tcell.EventKey {
	t.Helper()
	select {
	case ev := <-events:
		ek, ok := ev.(*tcell.EventKey)
		require.That(t, ok, "expected *EventKey, got %T", ev)
		return ek
	case <-time.After(2 * time.Second):
		t.Fatal("no key event")
	}
	return nil
}

func expectNoEvent(t *testing.T, events <-chan tcell.Event, d time.Duration) {
	t.Helper()
	select {
	case ev := <-events:
		t.Fatalf("unexpected event %v", ev)
	case <-time.After(d):
	}
}

// Suspend hands the terminal back the way Fini does (region cleared or alt
// screen left, raw mode restored) but keeps the Screen alive; Resume takes
// the terminal again and the next Show repaints everything from the back
// buffer, including what was drawn into it while suspended.
func TestScreen_SuspendRestoresAndResumeRepaints(t *testing.T) {
	s, out, restoreCalled := newTestScreen(5, 80, 24, strings.NewReader(""))
	s.input = &gatedInput{}
	require.NoError(t, s.Init())
	defer s.Fini()
	s.SetContent(0, 0, 'x', nil, tcell.StyleDefault)
	s.Show()

	out.Reset()
	require.NoError(t, s.Suspend())
	assert.That(t, *restoreCalled, "suspend restores raw mode")
	assert.ContainsString(t, out.String(), string(finiSequence(false, 19)), "suspend clears the region")

	out.Reset()
	s.SetContent(1, 0, 'y', nil, tcell.StyleDefault)
	s.Show()
	s.Sync()
	assert.Equal(t, out.String(), "", "no writes while suspended")

	*restoreCalled = false
	require.NoError(t, s.Resume())
	got := out.String()
	assert.ContainsString(t, got, "\x1b[?25l", "resume hides the cursor again")
	assert.ContainsString(t, got, "\x1b[20;1H\x1b[J", "resume lays the region out again")

	out.Reset()
	s.Show()
	got = out.String()
	assert.ContainsString(t, got, "\x1b[20;1H\x1b[mx", "cell drawn before the suspend is repainted")
	assert.ContainsString(t, got, "\x1b[20;2H", "cell drawn during the suspend is painted")
	assert.That(t, strings.Count(got, "H") >= 80*5, "resume must repaint the whole grid, got %d cells", strings.Count(got, "H"))

	s.Fini()
	assert.That(t, *restoreCalled, "fini after resume restores raw mode")
}

// Input stops while suspended -- the child that borrowed the terminal reads
// it -- and comes back on resume, on the same ChannelEvents.
func TestScreen_SuspendPausesInput(t *testing.T) {
	s, _, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	in := &gatedInput{}
	s.input = in
	require.NoError(t, s.Init())
	defer s.Fini()
	events := make(chan tcell.Event, 8)
	go s.ChannelEvents(events, nil)

	in.push('a')
	assert.Equal(t, recvKey(t, events).Rune(), 'a')

	require.NoError(t, s.Suspend())
	setups, teardowns := in.counts()
	assert.Equal(t, setups, 1)
	assert.Equal(t, teardowns, 1)
	in.push('b')
	expectNoEvent(t, events, 50*time.Millisecond)

	require.NoError(t, s.Resume())
	assert.Equal(t, recvKey(t, events).Rune(), 'b')
	setups, _ = in.counts()
	assert.Equal(t, setups, 2)
}

// Type-ahead that queued while the terminal was borrowed is replayed on
// resume, like the type-ahead Init finds.
func TestScreen_ResumeReplaysTypeAhead(t *testing.T) {
	s, _, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	s.input = &gatedInput{}
	require.NoError(t, s.Init())
	defer s.Fini()
	events := make(chan tcell.Event, 8)
	go s.ChannelEvents(events, nil)

	require.NoError(t, s.Suspend())
	s.queryRow = func() (int, []byte) { return 0, []byte("zq") }
	require.NoError(t, s.Resume())
	assert.Equal(t, recvKey(t, events).Rune(), 'z')
	assert.Equal(t, recvKey(t, events).Rune(), 'q')
}

// An inline region is anchored again from wherever the cursor is after the
// suspend: the child left output behind, so the old anchor means nothing.
func TestScreen_ResumeReanchorsInline(t *testing.T) {
	s, out, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	s.input = &gatedInput{}
	s.queryRow = func() (int, []byte) { return 5, nil }
	require.NoError(t, s.Init())
	defer s.Fini()
	assert.Equal(t, s.yOrigin, 4)

	require.NoError(t, s.Suspend())
	s.queryRow = func() (int, []byte) { return 10, nil }
	out.Reset()
	require.NoError(t, s.Resume())
	assert.Equal(t, s.yOrigin, 9)
	assert.ContainsString(t, out.String(), "\x1b[10;1H\x1b[J")

	out.Reset()
	s.Fini()
	assert.ContainsString(t, out.String(), string(finiSequence(false, 9)), "fini clears the new region")
}

// The terminal may change size while suspended. A SIGWINCH in that window
// must not write anything; Resume measures again, lays out for the new
// size, and reports it as a resize event.
func TestScreen_ResizeWhileSuspended(t *testing.T) {
	s, setSize := resizableScreen(t, 5, 80, 24)
	defer s.Fini()
	events := make(chan tcell.Event, 4)
	go s.ChannelEvents(events, nil)
	out := s.out.(*bytes.Buffer)

	require.NoError(t, s.Suspend())
	out.Reset()
	setSize(100, 30)
	s.winch <- syscall.SIGWINCH
	recvResize(t, events)
	assert.Equal(t, out.String(), "", "no writes while suspended")

	require.NoError(t, s.Resume())
	// The resize event is the sync point: its handler wrote to out before
	// sending it, so out is quiet once it has arrived.
	rw, rh := recvResize(t, events)
	assert.Equal(t, rw, 100)
	assert.Equal(t, rh, 5)
	w, h := s.Size()
	assert.Equal(t, w, 100)
	assert.Equal(t, h, 5)
	assert.ContainsString(t, out.String(), "\x1b[26;1H\x1b[J", "region laid out for the new height")
}

// A signal while suspended has no terminal state to undo: nothing is
// written, raw mode is not restored twice, and the signal is still
// re-raised. The screen is spent afterwards.
func TestScreen_SignalWhileSuspended(t *testing.T) {
	s, out, restoreCalled := newTestScreen(5, 80, 24, strings.NewReader(""))
	s.input = &gatedInput{}
	raised := make(chan os.Signal, 1)
	s.raiseSignal = func(sig os.Signal) { raised <- sig }
	require.NoError(t, s.Init())
	defer s.Fini()

	require.NoError(t, s.Suspend())
	*restoreCalled = false
	out.Reset()
	s.sigCh <- syscall.SIGTERM
	select {
	case <-raised:
	case <-time.After(2 * time.Second):
		t.Fatal("signal was never re-raised")
	}
	assert.Equal(t, out.String(), "")
	assert.That(t, !*restoreCalled, "nothing to restore while suspended")
	assert.Error(t, s.Resume(), assert.AnyError)
}

// Lifecycle edges: Suspend needs a running screen and Resume a suspended
// one; Fini while suspended only releases what is still held.
func TestScreen_SuspendLifecycle(t *testing.T) {
	s, out, restoreCalled := newTestScreen(5, 80, 24, strings.NewReader(""))
	s.input = &gatedInput{}
	closed := 0
	s.closeIO = func() { closed++ }
	assert.Error(t, s.Suspend(), assert.AnyError)
	assert.Error(t, s.Resume(), assert.AnyError)

	require.NoError(t, s.Init())
	assert.Error(t, s.Resume(), assert.AnyError)
	require.NoError(t, s.Suspend())
	assert.Error(t, s.Suspend(), assert.AnyError)

	*restoreCalled = false
	out.Reset()
	s.Fini()
	assert.Equal(t, out.String(), "", "fini while suspended writes nothing")
	assert.That(t, !*restoreCalled, "fini while suspended has no raw mode to restore")
	assert.Equal(t, closed, 1)
	assert.Error(t, s.Resume(), assert.AnyError)
	assert.Error(t, s.Suspend(), assert.AnyError)
}

// Bracketed paste follows the terminal: off while suspended, back on resume.
func TestScreen_SuspendTogglesPasteMode(t *testing.T) {
	s, out, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	s.input = &gatedInput{}
	s.EnablePaste()
	require.NoError(t, s.Init())
	defer s.Fini()

	out.Reset()
	require.NoError(t, s.Suspend())
	assert.ContainsString(t, out.String(), disablePaste)
	out.Reset()
	require.NoError(t, s.Resume())
	assert.ContainsString(t, out.String(), enablePaste)
}

// A Resume whose raw-mode call fails leaves the screen suspended and its
// drawing calls silent; a later Resume may still succeed.
func TestScreen_ResumeRawFailureKeepsSuspendedAndAllowsRetry(t *testing.T) {
	s, out, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	s.input = &gatedInput{}
	s.EnablePaste()
	require.NoError(t, s.Init())
	defer s.Fini()
	require.NoError(t, s.Suspend())
	out.Reset()

	enterRaw := s.enterRaw
	boom := errors.New("raw mode unavailable")
	s.enterRaw = func() (func(), error) { return nil, boom }
	assert.Error(t, s.Resume(), boom)
	assert.That(t, s.suspended, "failed resume must keep the terminal suspended")
	assert.That(t, s.restore == nil, "failed resume must not acquire raw mode")
	assert.That(t, s.readStop == nil, "failed resume must not restart input")
	assert.Equal(t, s.phase, phaseRunning)
	assert.That(t, !s.done, "failed resume must allow a retry")
	s.Show()
	s.Sync()
	s.handleResize()
	assert.Equal(t, out.String(), "")

	s.enterRaw = enterRaw
	require.NoError(t, s.Resume())
	assert.That(t, !s.suspended, "successful retry must take the terminal again")
	assert.That(t, s.readStop != nil, "successful retry must restart input")
	assert.ContainsString(t, out.String(), enablePaste, "retry must restore wanted paste mode")
}

// rawTracker is an enterRaw stub that records whether the terminal is in raw
// mode right now, so a signal hook can see what the re-raise would leave
// behind.
type rawTracker struct{ raw bool }

func (r *rawTracker) enterRaw() (func(), error) {
	r.raw = true
	return func() { r.raw = false }, nil
}

// A signal that lands while Suspend is between marking the screen suspended
// and handing the terminal back must still find the terminal restored when
// it is re-raised: cleanup must not skip the hand-back just because Suspend
// has started it. The signal is injected from the reader's teardown, which
// runs inside Suspend's stopReader.
func TestScreen_SignalInsideSuspendReraisesRestored(t *testing.T) {
	s, _, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	tr := &rawTracker{}
	s.enterRaw = tr.enterRaw
	in := &gatedInput{}
	s.input = in
	rawAtRaise := make(chan bool, 1)
	s.raiseSignal = func(os.Signal) { rawAtRaise <- tr.raw }
	in.onTeardown = func() {
		s.sigCh <- syscall.SIGTERM
		// Give signalLoop its chance to run cleanup in the window.
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(t, s.Init())
	defer s.Fini()

	require.NoError(t, s.Suspend())
	select {
	case raw := <-rawAtRaise:
		assert.That(t, !raw, "re-raised with the terminal still in raw mode")
	case <-time.After(2 * time.Second):
		t.Fatal("signal was never re-raised")
	}
}

// The same window exists in Resume, between entering raw mode and taking
// the terminal under the lock: a signal there sees the screen as suspended,
// restores nothing, and re-raises onto a raw terminal. The signal is
// injected from queryRow, which Resume calls after enterRaw.
func TestScreen_SignalInsideResumeReraisesRestored(t *testing.T) {
	s, _, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	tr := &rawTracker{}
	s.enterRaw = tr.enterRaw
	s.input = &gatedInput{}
	rawAtRaise := make(chan bool, 1)
	s.raiseSignal = func(os.Signal) { rawAtRaise <- tr.raw }
	require.NoError(t, s.Init())
	defer s.Fini()
	require.NoError(t, s.Suspend())

	s.queryRow = func() (int, []byte) {
		s.sigCh <- syscall.SIGTERM
		time.Sleep(20 * time.Millisecond)
		return 0, nil
	}
	require.NoError(t, s.Resume())
	select {
	case raw := <-rawAtRaise:
		assert.That(t, !raw, "re-raised with the terminal still in raw mode")
	case <-time.After(2 * time.Second):
		t.Fatal("signal was never re-raised")
	}
}
