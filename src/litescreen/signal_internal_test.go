package litescreen

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// A SIGINT/SIGTERM from outside (Ctrl-C arrives as a byte in raw mode) must
// hand the terminal back before the process dies: restore raw mode, emit the
// fini sequence, then re-raise so the default handler sets the exit status.
func TestScreen_SignalRestoresThenReraises(t *testing.T) {
	s, out, restoreCalled := newTestScreen(5, 80, 24, strings.NewReader(""))
	raised := make(chan os.Signal, 1)
	var stopped []chan<- os.Signal
	s.stopSignal = func(ch chan<- os.Signal) { stopped = append(stopped, ch) }
	s.raiseSignal = func(sig os.Signal) { raised <- sig }
	require.NoError(t, s.Init())
	out.Reset()

	s.sigCh <- syscall.SIGTERM
	select {
	case sig := <-raised:
		assert.Equal(t, sig, os.Signal(syscall.SIGTERM))
	case <-time.After(2 * time.Second):
		t.Fatal("signal was never re-raised")
	}

	// The channel send above happens-before the receive, and cleanup runs
	// before raiseSignal, so these reads are ordered after it.
	assert.That(t, *restoreCalled, "raw mode must be restored before re-raising")
	// Only our own registration is withdrawn before the re-raise, so a
	// handler the consumer registered still gets the signal.
	require.That(t, len(stopped) == 1, "expected one stopSignal call before the re-raise, got %d", len(stopped))
	assert.That(t, stopped[0] == (chan<- os.Signal)(s.sigCh), "the screen must stop its own channel before re-raising")
	assert.ContainsString(t, out.String(), string(finiSequence(false, s.yOrigin)), "fini sequence before re-raise")

	// The terminal is handed back: a draw still in flight must not reach it.
	out.Reset()
	s.SetContent(0, 0, 'x', nil, tcell.StyleDefault)
	s.Show()
	assert.Equal(t, out.String(), "")

	// Fini afterwards (deferred in the caller) is still safe and does not
	// restore twice.
	*restoreCalled = false
	s.Fini()
	assert.That(t, !*restoreCalled, "cleanup must run once")
}

// Fini closes the signal channel, which must end the signal goroutine
// without it treating the close as a signal.
func TestScreen_FiniEndsSignalLoopQuietly(t *testing.T) {
	s, _, _ := newTestScreen(5, 80, 24, strings.NewReader(""))
	raised := make(chan os.Signal, 1)
	s.raiseSignal = func(sig os.Signal) { raised <- sig }
	require.NoError(t, s.Init())

	s.Fini()
	select {
	case sig := <-raised:
		t.Fatalf("Fini must not re-raise anything, got %v", sig)
	case <-time.After(50 * time.Millisecond):
	}
}

// The production re-raise must leave every other handler for the signal in
// place: a consumer that registered its own Notify for SIGINT decides what
// happens next, and the process lives if it wants it to. signal.Reset would
// drop that handler and the re-raise would kill the process. This test is
// also that consumer: with its own handler registered, the SIGINT the hook
// sends to the test process is caught here instead of ending the run.
func TestRealHooks_ReraiseReachesOtherHandlers(t *testing.T) {
	mine := make(chan os.Signal, 1)
	signal.Notify(mine, syscall.SIGINT)
	defer signal.Stop(mine)

	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	require.NoError(t, err)
	defer devnull.Close()
	h := realHooks(devnull, devnull)

	ours := make(chan os.Signal, 1)
	h.notifySig(ours)
	h.stopSignal(ours)
	h.raiseSignal(syscall.SIGINT)

	select {
	case sig := <-mine:
		assert.Equal(t, sig, os.Signal(syscall.SIGINT))
	case <-time.After(2 * time.Second):
		t.Fatal("the re-raised signal never reached the other handler")
	}
	select {
	case sig := <-ours:
		t.Fatalf("stopped channel must not receive, got %v", sig)
	case <-time.After(50 * time.Millisecond):
	}
}
