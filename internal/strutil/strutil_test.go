package strutil_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/strutil"
)

func TestSplitLines(t *testing.T) {
	out := strutil.SplitLines("a\n\n b \n c  ")
	assert.Len(t, out, 3, "three non-empty lines")
	assert.That(t, out[1] == "b", "second line trimmed")
}
