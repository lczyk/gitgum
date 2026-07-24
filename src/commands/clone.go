package commands

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/lczyk/gitgum/internal/doctor"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

// CloneCommand clones a repository the way `git clone` does, but pre-applies
// gg's doctor opinions so a fresh clone is doctor-clean from the first commit:
//
//   - remote-naming: the remote is named after the forge user/org (canonical),
//     not "origin". doctor would otherwise flag `git clone`'s default origin.
//   - dir-naming: the clone lands in a dir named exactly "<repo>", which is
//     what doctor's "<repo>" / "<repo>-N" pattern wants. an explicit DIR that
//     breaks the pattern still clones, with a heads-up.
//
// It also normalises the ways a repo can be spelled (see git.ParseRepoRef):
// full urls, host shorthand ("github.com/u/r", "www.github.com/u/r"), and bare
// "user/repo" shorthand -- the latter is resolved by probing github, gitlab and
// codeberg for existence, prompting with a picker when more than one matches.
//
// Urls on an unmodelled host (self-hosted forge, bitbucket, a local path) fall
// back to plain `git clone` with git's default origin.
type CloneCommand struct {
	cmdIO
	Depth int `long:"depth" description:"Create a shallow clone with the given history depth"`
	Args  struct {
		URL string `positional-arg-name:"URL" required:"yes"`
		Dir string `positional-arg-name:"DIR"`
	} `positional-args:"yes"`

	// probe overrides the repo-existence check; nil uses the real network probe.
	probe func(url string) bool
}

// clonePlan is the doctor-aware translation of a clone request into the
// `git clone` invocation gg will run, plus any advisory the user should see.
type clonePlan struct {
	args   []string // full `git clone ...` argv (sans the `git`)
	remote string   // remote name git will create ("" -> git's default origin)
	dir    string   // directory the clone lands in ("" -> git's default)
	note   string   // advisory to print to stderr ("" -> nothing to say)
}

func (c *CloneCommand) Execute(args []string) error {
	ref, ok := git.ParseRepoRef(c.Args.URL)

	var plan clonePlan
	switch {
	case ok && ref.Shorthand():
		resolved, err := c.resolveShorthand(ref)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.err(), "resolved %s -> %s\n",
			paint(ansiBoldCyan, ref.User+"/"+ref.Repo), resolved.URL())
		plan = buildClonePlan(resolved, c.Args.URL, c.Args.Dir, c.Depth)

	case ok && ref.Forge != git.ForgeUnknown:
		plan = buildClonePlan(ref, c.Args.URL, c.Args.Dir, c.Depth)

	default:
		note := ""
		if ok { // parsed, but a host we don't model
			note = fmt.Sprintf("%s %q is not a known forge (github/gitlab/codeberg); cloning with git's defaults (origin).",
				paint(ansiBoldYellow, "note:"), ref.Host)
		}
		plan = plainClonePlan(c.Args.URL, c.Args.Dir, c.Depth, note)
	}

	if plan.note != "" {
		fmt.Fprintln(c.err(), plan.note)
	}

	// clone streams progress live and preserves the user's git config
	// (credential helpers etc) so private repos authenticate.
	if _, stderr, err := c.repo().RunWriteStream(plan.args...); err != nil {
		return fmt.Errorf("git clone failed: %w\n%s", err, stderr)
	}

	// Show the whole tree's compact diffstat -- every file as an addition,
	// diffed against the (runtime-resolved) empty tree -- the same coloured
	// summary `gg pull` prints, so you see the shape of what you just cloned.
	// Best-effort: a diagnostic hiccup never fails a successful clone.
	if plan.dir != "" {
		cloned := git.Repo{Dir: plan.dir}
		if empty, err := cloned.EmptyTree(); err == nil {
			if summary, derr := compactSummary(cloned, empty, "HEAD"); derr == nil && summary != "" {
				fmt.Fprintln(c.out(), summary)
			}
		}
	}

	if plan.remote != "" {
		fmt.Fprintf(c.out(), "\nCloned into %s (remote \"%s\").\n",
			paint(ansiBoldGreen, plan.dir), paint(ansiBoldCyan, plan.remote))
	}
	return nil
}

