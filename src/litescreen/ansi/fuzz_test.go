package ansi_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
)

// FuzzParseConsistency pins the invariants the three entry points share: Parse,
// Strip and ParseStyles walk the same way, so their rune counts and indices
// line up, StyleAt reproduces Parse's per-rune style from the spans, and no
// escape byte ever survives into the text.
func FuzzParseConsistency(f *testing.F) {
	for _, seed := range []string{
		"", "plain", "\x1b[31mred\x1b[0m", "\x1b[1;38;5;200;48;2;1;2;3mX", "\x1b[38:2::255:0:0mX",
		"\x1b(B\x1b[m", "a\x1b", "a\x1b\u00e9b", "\x1b]0;title\x07x", "\x1b]8;;u\x1b\\y", "\x1b[>4;2mX",
		"\x1b[", "\x1b[m\x1b[m", "line\nnext\rback", "\xff\xfe", "\x1b[99999999999m",
	} {
		f.Add(seed)
	}
	base := tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true)
	f.Fuzz(func(t *testing.T, s string) {
		runes := ansi.Parse(s, base)
		stripped := ansi.Strip(s)
		if got, want := utf8.RuneCountInString(stripped), len(runes); got != want {
			t.Fatalf("Strip has %d runes, Parse %d, for %q", got, want, s)
		}
		if strings.ContainsRune(stripped, 0x1b) {
			t.Fatalf("ESC survived Strip of %q: %q", s, stripped)
		}

		spans := ansi.ParseStyles(s, base)
		if len(runes) == 0 {
			if spans != nil {
				t.Fatalf("spans without runes for %q: %v", s, spans)
			}
			return
		}
		if spans[0].Start != 0 {
			t.Fatalf("first span starts at %d for %q", spans[0].Start, s)
		}
		for i := 1; i < len(spans); i++ {
			if spans[i].Start <= spans[i-1].Start || spans[i].Start >= len(runes) {
				t.Fatalf("span %d start %d out of order or range (%d runes) for %q", i, spans[i].Start, len(runes), s)
			}
		}
		for i, r := range runes {
			if st := ansi.StyleAt(spans, i, base); st != r.Style {
				t.Fatalf("rune %d of %q: StyleAt %v, Parse %v", i, s, st, r.Style)
			}
		}

		set := 0
		ansi.WriteToScreen(func(int, int, rune, tcell.Style) { set++ }, 0, 0, s, base)
		moves := strings.Count(stripped, "\n") + strings.Count(stripped, "\r")
		if set+moves != len(runes) {
			t.Fatalf("WriteToScreen set %d cells + %d moves, Parse has %d runes, for %q", set, moves, len(runes), s)
		}
	})
}
