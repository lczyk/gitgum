package git

import (
	"context"
	"fmt"
)

// RemoteURL returns the URL configured for a remote (the fetch URL).
func (r Repo) RemoteURL(name string) (string, error) {
	stdout, stderr, err := r.run("remote", "get-url", name)
	if err != nil {
		return "", fmt.Errorf("git remote get-url %s: %w: %s", name, err, stderr)
	}
	return stdout, nil
}

func RemoteURL(name string) (string, error) { return CWD().RemoteURL(name) }

// ParseGitHubURL extracts the user/org and repo name from a github remote URL.
// It is a github-only view of ParseRepoRef, kept for callers that only care
// about github; new code that needs to distinguish forges should use
// ParseRepoRef directly.
func ParseGitHubURL(raw string) (user, repo string, ok bool) {
	ref, ok := ParseRepoRef(raw)
	if !ok || ref.Forge != ForgeGitHub {
		return "", "", false
	}
	return ref.User, ref.Repo, true
}

// RemoteReachable reports whether `git ls-remote <url>` succeeds, i.e. the repo
// exists and is readable (public, or private with the caller's creds). A
// network failure or a missing repo both read as false; GIT_TERMINAL_PROMPT=0
// keeps auth prompts from hanging the probe. Used by clone to resolve a bare
// "user/repo" shorthand across candidate forges.
func (r Repo) RemoteReachable(url string) bool {
	_, _, err := r.runReadNet(context.Background(), "ls-remote", url)
	return err == nil
}

func RemoteReachable(url string) bool { return CWD().RemoteReachable(url) }
