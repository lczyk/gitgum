package fuzzyfinder_test

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	fuzzyfinder "github.com/lczyk/gitgum/src/fuzzyfinder"
)

func ExampleFind() {
	items := []string{"foo", "bar", "baz"}
	idx, _ := fuzzyfinder.Find(items)
	fmt.Println(items[idx])
}

func ExampleFindMulti() {
	items := []string{"foo", "bar", "baz"}
	idxs, _ := fuzzyfinder.FindMulti(items)
	for _, idx := range idxs {
		fmt.Println(items[idx])
	}
}

func ExampleTerminalMock() {
	f, term := fuzzyfinder.NewWithMockedTerminal()
	term.SetEvents(
		tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'o', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'o', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone),
	)

	f.Find([]string{"foo", "bar", "baz"})

	term.GetResult()
}
