package fuzzyfinder_test

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/google/go-cmp/cmp"
	fuzzyfinder "github.com/lczyk/gitgum/src/fuzzyfinder"
)

var (
	update = flag.Bool("update", false, "update golden files")
	real   = flag.Bool("real", false, "display the actual layout to the terminal")
)

func init() {
	testing.Init()
	flag.Parse()
	if *update {
		if err := os.RemoveAll(filepath.Join("testdata", "fixtures")); err != nil {
			log.Fatalf("RemoveAll should not return an error, but got '%s'", err)
		}
		if err := os.MkdirAll(filepath.Join("testdata", "fixtures"), 0755); err != nil {
			log.Fatalf("MkdirAll should not return an error, but got '%s'", err)
		}
	}
}

func assertWithGolden(t *testing.T, f func(t *testing.T) string) {
	name := t.Name()
	r := strings.NewReplacer(
		"/", "-",
		" ", "_",
		"=", "-",
		"'", "",
		`"`, "",
		",", "",
	)
	normalizeFilename := func(name string) string {
		fname := r.Replace(strings.ToLower(name)) + ".golden"
		return filepath.Join("testdata", "fixtures", fname)
	}

	actual := f(t)

	fname := normalizeFilename(name)

	if *update {
		if err := os.WriteFile(fname, []byte(actual), 0600); err != nil {
			t.Fatalf("failed to update the golden file: %s", err)
		}
		return
	}

	// Load the golden file.
	b, err := os.ReadFile(fname)
	if err != nil {
		t.Fatalf("failed to load a golden file: %s", err)
	}
	expected := string(b)
	if runtime.GOOS == "windows" {
		expected = strings.ReplaceAll(expected, "\r\n", "\n")
	}

	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf("wrong result: \n%s", diff)
	}
}

type track struct {
	Name   string
	Artist string
	Album  string
}

func trackNames() []string {
	out := make([]string, len(tracks))
	for i, t := range tracks {
		out[i] = t.Name
	}
	return out
}

var tracks = []*track{
	{"あの日自分が出て行ってやっつけた時のことをまだ覚えている人の為に", "", ""},
	{"ヒトリノ夜", "ポルノグラフィティ", "ロマンチスト・エゴイスト"},
	{"adrenaline!!!", "TrySail", "TAILWIND"},
	{"ソラニン", "ASIAN KUNG-FU GENERATION", "ソラニン"},
	{"closing", "AQUAPLUS", "WHITE ALBUM2"},
	{"glow", "keeno", "in the rain"},
	{"メーベル", "バルーン", "Corridor"},
	{"ICHIDAIJI", "ポルカドットスティングレイ", "一大事"},
	{"Catch the Moment", "LiSA", "Catch the Moment"},
}

func TestReal(t *testing.T) {
	if !*real {
		t.Skip("--real is disabled")
		return
	}
	_, err := fuzzyfinder.Find(trackNames())
	if err != nil {
		t.Fatalf("err is not nil: %s", err)
	}
}

