package commands

import (
	"context"
	"fmt"

	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

// stubSelector replays canned answers for selector calls. Tests use it to
// drive command Execute paths without a TTY. Each method consumes one entry
// from its queue; running out fails the call so missing scripted answers
// surface as test failures rather than hangs.
type stubSelector struct {
	selectAnswers      []string
	multiSelectAnswers [][]string
	promptAnswers      []string
	confirmAnswers     []bool
	// selectErrs scripts errors for Select / SelectStream, consumed before
	// selectAnswers. Lets tests drive the cancellation paths (ui.ErrCancelled)
	// that a scripted answer can't reach.
	selectErrs []error

	selectCalls      []selectCall
	multiSelectCalls []selectCall
	promptCalls      []selectCall
	confirmCalls     []confirmCall
}

type selectCall struct {
	Prompt  string
	Options []string
	Stream  bool
}

type confirmCall struct {
	Prompt     string
	DefaultYes bool
}

func (s *stubSelector) Select(prompt string, options []string, initialQuery ...string) (string, error) {
	s.selectCalls = append(s.selectCalls, selectCall{Prompt: prompt, Options: options})
	if err := s.nextSelectErr(); err != nil {
		return "", err
	}
	if len(s.selectAnswers) == 0 {
		return "", fmt.Errorf("stubSelector: unexpected Select call %q", prompt)
	}
	answer := s.selectAnswers[0]
	s.selectAnswers = s.selectAnswers[1:]
	return answer, nil
}

func (s *stubSelector) SelectStream(ctx context.Context, prompt string, src ff.Source, unselectable func(string) bool) (string, error) {
	s.selectCalls = append(s.selectCalls, selectCall{Prompt: prompt, Stream: true})
	if err := s.nextSelectErr(); err != nil {
		return "", err
	}
	if len(s.selectAnswers) == 0 {
		return "", fmt.Errorf("stubSelector: unexpected SelectStream call %q", prompt)
	}
	answer := s.selectAnswers[0]
	s.selectAnswers = s.selectAnswers[1:]
	// mirror the real picker: an unselectable item can never be returned.
	if unselectable != nil && unselectable(answer) {
		return "", fmt.Errorf("stubSelector: answer %q is unselectable", answer)
	}
	return answer, nil
}

// nextSelectErr pops the next scripted Select error, or nil when none is left.
func (s *stubSelector) nextSelectErr() error {
	if len(s.selectErrs) == 0 {
		return nil
	}
	err := s.selectErrs[0]
	s.selectErrs = s.selectErrs[1:]
	return err
}

func (s *stubSelector) MultiSelect(prompt string, options []string) ([]string, error) {
	s.multiSelectCalls = append(s.multiSelectCalls, selectCall{Prompt: prompt, Options: options})
	if len(s.multiSelectAnswers) == 0 {
		return nil, fmt.Errorf("stubSelector: unexpected MultiSelect call %q", prompt)
	}
	answer := s.multiSelectAnswers[0]
	s.multiSelectAnswers = s.multiSelectAnswers[1:]
	return answer, nil
}

func (s *stubSelector) Prompt(question string) (string, error) {
	s.promptCalls = append(s.promptCalls, selectCall{Prompt: question})
	if len(s.promptAnswers) == 0 {
		return "", fmt.Errorf("stubSelector: unexpected Prompt call %q", question)
	}
	answer := s.promptAnswers[0]
	s.promptAnswers = s.promptAnswers[1:]
	return answer, nil
}

func (s *stubSelector) Confirm(prompt string, defaultYes bool) (bool, error) {
	s.confirmCalls = append(s.confirmCalls, confirmCall{Prompt: prompt, DefaultYes: defaultYes})
	if len(s.confirmAnswers) == 0 {
		return false, fmt.Errorf("stubSelector: unexpected Confirm call %q", prompt)
	}
	answer := s.confirmAnswers[0]
	s.confirmAnswers = s.confirmAnswers[1:]
	return answer, nil
}
