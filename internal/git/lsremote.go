package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// probeTimeout bounds RemoteBranchTip: long enough for a slow link, short
// enough that an unreachable host fails the command instead of holding it for
// TCP's own timeout.
var probeTimeout = 15 * time.Second

// RemoteBranchTip asks remote for the tip of branch, in one ls-remote. exists
// is false when the remote answered without that branch; err is non-nil when
// it couldn't be asked at all -- unreachable, refused auth, or no answer
// within probeTimeout.
func (r Repo) RemoteBranchTip(remote, branch string) (tip string, exists bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	stdout, stderr, err := r.runReadNet(ctx, "ls-remote", "--exit-code", remote, "refs/heads/"+branch)
	if err == nil {
		tip, _, _ = strings.Cut(strings.TrimSpace(stdout), "\t")
		return tip, true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return "", false, nil
	}
	if ctx.Err() != nil {
		return "", false, fmt.Errorf("git ls-remote %s: no answer within %s", remote, probeTimeout)
	}
	return "", false, fmt.Errorf("git ls-remote %s: %w: %s", remote, err, strings.TrimSpace(stderr))
}

// LsRemote returns the raw stdout of `git ls-remote <remote>`. Network op,
// but read-shaped (no working-tree mutation), so runs under the read profile
// for parse-stable output -- via runReadNet so global/system gitconfig (and
// thus private-repo auth) is preserved.
func (r Repo) LsRemote(remote string) (string, error) {
	stdout, stderr, err := r.runReadNet(context.Background(), "ls-remote", remote)
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s: %w: %s", remote, err, stderr)
	}
	return stdout, nil
}
