package git

import "strings"

// Worktree is one entry parsed out of `git worktree list --porcelain`.
type Worktree struct {
	Path     string
	Head     string // full sha; "" for a bare entry
	Branch   string // short branch name; "" when detached or bare
	Detached bool
	Bare     bool
	Prunable bool // registered but its gitdir/worktree is gone (git worktree prune candidate)
}

// Worktrees lists the repo's worktrees (the main worktree first, as git orders
// them). It runs `git worktree list --porcelain` and parses the result.
func (r Repo) Worktrees() ([]Worktree, error) {
	stdout, _, err := r.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return ParseWorktreePorcelain(stdout), nil
}

// ParseWorktreePorcelain parses `git worktree list --porcelain` output:
//
//	worktree /path/to/main
//	HEAD 26c3916...
//	branch refs/heads/main
//
//	worktree /path/to/other
//	HEAD abc1234...
//	detached
//
// Entries are separated by blank lines; unknown attributes (locked, prunable)
// are ignored. Exposed for callers that already hold the raw text; most should
// use Repo.Worktrees instead.
func ParseWorktreePorcelain(raw string) []Worktree {
	var out []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case cur == nil:
			// stray line before the first worktree stanza
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Detached = true
		case line == "bare":
			cur.Bare = true
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			cur.Prunable = true
		}
	}
	flush()
	return out
}
