package scoring

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestCalculate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		s1, s2    string
		wantErr   bool
		wantScore int
		wantPos   [2]int
	}{
		{"empty strings", "", "", false, 0, [2]int{-1, -1}},
		{"equal length", "foo", "foo", false, 108, [2]int{0, 2}},
		{"s1 longer than s2", "TACGGGCCCGCTA", "TAGCCCTA", false, 78, [2]int{0, 12}},
		{"s2 longer than s1", "foo", "foobar", true, 0, [2]int{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			score, pos, err := Calculate(c.s1, c.s2)
			if c.wantErr {
				assert.Error(t, err, assert.AnyError, "expected error, got nil")
				assert.Equal(t, c.wantScore, score)
				assert.Equal(t, c.wantPos, pos)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, c.wantScore, score)
			assert.Equal(t, c.wantPos, pos)
		})
	}
}
