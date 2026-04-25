// Package fuzzyfinder provides terminal user interfaces for fuzzy-finding.
//
// Note that, all functions are not goroutine-safe.
package fuzzyfinder

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/fuzzyfinder/matching"
	runewidth "github.com/mattn/go-runewidth"
)

var (
	// ErrAbort is returned from Find* functions if there are no selections.
	ErrAbort   = errors.New("abort")
	errEntered = errors.New("entered")
)

type state struct {
	items      []string           // All item names.
	allMatched []matching.Matched // All items.
	matched    []matching.Matched // Matched items against the input.

	// x is the current index of the prompt line.
	x int
	// cursorX is the position of prompt line.
	// Note that cursorX is the actual width of input runes.
	cursorX int

	// The current index of filtered items (matched).
	// The initial value is 0.
	y int
	// cursorY is the position of item line.
	// Note that the max size of cursorY depends on max height.
	cursorY int

	input []rune

	// selections holds whether a key is selected or not. Each key is
	// an index of an item (Matched.Idx). Each value represents the position
	// which it is selected.
	selection map[int]int
	// selectionIdx holds the next index, which is used to a selection's value.
	selectionIdx int
}

type finder struct {
	term      tcell.Screen
	stateMu   sync.RWMutex
	state     state
	drawTimer *time.Timer
	eventCh   chan struct{}
	opt       *opt
	multi     bool

	termEventsChan <-chan tcell.Event
}

func (f *finder) initFinder(items []string, matched []matching.Matched, opt opt) error {
	if f.term == nil {
		screen, err := tcell.NewScreen()
		if err != nil {
			return fmt.Errorf("failed to new screen: %w", err)
		}
		f.term = screen
		if err := f.term.Init(); err != nil {
			return fmt.Errorf("failed to initialize screen: %w", err)
		}

		eventsChan := make(chan tcell.Event)
		go f.term.ChannelEvents(eventsChan, nil)
		f.termEventsChan = eventsChan
	}

	f.opt = &opt
	f.state = state{}

	if f.multi {
		f.state.selection = map[int]int{}
		f.state.selectionIdx = 1
	}

	f.state.items = items
	f.state.matched = matched
	f.state.allMatched = matched

	if !isInTesting() {
		f.drawTimer = time.AfterFunc(0, func() {
			f.stateMu.Lock()
			f._draw()
			f.stateMu.Unlock()
			f.term.Show()
		})
		f.drawTimer.Stop()
	}
	f.eventCh = make(chan struct{}, 30) // A large value

	if opt.query != "" {
		f.state.input = []rune(opt.query)
		f.state.cursorX = runewidth.StringWidth(opt.query)
		f.state.x = len(opt.query)
		f.filter()
	}

	return nil
}

func (f *finder) updateItems(items []string, matched []matching.Matched) {
	f.stateMu.Lock()
	f.state.items = items
	f.state.matched = matched
	f.state.allMatched = matched
	f.stateMu.Unlock()
	f.eventCh <- struct{}{}
}

