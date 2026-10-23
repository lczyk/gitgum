package fuzzyfinder

import (
	"iter"
	"sync"
)

// ID is a stable handle for one item in a Feed. IDs are assigned from 0 in the
// order items are added and never reused, so for a feed built by NewFeedFrom
// the ID of an item is its index in the input slice.
type ID uint64

// Flags tune how the picker treats one item. The zero value is a plain,
// selectable item.
type Flags uint8

const (
	// Unselectable marks an item display-only: it still appears, matches the
	// query and can hold the cursor, but Tab won't toggle it and Enter won't
	// confirm it, and it is drawn dimmed to say so. If the query matches no
	// selectable item at all, Enter ends the picker with an empty selection
	// rather than being swallowed (see Result).
	Unselectable Flags = 1 << iota
)

// Feed is the goroutine-safe item list the picker consumes. Any number of
// producers may Add, Update and Remove concurrently, before or while a picker
// is open; none of the calls ever wait on the picker. The picker is woken on
// every change and reads only what changed since it last looked, so the cost
// of a change doesn't grow with the size of the list.
//
// Items keep insertion order. Cursor and selection follow an item by its ID,
// so they survive edits and removals of other items, and edits of their own.
//
// A Feed serves one picker at a time; a later Find on the same Feed starts
// from its current contents.
type Feed struct {
	mu     sync.Mutex
	slots  []slot // indexed by ID
	live   int
	closed bool

	// Change tracking for the attached picker. IDs below drained have been
	// handed over; updated and removed only ever hold such IDs -- a change to
	// an item the picker hasn't seen yet is picked up when it's first handed
	// over.
	drained ID
	updated map[ID]struct{}
	removed map[ID]struct{}

	notify chan struct{} // cap 1: a pending wakeup, coalesced
	done   chan struct{} // closed by Close
}

type slot struct {
	text  string
	flags Flags
	live  bool
}

// NewFeed returns an empty, open Feed.
func NewFeed() *Feed {
	return &Feed{
		updated: map[ID]struct{}{},
		removed: map[ID]struct{}{},
		notify:  make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
}

// NewFeedFrom returns a closed Feed holding a copy of items: a fixed list.
// Update and Remove still work on it; Add does not.
func NewFeedFrom(items []string) *Feed {
	f := NewFeed()
	f.AddAll(items...)
	f.Close()
	return f
}

// Add appends item and returns its ID. Add panics if the feed is closed.
func (f *Feed) Add(item string) ID { return f.AddWith(item, 0) }

// AddWith is Add for an item with flags.
func (f *Feed) AddWith(item string, flags Flags) ID {
	f.mu.Lock()
	id := f.addLocked(item, flags)
	f.mu.Unlock()
	f.wake()
	return id
}

// AddAll appends items in order, under one lock and one wakeup. They get
// consecutive IDs. AddAll panics if the feed is closed.
func (f *Feed) AddAll(items ...string) {
	if len(items) == 0 {
		return
	}
	f.mu.Lock()
	for _, it := range items {
		f.addLocked(it, 0)
	}
	f.mu.Unlock()
	f.wake()
}

func (f *Feed) addLocked(item string, flags Flags) ID {
	if f.closed {
		panic("fuzzyfinder: Add on closed Feed")
	}
	id := ID(len(f.slots))
	f.slots = append(f.slots, slot{text: item, flags: flags, live: true})
	f.live++
	return id
}

// Update replaces the text of item id, keeping its flags. It reports false if
// id was never added or has been removed.
func (f *Feed) Update(id ID, item string) bool {
	return f.edit(id, func(s *slot) { s.text = item })
}

// SetFlags replaces the flags of item id. It reports false if id was never
// added or has been removed.
func (f *Feed) SetFlags(id ID, flags Flags) bool {
	return f.edit(id, func(s *slot) { s.flags = flags })
}

func (f *Feed) edit(id ID, change func(*slot)) bool {
	f.mu.Lock()
	if !f.liveLocked(id) {
		f.mu.Unlock()
		return false
	}
	change(&f.slots[id])
	if id < f.drained {
		f.updated[id] = struct{}{}
	}
	f.mu.Unlock()
	f.wake()
	return true
}

// Remove drops item id. It reports false if id was never added or has already
// been removed.
func (f *Feed) Remove(id ID) bool {
	f.mu.Lock()
	if !f.liveLocked(id) {
		f.mu.Unlock()
		return false
	}
	f.slots[id] = slot{}
	f.live--
	if id < f.drained {
		delete(f.updated, id)
		f.removed[id] = struct{}{}
	}
	f.mu.Unlock()
	f.wake()
	return true
}

// Get returns the current text of item id, and false if id was never added or
// has been removed.
func (f *Feed) Get(id ID) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.liveLocked(id) {
		return "", false
	}
	return f.slots[id].text, true
}

