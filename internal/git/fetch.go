package git

import (
	"context"
	"fmt"
)

// Fetch runs `git fetch <remote> <refspec>` with live progress streamed
// to the user's terminal. Empty refspec fetches the remote's defaults.
func (r Repo) Fetch(remote, refspec string) error {
	args := []string{"fetch", remote}
	if refspec != "" {
		args = append(args, refspec)
	}
	if err := r.runWriteStreaming(context.Background(), args...); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}
	return nil
}

// FetchObject fetches one object by hash, bounded to that object alone. In a
// shallow clone this is how a commit outside the boundary is obtained without
// pulling the history between here and there -- git records a new shallow
// point for it rather than walking its ancestry.
//
// Servers gate unadvertised-object requests behind uploadpack.allowReachableSHA1InWant;
// github and gitlab enable it, plenty of self-hosted servers do not, so callers
// must be ready for this to fail for permission rather than for absence.
func (r Repo) FetchObject(remote, sha string) error {
	if err := r.runWriteStreaming(context.Background(), "fetch", "--depth", "1", remote, sha); err != nil {
		return fmt.Errorf("git fetch %s: %w", sha, err)
	}
	return nil
}

// Deepen extends a shallow clone's boundary back to a date, so history around
// an already-fetched commit becomes readable. Dates come from git itself
// (CommitDate), so the format matches what --shallow-since accepts.
func (r Repo) Deepen(remote, since string) error {
	if err := r.runWriteStreaming(context.Background(), "fetch", "--shallow-since="+since, remote); err != nil {
		return fmt.Errorf("git fetch --shallow-since: %w", err)
	}
	return nil
}
