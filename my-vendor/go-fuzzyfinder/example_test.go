package fuzzyfinder_test

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	fuzzyfinder "github.com/ktr0731/go-fuzzyfinder"
)

func ExampleFind() {
	slice := []struct {
		id   string
		name string
	}{
		{"id1", "foo"},
		{"id2", "bar"},
		{"id3", "baz"},
	}
	idx, _ := fuzzyfinder.Find(slice, func(i int) string {
		return fmt.Sprintf("[%s] %s", slice[i].id, slice[i].name)
	})
	fmt.Println(slice[idx])
}

func ExampleFind_previewWindow() {
	slice := []struct {
		id   string
		name string
	}{
		{"id1", "foo"},
		{"id2", "bar"},
		{"id3", "baz"},
	}
	idx, _ := fuzzyfinder.Find(
		slice,
		func(i int) string {
			return fmt.Sprintf("[%s] %s", slice[i].id, slice[i].name)
		},
		fuzzyfinder.WithPreviewWindow(func(i, width, _ int) string {
			if i == -1 {
				return "no results"
			}
			s := fmt.Sprintf("%s is selected", slice[i].name)
			if width < len([]rune(s)) {
				return slice[i].name
			}
			return s
		}))
	fmt.Println(slice[idx])
}

func ExampleFindMulti() {
	slice := []struct {
		id   string
		name string
	}{
		{"id1", "foo"},
		{"id2", "bar"},
		{"id3", "baz"},
	}
	idxs, _ := fuzzyfinder.FindMulti(slice, func(i int) string {
		return fmt.Sprintf("[%s] %s", slice[i].id, slice[i].name)
	})
	for _, idx := range idxs {
		fmt.Println(slice[idx])
	}
}

func ExampleTerminalMock() {
	// NewWithMockedTerminal returns the finder and the mock — use f.Find so
	// the mock terminal is the one actually receiving events.
	f, term := fuzzyfinder.NewWithMockedTerminal()
	term.SetEvents(
		tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'o', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'o', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone),
	)

	slice := []string{"foo", "bar", "baz"}
	f.Find(slice, func(i int) string { return slice[i] })

	term.GetResult() // compare against golden files in real tests
}
