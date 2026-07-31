package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/lczyk/gitgum/internal/doctor"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/pr"
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
	// lsRemote overrides the pre-flight ref listing; nil uses the real one.
	lsRemote func(remote string) (string, error)
}

func (c *CloneCommand) refs() func(string) (string, error) {
	if c.lsRemote != nil {
		return c.lsRemote
	}
	return c.repo().LsRemote
}

// routePlan is what the pre-flight made of a url's route: at most one of these
// is set, and each drives a different part of the clone.
type routePlan struct {
	branch string // RouteTree: resolved branch, cloned directly via --branch
	pr     pr.Ref // RoutePR: checked out after the clone
	commit string // RouteCommit: detached at after the clone
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
	parsed, ok := git.ParseForgeURL(c.Args.URL)
	ref, route := parsed.Ref, parsed.Route

	if ok {
		if done, err := c.checkExistingDest(ref, route); done {
			return err
		}
	}

	var plan clonePlan
	var rp routePlan
	switch {
	case ok && ref.Shorthand():
		resolved, err := c.resolveShorthand(ref)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.err(), "resolved %s -> %s\n",
			paint(ansiBoldCyan, ref.Path), resolved.URL())
		ref = resolved
		fallthrough

	case ok && ref.Forge != git.ForgeUnknown:
		var err error
		// Everything the route needs is settled before a single object moves,
		// so a request that cannot be satisfied costs nothing.
		if rp, err = c.preflight(ref, route); err != nil {
			return err
		}
		plan = buildClonePlan(ref, route, rp.branch, c.Args.URL, c.Args.Dir, c.Depth)

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
	if err := c.repo().RunWriteStream(plan.args...); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
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

	return c.applyRoute(plan, rp)
}

// applyRoute finishes what the url asked for once the repo is on disk. A
// branch route is already done -- the clone landed on it. Failure here leaves
// the repo exactly as a plain clone would have: the pre-flight has already
// ruled out the likely causes, so anything reaching this point is unexpected
// and shouldn't leave a half-made PR branch behind.
func (c *CloneCommand) applyRoute(plan clonePlan, rp routePlan) error {
	cloned := git.Repo{Dir: plan.dir}
	switch {
	case rp.pr.Number != 0:
		landed, _ := cloned.GetCurrentBranch() // the branch a plain clone leaves you on
		sub := &CheckoutPRCommand{cmdIO: cmdIO{Out: c.Out, Err: c.Err, UI: c.UI, Repo: cloned}}
		if err := sub.checkoutPR(plan.remote, rp.pr.Number, rp.pr.Type); err != nil {
			undoPRCheckout(cloned, landed, pr.BranchName(plan.remote, rp.pr.Number))
			return fmt.Errorf("cloned into %s, but checking out pull request #%d failed: %w",
				plan.dir, rp.pr.Number, err)
		}
	case rp.commit != "":
		return c.checkoutCommit(cloned, plan, rp.commit)
	}
	return nil
}

// checkoutCommit lands on the commit a url named. A shallow clone usually
// won't have it, so it is fetched on its own -- one commit and its tree, not
// the history between here and there, which keeps the clone as shallow as was
// asked for. That leaves the commit with no local ancestry, which is offered
// separately because reaching the commit and having history around it are
// different wants, and whoever passed --depth has already said something about
// the second.
//
// A server that refuses unadvertised-object requests leaves the repo exactly as
// a plain clone would: default branch, nothing detached, and a warning saying
// so rather than a failure, since the clone itself succeeded.
func (c *CloneCommand) checkoutCommit(cloned git.Repo, plan clonePlan, sha string) error {
	if !cloned.HasObject(sha) {
		if err := cloned.FetchObject(plan.remote, sha); err != nil || !cloned.HasObject(sha) {
			fmt.Fprintf(c.err(), "%s %s is outside this shallow clone and %s would not serve it on its own.\n"+
				"      the clone is fine and sits on the default branch; re-run without --depth to reach that commit.\n",
				paint(ansiBoldYellow, "warning:"), paint(ansiBoldCyan, sha), plan.remote)
			return nil
		}
	}

	if err := cloned.Checkout(sha); err != nil {
		return fmt.Errorf("cloned into %s, but checking out commit %s failed: %w", plan.dir, sha, err)
	}
	fmt.Fprintf(c.out(), "Detached at %s. To keep work from here, cut a branch with %s.\n",
		paint(ansiBoldCyan, sha), paint(ansiBoldCyan, "gg branch"))

	c.offerDeepen(cloned, plan.remote, sha)
	return nil
}

