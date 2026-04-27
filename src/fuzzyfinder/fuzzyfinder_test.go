package fuzzyfinder_test

// cspell:ignore adrele adrena keeno fname AQUAPLUS ICHIDAIJI обичам Здравей zdravej

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/lczyk/assert"
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

func assertWithGolden(t *testing.T, f func() string) {
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

	actual := f()

	fname := normalizeFilename(name)

	if *update {
		assert.NoError(t, os.WriteFile(fname, []byte(actual), 0600), "update golden")
		return
	}

	b, err := os.ReadFile(fname)
	assert.NoError(t, err, "load golden")
	expected := string(b)
	if runtime.GOOS == "windows" {
		expected = strings.ReplaceAll(expected, "\r\n", "\n")
	}

	diff := cmp.Diff(expected, actual)
	assert.That(t, diff == "", "wrong result: \n%s", diff)
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
	names := trackNames()
	_, err := fuzzyfinder.Find(context.Background(), &names, nil, fuzzyfinder.Opt{})
	assert.NoError(t, err)
}

// makeNumberedItems returns ["item-00", "item-01", ...] of length n. Used
// by paging tests where we want predictable, large-enough item sets.
func makeNumberedItems(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = fmt.Sprintf("item-%02d", i)
	}
	return out
}

func TestFind(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		events []tcell.Event
		opt    fuzzyfinder.Opt
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
		"backspace doesn't change x if cursorX is 0": {
			events: append(runes("a"), keys([]input{
				{tcell.KeyCtrlA, 'A', tcell.ModCtrl},
				{tcell.KeyBackspace, rune(tcell.KeyBackspace), tcell.ModNone},
				{tcell.KeyRight, rune(tcell.KeyRight), tcell.ModNone},
			}...)...),
		},
		"input zdravej": {events: runes("Здравей")},
		"left-right cyrillic": {
			events: append(runes("Здравей"), keys([]input{
				{tcell.KeyLeft, rune(tcell.KeyLeft), tcell.ModNone},
				{tcell.KeyLeft, rune(tcell.KeyLeft), tcell.ModNone},
				{tcell.KeyRight, rune(tcell.KeyRight), tcell.ModNone},
			}...)...),
		},
		"backspace cyrillic": {
			events: append(runes("Здравей"), keys([]input{
				{tcell.KeyBackspace, rune(tcell.KeyBackspace), tcell.ModNone},
				{tcell.KeyBackspace, rune(tcell.KeyBackspace), tcell.ModNone},
			}...)...),
		},
		"ctrl-w cyrillic": {events: append(runes("Аз обичам"), keys(input{tcell.KeyCtrlW, 'W', tcell.ModCtrl})...)},
		"header line":     {opt: fuzzyfinder.Opt{Header: "Search?"}},
		"header line which exceeds max characters": {opt: fuzzyfinder.Opt{Header: "What do you want to search for?"}},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			events := c.events

			f, term := fuzzyfinder.NewWithMockedTerminal()
			events = append(events, key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
			term.SetEvents(events...)

			assertWithGolden(t, func() string {
				names := trackNames()
				_, err := f.Find(context.Background(), &names, nil, c.opt)
				assert.Error(t, err, fuzzyfinder.ErrAbort)

				res := term.GetResult()
				return res
			})
		})
	}
}

// TestFind_pagination exercises page-aligned navigation against a 30-item
// list. With screenHeight=10, pageSize=8 → 4 pages (last is partial).
func TestFind_pagination(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		events []tcell.Event
		opt    fuzzyfinder.Opt
	}{
		"initial": {},
		"pg-dn lands on page 2 first item": {
			// Default mode: PgDn = pageTowardPrompt → wraps backward to last page.
			events: keys(input{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone}),
		},
		"reverse pg-dn first of next page": {
			// Reverse: PgDn → page 1, cursor at item 8 visually on top.
			opt:    fuzzyfinder.Opt{Reverse: true},
			events: keys(input{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone}),
		},
		"reverse up at top of page 1 jumps to last of page 0": {
			opt: fuzzyfinder.Opt{Reverse: true},
			events: keys([]input{
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
				{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
			}...),
		},
		"reverse pg-dn preserves cursor offset": {
			// Down twice (cursorY=2) then PgDn → land on cursorY=2 of page 1
			// (= item 10).
			opt: fuzzyfinder.Opt{Reverse: true},
			events: keys([]input{
				{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone},
				{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone},
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
			}...),
		},
		"reverse ctrl+down behaves as pg-dn": {
			opt:    fuzzyfinder.Opt{Reverse: true},
			events: keys(input{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModCtrl}),
		},
		"reverse pg-dn 4x cycles back to page 1": {
			opt: fuzzyfinder.Opt{Reverse: true},
			events: keys([]input{
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
				{tcell.KeyPgDn, rune(tcell.KeyPgDn), tcell.ModNone},
			}...),
		},
		"reverse pg-up cycles to last partial page": {
			opt:    fuzzyfinder.Opt{Reverse: true},
			events: keys(input{tcell.KeyPgUp, rune(tcell.KeyPgUp), tcell.ModNone}),
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, term := fuzzyfinder.NewWithMockedTerminal()
			events := append(c.events, key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
			term.SetEvents(events...)

			assertWithGolden(t, func() string {
				items := makeNumberedItems(30)
				_, err := f.Find(context.Background(), &items, nil, c.opt)
				assert.Error(t, err, fuzzyfinder.ErrAbort)
				return term.GetResult()
			})
		})
	}
}

