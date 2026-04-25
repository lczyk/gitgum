package ui

import (
	"context"
	"errors"
	"sync"
	"testing"

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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
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
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestSelectEmptyOptions(t *testing.T) {
	_, err := Select("test", nil)
	if err == nil {
		t.Fatal("expected error for empty options, got nil")
	}
}

func TestSelectAbortMapsToErrCancelled(t *testing.T) {
	_, err := selectWith(func(_ context.Context, _ *[]string, _ sync.Locker, _ fuzzyfinder.Opt) ([]int, error) {
		return nil, fuzzyfinder.ErrAbort
	}, "test", []string{"a"})
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("expected ErrCancelled, got %v", err)
	}
}

func TestSelectFinderError(t *testing.T) {
	sentinel := errors.New("boom")
	_, err := selectWith(func(_ context.Context, _ *[]string, _ sync.Locker, _ fuzzyfinder.Opt) ([]int, error) {
		return nil, sentinel
	}, "test", []string{"a"})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel wrapped in finder error, got %v", err)
	}
}
