package fuzzyfinder

import (
	"sync"
	"testing"

	"github.com/lczyk/assert"
)

func TestFeed_NewEmpty(t *testing.T) {
	f := NewFeed()
	assert.EqualArrays(t, f.Items(), []string{})
	assert.Equal(t, f.Len(), 0)
}

func TestFeed_IDsAreSequential(t *testing.T) {
	f := NewFeed()
	assert.Equal(t, f.Add("a"), ID(0))
	assert.Equal(t, f.Add("b"), ID(1))
	f.AddAll("c", "d")
	assert.Equal(t, f.Add("e"), ID(4))
	assert.EqualArrays(t, f.Items(), []string{"a", "b", "c", "d", "e"})
}

func TestFeed_NewFeedFromCopiesAndCloses(t *testing.T) {
	in := []string{"a", "b"}
	f := NewFeedFrom(in)
	in[0] = "mutated"
	assert.EqualArrays(t, f.Items(), []string{"a", "b"})
	select {
	case <-f.Done():
	default:
		t.Fatal("NewFeedFrom should return a closed feed")
	}
}

func TestFeed_UpdateRemoveGet(t *testing.T) {
	f := NewFeedFrom([]string{"a", "b", "c"})
	assert.That(t, f.Update(1, "B"), "update of live id should succeed")
	assert.That(t, f.Remove(0), "remove of live id should succeed")
	assert.EqualArrays(t, f.Items(), []string{"B", "c"})
	assert.Equal(t, f.Len(), 2)

	got, ok := f.Get(1)
	assert.That(t, ok, "id 1 is live")
	assert.Equal(t, got, "B")
	_, ok = f.Get(0)
	assert.That(t, !ok, "id 0 was removed")

	assert.That(t, !f.Remove(0), "second remove reports false")
	assert.That(t, !f.Update(0, "x"), "update of removed id reports false")
	assert.That(t, !f.Update(9, "x"), "update of unknown id reports false")
}

func TestFeed_AddAfterClosePanics(t *testing.T) {
	f := NewFeed()
	f.Close()
	f.Close() // idempotent
	defer func() {
		assert.That(t, recover() != nil, "Add on a closed feed should panic")
	}()
	f.Add("x")
}

func TestFeed_DrainReportsOnlyChanges(t *testing.T) {
	f := NewFeed()
	f.AddAll("a", "b", "c")
	got := f.attach()
	assert.Equal(t, len(got), 3)
	assert.That(t, f.drain().empty(), "nothing changed since attach")

	f.Update(0, "A")
	f.Update(0, "AA") // coalesces with the update above
	f.Remove(1)
	f.Add("d")
	d := f.drain()
	assert.EqualArrays(t, d.updated, []entry{{id: 0, text: "AA"}})
	assert.EqualArrays(t, d.removed, []ID{1})
	assert.EqualArrays(t, d.added, []entry{{id: 3, text: "d"}})
	assert.That(t, f.drain().empty(), "drain resets")
}

// A change to an item the picker hasn't been handed yet folds into the add:
// the picker sees it once, in its final form, or not at all.
func TestFeed_ChangesToUndrainedItemsFoldIntoAdd(t *testing.T) {
	f := NewFeed()
	f.attach()
	a := f.Add("a")
	b := f.Add("b")
	f.Update(a, "A")
	f.Remove(b)
	d := f.drain()
	assert.EqualArrays(t, d.added, []entry{{id: a, text: "A"}})
	assert.Equal(t, len(d.updated), 0)
	assert.Equal(t, len(d.removed), 0)
}

// An update followed by a remove of the same handed-over item is just a
// remove.
func TestFeed_RemoveDropsPendingUpdate(t *testing.T) {
	f := NewFeedFrom([]string{"a"})
	f.attach()
	f.Update(0, "A")
	f.Remove(0)
	d := f.drain()
	assert.Equal(t, len(d.updated), 0)
	assert.EqualArrays(t, d.removed, []ID{0})
}

func TestFeed_MutationsWake(t *testing.T) {
	f := NewFeed()
	f.Add("a")
	f.Add("b") // coalesces into the pending wakeup
	<-f.notify
	select {
	case <-f.notify:
		t.Fatal("wakeups should coalesce")
	default:
	}
	f.Update(0, "x")
	<-f.notify
	f.Remove(0)
	<-f.notify
}

func TestFeed_ConcurrentProducers(t *testing.T) {
	f := NewFeed()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				id := f.Add("x")
				f.Update(id, "y")
			}
		})
	}
	wg.Wait()
	assert.Equal(t, f.Len(), 800)
	for _, it := range f.Items() {
		assert.Equal(t, it, "y")
	}
}
