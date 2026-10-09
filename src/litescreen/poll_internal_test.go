package litescreen

import (
	"bytes"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// pipeInput returns an fdInput reading from a pipe, set up, plus the write
// end for the test to feed. Both are torn down with the test.
func pipeInput(t *testing.T) (*fdInput, *os.File) {
	t.Helper()
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { pr.Close(); pw.Close() })
	in := inputFor(pr).(*fdInput)
	require.NoError(t, in.setup())
	t.Cleanup(in.teardown)
	return in, pw
}

// wait blocks until the fd has data, so readLoop does not have to poll.
func TestFdInput_WaitReturnsOnData(t *testing.T) {
	in, pw := pipeInput(t)
	go func() {
		time.Sleep(30 * time.Millisecond)
		pw.Write([]byte("x"))
	}()
	start := time.Now()
	in.wait(time.Second)
	assert.That(t, time.Since(start) < 500*time.Millisecond, "wait must return once data arrives")
	buf := make([]byte, 8)
	n, err := in.read(buf)
	require.NoError(t, err)
	assert.Equal(t, n, 1)
}

// wake interrupts a wait from another goroutine: Fini must not have to sit
// out the timeout before readLoop notices quit.
func TestFdInput_WaitReturnsOnWake(t *testing.T) {
	in, _ := pipeInput(t)
	go func() {
		time.Sleep(30 * time.Millisecond)
		in.wake()
	}()
	start := time.Now()
	in.wait(time.Second)
	assert.That(t, time.Since(start) < 500*time.Millisecond, "wake must end the wait")
}

func TestFdInput_WaitTimesOut(t *testing.T) {
	in, _ := pipeInput(t)
	start := time.Now()
	in.wait(30 * time.Millisecond)
	assert.That(t, time.Since(start) >= 30*time.Millisecond, "wait must hold for the timeout with nothing to read")
}

// Fini can call wake before readLoop has run setup, or after teardown; both
// must be no-ops rather than writes to a descriptor that is not ours.
func TestFdInput_WakeOutsideSetupIsSafe(t *testing.T) {
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	defer pr.Close()
	defer pw.Close()
	in := inputFor(pr).(*fdInput)
	in.wake()
	require.NoError(t, in.setup())
	in.teardown()
	in.wake()
}

// countingInput wraps an fdInput and counts reads; wait and wake pass
// through so readLoop takes the blocking path.
type countingInput struct {
	inner *fdInput
	reads atomic.Int64
}

func (c *countingInput) setup() error { return c.inner.setup() }
func (c *countingInput) teardown()    { c.inner.teardown() }
func (c *countingInput) read(buf []byte) (int, error) {
	c.reads.Add(1)
	return c.inner.read(buf)
}
func (c *countingInput) wait(d time.Duration) { c.inner.wait(d) }
func (c *countingInput) wake()                { c.inner.wake() }

// An idle readLoop blocks in wait; it does not wake every couple of
// milliseconds to find nothing. 300ms of silence used to cost ~150 reads.
func TestReadLoop_IdleWaitsInsteadOfSpinning(t *testing.T) {
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	defer pr.Close()
	defer pw.Close()
	var out bytes.Buffer
	s := &Screen{
		hooks:  stubHooks(80, 24, nil, nil),
		in:     pr,
		out:    &out,
		height: 5,
	}
	c := &countingInput{inner: inputFor(pr).(*fdInput)}
	s.input = c
	require.NoError(t, s.Init())
	time.Sleep(300 * time.Millisecond)
	s.Fini()
	reads := c.reads.Load()
	assert.That(t, reads <= 5, "idle readLoop made %d reads in 300ms", reads)
}

// Fini wakes a readLoop blocked in wait, so it returns well inside the wait
// timeout rather than at its end.
func TestScreen_FiniWakesBlockedReadLoop(t *testing.T) {
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	defer pr.Close()
	defer pw.Close()
	var out bytes.Buffer
	s := &Screen{
		hooks:  stubHooks(80, 24, nil, nil),
		in:     pr,
		out:    &out,
		height: 5,
	}
	require.NoError(t, s.Init())
	time.Sleep(20 * time.Millisecond) // let readLoop reach wait
	start := time.Now()
	s.Fini()
	assert.That(t, time.Since(start) < 100*time.Millisecond, "fini took %v", time.Since(start))
}
