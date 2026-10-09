package litescreen

import (
	"os"
	"testing"
	"time"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// pipeTTY stands in for /dev/tty: a pipe the test writes replies into for
// queryCursorRow to read, and /dev/null for the query itself to go to.
func pipeTTY(t *testing.T) (in, write, out *os.File) {
	t.Helper()
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { pr.Close(); pw.Close(); null.Close() })
	return pr, pw, null
}

// An input at EOF can never deliver a reply. Regression: a read returning
// (0, nil) matched neither the data branch nor the EAGAIN branch, so the loop
// spun without sleeping for the full 200ms deadline.
func TestQueryCursorRow_EOFReturnsAtOnce(t *testing.T) {
	in, write, out := pipeTTY(t)
	write.Close()

	start := time.Now()
	row, pending := queryCursorRow(in, out)
	elapsed := time.Since(start)

	assert.Equal(t, row, 0)
	assert.Equal(t, len(pending), 0)
	assert.That(t, elapsed < 100*time.Millisecond, "EOF should end the wait, took %v", elapsed)
}