func TestFind(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		events []tcell.Event
		opts   []fuzzyfinder.Option
	}{
		"initial":    {},
		"input lo":   {events: runes("lo")},
		"input glow": {events: runes("glow")},
		"arrow up-down": {
			events: keys([]input{
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone},
			}...)},
		"arrow left-right": {
			events: append(runes("ゆるふわ樹海"), keys([]input{
				{tcell.KeyLeft, rune(tcell.KeyLeft), tcell.ModNone},
				{tcell.KeyLeft, rune(tcell.KeyLeft), tcell.ModNone},
				{tcell.KeyRight, rune(tcell.KeyRight), tcell.ModNone},
			}...)...),
		},
		"backspace": {
			events: append(runes("adr .-"), keys([]input{
				{tcell.KeyBackspace, rune(tcell.KeyBackspace), tcell.ModNone},
				{tcell.KeyBackspace, rune(tcell.KeyBackspace), tcell.ModNone},
			}...)...),
		},
		"backspace empty": {events: keys(input{tcell.KeyBackspace2, rune(tcell.KeyBackspace2), tcell.ModNone})},
		"backspace2": {
			events: append(runes("オレンジ"), keys([]input{
				{tcell.KeyBackspace2, rune(tcell.KeyBackspace2), tcell.ModNone},
				{tcell.KeyBackspace2, rune(tcell.KeyBackspace2), tcell.ModNone},
			}...)...),
		},
		"arrow left backspace": {
			events: append(runes("オレンジ"), keys([]input{
				{tcell.KeyLeft, rune(tcell.KeyLeft), tcell.ModNone},
				{tcell.KeyBackspace, rune(tcell.KeyBackspace), tcell.ModNone},
			}...)...),
		},
		"delete": {
			events: append(runes("オレンジ"), keys([]input{
				{tcell.KeyCtrlA, 'A', tcell.ModCtrl},
				{tcell.KeyDelete, rune(tcell.KeyDelete), tcell.ModNone},
			}...)...),
		},
		"delete empty": {
			events: keys([]input{
				{tcell.KeyCtrlA, 'A', tcell.ModCtrl},
				{tcell.KeyDelete, rune(tcell.KeyDelete), tcell.ModNone},
			}...),
		},
		"ctrl-e": {
			events: append(runes("恋をしたのは"), keys([]input{
				{tcell.KeyCtrlA, 'A', tcell.ModCtrl},
				{tcell.KeyCtrlE, 'E', tcell.ModCtrl},
			}...)...),
		},
		"ctrl-w":       {events: append(runes("ハロ / ハワユ"), keys(input{tcell.KeyCtrlW, 'W', tcell.ModCtrl})...)},
		"ctrl-w empty": {events: keys(input{tcell.KeyCtrlW, 'W', tcell.ModCtrl})},
		"ctrl-u": {
			events: append(runes("恋をしたのは"), keys([]input{
				{tcell.KeyLeft, rune(tcell.KeyLeft), tcell.ModNone},
				{tcell.KeyCtrlU, 'U', tcell.ModCtrl},
				{tcell.KeyRight, rune(tcell.KeyRight), tcell.ModNone},
			}...)...),
		},
		"pg-up": {
			events: keys([]input{
				{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone},
			}...),
		},
		"pg-up twice": {
			events: keys([]input{
				{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone},
				{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone},
			}...),
		},
		"pg-dn": {
			events: keys([]input{
				{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone},
				{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone},
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
			}...),
		},
		"pg-dn twice": {
			events: keys([]input{
				{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone},
				{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone},
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
			}...),
		},
		"long item": {
			events: keys([]input{
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
			}...),
		},
		"paging": {
			events: keys([]input{
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
			}...),
		},
		"tab doesn't work": {events: keys(input{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone})},
		"backspace doesnt change x if cursorX is 0": {
			events: append(runes("a"), keys([]input{
				{tcell.KeyCtrlA, 'A', tcell.ModCtrl},
				{tcell.KeyBackspace, rune(tcell.KeyBackspace), tcell.ModNone},
				{tcell.KeyCtrlF, 'F', tcell.ModCtrl},
			}...)...),
		},
		"header line": {opts: []fuzzyfinder.Option{fuzzyfinder.WithHeader("Search?")}},
		"header line which exceeds max charaters": {opts: []fuzzyfinder.Option{fuzzyfinder.WithHeader("Waht do you want to search for?")}},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			events := c.events

			f, term := fuzzyfinder.NewWithMockedTerminal()
			events = append(events, key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
			term.SetEvents(events...)

			opts := append(
				c.opts,
				fuzzyfinder.WithMode(fuzzyfinder.ModeCaseSensitive),
			)

			assertWithGolden(t, func(t *testing.T) string {
				_, err := f.Find(trackNames(), opts...)
				if !errors.Is(err, fuzzyfinder.ErrAbort) {
					t.Fatalf("Find must return ErrAbort, but got '%s'", err)
				}

				res := term.GetResult()
				return res
			})
		})
	}
}

