package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

// This file is the doctor engine: the repo-health rules and the runner that
// produces findings, with no CLI/dispatch coupling. The `doctor` command (and
// any other caller that wants to reason about repo health) runs Diagnose and
// presents the []Finding however it likes. Command wiring and rendering live in
// doctor_command.go.

// Severity classifies a doctor finding.
//
//   - Fixable: a known, deterministic inconsistency with a suggested remediation
//     (e.g. a misnamed github remote). doctor prints the fix; it does not run it.
//   - Warning: something doctor can't reason about (custom remote url, divergent
//     upstreams). informational only; never blocks gg.
type Severity int

const (
	SevFixable Severity = iota
	SevWarning
)

// Finding is a single issue reported by a check.
type Finding struct {
	Check    string   // check id, e.g. "remote-naming"
	Severity Severity // Fixable | Warning
	Message  string   // human description of what's wrong
	Fix      string   // suggested remediation command; only meaningful for SevFixable
}

// doctorChecks is the ordered set of diagnostics doctor runs. Each returns its
// findings; internal git errors surface as warnings rather than aborting the run.
var doctorChecks = []func(git.Repo) []Finding{
	checkRemoteNaming,
	checkUpstreams,
	checkLayout,
}

// Diagnose runs every doctor check against r and returns the combined findings
// in discovery order (unsorted -- presentation decides ordering). It is the
// reusable entry point: the `doctor` command renders the result, but any caller
// wanting to reason about repo health can run this and inspect the findings.
func Diagnose(r git.Repo) []Finding {
	var findings []Finding
	for _, check := range doctorChecks {
		findings = append(findings, check(r)...)
	}
	return findings
}

// checkRemoteNaming enforces gg's opinion that a remote on a known forge is
// named after its user/org (nsklikas), not "origin". Urls on an unmodelled
// forge (or unparseable ones) can't be judged, so they surface as warnings
// rather than violations.
func checkRemoteNaming(r git.Repo) []Finding {
	remotes, err := r.GetRemotes()
	if err != nil {
		return []Finding{{Check: "remote-naming", Severity: SevWarning,
			Message: fmt.Sprintf("could not list remotes: %v", err)}}
	}
	var out []Finding
	for _, name := range remotes {
		url, err := r.RemoteURL(name)
		if err != nil {
			out = append(out, Finding{Check: "remote-naming", Severity: SevWarning,
				Message: fmt.Sprintf("could not read url for remote %q: %v", name, err)})
			continue
		}
		ref, ok := git.ParseRepoRef(url)
		if !ok || ref.Forge == git.ForgeUnknown {
			out = append(out, Finding{Check: "remote-naming", Severity: SevWarning,
				Message: fmt.Sprintf("remote %q has an unrecognised url %q; cannot verify its name", name, url)})
			continue
		}
		if name != ref.User {
			out = append(out, Finding{Check: "remote-naming", Severity: SevFixable,
				Message: fmt.Sprintf("remote %q points at %s user %q but is not named after it", name, ref.Forge, ref.User),
				Fix:     fmt.Sprintf("git remote rename %s %s", name, ref.User)})
		}
	}
	return out
}

// checkUpstreams warns when local branches track more than one remote. Which
// one is "right" is intent gg can't infer, so this is unfixable.
func checkUpstreams(r git.Repo) []Finding {
	branches, err := r.GetLocalBranches()
	if err != nil {
		return []Finding{{Check: "upstream-consistency", Severity: SevWarning,
			Message: fmt.Sprintf("could not list branches: %v", err)}}
	}
	remotes := map[string]bool{}
	for _, b := range branches {
		remote, _, err := r.GetBranchUpstream(b)
		if err != nil || remote == "" {
			continue
		}
		remotes[remote] = true
	}
	if len(remotes) <= 1 {
		return nil
	}
	return []Finding{{Check: "upstream-consistency", Severity: SevWarning,
		Message: fmt.Sprintf("local branches track more than one remote (%s); gg expects a single consistent upstream",
			strings.Join(sortedKeys(remotes), ", "))}}
}

