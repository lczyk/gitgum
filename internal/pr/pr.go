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

// refLine matches a PR head/merge ref in `git ls-remote` output.
var refLine = regexp.MustCompile(`^[a-f0-9]+\s+refs/pull/(\d+)/(head|merge)$`)

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

// ParseRefs extracts PR refs from git ls-remote output. When both head and
// merge exist for a PR, head wins.
func ParseRefs(lsRemoteOutput string) []Ref {
	byNumber := make(map[int]Ref)

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

// FetchRef returns the git ref this PR is fetched from, e.g. refs/pull/51/head.
func (m Meta) FetchRef() string {
	return fmt.Sprintf("refs/pull/%d/%s", m.Number, m.Type)
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
