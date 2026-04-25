package scoring

import (
	"testing"

	"github.com/lczyk/assert"
)

func Test_smithWaterman(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		s1, s2    string
		wantScore int
		wantPos   [2]int
	}{
		{"gap in corpus", "TACGGG-CCCGCTA", "TAGCCCTA", 68, [2]int{0, 13}},
		{"phrase with spaces", "FLY ME TO THE MOON", "MEON", 16, [2]int{4, 17}},
		// best DP cell matches only the first s2 char (maxJ < len(s2)-1), so the
		// remaining s2 chars are found by the forward scan; to must be inclusive.
		{"forward scan fills tail", "XAXBY", "AB", 5, [2]int{1, 3}},
		// match starts at s1[1]; backward scan must cross i=0 to set from=1, not default 0.
		{"backward scan crosses start", "XAB", "AB", 33, [2]int{1, 2}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			score, pos := smithWaterman([]rune(c.s1), []rune(c.s2))
			assert.Equal(t, c.wantScore, score)
			assert.Equal(t, c.wantPos, pos)
		})
	}
}

func Benchmark_smithWaterman(b *testing.B) {
	for i := 0; i < b.N; i++ {
		smithWaterman([]rune("TACGGGCCCGCTA"), []rune("TAGCCCTA"))
	}
}
