package fuzzyfinder

import (
	"fmt"
	"testing"
)

// benchItems builds a slice of n distinct items: "item-0000", "item-0001", ...
func benchItems(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = fmt.Sprintf("item-%06d", i)
	}
	return out
}

func BenchmarkSliceSource_Add(b *testing.B) {
	src := NewSliceSource()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		src.Add("item")
		_ = i
	}
}

func BenchmarkSliceSource_Snapshot(b *testing.B) {
	for _, n := range []int{10, 100, 1000, 10_000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			src := NewSliceSourceFrom(benchItems(n))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = src.Snapshot()
			}
		})
	}
}

func BenchmarkSliceSource_Version(b *testing.B) {
	src := NewSliceSourceFrom(benchItems(1000))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = src.Version()
	}
}

func BenchmarkSliceSource_RemoveFunc(b *testing.B) {
	for _, n := range []int{100, 1000, 10_000} {
		b.Run(fmt.Sprintf("n=%d_remove_half", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				src := NewSliceSourceFrom(benchItems(n))
				b.StartTimer()
				src.RemoveFunc(func(s string) bool {
					// Drop every other item.
					return s[len(s)-1]&1 == 0
				})
			}
		})
	}
}

func BenchmarkKeyOf(b *testing.B) {
	for _, n := range []int{100, 1000, 10_000} {
		items := benchItems(n)
		b.Run(fmt.Sprintf("n=%d_unique", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				_ = keyOf(items, i%n)
			}
		})
	}
}

func BenchmarkFindKey(b *testing.B) {
	for _, n := range []int{100, 1000, 10_000} {
		items := benchItems(n)
		b.Run(fmt.Sprintf("n=%d_hit_middle", n), func(b *testing.B) {
			key := keyOf(items, n/2)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = findKey(items, key)
			}
		})
	}
}

// BenchmarkUpdateItems_Steady measures resync cost when nothing actually
// changed (the SliceSource semantically replays the same data) — exercises the
// identity-capture + remap hot path with a steady cursor and selection.
func BenchmarkUpdateItems_Steady(b *testing.B) {
	for _, n := range []int{100, 1000, 10_000} {
		items := benchItems(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			f := newTestFinder(items, false)
			f.state.y = n / 2
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				f.updateItems(items)
				// drain redraw signal so the buffered eventCh doesn't fill
				select {
				case <-f.eventCh:
				default:
				}
			}
		})
	}
}

// BenchmarkUpdateItems_Removal measures resync cost when a single item was
// removed — exercises remap of cursor and selection across changed indices.
func BenchmarkUpdateItems_Removal(b *testing.B) {
	for _, n := range []int{100, 1000, 10_000} {
		items := benchItems(n)
		smaller := items[1:]
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				f := newTestFinder(items, true)
				f.state.y = n / 2
				f.state.selection = map[int]int{0: 1, n / 2: 2, n - 1: 3}
				f.state.selectionIdx = 4
				b.StartTimer()
				f.updateItems(smaller)
				select {
				case <-f.eventCh:
				default:
				}
			}
		})
	}
}
