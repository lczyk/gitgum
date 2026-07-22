package commands

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/strutil"
)

var (
	prRegex          = regexp.MustCompile(`^[a-f0-9]+\s+refs/pull/(\d+)/(head|merge)$`)
	prSelectionRegex = regexp.MustCompile(`^PR #(\d+) \((head|merge)\)$`)
)

// PRRef is a pull request advertised by a remote (from git ls-remote).
type PRRef struct {
	Number int
	Type   string // "head" or "merge"
}

// parsePRRefs extracts PR refs from git ls-remote output. When both head and
// merge exist for a PR, head wins.
func parsePRRefs(lsRemoteOutput string) []PRRef {
	prMap := make(map[int]PRRef)

	for _, line := range strutil.SplitLines(lsRemoteOutput) {
		matches := prRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		prNumber, _ := strconv.Atoi(matches[1]) // regex guarantees \d+
		prType := matches[2]

		existing, found := prMap[prNumber]
		if !found || (existing.Type == "merge" && prType == "head") {
			prMap[prNumber] = PRRef{Number: prNumber, Type: prType}
		}
	}

	prRefs := make([]PRRef, 0, len(prMap))
	for _, pr := range prMap {
		prRefs = append(prRefs, pr)
	}
	sort.Slice(prRefs, func(i, j int) bool {
		return prRefs[i].Number > prRefs[j].Number
	})

	return prRefs
}

// formatPROptions renders PR refs as picker labels ("PR #N (head)").
func formatPROptions(prRefs []PRRef) []string {
	options := make([]string, len(prRefs))
	for i, pr := range prRefs {
		options[i] = fmt.Sprintf("PR #%d (%s)", pr.Number, pr.Type)
	}
	return options
}

// parsePRSelection recovers the PR number and type from a picker label.
func parsePRSelection(selection string) (int, string, error) {
	matches := prSelectionRegex.FindStringSubmatch(selection)
	if len(matches) != 3 {
		return 0, "", fmt.Errorf("invalid PR selection format: %s", selection)
	}
	prNumber, _ := strconv.Atoi(matches[1]) // regex guarantees \d+
	prType := matches[2]
	return prNumber, prType, nil
}

// PR branches (from `gg checkout-pr`) are named pr/<remote>/<number> so the row
// reads as a PR at a glance, and carry the same identity in repo-local config
// under the branch.<name>.gitgum-pr-* keys. The name is legibility; config is
// the source of truth `gg pull` reads to re-fetch the PR head.
const (
	prCfgRemote = "gitgum-pr-remote"
	prCfgNumber = "gitgum-pr-number"
	prCfgType   = "gitgum-pr-type"
)

// prBranchNameRegex matches the pr/<remote>/<number> branch name. The remote
// segment is greedy-but-slash-free-per-segment; git remote names can't contain
// '/', so a single middle segment is the remote and the trailing digits the PR.
var prBranchNameRegex = regexp.MustCompile(`^pr/([^/]+)/(\d+)$`)

// prMeta is the PR a local branch mirrors.
type prMeta struct {
	remote string
	number int
	typ    string // "head" or "merge"
}

// prBranchName is the local branch name for a PR: pr/<remote>/<number>.
func prBranchName(remote string, number int) string {
	return fmt.Sprintf("pr/%s/%d", remote, number)
}

// ref returns the git ref this PR is fetched from, e.g. refs/pull/51/head.
func (m prMeta) ref() string {
	return fmt.Sprintf("refs/pull/%d/%s", m.number, m.typ)
}

// writePRMeta records the PR identity on the branch in repo-local config.
func writePRMeta(r git.Repo, branch string, m prMeta) error {
	if err := r.BranchConfigSet(branch, prCfgRemote, m.remote); err != nil {
		return err
	}
	if err := r.BranchConfigSet(branch, prCfgNumber, strconv.Itoa(m.number)); err != nil {
		return err
	}
	return r.BranchConfigSet(branch, prCfgType, m.typ)
}

// readPRMeta recovers the PR a branch mirrors. Config is authoritative; if it's
// absent (e.g. a branch made before this metadata existed) the pr/<remote>/<num>
// name is parsed as a fallback, defaulting to the "head" ref. ok is false when
// the branch is not a PR branch at all.
func readPRMeta(r git.Repo, branch string) (prMeta, bool, error) {
	remote, err := r.BranchConfigGet(branch, prCfgRemote)
	if err != nil {
		return prMeta{}, false, err
	}
	if remote != "" {
		numStr, err := r.BranchConfigGet(branch, prCfgNumber)
		if err != nil {
			return prMeta{}, false, err
		}
		number, err := strconv.Atoi(numStr)
		if err != nil {
			return prMeta{}, false, fmt.Errorf("branch %q has a malformed PR number %q", branch, numStr)
		}
		typ, err := r.BranchConfigGet(branch, prCfgType)
		if err != nil {
			return prMeta{}, false, err
		}
		if typ == "" {
			typ = "head"
		}
		return prMeta{remote: remote, number: number, typ: typ}, true, nil
	}

	// Fallback: derive from the pr/<remote>/<number> name.
	if m := prBranchNameRegex.FindStringSubmatch(branch); m != nil {
		number, _ := strconv.Atoi(m[2]) // regex guarantees \d+
		return prMeta{remote: m[1], number: number, typ: "head"}, true, nil
	}
	return prMeta{}, false, nil
}