// _draw is used from draw with a timer.
func (f *finder) _draw() {
	width, height := f.term.Size()
	f.term.Clear()

	maxWidth := width
	maxHeight := height

	// prompt line
	var promptLinePad int

	for _, r := range f.opt.promptString {
		style := tcell.StyleDefault.
			Foreground(tcell.ColorBlue).
			Background(tcell.ColorDefault)

		f.term.SetContent(promptLinePad, maxHeight-1, r, nil, style)
		promptLinePad++
	}
	var w int
	for _, r := range f.state.input {
		style := tcell.StyleDefault.
			Foreground(tcell.ColorDefault).
			Background(tcell.ColorDefault).
			Bold(true)

		// Add a space between '>' and runes.
		f.term.SetContent(promptLinePad+w, maxHeight-1, r, nil, style)
		w += runewidth.RuneWidth(r)
	}
	f.term.ShowCursor(promptLinePad+f.state.cursorX, maxHeight-1)

	maxHeight--

	// Header line
	if len(f.opt.header) > 0 {
		w = 0
		for _, r := range runewidth.Truncate(f.opt.header, maxWidth-2, "..") {
			style := tcell.StyleDefault.
				Foreground(tcell.ColorGreen).
				Background(tcell.ColorDefault)
			f.term.SetContent(2+w, maxHeight-1, r, nil, style)
			w += runewidth.RuneWidth(r)
		}
		maxHeight--
	}

	// Number line
	for i, r := range fmt.Sprintf("%d/%d", len(f.state.matched), len(f.state.items)) {
		style := tcell.StyleDefault.
			Foreground(tcell.ColorYellow).
			Background(tcell.ColorDefault)

		f.term.SetContent(2+i, maxHeight-1, r, nil, style)
	}
	maxHeight--

	// Item lines
	itemAreaHeight := maxHeight - 1
	// slice from the bottom-most visible item upward
	matched := f.state.matched[f.state.y-f.state.cursorY:]
	words := strings.Fields(string(f.state.input))

	for i, m := range matched {
		if i > itemAreaHeight {
			break
		}
		if i == f.state.cursorY {
			style := tcell.StyleDefault.
				Foreground(tcell.ColorRed).
				Background(tcell.ColorBlack)

			f.term.SetContent(0, maxHeight-1-i, '>', nil, style)
			f.term.SetContent(1, maxHeight-1-i, ' ', nil, style)
		}

		if f.multi {
			if _, ok := f.state.selection[m.Idx]; ok {
				style := tcell.StyleDefault.
					Foreground(tcell.ColorRed).
					Background(tcell.ColorBlack)

				f.term.SetContent(1, maxHeight-1-i, '>', nil, style)
			}
		}

		// Compute positions to highlight for multi-word matching
		var highlightPositions map[int]bool
		itemRunes := []rune(f.state.items[m.Idx])
		lowerItemRunes := []rune(strings.ToLower(f.state.items[m.Idx]))
		for _, word := range words {
			lowerWord := strings.ToLower(word)
			wordRunes := []rune(lowerWord)
			for i := 0; i <= len(lowerItemRunes)-len(wordRunes); i++ {
				match := true
				for k := 0; k < len(wordRunes); k++ {
					if lowerItemRunes[i+k] != wordRunes[k] {
						match = false
						break
					}
				}
				if match {
					if highlightPositions == nil {
						highlightPositions = make(map[int]bool)
					}
					for k := 0; k < len(wordRunes); k++ {
						highlightPositions[i+k] = true
					}
				}
			}
		}

		w := 2
		for j, r := range itemRunes {
			style := tcell.StyleDefault.
				Foreground(tcell.ColorDefault).
				Background(tcell.ColorDefault)
			// Highlight selected strings.
			hasHighlighted := highlightPositions[j]
			if hasHighlighted {
				style = tcell.StyleDefault.
					Foreground(tcell.ColorGreen).
					Background(tcell.ColorDefault)
			}
			if i == f.state.cursorY {
				if hasHighlighted {
					style = tcell.StyleDefault.
						Foreground(tcell.ColorDarkCyan).
						Bold(true).
						Background(tcell.ColorBlack)
				} else {
					style = tcell.StyleDefault.
						Foreground(tcell.ColorYellow).
						Bold(true).
						Background(tcell.ColorBlack)
				}
			}

			rw := runewidth.RuneWidth(r)
			// Shorten item cells.
			if w+rw+2 > maxWidth {
				f.term.SetContent(w, maxHeight-1-i, '.', nil, style)
				f.term.SetContent(w+1, maxHeight-1-i, '.', nil, style)
				break
			} else {
				f.term.SetContent(w, maxHeight-1-i, r, nil, style)
				w += rw
			}
		}
	}
}

func (f *finder) draw(d time.Duration) {
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()

	if isInTesting() {
		// Don't use goroutine scheduling.
		f._draw()
		f.term.Show()
	} else {
		f.drawTimer.Reset(d)
	}
}