// offerDeepen asks whether to pull in the history around a commit that was
// fetched on its own. The commit's own date is the only measure available:
// how many commits deep it sits cannot be computed without first fetching the
// very history the question is about. Declining is the expected answer often
// enough that this never fails the command.
func (c *CloneCommand) offerDeepen(cloned git.Repo, remote, sha string) {
	if !cloned.IsShallow() {
		return // full clone already has everything around it
	}
	since, err := cloned.CommitDate(sha)
	if err != nil {
		return
	}
	fmt.Fprintf(c.out(), "Its history is not here -- the commit was fetched on its own.\n")
	confirmed, err := c.sel().Confirm(
		fmt.Sprintf("Deepen the clone to %s so history around it is readable?", since), false)
	if err != nil || !confirmed {
		return
	}
	if err := cloned.Deepen(remote, since); err != nil {
		fmt.Fprintf(c.err(), "%s deepening failed; the commit is still checked out: %v\n",
			paint(ansiBoldYellow, "warning:"), err)
	}
}

// undoPRCheckout returns a freshly cloned repo to the state a plain clone would
// have left, after the PR checkout failed part-way. Best-effort: the caller is
// already reporting a failure, and a repo that resists tidying is not a second
// error worth stacking on the first.
func undoPRCheckout(r git.Repo, landed, branch string) {
	if landed != "" {
		_ = r.Checkout(landed)
	}
	if branch != "" && r.BranchExists(branch) {
		_, _, _ = r.RunWrite("branch", "-D", branch)
	}
}

// preflight resolves what the route points at before anything is transferred,
// using one ref listing. A PR must actually exist; a branch ref is matched
// against the remote's real refs, which is the only way to tell where a
// slash-containing branch name ends and a file path under it begins.
func (c *CloneCommand) preflight(ref git.RepoRef, route git.Route) (routePlan, error) {
	switch route.Kind {
	case git.RouteCommit:
		// shas are not advertised, so there is nothing to check against
		return routePlan{commit: route.Ref}, nil
	case git.RoutePR, git.RouteTree:
	default:
		return routePlan{}, nil
	}

	out, err := c.refs()(ref.URL())
	if err != nil {
		return routePlan{}, fmt.Errorf("listing refs on %s: %w", ref.URL(), err)
	}

	if route.Kind == git.RoutePR {
		for _, candidate := range pr.ParseRefs(ref.Forge, out) {
			if candidate.Number == route.Number {
				return routePlan{pr: candidate}, nil
			}
		}
		return routePlan{}, fmt.Errorf("%s has no pull request #%d", ref.Path, route.Number)
	}

	branch, ok := longestBranchMatch(remoteHeads(out), route.Ref)
	if !ok {
		return routePlan{}, fmt.Errorf("%s has no branch matching %q", ref.Path, route.Ref)
	}
	return routePlan{branch: branch}, nil
}

// remoteHeads pulls the branch names out of `git ls-remote` output.
func remoteHeads(lsRemoteOutput string) []string {
	var heads []string
	for _, line := range strings.Split(lsRemoteOutput, "\n") {
		_, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok && name != "" {
			heads = append(heads, name)
		}
	}
	return heads
}

// longestBranchMatch picks the branch a "/tree/<tail>" url meant. The tail is
// the branch plus, possibly, a path inside it -- and since branch names contain
// slashes too ("feat/x"), only the remote's actual refs can say where one ends.
// The longest match wins, which makes an exact branch name beat a shorter
// branch that merely prefixes it.
func longestBranchMatch(heads []string, tail string) (string, bool) {
	best := ""
	for _, head := range heads {
		if tail != head && !strings.HasPrefix(tail, head+"/") {
			continue
		}
		if len(head) > len(best) {
			best = head
		}
	}
	return best, best != ""
}

// checkExistingDest short-circuits the clone when the destination dir already
// exists and is non-empty -- `git clone` would refuse anyway, so decide up
// front, before any shorthand resolution hits the network. A repo whose remote
// already points at the requested user/repo means there is nothing to do
// (done, nil); any other occupant -- a plain dir, a nested path inside some
// other repo, a clone of something else -- is done with an error. An absent or
// empty dir is not done: git clone handles both.
//
// A PR url against a repo that is already here is the one case worth acting
// on rather than reporting: the number is the part the user actually typed.
// Checking it out moves an existing repo off whatever branch it was on, so it
// asks first.
func (c *CloneCommand) checkExistingDest(want git.RepoRef, route git.Route) (done bool, err error) {
	dir := c.Args.Dir
	if dir == "" {
		dir = want.Repo()
	}
	if entries, rerr := os.ReadDir(dir); rerr != nil || len(entries) == 0 {
		return false, nil
	}

	// .git presence distinguishes a repo root from a dir merely inside one --
	// rev-parse would happily answer for a parent repo.
	if _, serr := os.Stat(filepath.Join(dir, ".git")); serr != nil {
		return true, fmt.Errorf("destination %q already exists and is not a git repository", dir)
	}

	existing := git.Repo{Dir: dir}
	remotes, _ := existing.GetRemotes()
	var others []string
	for _, name := range remotes {
		url, uerr := existing.RemoteURL(name)
		if uerr != nil {
			continue
		}
		rref, rok := git.ParseRepoRef(url)
		if !rok {
			continue
		}
		if remoteMatches(rref, want) {
			fmt.Fprintf(c.out(), "%s is already cloned into %s (remote \"%s\").\n",
				paint(ansiBoldCyan, want.Path),
				paint(ansiBoldGreen, dir), paint(ansiBoldCyan, name))
			if route.Kind != git.RoutePR {
				return true, nil
			}
			return true, c.checkoutPRInExisting(dir, name, route.Number)
		}
		others = append(others, refLabel(rref))
	}
	if len(others) > 0 {
		return true, fmt.Errorf("destination %q already exists but is a clone of %s, not %s",
			dir, strings.Join(others, ", "), refLabel(want))
	}
	return true, fmt.Errorf("destination %q already exists and is a git repository, but none of its remotes point at %s",
		dir, refLabel(want))
}