func TestFind_hotReload(t *testing.T) {
	t.Parallel()

	f, term := fuzzyfinder.NewWithMockedTerminal()
	events := append(runes("adrena"), keys(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone})...)
	term.SetEvents(events...)

	names := trackNames()
	assertWithGolden(t, func(t *testing.T) string {
		_, err := f.FindLive(
			&names,
			&sync.Mutex{},
			fuzzyfinder.WithMode(fuzzyfinder.ModeCaseSensitive),
		)
		if !errors.Is(err, fuzzyfinder.ErrAbort) {
			t.Fatalf("Find must return ErrAbort, but got '%s'", err)
		}

		res := term.GetResult()
		return res
	})
}

func TestFind_hotReloadLock(t *testing.T) {
	t.Parallel()

	f, term := fuzzyfinder.NewWithMockedTerminal()
	events := append(runes("adrena"), keys(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone})...)
	term.SetEvents(events...)

	var mu sync.RWMutex
	names := trackNames()
	assertWithGolden(t, func(t *testing.T) string {
		_, err := f.FindLive(
			&names,
			mu.RLocker(),
			fuzzyfinder.WithMode(fuzzyfinder.ModeCaseSensitive),
		)
		if !errors.Is(err, fuzzyfinder.ErrAbort) {
			t.Fatalf("Find must return ErrAbort, but got '%s'", err)
		}

		res := term.GetResult()
		return res
	})
}

func TestFind_enter(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		events   []tcell.Event
		expected int
	}{
		"initial":                      {events: keys(input{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone}), expected: 0},
		"mode smart to case-sensitive": {events: runes("JI"), expected: 7},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			events := c.events

			f, term := fuzzyfinder.NewWithMockedTerminal()
			events = append(events, key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}))
			term.SetEvents(events...)

			idx, err := f.Find(trackNames())

			if err != nil {
				t.Fatalf("Find must not return an error, but got '%s'", err)
			}
			if idx != c.expected {
				t.Errorf("expected index: %d, but got %d", c.expected, idx)
			}
		})
	}
}

func TestFind_withContext(t *testing.T) {
	t.Parallel()

	f, term := fuzzyfinder.NewWithMockedTerminal()
	events := append(runes("adrena"), keys(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone})...)
	term.SetEvents(events...)

	cancelledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()

	assertWithGolden(t, func(t *testing.T) string {
		_, err := f.Find(trackNames(), fuzzyfinder.WithContext(cancelledCtx))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Find must return ErrAbort, but got '%s'", err)
		}

		res := term.GetResult()
		return res
	})
}

func TestFind_WithQuery(t *testing.T) {
	t.Parallel()
	var (
		things = []string{"one", "three2one"}
		events = append(runes("one"), key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}))
	)

	t.Run("no initial query", func(t *testing.T) {
		f, term := fuzzyfinder.NewWithMockedTerminal()
		term.SetEvents(events...)

		assertWithGolden(t, func(t *testing.T) string {
			idx, err := f.Find(things)
			if err != nil {
				t.Fatalf("Find must not return an error, but got '%s'", err)
			}
			if idx != 0 {
				t.Errorf("expected index: 0, but got %d", idx)
			}
			res := term.GetResult()
			return res
		})
	})

	t.Run("has initial query", func(t *testing.T) {
		f, term := fuzzyfinder.NewWithMockedTerminal()
		term.SetEvents(events...)

		assertWithGolden(t, func(t *testing.T) string {
			idx, err := f.Find(things, fuzzyfinder.WithQuery("three2"))

			if err != nil {
				t.Fatalf("Find must not return an error, but got '%s'", err)
			}
			if idx != 1 {
				t.Errorf("expected index: 1, but got %d", idx)
			}
			res := term.GetResult()
			return res
		})
	})
}

