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
			for i, m := range matched {
				assert.Equal(t, c.want[i], m.Idx)
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
		"Lorem ipsum dolor sit amet, consectetuer adipiscing elit",
		"Aenean commodo ligula eget dolor",
		"Aenean massa",
		"Cum sociis natoque penatibus et magnis dis parturient montes, nascetur ridiculus mus",
		"Donec quam felis, ultricies nec, pellentesque eu, pretium quis, sem",
		"Nulla consequat massa quis enim",
		"Donec pede justo, fringilla vel, aliquet nec, vulputate eget, arcu",
		"In enim justo, rhoncus ut, imperdiet a, venenatis vitae, justo",
		"Nullam dictum felis eu pede mollis pretium",
		"Integer tincidunt",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matching.FindAll("cas hr", benchSlice)
	}
}
