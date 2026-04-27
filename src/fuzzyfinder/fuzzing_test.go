//go:build fuzz
// +build fuzz

// cspell:ignore fuzzout tbkeys

package fuzzyfinder_test

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"
	fuzz "github.com/google/gofuzz"
	"github.com/lczyk/assert"
	fuzzyfinder "github.com/lczyk/gitgum/src/fuzzyfinder"
)

var (
	out       = flag.String("fuzzout", "fuzz.out", "fuzzing error cases")
	hotReload = flag.Bool("hotreload", false, "enable hot-reloading")
	numCases  = flag.Int("numCases", 30, "number of test cases")
	numEvents = flag.Int("numEvents", 10, "number of events")
)

// TestFuzz executes fuzzing tests.
//
// Example:
//
//	go test -tags fuzz -run TestFuzz -numCases 10 -numEvents 10
func TestFuzz(t *testing.T) {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789一花二乃三玖四葉五月")
	tbkeys := []tcell.Key{
		tcell.KeyCtrlA, tcell.KeyCtrlB, tcell.KeyCtrlE, tcell.KeyCtrlF,
		tcell.KeyBackspace, tcell.KeyTab, tcell.KeyCtrlJ, tcell.KeyCtrlK,
		tcell.KeyCtrlN, tcell.KeyCtrlP, tcell.KeyCtrlU, tcell.KeyCtrlW,
		tcell.KeyBackspace2, tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
	}
	keyMap := map[tcell.Key]string{
		tcell.KeyCtrlA: "A", tcell.KeyCtrlB: "B", tcell.KeyCtrlE: "E", tcell.KeyCtrlF: "F",
		tcell.KeyBackspace: "backspace", tcell.KeyTab: "tab",
		tcell.KeyCtrlJ: "J", tcell.KeyCtrlK: "K", tcell.KeyCtrlN: "N", tcell.KeyCtrlP: "P",
		tcell.KeyCtrlU: "U", tcell.KeyCtrlW: "W", tcell.KeyBackspace2: "backspace2",
		tcell.KeyUp: "up", tcell.KeyDown: "down", tcell.KeyLeft: "left", tcell.KeyRight: "right",
	}

	f, err := os.Create(*out)
	assert.NoError(t, err, "create fuzz output file")
	defer f.Close()

	fuzzer := fuzz.New()

	for i := 0; i < rand.Intn(*numCases)+10; i++ {
		n := rand.Intn(*numEvents)
		events := make([]tcell.Event, n)
		for j := 0; j < n; j++ {
			if rand.Intn(10) > 3 {
				events[j] = ch(letters[rand.Intn(len(letters))])
			} else {
				k := tbkeys[rand.Intn(len(tbkeys))]
				events[j] = key(input{k, rune(k), tcell.ModNone})
			}
		}

		var name string
		for _, e := range events {
			ek := e.(*tcell.EventKey)
			if ek.Rune() != 0 {
				name += string(ek.Rune())
			} else {
				name += "[" + keyMap[ek.Key()] + "]"
			}
		}

		t.Run(name, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					fmt.Fprintln(f, name)
					t.Errorf("panicked: %s", name)
				}
			}()

			var mu sync.Mutex
			items := trackNames()

			finder, term := fuzzyfinder.NewWithMockedTerminal()
			term.SetEvents(append(events, key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))...)

			var promptStr, header string
			fuzzer.Fuzz(&promptStr)
			fuzzer.Fuzz(&header)
			opt := fuzzyfinder.Opt{Prompt: promptStr, Header: header}

			if *hotReload {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go func() {
					for {
						select {
						case <-ctx.Done():
							return
						default:
							var s string
							fuzzer.Fuzz(&s)
							mu.Lock()
							items = append(items, s)
							mu.Unlock()
						}
					}
				}()
				_, err := finder.Find(ctx, &items, &mu, opt)
				assert.Error(t, err, fuzzyfinder.ErrAbort)
			} else {
				_, err := finder.Find(context.Background(), &items, nil, opt)
				assert.Error(t, err, fuzzyfinder.ErrAbort)
			}
		})
	}
}
