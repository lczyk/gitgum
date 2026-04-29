package litescreen_test

import (
	"io"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen"
)

// fillScreen writes a rune to every cell of s, offset by `seed` so successive
// calls with different seeds produce different content. Used by the bench
// setup to give Show / Sync something non-trivial to emit, and by the
// per-iter fill in Show_FullDiff to ensure every cell actually differs from
// the prior frame (otherwise Show short-circuits on cellEqual and the
// "FullDiff" label is a lie).
func fillScreen(s *litescreen.Screen, w, h, seed int) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := rune('A' + ((x + y + seed) % 26))
			s.SetContent(x, y, r, nil, tcell.StyleDefault)
		}
	}
}

// newBenchScreen constructs a fullscreen Screen wired to io.Discard so the
// benchmark measures composition cost, not terminal write throughput. The
// returned Init has already been called; caller is responsible for Fini.
func newBenchScreen(b *testing.B, w, h int) *litescreen.Screen {
	b.Helper()
	s, err := litescreen.NewWithOptions(litescreen.Options{
		Out:  io.Discard,
		Size: fixedSize(w, h),
	})
	if err != nil {
		b.Fatalf("NewWithOptions: %v", err)
	}
	if err := s.Init(); err != nil {
		b.Fatalf("Init: %v", err)
	}
	return s
}

var benchSizes = []struct {
	name string
	w, h int
}{
	{"80x24", 80, 24},
	{"200x60", 200, 60},
	{"400x120", 400, 120},
}

// BenchmarkScreen_Show_NoChange measures Show when back == front. This is
// the cheap path: framebuf.flush emits only the cursor-hide / wrap-off /
// wrap-on / cursor-show framing and skips every cell. Baseline for the
// other Show / Sync numbers below.
func BenchmarkScreen_Show_NoChange(b *testing.B) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			s := newBenchScreen(b, sz.w, sz.h)
			defer s.Fini()
			fillScreen(s, sz.w, sz.h, 0)
			s.Show() // prime: front catches up to back
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				s.Show()
			}
		})
	}
}

// BenchmarkScreen_Show_FullDiff measures Show after every cell has been
// rewritten (Clear + fillScreen). Every cell ends up in the diff, so this
// is the upper bound for Show's per-frame cost.
func BenchmarkScreen_Show_FullDiff(b *testing.B) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			s := newBenchScreen(b, sz.w, sz.h)
			defer s.Fini()
			fillScreen(s, sz.w, sz.h, 0)
			s.Show() // prime
			b.ReportAllocs()
			b.ResetTimer()
			i := 1
			for b.Loop() {
				s.Clear()
				fillScreen(s, sz.w, sz.h, i)
				s.Show()
				i++
			}
		})
	}
}

// BenchmarkScreen_Sync measures the anti-tearing path: every cell re-emitted
// regardless of diff state, plus the leading region-clear. Compare against
// Show_NoChange (idle) and Show_FullDiff (worst-case real change) to size
// the cost of running aggressive redraws on a timer.
func BenchmarkScreen_Sync(b *testing.B) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			s := newBenchScreen(b, sz.w, sz.h)
			defer s.Fini()
			fillScreen(s, sz.w, sz.h, 0)
			s.Show() // prime: back == front before each Sync, so the
			//                  win Sync gives over Show is purely from
			//                  forcing the full re-emit.
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				s.Sync()
			}
		})
	}
}

// BenchmarkScreen_Sync_AfterEdit measures Sync when the back buffer also
// changes between calls. This is the realistic shape during streaming
// pipelines (find | ff): every redraw rewrites items AND forces full
// repaint to overwrite tearing.
func BenchmarkScreen_Sync_AfterEdit(b *testing.B) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			s := newBenchScreen(b, sz.w, sz.h)
			defer s.Fini()
			fillScreen(s, sz.w, sz.h, 0)
			s.Show()
			b.ReportAllocs()
			b.ResetTimer()
			i := 0
			for b.Loop() {
				// Cycle one cell so Clear+refill mimics streaming churn.
				s.SetContent(0, 0, rune('A'+i%26), nil, tcell.StyleDefault)
				s.Sync()
				i++
			}
		})
	}
}