func TestFind_WithSelectOne(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		things   []string
		moreOpts []fuzzyfinder.Option
		expected int
		abort    bool
	}{
		"only one option": {
			things:   []string{"one"},
			expected: 0,
		},
		"more than one": {
			things: []string{"one", "two"},
			abort:  true,
		},
		"has initial query": {
			things: []string{"one", "two"},
			moreOpts: []fuzzyfinder.Option{
				fuzzyfinder.WithQuery("two"),
			},
			expected: 1,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, term := fuzzyfinder.NewWithMockedTerminal()
			term.SetEvents(key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))

			assertWithGolden(t, func(t *testing.T) string {
				idx, err := f.Find(c.things, append(c.moreOpts, fuzzyfinder.WithSelectOne())...)
				if c.abort {
					if !errors.Is(err, fuzzyfinder.ErrAbort) {
						t.Fatalf("Find must return ErrAbort, but got '%s'", err)
					}
				} else {
					if err != nil {
						t.Fatalf("Find must not return an error, but got '%s'", err)
					}
					if idx != c.expected {
						t.Errorf("expected index: %d, but got %d", c.expected, idx)
					}
				}
				res := term.GetResult()
				return res
			})
		})
	}
}

func TestFindMulti(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		events   []tcell.Event
		expected []int
		abort    bool
	}{
		"input glow": {events: runes("glow"), expected: []int{0}},
		"select two items": {events: keys([]input{
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
			{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
		}...), expected: []int{0, 1}},
		"select two items with another order": {events: keys([]input{
			{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
		}...), expected: []int{1, 0}},
		"toggle": {events: keys([]input{
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
		}...), expected: []int{0}},
		"empty result": {events: runes("fffffff"), abort: true},
		"resize window": {events: []tcell.Event{
			tcell.NewEventResize(10, 10),
		}, expected: []int{0}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			events := c.events

			f, term := fuzzyfinder.NewWithMockedTerminal()
			events = append(events, key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}))
			term.SetEvents(events...)

			idxs, err := f.FindMulti(trackNames())
			if c.abort {
				if !errors.Is(err, fuzzyfinder.ErrAbort) {
					t.Fatalf("Find must return ErrAbort, but got '%s'", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Find must not return an error, but got '%s'", err)
			}
			expectedSelectedNum := len(c.expected)
			if n := len(idxs); n != expectedSelectedNum {
				t.Errorf("expected the number of selected items is %d, but actual %d", expectedSelectedNum, n)
			}
		})
	}
}

func BenchmarkFind(b *testing.B) {
	b.Run("normal", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			f, term := fuzzyfinder.NewWithMockedTerminal()
			events := append(runes("adrele!!"), key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
			term.SetEvents(events...)

			_, err := f.Find(trackNames())
			if !errors.Is(err, fuzzyfinder.ErrAbort) {
				b.Fatalf("expected ErrAbort, but got '%s'", err)
			}
		}
	})

	b.Run("hotreload", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			f, term := fuzzyfinder.NewWithMockedTerminal()
			events := append(runes("adrele!!"), key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
			term.SetEvents(events...)

			names := trackNames()
			_, err := f.FindLive(&names, &sync.Mutex{})
			if !errors.Is(err, fuzzyfinder.ErrAbort) {
				b.Fatalf("expected ErrAbort, but got '%s'", err)
			}
		}
	})
}

func runes(s string) []tcell.Event {
	r := []rune(s)
	e := make([]tcell.Event, 0, len(r))
	for _, r := range r {
		e = append(e, ch(r))
	}
	return e
}

func ch(r rune) tcell.Event {
	return key(input{tcell.KeyRune, r, tcell.ModNone})
}

func key(input input) tcell.Event {
	return tcell.NewEventKey(input.key, input.ch, input.mod)
}

func keys(inputs ...input) []tcell.Event {
	k := make([]tcell.Event, 0, len(inputs))
	for _, in := range inputs {
		k = append(k, key(in))
	}
	return k
}

type input struct {
	key tcell.Key
	ch  rune
	mod tcell.ModMask
}
