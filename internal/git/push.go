package git

import (
	"context"
	"fmt"
)

// Push runs `git push` with no args, relying on the current branch's
// upstream config. Stderr is streamed live so the user sees progress;
// GIT_TERMINAL_PROMPT=0 is set in the env, so missing creds fail fast
// rather than hang on a tty prompt (cred helpers still work).
func (r Repo) Push() error {
	if err := r.runWriteStreaming(context.Background(), "push"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

func Push() error { return CWD().Push() }
