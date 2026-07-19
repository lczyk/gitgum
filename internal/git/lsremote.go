package git

import (
	"context"
	"fmt"
)

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

func LsRemote(remote string) (string, error) { return CWD().LsRemote(remote) }
