// cspell:ignore Änger Tiggers

package matching_test

import (
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
