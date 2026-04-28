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
	selectAnswers  []string
	confirmAnswers []bool

	selectCalls  []selectCall
	confirmCalls []confirmCall
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
	if len(s.selectAnswers) == 0 {
		return "", fmt.Errorf("stubSelector: unexpected Select call %q", prompt)
	}
	answer := s.selectAnswers[0]
	s.selectAnswers = s.selectAnswers[1:]
	return answer, nil
}

func (s *stubSelector) SelectStream(ctx context.Context, prompt string, src *ff.SliceSource) (string, error) {
	s.selectCalls = append(s.selectCalls, selectCall{Prompt: prompt, Stream: true})
	if len(s.selectAnswers) == 0 {
		return "", fmt.Errorf("stubSelector: unexpected SelectStream call %q", prompt)
	}
	answer := s.selectAnswers[0]
	s.selectAnswers = s.selectAnswers[1:]
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
