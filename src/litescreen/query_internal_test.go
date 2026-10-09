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

func TestQueryCursorRow_ParsesReply(t *testing.T) {
	in, write, out := pipeTTY(t)
	_, err := write.WriteString("\x1b[12;1R")
	require.NoError(t, err)

	row, pending := queryCursorRow(in, out)
	assert.Equal(t, row, 12)
	assert.Equal(t, len(pending), 0)
}

// Keystrokes typed before the picker was ready sit in the queue ahead of (and
// after) the reply; they come back as pending, in order, with the report cut
// out.
func TestQueryCursorRow_KeepsTypeAhead(t *testing.T) {
	in, write, out := pipeTTY(t)
	_, err := write.WriteString("ab\x1b[A\x1b[5;1Rxy")
	require.NoError(t, err)

	row, pending := queryCursorRow(in, out)
	assert.Equal(t, row, 5)
	assert.Equal(t, string(pending), "ab\x1b[Axy")
}

// A stale report from an earlier query is superseded by the fresh one.
func TestQueryCursorRow_LastReplyWins(t *testing.T) {
	in, write, out := pipeTTY(t)
	_, err := write.WriteString("\x1b[3;1R\x1b[9;1R")
	require.NoError(t, err)

	row, _ := queryCursorRow(in, out)
	assert.Equal(t, row, 9)
}

// No reply within the deadline: row 0 (callers bottom-anchor) and the
// type-ahead still comes back rather than being swallowed.
func TestQueryCursorRow_TimesOutKeepingTypeAhead(t *testing.T) {
	in, write, out := pipeTTY(t)
	_, err := write.WriteString("zz")
	require.NoError(t, err)

	start := time.Now()
	row, pending := queryCursorRow(in, out)
	elapsed := time.Since(start)

	assert.Equal(t, row, 0)
	assert.Equal(t, string(pending), "zz")
	assert.That(t, elapsed >= 150*time.Millisecond, "should wait out the deadline, took %v", elapsed)
	assert.That(t, elapsed < 2*time.Second, "deadline overshoot: %v", elapsed)
}

// A reply split across two writes must not be cut short at the first half.
func TestQueryCursorRow_ReplyArrivesInPieces(t *testing.T) {
	in, write, out := pipeTTY(t)
	_, err := write.WriteString("\x1b[2")
	require.NoError(t, err)
	go func() {
		time.Sleep(30 * time.Millisecond)
		write.WriteString("1;1R")
	}()

	row, pending := queryCursorRow(in, out)
	assert.Equal(t, row, 21)
	assert.Equal(t, len(pending), 0)
}
