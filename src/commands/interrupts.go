package commands

import (
	"os"
	"os/signal"
	"time"

	"github.com/lczyk/gitgum/internal/ui"
)

// interrupts catches Ctrl-C while a command is changing things, so gg lives to
// undo what it has done -- and to run its deferred cleanups, the auto-stash pop
// above all -- instead of dying part-way; git, sent the same signal, still
// stops. Every command takes one before its first write and holds it until the
// last; a prompt never sees the signal, since the picker owns the terminal in
// raw mode and reports Ctrl-C as a cancel itself.
type interrupts struct {
	ch  chan os.Signal
	hit bool
}

func catchInterrupts() *interrupts {
	i := &interrupts{ch: make(chan os.Signal, 1)}
	signal.Notify(i.ch, os.Interrupt)
	return i
}

func (i *interrupts) stop() { signal.Stop(i.ch) }

// seen reports whether a Ctrl-C has arrived since catching began.
func (i *interrupts) seen() bool {
	if !i.hit {
		select {
		case <-i.ch:
			i.hit = true
		default:
		}
	}
	return i.hit
}

// or turns a failure into a cancel when a Ctrl-C caused it. The signal can
// trail the failure it caused by a moment, so it is given one.
func (i *interrupts) or(err error) error {
	if err == nil {
		return nil
	}
	if !i.hit {
		select {
		case <-i.ch:
			i.hit = true
		case <-time.After(100 * time.Millisecond):
		}
	}
	if i.hit {
		return ui.ErrCancelled
	}
	return err
}
