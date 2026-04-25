// Package fuzzyfinder provides terminal user interfaces for fuzzy-finding.
//
// Note that, all functions are not goroutine-safe.
package fuzzyfinder

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/fuzzyfinder/litescreen"
	"github.com/lczyk/gitgum/src/fuzzyfinder/matching"
	runewidth "github.com/mattn/go-runewidth"
)

// newScreen returns the screen backend selected by the FF_RENDERER env var.
// Default is tcell (existing alt-screen behavior). FF_RENDERER=lite uses our
// own ANSI renderer — currently fullscreen-only, the foundation for inline
// (--height) mode.
func newScreen() (screen, error) {
	if os.Getenv("FF_RENDERER") == "lite" {
		return litescreen.New()
	}
	return tcell.NewScreen()
}

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

// screen is the subset of tcell.Screen the finder actually uses. Pulling it
// out as a local interface lets us swap in a non-tcell renderer later (e.g.
// an inline ANSI renderer that preserves terminal scrollback) without
// touching the finder logic. tcell.Screen and tcell.SimulationScreen both
// satisfy it implicitly.
type screen interface {
	Init() error
	Fini()
	Size() (int, int)
	Clear()
	SetContent(x, y int, mainc rune, combc []rune, style tcell.Style)
	ShowCursor(x, y int)
	Show()
	ChannelEvents(ch chan<- tcell.Event, quit <-chan struct{})
}

type finder struct {
	term      screen
	stateMu   sync.RWMutex
	state     state
	drawTimer *time.Timer
	eventCh   chan struct{}
	opt       *Opt
	multi     bool

	termEventsChan <-chan tcell.Event
}

