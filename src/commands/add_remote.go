package commands

import (
	"fmt"

	"github.com/lczyk/gitgum/internal/git"
)

// AddRemoteCommand adds a git remote using gg's naming rules: the remote is
// named after the forge user/org (canonical), matching what `gg clone` creates
// and `gg doctor` remote-naming expects -- not "origin".
//
// It accepts the same repo spellings as clone (see git.ParseRepoRef): full
// urls, host shorthand ("github.com/u/r"), forge-name shorthand ("github/u/r"),
// and bare "user/repo" resolved by probing github/gitlab/codeberg. Local-path
// remotes are not supported.
//
// The add is transactional with respect to the fetch: the remote is created,
// then fetched; if the fetch fails the remote is removed again, so a bad url or
// unreachable repo never leaves a half-configured remote behind.
type AddRemoteCommand struct {
	cmdIO
	Args struct {
		Remote string `positional-arg-name:"REMOTE" required:"yes"`
	} `positional-args:"yes"`

	// probe overrides the repo-existence check for bare shorthand; nil uses the
	// real network probe.
	probe func(url string) bool
}

func (c *AddRemoteCommand) Execute(args []string) error {
	if err := c.repo().CheckInRepo(); err != nil {
		return err
	}

	ref, ok := git.ParseRepoRef(c.Args.Remote)
	if !ok {
		return fmt.Errorf("%q is not a repo ref i can resolve "+
			"(expected user/repo, forge/user/repo, or a forge url; local paths are not supported)",
			c.Args.Remote)
	}

	url, err := c.resolveURL(ref)
	if err != nil {
		return err
	}
	name := ref.User

	// Duplicate handling: a same-named remote is an error unless it already
	// points at this exact url (an idempotent re-add is a no-op).
	remotes, err := c.repo().GetRemotes()
	if err != nil {
		return fmt.Errorf("listing remotes: %w", err)
	}
	for _, r := range remotes {
		if r != name {
			continue
		}
		existing, _ := c.repo().RemoteURL(name)
		if existing == url {
			fmt.Fprintf(c.out(), "remote %s already points at %s; nothing to do.\n",
				paint(ansiBoldCyan, name), url)
			return nil
		}
		return fmt.Errorf("remote %q already exists (points at %s); "+
			"use `git remote set-url %s %s` to repoint it", name, existing, name, url)
	}

	if err := c.repo().AddRemote(name, url); err != nil {
		return fmt.Errorf("adding remote: %w", err)
	}
	fmt.Fprintf(c.out(), "added remote %s -> %s\n", paint(ansiBoldCyan, name), url)

	// Transactional fetch: roll the remote back if the fetch fails, so an
	// unreachable/misspelled repo doesn't leave a dangling remote.
	if err := c.repo().Fetch(name, ""); err != nil {
		if rmErr := c.repo().RemoveRemote(name); rmErr != nil {
			return fmt.Errorf("fetch from new remote %q failed (%w); "+
				"rolling it back also failed: %v", name, err, rmErr)
		}
		return fmt.Errorf("fetch from new remote %q failed, removed it again: %w", name, err)
	}

	fmt.Fprintf(c.out(), "fetched %s.\n", paint(ansiBoldGreen, name))
	return nil
}

// resolveURL turns a parsed ref into the clone url the remote will use. A bare
// "user/repo" shorthand is resolved by probing the modelled forges (with a
// picker on multiple hits); a known-forge or unmodelled-host ref uses its url
// directly. Real urls (https/ssh/git) are preserved verbatim so an ssh remote
// isn't silently rewritten to https; bare spellings are canonicalised.
func (c *AddRemoteCommand) resolveURL(ref git.RepoRef) (string, error) {
	switch {
	case ref.Shorthand():
		probe := c.probe
		if probe == nil {
			probe = func(u string) bool { return c.repo().RemoteReachable(u) }
		}
		resolved, err := resolveShorthand(c.sel(), probe, ref)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(c.err(), "resolved %s -> %s\n",
			paint(ansiBoldCyan, resolved.User+"/"+resolved.Repo), resolved.URL())
		return resolved.URL(), nil
	case !isBareSpelling(c.Args.Remote):
		return c.Args.Remote, nil // real url: keep the exact scheme/transport
	default:
		return ref.URL(), nil // bare spelling on a known/unmodelled host: canonicalise
	}
}
