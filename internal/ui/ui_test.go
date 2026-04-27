package ui

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/fuzzyfinder"
)

func TestConfirmWith(t *testing.T) {
	tests := []struct {
		name           string
		defaultYes     bool
		selectorReturn string
		want           bool
	}{
		{"default yes, picks yes", true, "yes", true},
		{"default yes, picks no", true, "no", false},
		{"default no, picks yes", false, "yes", true},
		{"default no, picks no", false, "no", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := confirmWith(
				func(_ string, _ []string, _ ...string) (string, error) {
					return tc.selectorReturn, nil
				},
				"Test?", tc.defaultYes,
			)
			assert.NoError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestConfirmWithSelectorError(t *testing.T) {
	sentinel := errors.New("boom")
	_, err := confirmWith(
		func(_ string, _ []string, _ ...string) (string, error) {
			return "", sentinel
		},
		"Test?", true,
	)
	assert.Error(t, err, sentinel)
}

func TestSelectEmptyOptions(t *testing.T) {
	_, err := Select("test", nil)
	assert.Error(t, err, assert.AnyError, "expected error for empty options")
}

func TestSelectAbortMapsToErrCancelled(t *testing.T) {
	_, err := selectWith(func(_ context.Context, _ *[]string, _ sync.Locker, _ fuzzyfinder.Opt) ([]int, error) {
		return nil, fuzzyfinder.ErrAbort
	}, 10, "test", []string{"a"})
	assert.Error(t, err, ErrCancelled)
}

func TestSelectFinderError(t *testing.T) {
	sentinel := errors.New("boom")
	_, err := selectWith(func(_ context.Context, _ *[]string, _ sync.Locker, _ fuzzyfinder.Opt) ([]int, error) {
		return nil, sentinel
	}, 10, "test", []string{"a"})
	assert.Error(t, err, sentinel)
}