// checkLayout covers the directory/worktree opinions in one pass, since they
// share the worktree list and the canonical repo name:
//
//   - dir-naming: every worktree dir matches "<repo>" or "<repo>-N".
//   - worktree-parent: all worktrees live under one parent dir.
//   - adjacent-worktree: sibling dirs matching the naming pattern that aren't
//     worktrees of this repo (likely stray clones). Names only -- siblings are
//     never opened.
func checkLayout(r git.Repo) []Finding {
	wts, err := r.Worktrees()
	if err != nil {
		return []Finding{{Check: "worktree-parent", Severity: SevWarning,
			Message: fmt.Sprintf("could not list worktrees: %v", err)}}
	}
	if len(wts) == 0 {
		return nil
	}

	repoName, nameOK, out := canonicalRepoName(r)

	// The main worktree is listed first; its parent is the canonical home dir.
	mainParent := filepath.Dir(wts[0].Path)
	wtPaths := map[string]bool{}
	parents := map[string][]string{}

	for _, wt := range wts {
		if wt.Bare {
			continue
		}
		clean := filepath.Clean(wt.Path)
		wtPaths[clean] = true
		parent := filepath.Dir(clean)
		parents[parent] = append(parents[parent], filepath.Base(clean))

		if nameOK && !matchesRepoDir(filepath.Base(clean), repoName) {
			out = append(out, Finding{Check: "dir-naming", Severity: SevFixable,
				Message: fmt.Sprintf("worktree dir %q does not match the %q / %q-N pattern", clean, repoName, repoName),
				Fix:     fmt.Sprintf("mv %s %s", clean, filepath.Join(parent, repoName))})
		}
	}

	// worktree-parent: everything should sit next to the main worktree.
	if len(parents) > 1 {
		for _, parent := range sortedKeys(toSet(parents)) {
			if parent == mainParent {
				continue
			}
			out = append(out, Finding{Check: "worktree-parent", Severity: SevWarning,
				Message: fmt.Sprintf("worktree(s) %s live in %q, not next to the main worktree in %q",
					strings.Join(parents[parent], ", "), parent, mainParent)})
		}
	}

	// adjacent-worktree: sibling dirs matching the pattern that aren't worktrees.
	if nameOK {
		entries, err := os.ReadDir(mainParent)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() || !matchesRepoDir(e.Name(), repoName) {
					continue
				}
				full := filepath.Join(mainParent, e.Name())
				if wtPaths[full] {
					continue
				}
				out = append(out, Finding{Check: "adjacent-worktree", Severity: SevWarning,
					Message: fmt.Sprintf("dir %q matches the repo naming pattern but is not a worktree of this repo", full)})
			}
		}
	}

	return out
}

// canonicalRepoName derives the repo name (the "REPO" in github.com/USER/REPO)
// agreed on by all known-forge remotes. ok is false when no such remote exists
// (nothing to check against) or when remotes disagree (returned as a warning).
func canonicalRepoName(r git.Repo) (name string, ok bool, findings []Finding) {
	remotes, err := r.GetRemotes()
	if err != nil {
		return "", false, nil
	}
	seen := map[string]bool{}
	for _, rem := range remotes {
		url, err := r.RemoteURL(rem)
		if err != nil {
			continue
		}
		if ref, ok := git.ParseRepoRef(url); ok && ref.Forge != git.ForgeUnknown {
			seen[ref.Repo] = true
		}
	}
	switch len(seen) {
	case 0:
		return "", false, nil
	case 1:
		return sortedKeys(seen)[0], true, nil
	default:
		return "", false, []Finding{{Check: "dir-naming", Severity: SevWarning,
			Message: fmt.Sprintf("forge remotes disagree on the repo name (%s); cannot check directory naming",
				strings.Join(sortedKeys(seen), ", "))}}
	}
}

// matchesRepoDir reports whether base is "<repo>" or "<repo>-N" for a positive
// integer N (the worktree-sibling naming pattern: foo, foo-2, foo-3). It is
// also gg clone's dir-naming rule, so the two agree on what a clean dir looks
// like.
func matchesRepoDir(base, repo string) bool {
	if base == repo {
		return true
	}
	rest, ok := strings.CutPrefix(base, repo+"-")
	if !ok || rest == "" {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toSet[V any](m map[string]V) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}
