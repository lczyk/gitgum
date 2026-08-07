package git

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// RemoteURLs lists every url configured for every remote, "<name> <url>" per
// entry, sorted and deduplicated: a remote whose push url matches its fetch
// url is one entry, one whose push url differs is two.
//
// It reads the config rather than `git remote -v`, whose columns are laid out
// for a reader and whose "(fetch)" / "(push)" suffixes would have to be parsed
// back off. A remote name may contain '.', so the key is trimmed at both ends
// rather than split on the separator.
func (r Repo) RemoteURLs() ([]string, error) {
	stdout, _, err := r.run("config", "--get-regexp", `^remote\..*\.(url|pushurl)$`)
	if err != nil {
		// config exits non-zero when nothing matches, which is a repo with no
		// remotes rather than a repo that could not be read.
		return nil, nil
	}
	seen := map[string]bool{}
	var out []string
	for line := range strings.SplitSeq(stdout, "\n") {
		key, url, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || url == "" {
			continue
		}
		name := strings.TrimPrefix(key, "remote.")
		name = strings.TrimSuffix(strings.TrimSuffix(name, ".pushurl"), ".url")
		if name == "" {
			continue
		}
		entry := name + " " + url
		if !seen[entry] {
			seen[entry] = true
			out = append(out, entry)
		}
	}
	sort.Strings(out)
	return out, nil
}

// RemoteURL returns the URL configured for a remote (the fetch URL).
func (r Repo) RemoteURL(name string) (string, error) {
	stdout, stderr, err := r.run("remote", "get-url", name)
	if err != nil {
		return "", fmt.Errorf("git remote get-url %s: %w: %s", name, err, stderr)
	}
	return stdout, nil
}

// ParseGitHubURL extracts the user/org and repo name from a github remote URL.
// It is a github-only view of ParseRepoRef, kept for callers that only care
// about github; new code that needs to distinguish forges should use
// ParseRepoRef directly.
func ParseGitHubURL(raw string) (user, repo string, ok bool) {
	ref, ok := ParseRepoRef(raw)
	if !ok || ref.Forge != ForgeGitHub {
		return "", "", false
	}
	return ref.Owner(), ref.Repo(), true
}

// AddRemote configures a new remote named name pointing at url
// (`git remote add name url`). It errors if a remote by that name already
// exists.
func (r Repo) AddRemote(name, url string) error {
	if _, stderr, err := r.RunWrite("remote", "add", name, url); err != nil {
		return fmt.Errorf("git remote add %s: %w: %s", name, err, stderr)
	}
	return nil
}

// RemoveRemote deletes the named remote (`git remote remove name`). Used to
// roll back a freshly-added remote when a follow-up fetch fails.
func (r Repo) RemoveRemote(name string) error {
	if _, stderr, err := r.RunWrite("remote", "remove", name); err != nil {
		return fmt.Errorf("git remote remove %s: %w: %s", name, err, stderr)
	}
	return nil
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