func TestFind_hotReload(t *testing.T) {
	t.Parallel()

	f, term := fuzzyfinder.NewWithMockedTerminal()
	events := append(runes("adrena"), key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
	term.SetEvents(events...)

	names := trackNames()
	assertWithGolden(t, func() string {
		_, err := f.Find(
			context.Background(),
			&names,
			&sync.Mutex{},
			fuzzyfinder.Opt{},
		)
		assert.Error(t, err, fuzzyfinder.ErrAbort)

		res := term.GetResult()
		return res
	})
}

func TestFind_hotReloadLock(t *testing.T) {
	t.Parallel()

	f, term := fuzzyfinder.NewWithMockedTerminal()
	events := append(runes("adrena"), key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
	term.SetEvents(events...)

	var mu sync.RWMutex
	names := trackNames()
	assertWithGolden(t, func() string {
		_, err := f.Find(
			context.Background(),
			&names,
			mu.RLocker(),
			fuzzyfinder.Opt{},
		)
		assert.Error(t, err, fuzzyfinder.ErrAbort)

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

			names := trackNames()
			idxs, err := f.Find(context.Background(), &names, nil, fuzzyfinder.Opt{})
			assert.NoError(t, err)
			assert.Equal(t, c.expected, idxs[0])
		})
	}
}

func TestFind_withContext(t *testing.T) {
	t.Parallel()

	f, term := fuzzyfinder.NewWithMockedTerminal()
	events := append(runes("adrena"), key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))
	term.SetEvents(events...)

	cancelledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()

	assertWithGolden(t, func() string {
		names := trackNames()
		_, err := f.Find(cancelledCtx, &names, nil, fuzzyfinder.Opt{})
		assert.Error(t, err, context.Canceled)

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

		assertWithGolden(t, func() string {
			idxs, err := f.Find(context.Background(), &things, nil, fuzzyfinder.Opt{})
			assert.NoError(t, err)
			assert.Equal(t, 0, idxs[0])
			return term.GetResult()
		})
	})

	t.Run("has initial query", func(t *testing.T) {
		f, term := fuzzyfinder.NewWithMockedTerminal()
		term.SetEvents(events...)

		assertWithGolden(t, func() string {
			idxs, err := f.Find(context.Background(), &things, nil, fuzzyfinder.Opt{Query: "three2"})
			assert.NoError(t, err)
			assert.Equal(t, 1, idxs[0])
			return term.GetResult()
		})
	})
}

func TestFind_WithSelectOne(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		things   []string
		query    string
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
			things:   []string{"one", "two"},
			query:    "two",
			expected: 1,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, term := fuzzyfinder.NewWithMockedTerminal()
			term.SetEvents(key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))

			assertWithGolden(t, func() string {
				things := c.things
				idxs, err := f.Find(context.Background(), &things, nil, fuzzyfinder.Opt{
					Query:     c.query,
					SelectOne: true,
				})
				if c.abort {
					assert.Error(t, err, fuzzyfinder.ErrAbort)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, c.expected, idxs[0])
				}
				return term.GetResult()
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
		"input glow": {events: runes("glow"), expected: []int{5}},
		"select two items": {events: keys([]input{
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
		}...), expected: []int{0, 1}},
		"select two items with another order": {events: keys([]input{
			{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
			{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone},
			{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
		}...), expected: []int{1, 0}},
		"toggle": {events: keys([]input{
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
			{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone},
			{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone},
			{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone},
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

			names := trackNames()
			idxs, err := f.Find(context.Background(), &names, nil, fuzzyfinder.Opt{Multi: true})
			if c.abort {
				assert.Error(t, err, fuzzyfinder.ErrAbort)
				return
			}
			assert.NoError(t, err)
			assert.EqualArrays(t, c.expected, idxs)
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

			names := trackNames()
			_, err := f.Find(context.Background(), &names, nil, fuzzyfinder.Opt{})
			assert.Error(b, err, fuzzyfinder.ErrAbort)
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
			_, err := f.Find(context.Background(), &names, &sync.Mutex{}, fuzzyfinder.Opt{})
			assert.Error(b, err, fuzzyfinder.ErrAbort)
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
