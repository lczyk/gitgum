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

func BenchmarkFeed_Add(b *testing.B) {
	f := NewFeed()
	b.ReportAllocs()
	for b.Loop() {
		f.Add("item")
	}
}

// BenchmarkFeed_DrainOneAdd measures the per-wakeup cost of a streaming
// producer: one new item on top of an n-item feed. It must not grow with n.
func BenchmarkFeed_DrainOneAdd(b *testing.B) {
	for _, n := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			f := NewFeed()
			f.AddAll(benchItems(n)...)
			f.attach()
			b.ReportAllocs()
			for b.Loop() {
				f.Add("item")
				_ = f.drain()
			}
		})
	}
}

// newBenchFinder builds a finder over items without a terminal.
func newBenchFinder(b *testing.B, items []string, query string) *finder {
	b.Helper()
	f := newHeadlessFinder(false)
	if err := f.initItems(items, Opt{Query: query}); err != nil {
		b.Fatal(err)
	}
	return f
}

// BenchmarkApplyDelta_Append measures folding one appended item into an
// n-item view, with and without a query. It must not grow with n.
func BenchmarkApplyDelta_Append(b *testing.B) {
	for _, query := range []string{"", "item 5"} {
		for _, n := range []int{100, 10_000} {
			b.Run(fmt.Sprintf("query=%q/n=%d", query, n), func(b *testing.B) {
				f := newBenchFinder(b, benchItems(n), query)
				next := ID(n)
				b.ReportAllocs()
				for b.Loop() {
					f.applyDelta(delta{added: []entry{{id: next, text: "item-555555"}}})
					next++
				}
			})
		}
	}
}

// BenchmarkApplyDelta_Update measures re-matching one edited item.
func BenchmarkApplyDelta_Update(b *testing.B) {
	for _, n := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			f := newBenchFinder(b, benchItems(n), "item 5")
			texts := [2]string{"item-500000", "item-000000"}
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				f.applyDelta(delta{updated: []entry{{id: ID(n / 2), text: texts[i&1]}}})
			}
		})
	}
}

// BenchmarkApplyDelta_Remove measures dropping one item, which compacts the
// view and so is O(n).
func BenchmarkApplyDelta_Remove(b *testing.B) {
	for _, n := range []int{100, 10_000} {
		items := benchItems(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			// NOTE: legacy b.N loop -- b.Loop() is incompatible with
			// StopTimer/StartTimer (the adaptive scaler never converges).
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				f := newBenchFinder(b, items, "")
				f.state.y = n / 2
				b.StartTimer()
				f.applyDelta(delta{removed: []ID{ID(n / 4)}})
			}
		})
	}
}
