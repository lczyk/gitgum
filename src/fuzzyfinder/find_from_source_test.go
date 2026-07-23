package fuzzyfinder_test

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

func TestFind_BasicEnter(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	term.SetEvents(key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}))

	src := ff.NewSliceSourceFrom([]string{"alpha", "beta", "gamma"})
	res, err := f.Find(context.Background(), src, ff.Opt{})
	require.NoError(t, err)
	assert.EqualArrays(t, res.Items, []string{"alpha"})
}

func TestFind_QuerySelectsMatch(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	events := append(runes("gam"), key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}))
	term.SetEvents(events...)

	src := ff.NewSliceSourceFrom([]string{"alpha", "beta", "gamma"})
	res, err := f.Find(context.Background(), src, ff.Opt{})
	require.NoError(t, err)
	assert.EqualArrays(t, res.Items, []string{"gamma"})
	assert.Equal(t, "gam", res.Query)
}

func TestFind_AbortReturnsErrAbort(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	term.SetEvents(key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))

	src := ff.NewSliceSourceFrom([]string{"a", "b"})
	res, err := f.Find(context.Background(), src, ff.Opt{})
	assert.Error(t, err, ff.ErrAbort)
	assert.Nil(t, res.Items, "items should be nil on abort")
}

func TestFind_NilSourceErrors(t *testing.T) {
	t.Parallel()

	f, _ := ff.NewWithMockedTerminal()
	_, err := f.Find(context.Background(), nil, ff.Opt{})
	assert.Error(t, err, assert.AnyError)
}

func TestFind_SelectOneAfterPopulate(t *testing.T) {
	t.Parallel()

	f, _ := ff.NewWithMockedTerminal()
	src := ff.NewSliceSourceFrom([]string{"only"})

	res, err := f.Find(context.Background(), src, ff.Opt{SelectOne: true})
	require.NoError(t, err)
	assert.EqualArrays(t, res.Items, []string{"only"})
}

// Public-API screen injection: the picker must Init the injected screen,
// wire event delivery itself, and Fini it on exit (picker owns the
// lifecycle), unlike the NewWithMockedTerminal test seam where the helper
// retains ownership. Uses the package-level Find so nothing reaches the
// finder internals.
func TestFind_InjectedScreen(t *testing.T) {
	t.Parallel()

	scr := &injectScreen{}
	src := ff.NewSliceSourceFrom([]string{"alpha", "beta"})
	res, err := ff.Find(context.Background(), src, ff.Opt{Screen: scr})
	require.NoError(t, err)
	assert.EqualArrays(t, res.Items, []string{"alpha"})
	assert.That(t, scr.inited, "picker should Init the injected screen")
	assert.That(t, scr.finied, "picker should Fini the injected screen")
}

// injectScreen is a minimal Screen fake: draws go nowhere, and the event
// pump yields a single Enter so the run confirms the first item. (A
// tcell.SimulationScreen can't serve here: its Init recreates the event
// channel, dropping events injected before the picker-owned Init.)
type injectScreen struct {
	inited bool
	finied bool
}

func (s *injectScreen) Init() error      { s.inited = true; return nil }
func (s *injectScreen) Fini()            { s.finied = true }
func (s *injectScreen) Size() (int, int) { return 60, 10 }
func (s *injectScreen) Clear()           {}
func (s *injectScreen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
}
func (s *injectScreen) ShowCursor(x, y int) {}
func (s *injectScreen) Show()               {}
func (s *injectScreen) Sync()               {}

func (s *injectScreen) ChannelEvents(ch chan<- tcell.Event, quit <-chan struct{}) {
	ch <- tcell.NewEventKey(tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone)
}

func TestFind_MultiSelect(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	// Tab toggles selection AND advances the cursor (same convention as
	// existing TestFindMulti). Two Tabs select items 0 and 1.
	term.SetEvents(keys(
		input{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
		input{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
		input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone},
	)...)

	src := ff.NewSliceSourceFrom([]string{"alpha", "beta", "gamma"})
	res, err := f.Find(context.Background(), src, ff.Opt{Multi: true})
	require.NoError(t, err)
	assert.EqualArraysUnordered(t, res.Items, []string{"alpha", "beta"})
}