// readKey reads a key input.
// It returns ErrAbort if esc, CTRL-C or CTRL-D keys are inputted,
// errEntered in case of enter key, and a context error when the passed
// context is cancelled.
func (f *finder) readKey(ctx context.Context) error {
	f.stateMu.RLock()
	prevInputLen := len(f.state.input)
	f.stateMu.RUnlock()
	defer func() {
		f.stateMu.RLock()
		currentInputLen := len(f.state.input)
		f.stateMu.RUnlock()
		if prevInputLen != currentInputLen {
			f.eventCh <- struct{}{}
		}
	}()

	var e tcell.Event

	select {
	case ee := <-f.termEventsChan:
		e = ee
	case <-ctx.Done():
		return ctx.Err()
	}

	f.stateMu.Lock()
	defer f.stateMu.Unlock()

	_, screenHeight := f.term.Size()
	matchedLinesCount := len(f.state.matched)

	// Max number of lines to scroll by using PgUp and PgDn
	var pageScrollBy = screenHeight - 3

	switch e := e.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyEsc, tcell.KeyCtrlC, tcell.KeyCtrlD:
			return ErrAbort
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if len(f.state.input) == 0 {
				return nil
			}
			if f.state.x == 0 {
				return nil
			}
			x := f.state.x
			f.state.cursorX -= runewidth.RuneWidth(f.state.input[x-1])
			f.state.x--
			f.state.input = append(f.state.input[:x-1], f.state.input[x:]...)
		case tcell.KeyDelete:
			if f.state.x == len(f.state.input) {
				return nil
			}
			x := f.state.x

			f.state.input = append(f.state.input[:x], f.state.input[x+1:]...)
		case tcell.KeyEnter:
			return errEntered
		case tcell.KeyLeft, tcell.KeyCtrlB:
			if f.state.x > 0 {
				f.state.cursorX -= runewidth.RuneWidth(f.state.input[f.state.x-1])
				f.state.x--
			}
		case tcell.KeyRight, tcell.KeyCtrlF:
			if f.state.x < len(f.state.input) {
				f.state.cursorX += runewidth.RuneWidth(f.state.input[f.state.x])
				f.state.x++
			}
		case tcell.KeyCtrlA, tcell.KeyHome:
			f.state.cursorX = 0
			f.state.x = 0
		case tcell.KeyCtrlE, tcell.KeyEnd:
			f.state.cursorX = runewidth.StringWidth(string(f.state.input))
			f.state.x = len(f.state.input)
		case tcell.KeyCtrlW:
			in := f.state.input[:f.state.x]
			inStr := string(in)
			pos := strings.LastIndex(strings.TrimRightFunc(inStr, unicode.IsSpace), " ")
			if pos == -1 {
				f.state.input = []rune{}
				f.state.cursorX = 0
				f.state.x = 0
				return nil
			}
			pos = utf8.RuneCountInString(inStr[:pos])
			newIn := f.state.input[:pos+1]
			f.state.input = newIn
			f.state.cursorX = runewidth.StringWidth(string(newIn))
			f.state.x = len(newIn)
		case tcell.KeyCtrlU:
			f.state.input = f.state.input[f.state.x:]
			f.state.cursorX = 0
			f.state.x = 0
		case tcell.KeyUp, tcell.KeyCtrlK, tcell.KeyCtrlP:
			if f.state.y+1 < matchedLinesCount {
				f.state.y++
			}
			if f.state.cursorY+1 < min(matchedLinesCount, screenHeight-2) {
				f.state.cursorY++
			}
		case tcell.KeyDown, tcell.KeyCtrlJ, tcell.KeyCtrlN:
			if f.state.y > 0 {
				f.state.y--
			}
			if f.state.cursorY-1 >= 0 {
				f.state.cursorY--
			}
		case tcell.KeyPgUp:
			f.state.y += min(pageScrollBy, matchedLinesCount-1-f.state.y)
			maxCursorY := min(screenHeight-3, matchedLinesCount-1)
			f.state.cursorY += min(pageScrollBy, maxCursorY-f.state.cursorY)
		case tcell.KeyPgDn:
			f.state.y -= min(pageScrollBy, f.state.y)
			f.state.cursorY -= min(pageScrollBy, f.state.cursorY)
		case tcell.KeyTab:
			if !f.multi {
				return nil
			}
			idx := f.state.matched[f.state.y].Idx
			if _, ok := f.state.selection[idx]; ok {
				delete(f.state.selection, idx)
			} else {
				f.state.selection[idx] = f.state.selectionIdx
				f.state.selectionIdx++
			}
			if f.state.y > 0 {
				f.state.y--
			}
			if f.state.cursorY > 0 {
				f.state.cursorY--
			}
		default:
			if e.Rune() != 0 {
				width, _ := f.term.Size()
				maxLineWidth := width - 2 - 1
				if len(f.state.input)+1 > maxLineWidth {
					// Discard inputted rune.
					return nil
				}

				x := f.state.x
				f.state.input = append(f.state.input[:x], append([]rune{e.Rune()}, f.state.input[x:]...)...)
				f.state.cursorX += runewidth.RuneWidth(e.Rune())
				f.state.x++
			}
		}
	case *tcell.EventResize:
		f.term.Clear()

		width, height := f.term.Size()
		itemAreaHeight := height - 2 - 1
		if itemAreaHeight >= 0 && f.state.cursorY > itemAreaHeight {
			f.state.cursorY = itemAreaHeight
		}

		maxLineWidth := width - 2 - 1
		if maxLineWidth < 0 {
			f.state.input = nil
			f.state.cursorX = 0
			f.state.x = 0
		} else if len(f.state.input)+1 > maxLineWidth {
			// Discard inputted rune.
			f.state.input = f.state.input[:maxLineWidth]
			f.state.cursorX = runewidth.StringWidth(string(f.state.input))
			f.state.x = maxLineWidth
		}
	}
	return nil
}

