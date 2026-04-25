package strutil_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/strutil"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"   \n\n   ", nil},
		{"hello", []string{"hello"}},
		{"a\n\n b \n c  ", []string{"a", "b", "c"}},
		{" \t padded \t ", []string{"padded"}},
	}
	for _, tc := range tests {
		got := strutil.SplitLines(tc.input)
		assert.EqualArrays(t, got, tc.want)
	}
}
