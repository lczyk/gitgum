package strutil_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/strutil"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   \n\n   ", nil},
		{"single line", "hello", []string{"hello"}},
		{"multi-line with blanks", "a\n\n b \n c  ", []string{"a", "b", "c"}},
		{"tab padding", " \t padded \t ", []string{"padded"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := strutil.SplitLines(tc.input)
			assert.EqualArrays(t, got, tc.want)
		})
	}
}