func (f *finder) filter() {
	f.stateMu.RLock()
	if len(f.state.input) == 0 {
		f.stateMu.RUnlock()
		f.stateMu.Lock()
		defer f.stateMu.Unlock()
		f.state.matched = f.state.allMatched
		return
	}

	// FindAll may take a lot of time, so it is desired to use RLock to avoid goroutine blocking.
	opts := []matching.Option{matching.WithMode(matching.Mode(f.opt.mode))}
	if f.opt.matcher != nil {
		opts = append(opts, matching.WithMatcher(f.opt.matcher))
	}
	matchedItems := matching.FindAll(string(f.state.input), f.state.items, opts...)
	f.stateMu.RUnlock()

	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	f.state.matched = matchedItems
	if len(f.state.matched) == 0 {
		f.state.cursorY = 0
		f.state.y = 0
		return
	}

	switch {
	case f.state.cursorY >= len(f.state.matched):
		f.state.cursorY = len(f.state.matched) - 1
		f.state.y = len(f.state.matched) - 1
	case f.state.y >= len(f.state.matched):
		f.state.y = len(f.state.matched) - 1
	}
}

// findStatic runs the picker against a fixed slice of items.
func (f *finder) findStatic(items []string, opts []Option, multi bool) ([]int, error) {
	opt := defaultOption
	for _, o := range opts {
		o(&opt)
	}
	f.multi = multi

	matched := makeMatched(len(items))

	parentCtx := context.Background()
	if opt.ctx != nil {
		parentCtx = opt.ctx
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	if err := f.initFinder(items, matched, opt); err != nil {
		return nil, fmt.Errorf("failed to initialize the fuzzy finder: %w", err)
	}
	return f.runLoop(ctx, &opt)
}

// findLive runs the picker against a slice that may grow concurrently;
// lock guards reads of *itemsPtr while items are appended by the caller.
func (f *finder) findLive(itemsPtr *[]string, lock sync.Locker, opts []Option, multi bool) ([]int, error) {
	if itemsPtr == nil {
		return nil, errors.New("items pointer must not be nil")
	}
	if lock == nil {
		return nil, errors.New("lock must not be nil")
	}

	opt := defaultOption
	for _, o := range opts {
		o(&opt)
	}
	f.multi = multi

	parentCtx := context.Background()
	if opt.ctx != nil {
		parentCtx = opt.ctx
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	lock.Lock()
	itemsCopy := append([]string(nil), (*itemsPtr)...)
	lock.Unlock()
	matched := makeMatched(len(itemsCopy))

	inited := make(chan struct{})
	go func() {
		<-inited
		var prev int
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Millisecond):
				lock.Lock()
				curr := len(*itemsPtr)
				if prev != curr {
					itemsCopy = append([]string(nil), (*itemsPtr)...)
					f.updateItems(itemsCopy, makeMatched(curr))
				}
				lock.Unlock()
				prev = curr
			}
		}
	}()

	if err := f.initFinder(itemsCopy, matched, opt); err != nil {
		return nil, fmt.Errorf("failed to initialize the fuzzy finder: %w", err)
	}
	close(inited)
	return f.runLoop(ctx, &opt)
}

