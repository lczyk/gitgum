package litescreen

import (
	"os"
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
