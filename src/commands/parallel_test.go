package commands

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/lczyk/assert"
)

func TestRunConcurrent_AllNilReturnsNil(t *testing.T) {
	t.Parallel()
	var ran int32
	err := runConcurrent(
		func() error { atomic.AddInt32(&ran, 1); return nil },
		func() error { atomic.AddInt32(&ran, 1); return nil },
		func() error { atomic.AddInt32(&ran, 1); return nil },
	)
	assert.NoError(t, err)
	assert.Equal(t, int(atomic.LoadInt32(&ran)), 3)
}

// The first error in fn order wins even when a later fn also fails -- delete's
// empty-repo guard relies on this to beat a sibling's cryptic error.
func TestRunConcurrent_FirstErrorWins(t *testing.T) {
	t.Parallel()
	first := errors.New("first")
	second := errors.New("second")
	err := runConcurrent(
		func() error { return nil },
		func() error { return first },
		func() error { return second },
	)
	assert.That(t, errors.Is(err, first), "should return the first erroring fn's error")
}

// Every fn runs to completion even when an earlier one has already failed --
// there's no early cancel.
func TestRunConcurrent_AllRunDespiteError(t *testing.T) {
	t.Parallel()
	var ran int32
	err := runConcurrent(
		func() error { atomic.AddInt32(&ran, 1); return errors.New("boom") },
		func() error { atomic.AddInt32(&ran, 1); return nil },
		func() error { atomic.AddInt32(&ran, 1); return nil },
	)
	assert.Error(t, err, assert.AnyError)
	assert.Equal(t, int(atomic.LoadInt32(&ran)), 3)
}
