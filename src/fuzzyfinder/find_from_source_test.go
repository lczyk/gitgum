package fuzzyfinder_test

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

func TestFindFromSource_BasicEnter(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	term.SetEvents(key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}))

	src := ff.NewSliceSourceFrom([]string{"alpha", "beta", "gamma"})
	got, err := f.FindFromSource(context.Background(), src, ff.Opt{})
	assert.NoError(t, err)
	assert.EqualArrays(t, got, []string{"alpha"})
}

func TestFindFromSource_QuerySelectsMatch(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	events := append(runes("gam"), key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}))
	term.SetEvents(events...)

	src := ff.NewSliceSourceFrom([]string{"alpha", "beta", "gamma"})
	got, err := f.FindFromSource(context.Background(), src, ff.Opt{})
	assert.NoError(t, err)
	assert.EqualArrays(t, got, []string{"gamma"})
}

func TestFindFromSource_AbortReturnsErrAbort(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	term.SetEvents(key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))

	src := ff.NewSliceSourceFrom([]string{"a", "b"})
	got, err := f.FindFromSource(context.Background(), src, ff.Opt{})
	assert.Error(t, err, ff.ErrAbort)
	assert.That(t, got == nil, "got should be nil on abort")
}

func TestFindFromSource_NilSourceErrors(t *testing.T) {
	t.Parallel()

	f, _ := ff.NewWithMockedTerminal()
	_, err := f.FindFromSource(context.Background(), nil, ff.Opt{})
	assert.Error(t, err, assert.AnyError)
}

func TestFindFromSource_SelectOneAfterPopulate(t *testing.T) {
	t.Parallel()

	f, _ := ff.NewWithMockedTerminal()
	src := ff.NewSliceSourceFrom([]string{"only"})

	got, err := f.FindFromSource(context.Background(), src, ff.Opt{SelectOne: true})
	assert.NoError(t, err)
	assert.EqualArrays(t, got, []string{"only"})
}

func TestFindFromSource_MultiSelect(t *testing.T) {
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
	got, err := f.FindFromSource(context.Background(), src, ff.Opt{Multi: true})
	assert.NoError(t, err)
	assert.EqualArraysUnordered(t, got, []string{"alpha", "beta"})
}
