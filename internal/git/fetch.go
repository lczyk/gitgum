package git

import (
	"context"
	"fmt"
	"strings"
)

// Fetch runs `git fetch <remote> <refspec>` with live progress streamed
// to the user's terminal. Empty refspec fetches the remote's defaults.
func (r Repo) Fetch(remote, refspec string) error {
	args := []string{"fetch", remote}
	if refspec != "" {
		args = append(args, refspec)
	}
	if _, stderr, err := r.runWriteStreaming(context.Background(), args...); err != nil {
		return fmt.Errorf("git fetch: %w: %s", err, strings.TrimSpace(stderr))
	}
	return nil
}

func Fetch(remote, refspec string) error { return CWD().Fetch(remote, refspec) }
