// Package pr holds gg's pull-request conventions: how a PR is named as a local
// branch, where that identity is recorded, and which ref it is fetched from.
// It sits below the command layer because three callers need the same answers
// -- `gg checkout-pr` creating branches, `gg pull` refreshing them, and
// `gg doctor` recognising and renaming them -- and a convention implemented
// twice is a convention that drifts.
package pr

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/strutil"
)

// Where a forge advertises its pull requests. Gitea (and so codeberg) copies
// github's refs/pull/N/{head,merge}; gitlab calls them merge requests and
// namespaces them separately.
var refLines = map[git.Forge]*regexp.Regexp{
	git.ForgeGitLab: regexp.MustCompile(`^[a-f0-9]+\s+refs/merge-requests/(\d+)/(head|merge)$`),
}

var defaultRefLine = regexp.MustCompile(`^[a-f0-9]+\s+refs/pull/(\d+)/(head|merge)$`)

func refLineFor(f git.Forge) *regexp.Regexp {
	if re, ok := refLines[f]; ok {
		return re
	}
	return defaultRefLine
}

func refPrefix(f git.Forge) string {
	if f == git.ForgeGitLab {
		return "refs/merge-requests/"
	}
	return "refs/pull/"
}

// ForgeOf reports which forge a remote points at, which is what selects the
// ref shape. It is derived on demand rather than recorded against the branch:
// the remote's url is the authority, and a stored copy would disagree with it
// the moment a remote is repointed. An unrecognised remote falls back to the
// github shape, which is also what self-hosted gitea uses.
func ForgeOf(r git.Repo, remote string) git.Forge {
	url, err := r.RemoteURL(remote)
	if err != nil {
		return git.ForgeUnknown
	}
	ref, ok := git.ParseRepoRef(url)
	if !ok {
		return git.ForgeUnknown
	}
	return ref.Forge
}

// branchNameRegex matches the pr/<remote>/<number> branch name. The remote
// segment is greedy-but-slash-free-per-segment; git remote names can't contain
// '/', so a single middle segment is the remote and the trailing digits the PR.
var branchNameRegex = regexp.MustCompile(`^pr/([^/]+)/(\d+)$`)

// A PR branch carries its identity in repo-local config under these keys. The
// branch name is legibility; config is the source of truth `gg pull` reads to
// re-fetch the PR head.
const (
	cfgRemote = "gitgum-pr-remote"
	cfgNumber = "gitgum-pr-number"
	cfgType   = "gitgum-pr-type"
)

// Ref is a pull request advertised by a remote (from git ls-remote).
type Ref struct {
	Number int
	Type   string // "head" or "merge"
}

// ParseRefs extracts PR refs from git ls-remote output, reading whichever ref
// namespace the forge advertises them in. When both head and merge exist for a
// PR, head wins.
func ParseRefs(f git.Forge, lsRemoteOutput string) []Ref {
	byNumber := make(map[int]Ref)
	refLine := refLineFor(f)

	for _, line := range strutil.SplitLines(lsRemoteOutput) {
		matches := refLine.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		number, _ := strconv.Atoi(matches[1]) // regex guarantees \d+
		typ := matches[2]

		existing, found := byNumber[number]
		if !found || (existing.Type == "merge" && typ == "head") {
			byNumber[number] = Ref{Number: number, Type: typ}
		}
	}

	refs := make([]Ref, 0, len(byNumber))
	for _, ref := range byNumber {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Number > refs[j].Number })

	return refs
}

// Meta is the PR a local branch mirrors.
type Meta struct {
	Remote string
	Number int
	Type   string // "head" or "merge"
}

// BranchName is the local branch name for a PR: pr/<remote>/<number>.
func BranchName(remote string, number int) string {
	return fmt.Sprintf("pr/%s/%d", remote, number)
}

// ParseBranchName recovers the remote and number from a pr/<remote>/<number>
// branch name. ok is false for any other name.
func ParseBranchName(branch string) (remote string, number int, ok bool) {
	m := branchNameRegex.FindStringSubmatch(branch)
	if m == nil {
		return "", 0, false
	}
	number, _ = strconv.Atoi(m[2]) // regex guarantees \d+
	return m[1], number, true
}

// FetchRef returns the git ref this PR is fetched from on forge f, e.g.
// refs/pull/51/head on github or refs/merge-requests/51/head on gitlab.
func (m Meta) FetchRef(f git.Forge) string {
	return fmt.Sprintf("%s%d/%s", refPrefix(f), m.Number, m.Type)
}

// WriteMeta records the PR identity on the branch in repo-local config.
func WriteMeta(r git.Repo, branch string, m Meta) error {
	if err := r.BranchConfigSet(branch, cfgRemote, m.Remote); err != nil {
		return err
	}
	if err := r.BranchConfigSet(branch, cfgNumber, strconv.Itoa(m.Number)); err != nil {
		return err
	}
	return r.BranchConfigSet(branch, cfgType, m.Type)
}

// ReadMeta recovers the PR a branch mirrors. Config is authoritative; if it's
// absent (e.g. a branch made before this metadata existed) the pr/<remote>/<num>
// name is parsed as a fallback, defaulting to the "head" ref. ok is false when
// the branch is not a PR branch at all.
func ReadMeta(r git.Repo, branch string) (Meta, bool, error) {
	remote, err := r.BranchConfigGet(branch, cfgRemote)
	if err != nil {
		return Meta{}, false, err
	}
	if remote != "" {
		numStr, err := r.BranchConfigGet(branch, cfgNumber)
		if err != nil {
			return Meta{}, false, err
		}
		number, err := strconv.Atoi(numStr)
		if err != nil {
			return Meta{}, false, fmt.Errorf("branch %q has a malformed PR number %q", branch, numStr)
		}
		typ, err := r.BranchConfigGet(branch, cfgType)
		if err != nil {
			return Meta{}, false, err
		}
		if typ == "" {
			typ = "head"
		}
		return Meta{Remote: remote, Number: number, Type: typ}, true, nil
	}

	if remote, number, ok := ParseBranchName(branch); ok {
		return Meta{Remote: remote, Number: number, Type: "head"}, true, nil
	}
	return Meta{}, false, nil
}