func (f *finder) initFinder(items []string, matched []matching.Matched, opt Opt) error {
	if f.term == nil {
		s, err := newScreen()
		if err != nil {
			return fmt.Errorf("failed to new screen: %w", err)
		}
		f.term = s
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

	if opt.Query != "" {
		f.state.input = []rune(opt.Query)
		f.state.cursorX = runewidth.StringWidth(opt.Query)
		f.state.x = len(opt.Query)
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

	// Layout: rows are addressed as offsets from the prompt outward into the
	// item area. step is +1 in reverse mode (prompt at top, items grow down)
	// and -1 otherwise (prompt at bottom, items grow up).
	step := -1
	promptRow := height - 1
	if f.opt.Reverse {
		step = 1
		promptRow = 0
	}
	rowAt := func(offset int) int { return promptRow + step*offset }

	// Prompt line
	var promptLinePad int
	for _, r := range f.opt.Prompt {
		style := tcell.StyleDefault.Foreground(tcell.ColorBlue).Background(tcell.ColorDefault)
		f.term.SetContent(promptLinePad, promptRow, r, nil, style)
		promptLinePad++
	}
	var w int
	for _, r := range f.state.input {
		style := tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault).Bold(true)
		f.term.SetContent(promptLinePad+w, promptRow, r, nil, style)
		w += runewidth.RuneWidth(r)
	}
	f.term.ShowCursor(promptLinePad+f.state.cursorX, promptRow)

	// Header line (offset 1 from prompt) — only present when set.
	offset := 1
	if len(f.opt.Header) > 0 {
		w = 0
		for _, r := range runewidth.Truncate(f.opt.Header, maxWidth-2, "..") {
			style := tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorDefault)
			f.term.SetContent(2+w, rowAt(offset), r, nil, style)
			w += runewidth.RuneWidth(r)
		}
		offset++
	}

	// Item area sizing
	numberOffset := offset
	firstItemOffset := offset + 1
	// itemAreaHeight = inclusive count of usable item rows minus 1
	// Total band rows = height; rows used so far = firstItemOffset; remaining = height - firstItemOffset
	itemAreaHeight := height - firstItemOffset - 1
	if itemAreaHeight < 0 {
		itemAreaHeight = 0
	}
	pageSize := itemAreaHeight + 1
	topIdx := f.state.y - f.state.cursorY

	// Number line: counts + cursor row indicator (e.g. "12/40  row 7/40")
	numberLine := fmt.Sprintf("%d/%d", len(f.state.matched), len(f.state.items))
	if pageSize > 0 && len(f.state.matched) > pageSize {
		numberLine += fmt.Sprintf("  row %d/%d", f.state.y+1, len(f.state.matched))
	}
	for i, r := range numberLine {
		style := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorDefault)
		f.term.SetContent(2+i, rowAt(numberOffset), r, nil, style)
	}

	// Item lines: i=0 is the row closest to the prompt.
	matched := f.state.matched[topIdx:]
	words := strings.Fields(string(f.state.input))

	for i, m := range matched {
		if i > itemAreaHeight {
			break
		}
		row := rowAt(firstItemOffset + i)
		if i == f.state.cursorY {
			style := tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorBlack)
			f.term.SetContent(0, row, '>', nil, style)
			f.term.SetContent(1, row, ' ', nil, style)
		}

		if f.multi {
			if _, ok := f.state.selection[m.Idx]; ok {
				style := tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorBlack)
				f.term.SetContent(1, row, '>', nil, style)
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
			style := tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault)
			hasHighlighted := highlightPositions[j]
			if hasHighlighted {
				style = tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorDefault)
			}
			if i == f.state.cursorY {
				if hasHighlighted {
					style = tcell.StyleDefault.Foreground(tcell.ColorDarkCyan).Bold(true).Background(tcell.ColorBlack)
				} else {
					style = tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true).Background(tcell.ColorBlack)
				}
			}

			rw := runewidth.RuneWidth(r)
			if w+rw+2 > maxWidth {
				f.term.SetContent(w, row, '.', nil, style)
				f.term.SetContent(w+1, row, '.', nil, style)
				break
			}
			f.term.SetContent(w, row, r, nil, style)
			w += rw
		}
	}

	// Scrollbar in the rightmost column of the item area, only when the
	// matched list overflows the visible window.
	if pageSize > 0 && len(f.state.matched) > pageSize {
		trackHeight := pageSize
		total := len(f.state.matched)

		thumbSize := max(1, trackHeight*pageSize/total)
		if thumbSize > trackHeight {
			thumbSize = trackHeight
		}
		visibleFar := topIdx + min(itemAreaHeight, total-1-topIdx)
		maxOffset := trackHeight - thumbSize
		thumbOff := 0
		if total > pageSize {
			// In bottom-up mode, thumb at the top track row corresponds to
			// the worst-rank items (highest matched index). In reverse mode,
			// the track row closest to the prompt is the best item.
			thumbOff = (total - 1 - visibleFar) * maxOffset / (total - pageSize)
		}
		thumbOff = min(maxOffset, max(0, thumbOff))

		trackStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkGray).Background(tcell.ColorDefault)
		thumbStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorDefault)
		col := maxWidth - 1
		for row := 0; row < trackHeight; row++ {
			// Track row 0 is the row farthest from the prompt; row trackHeight-1
			// is closest. firstItemOffset+itemAreaHeight is the far end.
			y := rowAt(firstItemOffset + (trackHeight - 1 - row))
			if row >= thumbOff && row < thumbOff+thumbSize {
				f.term.SetContent(col, y, '█', nil, thumbStyle)
			} else {
				f.term.SetContent(col, y, '│', nil, trackStyle)
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
		case tcell.KeyLeft:
			if f.state.x > 0 {
				f.state.cursorX -= runewidth.RuneWidth(f.state.input[f.state.x-1])
				f.state.x--
			}
		case tcell.KeyRight:
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
			// Visually upward. In bottom-up layout that means away from the
			// bottom prompt; in reverse layout it means toward the top prompt.
			if f.opt.Reverse {
				f.scrollTowardPrompt(matchedLinesCount, screenHeight)
			} else {
				f.scrollAwayFromPrompt(matchedLinesCount, screenHeight)
			}
		case tcell.KeyDown, tcell.KeyCtrlJ, tcell.KeyCtrlN:
			if f.opt.Reverse {
				f.scrollAwayFromPrompt(matchedLinesCount, screenHeight)
			} else {
				f.scrollTowardPrompt(matchedLinesCount, screenHeight)
			}
		case tcell.KeyPgUp, tcell.KeyCtrlB:
			if f.opt.Reverse {
				f.pageTowardPrompt(pageScrollBy)
			} else {
				f.pageAwayFromPrompt(pageScrollBy, matchedLinesCount, screenHeight)
			}
		case tcell.KeyPgDn, tcell.KeyCtrlF:
			if f.opt.Reverse {
				f.pageAwayFromPrompt(pageScrollBy, matchedLinesCount, screenHeight)
			} else {
				f.pageTowardPrompt(pageScrollBy)
			}
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
			f.scrollAwayFromPrompt(matchedLinesCount, screenHeight)
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

// Cursor scroll helpers. "Toward prompt" moves toward the best match (state.y
// decreases); "away from prompt" moves toward worse matches (state.y
// increases). Both wrap when at the boundary.

func (f *finder) scrollAwayFromPrompt(matchedLinesCount, screenHeight int) {
	if f.state.y+1 < matchedLinesCount {
		f.state.y++
		if f.state.cursorY+1 < min(matchedLinesCount, screenHeight-2) {
			f.state.cursorY++
		}
		return
	}
	// At the far end: wrap back to the prompt edge.
	f.state.y = 0
	f.state.cursorY = 0
}

func (f *finder) scrollTowardPrompt(matchedLinesCount, screenHeight int) {
	if f.state.y > 0 {
		f.state.y--
		if f.state.cursorY > 0 {
			f.state.cursorY--
		}
		return
	}
	// At the prompt edge: wrap to the far end.
	if matchedLinesCount == 0 {
		return
	}
	f.state.y = matchedLinesCount - 1
	f.state.cursorY = min(matchedLinesCount-1, screenHeight-3)
}

func (f *finder) pageAwayFromPrompt(pageScrollBy, matchedLinesCount, screenHeight int) {
	f.state.y += min(pageScrollBy, matchedLinesCount-1-f.state.y)
	maxCursorY := min(screenHeight-3, matchedLinesCount-1)
	f.state.cursorY += min(pageScrollBy, maxCursorY-f.state.cursorY)
}

func (f *finder) pageTowardPrompt(pageScrollBy int) {
	f.state.y -= min(pageScrollBy, f.state.y)
	f.state.cursorY -= min(pageScrollBy, f.state.cursorY)
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
	matchedItems := matching.FindAll(string(f.state.input), f.state.items)
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

// find runs the picker. When lock is non-nil, the slice may grow concurrently
// and a background goroutine polls for new items. When lock is nil, items is
// treated as static — no polling, the slice is snapshotted once.
func (f *finder) find(ctx context.Context, itemsPtr *[]string, lock sync.Locker, opt Opt) ([]int, error) {
	if itemsPtr == nil {
		return nil, errors.New("items pointer must not be nil")
	}

	opt = opt.withDefaults()
	f.multi = opt.Multi

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	snapshot := func() []string {
		if lock == nil {
			return *itemsPtr
		}
		lock.Lock()
		defer lock.Unlock()
		return append([]string(nil), (*itemsPtr)...)
	}

	itemsCopy := snapshot()
	matched := makeMatched(len(itemsCopy))

	var inited chan struct{}
	if lock != nil {
		inited = make(chan struct{})
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
	}

	if err := f.initFinder(itemsCopy, matched, opt); err != nil {
		return nil, fmt.Errorf("failed to initialize the fuzzy finder: %w", err)
	}
	if inited != nil {
		close(inited)
	}
	return f.runLoop(ctx, &opt)
}

func makeMatched(n int) []matching.Matched {
	matched := make([]matching.Matched, n)
	for i := range matched {
		matched[i] = matching.Matched{Idx: i}
	}
	return matched
}

func (f *finder) runLoop(ctx context.Context, opt *Opt) ([]int, error) {
	if !isInTesting() {
		defer f.term.Fini()
	}

	if opt.SelectOne && len(f.state.matched) == 1 {
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

// Find displays a fuzzy-finder UI over items and returns the indices of the
// selected entries, or ErrAbort if the user cancels. With Opt.Multi=false,
// the returned slice always has exactly one element.
//
// Pass lock=nil for a static slice. Pass a non-nil lock when the slice may
// grow concurrently — the picker re-snapshots under lock on a 30ms cadence.
func Find(ctx context.Context, items *[]string, lock sync.Locker, opt Opt) ([]int, error) {
	f := &finder{}
	return f.Find(ctx, items, lock, opt)
}

func (f *finder) Find(ctx context.Context, items *[]string, lock sync.Locker, opt Opt) ([]int, error) {
	return f.find(ctx, items, lock, opt)
}

func isInTesting() bool {
	return flag.Lookup("test.v") != nil
}