// Flags returns the flags of item id, and false if id was never added or has
// been removed.
func (f *Feed) Flags(id ID) (Flags, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.liveLocked(id) {
		return 0, false
	}
	return f.slots[id].flags, true
}

// All iterates a snapshot of the live items, in feed order.
func (f *Feed) All() iter.Seq2[ID, string] {
	entries := f.entries()
	return func(yield func(ID, string) bool) {
		for _, e := range entries {
			if !yield(e.id, e.text) {
				return
			}
		}
	}
}

// Items returns the text of every live item, in feed order.
func (f *Feed) Items() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, f.live)
	for _, s := range f.slots {
		if s.live {
			out = append(out, s.text)
		}
	}
	return out
}

// Len returns the number of live items.
func (f *Feed) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live
}

// Close marks the end of the stream: no more Adds. A picker with
// Opt.SelectOne waits for it before deciding. Update and Remove keep working.
// Close is idempotent.
func (f *Feed) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	f.closed = true
	close(f.done)
}

// Done is closed once Close has been called.
func (f *Feed) Done() <-chan struct{} { return f.done }

func (f *Feed) liveLocked(id ID) bool {
	return id < ID(len(f.slots)) && f.slots[id].live
}

// wake posts a wakeup without blocking; a full channel already has one
// pending, which is all the picker needs.
func (f *Feed) wake() {
	select {
	case f.notify <- struct{}{}:
	default:
	}
}

// entry is one item as handed to the picker.
type entry struct {
	id    ID
	text  string
	flags Flags
}

// delta is everything that changed since the picker last drained.
type delta struct {
	added   []entry // ascending ID
	updated []entry
	removed []ID
}

func (d delta) empty() bool {
	return len(d.added) == 0 && len(d.updated) == 0 && len(d.removed) == 0
}

// attach hands a picker the full current contents and starts change tracking
// from here, discarding whatever an earlier picker left undrained.
func (f *Feed) attach() []entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.updated)
	clear(f.removed)
	f.drained = ID(len(f.slots))
	// A wakeup posted before now is for changes this snapshot already has.
	select {
	case <-f.notify:
	default:
	}
	return f.liveEntriesLocked(0)
}

// entries returns the current live items without touching change tracking.
func (f *Feed) entries() []entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.liveEntriesLocked(0)
}

func (f *Feed) liveEntriesLocked(from ID) []entry {
	out := make([]entry, 0, len(f.slots)-int(from))
	for i := from; i < ID(len(f.slots)); i++ {
		if s := f.slots[i]; s.live {
			out = append(out, entry{id: i, text: s.text, flags: s.flags})
		}
	}
	return out
}

// drain returns the changes since the last attach / drain and resets them.
func (f *Feed) drain() delta {
	f.mu.Lock()
	defer f.mu.Unlock()
	var d delta
	if len(f.removed) > 0 {
		d.removed = make([]ID, 0, len(f.removed))
		for id := range f.removed {
			d.removed = append(d.removed, id)
		}
		clear(f.removed)
	}
	if len(f.updated) > 0 {
		d.updated = make([]entry, 0, len(f.updated))
		for id := range f.updated {
			s := f.slots[id]
			d.updated = append(d.updated, entry{id: id, text: s.text, flags: s.flags})
		}
		clear(f.updated)
	}
	if f.drained < ID(len(f.slots)) {
		d.added = f.liveEntriesLocked(f.drained)
		f.drained = ID(len(f.slots))
	}
	return d
}
