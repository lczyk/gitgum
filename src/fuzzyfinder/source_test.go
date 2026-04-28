package fuzzyfinder

import (
	"sync"
	"testing"

	"github.com/lczyk/assert"
)

func TestSliceSource_NewEmpty(t *testing.T) {
	s := NewSliceSource()
	assert.EqualArrays(t, s.Snapshot(), []string{})
	assert.Equal(t, s.Len(), 0)
	assert.Equal(t, s.Version(), uint64(0))
}

func TestSliceSource_NewSliceSourceFromCopiesInput(t *testing.T) {
	in := []string{"a", "b", "c"}
	s := NewSliceSourceFrom(in)
	in[0] = "mutated"
	assert.EqualArrays(t, s.Snapshot(), []string{"a", "b", "c"})
	assert.Equal(t, s.Version(), uint64(1))
}

func TestSliceSource_AddBumpsVersion(t *testing.T) {
	s := NewSliceSource()
	s.Add("x")
	s.Add("y")
	assert.EqualArrays(t, s.Snapshot(), []string{"x", "y"})
	assert.Equal(t, s.Version(), uint64(2))
}

func TestSliceSource_RemoveAt(t *testing.T) {
	s := NewSliceSourceFrom([]string{"a", "b", "c"})
	v0 := s.Version()
	assert.That(t, s.RemoveAt(1), "remove in range should succeed")
	assert.EqualArrays(t, s.Snapshot(), []string{"a", "c"})
	assert.That(t, s.Version() > v0, "version bumps on removal")
}

func TestSliceSource_RemoveAtOutOfRange(t *testing.T) {
	s := NewSliceSourceFrom([]string{"a"})
	v0 := s.Version()
	assert.That(t, !s.RemoveAt(-1), "negative index returns false")
	assert.That(t, !s.RemoveAt(5), "too-large index returns false")
	assert.Equal(t, s.Version(), v0)
	assert.EqualArrays(t, s.Snapshot(), []string{"a"})
}

func TestSliceSource_RemoveFunc(t *testing.T) {
	s := NewSliceSourceFrom([]string{"keep", "drop", "keep", "drop"})
	v0 := s.Version()
	n := s.RemoveFunc(func(x string) bool { return x == "drop" })
	assert.Equal(t, n, 2)
	assert.EqualArrays(t, s.Snapshot(), []string{"keep", "keep"})
	assert.That(t, s.Version() > v0, "version bumps when something removed")
}

func TestSliceSource_RemoveFuncNoMatch(t *testing.T) {
	s := NewSliceSourceFrom([]string{"a", "b"})
	v0 := s.Version()
	n := s.RemoveFunc(func(string) bool { return false })
	assert.Equal(t, n, 0)
	assert.Equal(t, s.Version(), v0)
}

func TestSliceSource_Reset(t *testing.T) {
	s := NewSliceSourceFrom([]string{"a", "b"})
	v0 := s.Version()
	s.Reset([]string{"x", "y", "z"})
	assert.EqualArrays(t, s.Snapshot(), []string{"x", "y", "z"})
	assert.That(t, s.Version() > v0, "version bumps on reset")
}

func TestSliceSource_ResetCopiesInput(t *testing.T) {
	s := NewSliceSource()
	in := []string{"a", "b"}
	s.Reset(in)
	in[0] = "mutated"
	assert.EqualArrays(t, s.Snapshot(), []string{"a", "b"})
}

func TestSliceSource_SnapshotIndependent(t *testing.T) {
	s := NewSliceSourceFrom([]string{"a", "b"})
	snap := s.Snapshot()
	s.Add("c")
	assert.EqualArrays(t, snap, []string{"a", "b"})
	assert.EqualArrays(t, s.Snapshot(), []string{"a", "b", "c"})
}

// TestSliceSource_ConcurrentMutations: hammers Add/RemoveFunc from goroutines
// while another goroutine takes Snapshots. Goal is no data race / no panic.
// Run with `go test -race`.
func TestSliceSource_ConcurrentMutations(t *testing.T) {
	s := NewSliceSource()
	var wg sync.WaitGroup

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				s.Add("item")
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := range 200 {
			s.RemoveFunc(func(x string) bool { return x == "item" && j%3 == 0 })
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			_ = s.Snapshot()
		}
	}()
	wg.Wait()

	assert.That(t, s.Version() > 0, "some mutations occurred")
}

// Compile-time check that SliceSource satisfies Source and Versioned.
var (
	_ Source    = (*SliceSource)(nil)
	_ Versioned = (*SliceSource)(nil)
)
