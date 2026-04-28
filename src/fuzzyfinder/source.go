package fuzzyfinder

import (
	"sync"
	"sync/atomic"
)

// Source feeds items into the picker. The picker calls Snapshot whenever it
// needs to refresh its view of the items. Snapshot may return a slice that
// differs from a previous call in any way: appended, removed, or replaced.
//
// Implementations must be goroutine-safe: Snapshot is called from the picker's
// resync goroutine while the caller is typically mutating the underlying data
// on a different goroutine.
type Source interface {
	Snapshot() []string
}

// Versioned is an optional extension of Source. When a Source implements it,
// the picker uses Version to skip the Snapshot copy on idle ticks: it only
// resnapshots when Version has changed since the previous refresh.
//
// Implementations must bump the version on every mutation that could change
// the result of Snapshot.
type Versioned interface {
	Source
	Version() uint64
}

// SliceSource is a goroutine-safe Source backed by an internal slice.
// Use Add / RemoveAt / RemoveFunc / Reset to mutate; the picker's resync
// goroutine picks up changes on its next tick.
//
// SliceSource implements Versioned, so the picker only resnapshots when
// something actually changed.
type SliceSource struct {
	mu      sync.Mutex
	items   []string
	version atomic.Uint64
}

// NewSliceSource returns an empty SliceSource.
func NewSliceSource() *SliceSource { return &SliceSource{} }

// NewSliceSourceFrom returns a SliceSource initialized with a copy of items.
// Later mutations to the input slice by the caller do not affect the source.
func NewSliceSourceFrom(items []string) *SliceSource {
	s := &SliceSource{items: append([]string(nil), items...)}
	s.version.Store(1)
	return s
}

// Snapshot returns a copy of the current items.
func (s *SliceSource) Snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.items...)
}

// Version returns the current mutation counter. It bumps on every Add /
// RemoveAt (when in range) / RemoveFunc (when at least one item was removed) /
// Reset call.
func (s *SliceSource) Version() uint64 { return s.version.Load() }

// Len returns the current length under lock.
func (s *SliceSource) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

// Add appends item to the source.
func (s *SliceSource) Add(item string) {
	s.mu.Lock()
	s.items = append(s.items, item)
	s.mu.Unlock()
	s.version.Add(1)
}

// RemoveAt deletes the item at index i. Returns false if i is out of range.
func (s *SliceSource) RemoveAt(i int) bool {
	s.mu.Lock()
	if i < 0 || i >= len(s.items) {
		s.mu.Unlock()
		return false
	}
	s.items = append(s.items[:i], s.items[i+1:]...)
	s.mu.Unlock()
	s.version.Add(1)
	return true
}

// RemoveFunc removes every item for which pred returns true and returns the
// number of items removed.
func (s *SliceSource) RemoveFunc(pred func(string) bool) int {
	s.mu.Lock()
	out := s.items[:0]
	removed := 0
	for _, it := range s.items {
		if pred(it) {
			removed++
			continue
		}
		out = append(out, it)
	}
	s.items = out
	s.mu.Unlock()
	if removed > 0 {
		s.version.Add(1)
	}
	return removed
}

// Reset replaces all items atomically.
func (s *SliceSource) Reset(items []string) {
	cp := append([]string(nil), items...)
	s.mu.Lock()
	s.items = cp
	s.mu.Unlock()
	s.version.Add(1)
}

// legacyLockedSource adapts a caller-owned *[]string + sync.Locker (the
// existing Find API) to the Source interface. It implements Versioned with
// len(items) as the proxy counter — sufficient for the legacy append-only
// contract; equal-length mutations will not be detected.
type legacyLockedSource struct {
	items *[]string
	lock  sync.Locker
}

func (l *legacyLockedSource) Snapshot() []string {
	if l.lock == nil {
		return append([]string(nil), (*l.items)...)
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	return append([]string(nil), (*l.items)...)
}

func (l *legacyLockedSource) Version() uint64 {
	if l.lock == nil {
		return uint64(len(*l.items))
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	return uint64(len(*l.items))
}
