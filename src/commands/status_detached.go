package commands

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

// A refKind orders the containing refs: local branches, then branches that
// only exist on a remote, then tags.
type refKind int

const (
	kindLocal refKind = iota
	kindRemote
	kindTag
)

// headRef is a ref that contains the detached HEAD commit.
type headRef struct {
	name   string // branch or tag name, without the remote for kindRemote
	remote string // tracking remote of a local branch, or the remote a kindRemote lives on
	suffix string // "~N", empty when HEAD is not on the ref's first-parent chain
	dist   int    // commits between HEAD and the ref, for ordering
	kind   refKind
}

// detachedHeadRows renders the HEAD line for a detached HEAD. With exactly one
// containing ref it stays on one line, with none it says so, and with several
// the refs get a row each, closest first:
//
//	"* HEAD cf10b5f (origin/)feat/x~1"
//	"* HEAD cf10b5f (no branch)"
//	"* HEAD cf10b5f"
//	"    (origin/)feat/x~1"
//	"    (origin/)feat/y~3"
//
// The second return value is false when git wouldn't answer, in which case the
// caller keeps git's own "## HEAD (no branch)".
func (s *StatusCommand) detachedHeadRows() ([]string, bool) {
	r := s.repo()
	head, _, err := r.Run("rev-parse", "HEAD")
	if err != nil {
		return nil, false
	}
	head = strings.TrimSpace(head)
	short, _, err := r.Run("rev-parse", "--short", "HEAD")
	if err != nil {
		return nil, false
	}
	return formatDetachedRows(strings.TrimSpace(short), containingRefs(r, head), colorEnabled()), true
}

// containingRefs finds every branch and tag that has head as an ancestor:
// local branches, then remote branches with no local counterpart, then tags,
// each group closest-first.
func containingRefs(r git.Repo, head string) []headRef {
	var refs []headRef
	// for-each-ref rather than `branch --contains`: the latter also lists the
	// "(HEAD detached at abc1234)" pseudo-entry, which is not a branch.
	tracked := make(map[string]bool)
	for _, name := range gitLines(r, "for-each-ref", "--contains", head, "--format=%(refname:short)", "refs/heads") {
		ref := headRef{name: name, kind: kindLocal}
		ref.remote, _ = r.GetBranchTrackingRemote(name)
		ref.dist, ref.suffix = refDistance(r, "refs/heads/"+name, head)
		if ref.remote != "" {
			tracked[ref.remote+"/"+name] = true
		}
		refs = append(refs, ref)
	}
	for _, short := range gitLines(r, "for-each-ref", "--contains", head, "--format=%(refname:short)", "refs/remotes") {
		// "origin/HEAD" is a symref to the remote's default branch, not a
		// branch of its own; a ref already shown switch-style is a duplicate.
		if strings.HasSuffix(short, "/HEAD") || tracked[short] {
			continue
		}
		remote, name, ok := strings.Cut(short, "/")
		if !ok {
			continue
		}
		ref := headRef{name: name, remote: remote, kind: kindRemote}
		ref.dist, ref.suffix = refDistance(r, "refs/remotes/"+short, head)
		refs = append(refs, ref)
	}
	for _, name := range gitLines(r, "for-each-ref", "--contains", head, "--format=%(refname:short)", "refs/tags") {
		ref := headRef{name: name, kind: kindTag}
		ref.dist, ref.suffix = refDistance(r, "refs/tags/"+name, head)
		refs = append(refs, ref)
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].kind != refs[j].kind {
			return refs[i].kind < refs[j].kind
		}
		if refs[i].dist != refs[j].dist {
			return refs[i].dist < refs[j].dist
		}
		return refs[i].name < refs[j].name
	})
	return refs
}

// refDistance measures how far head sits below ref. dist counts every commit
// in ref that head can't reach, which orders refs sensibly even across merges.
// The "~N" suffix only comes back when ref~N really resolves to head, i.e. when
// head sits on ref's first-parent chain -- otherwise the notation would lie.
func refDistance(r git.Repo, ref, head string) (dist int, suffix string) {
	if out, _, err := r.Run("rev-list", "--count", head+".."+ref); err == nil {
		dist, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	out, _, err := r.Run("rev-list", "--count", "--first-parent", head+".."+ref)
	if err != nil {
		return dist, ""
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return dist, ""
	}
	got, _, err := r.Run("rev-parse", "--verify", fmt.Sprintf("%s~%d^{commit}", ref, n))
	if err != nil || strings.TrimSpace(got) != head {
		return dist, ""
	}
	return dist, fmt.Sprintf("~%d", n)
}

func gitLines(r git.Repo, args ...string) []string {
	out, _, err := r.Run(args...)
	if err != nil {
		return nil
	}
	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func formatDetachedRows(short string, refs []headRef, color bool) []string {
	head, sha := "HEAD", short
	if color {
		head = ansiBoldCyan + "HEAD" + ansiReset
		sha = ansiYellow + short + ansiReset
	}
	first := headMarker(color) + head + " " + sha

	switch len(refs) {
	case 0:
		none := "(no branch)"
		if color {
			none = ansiBoldRed + none + ansiReset
		}
		return []string{first + " " + none}
	case 1:
		return []string{first + " " + formatHeadRef(refs[0], color)}
	}
	rows := []string{first}
	for _, ref := range refs {
		rows = append(rows, "    "+formatHeadRef(ref, color))
	}
	return rows
}

// formatHeadRef renders one containing ref: branches switch-style whenever a
// remote is involved, tags in magenta, all with the "~N" distance suffix.
func formatHeadRef(ref headRef, color bool) string {
	var name string
	switch {
	case ref.kind == kindTag:
		name = ref.name
		if color {
			name = ansiBoldMagenta + name + ansiReset
		}
	case ref.remote != "":
		name = remoteSlashBranch(ref.remote, ref.name, color)
	default:
		name = ref.name
		if color {
			name = ansiBoldGreen + name + ansiReset
		}
	}
	suffix := ref.suffix
	if color && suffix != "" {
		suffix = ansiDim + suffix + ansiReset
	}
	return name + suffix
}