// checkoutPRInExisting offers to check a PR out in the repo that is already at
// the destination. Declining is not a failure -- the repo is there, which was
// the other half of what the url asked for.
func (c *CloneCommand) checkoutPRInExisting(dir, remote string, number int) error {
	confirmed, err := c.sel().Confirm(
		fmt.Sprintf("Check pull request #%d out there?", number), false)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			return nil
		}
		return err
	}
	if !confirmed {
		fmt.Fprintf(c.out(), "Left alone. Run %s in there when you want it.\n",
			paint(ansiBoldCyan, fmt.Sprintf("gg checkout-pr %s/%d", remote, number)))
		return nil
	}

	existing := git.Repo{Dir: dir}
	sub := &CheckoutPRCommand{cmdIO: cmdIO{Out: c.Out, Err: c.Err, UI: c.UI, Repo: existing}}
	sub.Args.PR = fmt.Sprintf("%s/%d", remote, number)
	return sub.Execute(nil)
}

// refLabel renders a ref for messages: host-qualified when a host is known, so
// a same-slug-different-host mismatch doesn't read as "X is not X".
func refLabel(r git.RepoRef) string {
	slug := r.Path
	if r.Host != "" {
		return r.Host + "/" + slug
	}
	return slug
}

// remoteMatches reports whether an existing remote's ref points at the
// requested repo. A bare shorthand matches its slug on any modelled forge --
// exactly the set resolution would have probed -- while a host-pinned request
// must match the host too.
func remoteMatches(remote, want git.RepoRef) bool {
	if remote.Path != want.Path {
		return false
	}
	if want.Shorthand() {
		return remote.Forge != git.ForgeUnknown
	}
	return remote.Host == want.Host
}

// resolveShorthand turns a bare "user/repo" into a concrete forge by probing
// each modelled forge for the repo's existence. Zero matches errors; one match
// is used directly; several prompt a picker (github-first order preserved). It
// supplies the real network probe (unless one was injected for tests) and
// delegates to the package-level resolveShorthand.
func (c *CloneCommand) resolveShorthand(ref git.RepoRef) (git.RepoRef, error) {
	probe := c.probe
	if probe == nil {
		probe = func(u string) bool { return c.repo().RemoteReachable(u) }
	}
	return resolveShorthand(c.sel(), probe, ref)
}

// resolveShorthand probes each modelled forge (concurrently) for a bare
// "user/repo" ref and returns the ref pinned to the forge that has it. Shared
// by clone and add-remote. probe must be non-nil.
func resolveShorthand(sel ui.Selector, probe func(string) bool, ref git.RepoRef) (git.RepoRef, error) {
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
		return ref, fmt.Errorf("%s not found on github, gitlab or codeberg", ref.Path)
	case 1:
		ref.Forge = byURL[hitURLs[0]]
	default:
		picked, err := sel.Select(
			fmt.Sprintf("%s exists on several forges; pick one", ref.Path), hitURLs)
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
// isn't silently downgraded. A url carrying a route is reconstructed too --
// what the user pasted addresses a page, not a repo. The remote is named after
// the user either way.
func buildClonePlan(ref git.RepoRef, route git.Route, branch, raw, dir string, depth int) clonePlan {
	url := raw
	if isBareSpelling(raw) || route.Kind != git.RouteNone {
		url = ref.URL()
	}

	if dir == "" {
		dir = ref.Repo() // doctor's dir-naming wants the bare repo name.
	}
	var note string
	if base := filepath.Base(filepath.Clean(dir)); !doctor.MatchesRepoDir(base, ref.Repo()) {
		note = fmt.Sprintf("%s dir %q does not match doctor's %q / %q-N pattern; `gg doctor` will flag it.",
			paint(ansiBoldYellow, "warning:"), base, ref.Repo(), ref.Repo())
	}

	args := []string{"clone", "-o", ref.Owner()}
	if branch != "" {
		// land on the branch directly rather than checking out the default
		// first and moving afterwards
		args = append(args, "--branch", branch)
	}
	args = appendDepth(args, depth)
	args = append(args, url, dir)
	return clonePlan{args: args, remote: ref.Owner(), dir: dir, note: note}
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
