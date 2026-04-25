package fuzzyfinder_test

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	fuzzyfinder "github.com/lczyk/gitgum/src/fuzzyfinder"
)

func TestFind_WithMatcher(t *testing.T) {
	t.Parallel()

	t.Run("rejects all returns abort", func(t *testing.T) {
		t.Parallel()
		// "a" fuzzy-matches "apple" and "banana"; custom matcher rejects all → abort
		f, term := fuzzyfinder.NewWithMockedTerminal()
		term.SetEvents(append(
			runes("a"),
			key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}),
		)...)

		_, err := f.Find(
			[]string{"apple", "banana", "cherry"},
			fuzzyfinder.WithMatcher(func(_, _ string) bool { return false }),
		)
		assert.Error(t, err, fuzzyfinder.ErrAbort)
	})

	t.Run("selects matched item", func(t *testing.T) {
		t.Parallel()
		// HasPrefix "apple" matches only "apple" (idx 0), not "pineapple" or "apricot"
		f, term := fuzzyfinder.NewWithMockedTerminal()
		term.SetEvents(append(
			runes("apple"),
			key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}),
		)...)

		idx, err := f.Find(
			[]string{"apple", "pineapple", "apricot"},
			fuzzyfinder.WithMatcher(func(query, item string) bool {
				return strings.HasPrefix(item, query)
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, 0, idx)
	})
}