func makeMatched(n int) []matching.Matched {
	matched := make([]matching.Matched, n)
	for i := range matched {
		matched[i] = matching.Matched{Idx: i}
	}
	return matched
}

func (f *finder) runLoop(ctx context.Context, opt *opt) ([]int, error) {
	if !isInTesting() {
		defer f.term.Fini()
	}

	if opt.selectOne && len(f.state.matched) == 1 {
		return []int{f.state.matched[0].Idx}, nil
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-f.eventCh:
				f.filter()
				f.draw(0)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			f.draw(10 * time.Millisecond)

			err := f.readKey(ctx)
			// hack for earning time to filter exec
			if isInTesting() {
				time.Sleep(50 * time.Millisecond)
			}
			switch {
			case errors.Is(err, ErrAbort):
				return nil, ErrAbort
			case errors.Is(err, errEntered):
				f.stateMu.RLock()
				defer f.stateMu.RUnlock()

				if len(f.state.matched) == 0 {
					return nil, ErrAbort
				}
				if f.multi {
					if len(f.state.selection) == 0 {
						return []int{f.state.matched[f.state.y].Idx}, nil
					}
					poss, idxs := make([]int, 0, len(f.state.selection)), make([]int, 0, len(f.state.selection))
					for idx, pos := range f.state.selection {
						idxs = append(idxs, idx)
						poss = append(poss, pos)
					}
					sort.Slice(idxs, func(i, j int) bool {
						return poss[i] < poss[j]
					})
					return idxs, nil
				}
				return []int{f.state.matched[f.state.y].Idx}, nil
			case err != nil:
				return nil, fmt.Errorf("failed to read a key: %w", err)
			}
		}
	}
}

// Find displays a fuzzy-finder UI over items and returns the index of the
// selected entry, or ErrAbort if the user cancels.
func Find(items []string, opts ...Option) (int, error) {
	f := &finder{}
	return f.Find(items, opts...)
}

func (f *finder) Find(items []string, opts ...Option) (int, error) {
	res, err := f.findStatic(items, opts, false)
	if err != nil {
		return 0, err
	}
	return res[0], nil
}

// FindMulti behaves like Find but lets the user select multiple items via Tab.
func FindMulti(items []string, opts ...Option) ([]int, error) {
	f := &finder{}
	return f.FindMulti(items, opts...)
}

func (f *finder) FindMulti(items []string, opts ...Option) ([]int, error) {
	return f.findStatic(items, opts, true)
}

// FindLive displays a fuzzy-finder UI over a slice that may grow concurrently.
// Callers append to *items under lock; the picker re-reads on a 30ms cadence.
func FindLive(items *[]string, lock sync.Locker, opts ...Option) (int, error) {
	f := &finder{}
	return f.FindLive(items, lock, opts...)
}

func (f *finder) FindLive(items *[]string, lock sync.Locker, opts ...Option) (int, error) {
	res, err := f.findLive(items, lock, opts, false)
	if err != nil {
		return 0, err
	}
	return res[0], nil
}

// FindMultiLive is the multi-select variant of FindLive.
func FindMultiLive(items *[]string, lock sync.Locker, opts ...Option) ([]int, error) {
	f := &finder{}
	return f.FindMultiLive(items, lock, opts...)
}

func (f *finder) FindMultiLive(items *[]string, lock sync.Locker, opts ...Option) ([]int, error) {
	return f.findLive(items, lock, opts, true)
}

func isInTesting() bool {
	return flag.Lookup("test.v") != nil
}
