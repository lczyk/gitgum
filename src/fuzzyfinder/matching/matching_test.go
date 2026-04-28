// cspell:ignore Änger Tiggers

package matching_test

import (
	"fmt"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/fuzzyfinder/matching"
)

func TestFindAll(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		query string
		want  []int // indices into slice
	}{
		"empty query matches all":        {"", []int{0, 1, 2}},
		"single word substring":          {"album", []int{0}},
		"case insensitive":               {"WHITE", []int{0}},
		"item case insensitive":          {"sound", []int{1}},
		"all words present in any order": {"snow inkle", []int{2}},
		"some word missing":              {"white sound", nil},
		"whitespace ignored":             {"   white    album   ", []int{0}},
	}
	slice := []string{
		"WHITE ALBUM",
		"SOUND OF DESTINY",
		"Twinkle Snow",
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			matched := matching.FindAll(c.query, slice)
			assert.Len(t, matched, len(c.want))
			for i, idx := range matched {
				assert.Equal(t, c.want[i], idx)
			}
		})
	}
}

func TestFindAll_unicode(t *testing.T) {
	t.Parallel()
	slice := []string{"日本語の本", "fußÄnger"}
	assert.Len(t, matching.FindAll("日本", slice), 1)
	assert.Len(t, matching.FindAll("Ä", slice), 1)
}

func BenchmarkFindAll(b *testing.B) {
	benchSlice := []string{
		"When you wake up in the morning, Pooh, what's the first thing you say to yourself?",
		"It is more fun to talk with someone who doesn't use long, difficult words",
		"A day without a friend is like a pot without a single drop of honey left inside",
		"Sometimes the smallest things take up the most room in your heart",
		"Rivers know this: there is no hurry, we shall get there some day",
		"How lucky I am to have something that makes saying goodbye so hard",
		"If there ever comes a day when we cannot be together, keep me in your heart",
		"You are braver than you believe, stronger than you seem, and smarter than you think",
		"Piglet noticed that even though he had a Very Small Heart, it could hold a rather large amount of Gratitude",
		"Tiggers do not like honey",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matching.FindAll("cas hr", benchSlice)
	}
}

// branchLikeCorpus builds a deterministic corpus of n strings shaped like
// branch names — the realistic gg switch / fuzzyfinder workload. Mixes
// prefixes ("feature/", "bugfix/", "release/", "user/...") and a stable case
// distribution so strings.ToLower has work to do on a realistic fraction of
// items.
func branchLikeCorpus(n int) []string {
	prefixes := []string{"feature/", "bugfix/", "release/", "user/alice/", "user/bob/", "hotfix/"}
	out := make([]string, n)
	for i := range n {
		p := prefixes[i%len(prefixes)]
		// Inject some uppercase to give ToLower realistic work (~1 in 6 items).
		if i%6 == 0 {
			out[i] = fmt.Sprintf("%sIssue-%05d-Refactor", p, i)
		} else {
			out[i] = fmt.Sprintf("%sissue-%05d-tweak", p, i)
		}
	}
	return out
}

// unicodeCorpus mirrors branchLikeCorpus but with non-ASCII content so we
// can see the cost of strings.ToLower / strings.Contains on multi-byte runes.
func unicodeCorpus(n int) []string {
	prefixes := []string{"功能/", "修复/", "リリース/", "ユーザー/"}
	out := make([]string, n)
	for i := range n {
		p := prefixes[i%len(prefixes)]
		out[i] = fmt.Sprintf("%s問題-%05d-変更", p, i)
	}
	return out
}

// BenchmarkFindAll_Sweep covers the user-typing hot path across realistic
// dimensions: corpus size × query shape. Item profile stays "branch-like"
// (short, mixed case) since that's the dominant workload; a separate
// unicode sub-bench measures the rune-aware ToLower cost.
func BenchmarkFindAll_Sweep(b *testing.B) {
	queries := []struct {
		name  string
		query string // chosen so the hit rate matches the name
	}{
		{"empty_all_match", ""},
		{"single_word_hits_some", "feature"},
		{"two_words_hits_few", "feature 00007"},
		{"miss_all", "absolutelynowhere"},
	}
	sizes := []int{100, 1000, 10_000}

	for _, n := range sizes {
		corpus := branchLikeCorpus(n)
		for _, q := range queries {
			b.Run(fmt.Sprintf("ascii/n=%d/%s", n, q.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					_ = matching.FindAll(q.query, corpus)
				}
			})
		}
	}

	// Single unicode sweep at one size — confirms order-of-magnitude cost
	// versus ASCII without exploding the matrix.
	corpusU := unicodeCorpus(1000)
	b.Run("unicode/n=1000/single_word_hits_some", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = matching.FindAll("問題", corpusU)
		}
	})
}

// BenchmarkFindAllLower mirrors BenchmarkFindAll_Sweep with a pre-lowercased
// corpus and query, isolating the substring-match cost from the per-call
// strings.ToLower allocations. The picker uses this hot path on every
// keystroke after caching itemsLower in state.
func BenchmarkFindAllLower(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"empty_all_match", ""},
		{"single_word_hits_some", "feature"},
		{"two_words_hits_few", "feature 00007"},
		{"miss_all", "absolutelynowhere"},
	}
	sizes := []int{100, 1000, 10_000}

	for _, n := range sizes {
		corpus := branchLikeCorpus(n)
		// Pre-lower the corpus once to mimic the cached state.itemsLower.
		lower := make([]string, n)
		for i, s := range corpus {
			lower[i] = lowerASCII(s)
		}
		for _, q := range queries {
			lq := lowerASCII(q.query)
			b.Run(fmt.Sprintf("ascii/n=%d/%s", n, q.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					_ = matching.FindAllLower(lq, lower)
				}
			})
		}
	}
}

// lowerASCII is strings.ToLower restricted to ASCII — sufficient for the
// benchmark corpora and keeps the bench setup obviously deterministic.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