// resolveShorthand turns a bare "user/repo" into a concrete forge by probing
// each modelled forge for the repo's existence. Zero matches errors; one match
// is used directly; several prompt a picker (github-first order preserved).
func (c *CloneCommand) resolveShorthand(ref git.RepoRef) (git.RepoRef, error) {
	probe := c.probe
	if probe == nil {
		probe = func(u string) bool { return c.repo().RemoteReachable(u) }
	}

	// probe every forge concurrently -- independent network reads, so the wait
	// is max(probe) not sum(probe). preference order (github first) is preserved
	// by collecting hits from the ordered forge list afterwards.
	forges := git.KnownForges()
	exists := make([]bool, len(forges))
	var wg sync.WaitGroup
	wg.Add(len(forges))
	for i, f := range forges {
		go func() {
			defer wg.Done()
			exists[i] = probe(ref.URLOn(f))
		}()
	}
	wg.Wait()

	byURL := map[string]git.Forge{}
	var hitURLs []string
	for i, f := range forges {
		if exists[i] {
			url := ref.URLOn(f)
			byURL[url] = f
			hitURLs = append(hitURLs, url)
		}
	}

	switch len(hitURLs) {
	case 0:
		return ref, fmt.Errorf("%s/%s not found on github, gitlab or codeberg", ref.User, ref.Repo)
	case 1:
		ref.Forge = byURL[hitURLs[0]]
	default:
		picked, err := c.sel().Select(
			fmt.Sprintf("%s/%s exists on several forges; pick one", ref.User, ref.Repo), hitURLs)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return ref, fmt.Errorf("aborted")
			}
			return ref, err
		}
		ref.Forge = byURL[picked]
	}
	ref.Host = ref.Forge.Host()
	return ref, nil
}

// buildClonePlan builds the git clone invocation for a resolved forge ref. When
// the input was a "bare" spelling (no scheme, no git@ userinfo) the url is
// reconstructed as canonical https so www./missing-scheme/shorthand all
// normalise; a real url (https/ssh/git) is cloned verbatim so an ssh remote
// isn't silently downgraded. The remote is named after the user either way.
func buildClonePlan(ref git.RepoRef, raw, dir string, depth int) clonePlan {
	url := raw
	if isBareSpelling(raw) {
		url = ref.URL()
	}

	if dir == "" {
		dir = ref.Repo // doctor's dir-naming wants the bare repo name.
	}
	var note string
	if base := filepath.Base(filepath.Clean(dir)); !doctor.MatchesRepoDir(base, ref.Repo) {
		note = fmt.Sprintf("%s dir %q does not match doctor's %q / %q-N pattern; `gg doctor` will flag it.",
			paint(ansiBoldYellow, "warning:"), base, ref.Repo, ref.Repo)
	}

	args := []string{"clone", "-o", ref.User}
	args = appendDepth(args, depth)
	args = append(args, url, dir)
	return clonePlan{args: args, remote: ref.User, dir: dir, note: note}
}

// plainClonePlan mirrors `git clone <url> [dir]` for inputs gg can't apply its
// naming opinions to (unmodelled host, local path).
func plainClonePlan(raw, dir string, depth int, note string) clonePlan {
	args := appendDepth([]string{"clone"}, depth)
	args = append(args, raw)
	if dir != "" {
		args = append(args, dir)
	}
	return clonePlan{args: args, dir: dir, note: note}
}

// isBareSpelling reports whether raw lacks a scheme and userinfo, i.e. it's a
// host-shorthand or bare "user/repo" that should be reconstructed rather than
// passed to git verbatim.
func isBareSpelling(raw string) bool {
	return !strings.Contains(raw, "://") && !strings.Contains(raw, "@")
}

// appendDepth adds the shallow-clone flags. --no-single-branch rides along with
// --depth because git otherwise implies --single-branch, which bakes a
// one-branch fetch refspec ("+refs/heads/main:refs/remotes/<r>/main") into the
// clone's config. Every branch but the cloned one then has an upstream that git
// cannot resolve -- `rev-parse @{u}` fatals with "not stored as a remote-tracking
// branch" -- so pull and push on it break, long after the clone is forgotten.
// Fetching every branch tip at the requested depth costs little and keeps the
// clone usable.
func appendDepth(args []string, depth int) []string {
	if depth > 0 {
		args = append(args, "--depth", strconv.Itoa(depth), "--no-single-branch")
	}
	return args
}
