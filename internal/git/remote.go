package git

import (
	"fmt"
	"strings"
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
// It accepts the shapes git surfaces in the wild:
//
//	https://github.com/USER/REPO(.git)
//	git@github.com:USER/REPO(.git)
//	ssh://git@github.com/USER/REPO(.git)
//	git://github.com/USER/REPO(.git)
//
// ok is false for any non-github host or a URL it can't decompose -- the caller
// treats those as "unknown pattern" rather than a naming violation.
func ParseGitHubURL(raw string) (user, repo string, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", false
	}
	// drop scheme (scheme://) if present
	if i := strings.Index(s, "://"); i != -1 {
		s = s[i+3:]
	}
	// drop userinfo (git@) if present
	if i := strings.LastIndex(s, "@"); i != -1 {
		s = s[i+1:]
	}
	// host is up to the first '/' (url form) or ':' (scp form)
	i := strings.IndexAny(s, "/:")
	if i == -1 {
		return "", "", false
	}
	host, path := s[:i], s[i+1:]
	if !strings.EqualFold(host, "github.com") {
		return "", "", false
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
