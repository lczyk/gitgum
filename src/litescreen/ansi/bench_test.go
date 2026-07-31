package ansi_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
)

// fixtureLen produces an ANSI-styled string of approximate byte length n.
// Alternates styled and plain segments to simulate the mix seen in real
// git output: some coloured tokens, some plain.
func fixtureLen(n int) string {
	escapes := []string{
		"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m",
		"\x1b[1m", "\x1b[2m", "\x1b[1;33m", "\x1b[0m",
	}
	var b strings.Builder
	b.Grow(n)
	chunks := []string{
		"feature/", "bugfix/", "release/",
		"user/alice/issue-00123-tweak", "hotfix/crash-loop-fix",
		"main", "develop", "refactor/small-thing",
	}
	// Step the escape per chunk, not per byte written: keying off b.Len()
	// happened to land on the same escape every time, so the whole fixture
	// carried two styles and nothing that measures style changes saw any.
	for i := 0; b.Len() < n; i++ {
		b.WriteString(escapes[i%len(escapes)])
		b.WriteString(chunks[i%len(chunks)])
	}
	return b.String()[:min(n, b.Len())]
}

func BenchmarkParse(b *testing.B) {
	sizes := []int{128, 1024, 8192, 65536}
	for _, n := range sizes {
		fixture := fixtureLen(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = ansi.Parse(fixture, tcell.StyleDefault)
			}
		})
	}
}

func BenchmarkParseStyles(b *testing.B) {
	sizes := []int{128, 1024, 8192, 65536}
	for _, n := range sizes {
		fixture := fixtureLen(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = ansi.ParseStyles(fixture, tcell.StyleDefault)
			}
		})
	}
}

func BenchmarkStrip(b *testing.B) {
	sizes := []int{128, 1024, 8192, 65536}
	for _, n := range sizes {
		fixture := fixtureLen(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = ansi.Strip(fixture)
			}
		})
	}
}
