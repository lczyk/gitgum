package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/strutil"
)

// Repo is a git repository identified by an absolute path. The zero value
// addresses the current working directory, matching the CLI's natural binding.
// Tests should construct Repo{Dir: tmpdir} and use the methods so they can
// run in parallel without sharing process cwd.
type Repo struct {
	Dir string
}

// CWD returns a Repo bound to the process's current working directory.
func CWD() Repo { return Repo{} }

// FileStatus represents the status of a file in git.
type FileStatus int

const (
	FileUntracked FileStatus = iota
	FileModified
	FileStaged
	FileDeleted
	FileUnknown
)

func parseFileStatus(statusCode string) FileStatus {
	if len(statusCode) < 2 {
		return FileUnknown
	}
	if statusCode[0] == '?' && statusCode[1] == '?' {
		return FileUntracked
	}
	if statusCode[0] != ' ' && statusCode[0] != '?' {
		if statusCode[0] == 'D' {
			return FileDeleted
		}
		return FileStaged
	}
	if statusCode[1] != ' ' && statusCode[1] != '?' {
		return FileModified
	}
	return FileUnknown
}

// run is a thin wrapper that calls runRead with a background context and
// trims whitespace from stdout/stderr. Most existing callers expect
// trimmed output (single-line refs, branch names); porcelain consumers
// that need raw output call runRead directly.
func (r Repo) run(args ...string) (string, string, error) {
	stdout, stderr, err := r.runRead(context.Background(), args...)
	return strings.TrimSpace(stdout), strings.TrimSpace(stderr), err
}

// Run is the exported, transitional read-only entry point for command
// callers migrating off cmdrun.Run("git", ...). Behaviour mirrors the old
// cmdrun.Run: trimmed stdout/stderr, background context. Specific named
// helpers (GetCurrentBranch, LsRemote, etc.) are preferred where they
// exist; this exists so the chokepoint can be enforced even for one-off
// invocations that don't yet have a dedicated helper.
func (r Repo) Run(args ...string) (string, string, error) {
	return r.run(args...)
}

func Run(args ...string) (string, string, error) { return CWD().Run(args...) }

// GetFileStatus returns the status of a file in git.
func (r Repo) GetFileStatus(file string) (FileStatus, error) {
	stdout, _, err := r.runRead(context.Background(), "status", "--porcelain", file)
	if err != nil || stdout == "" {
		return FileUnknown, err
	}
	return parseFileStatus(stdout), nil
}

// CheckInRepo verifies we're inside a git repository.
func (r Repo) CheckInRepo() error {
	if _, _, err := r.run("rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("not inside a git repository")
	}
	return nil
}

// GetLocalBranches returns a list of local git branches.
func (r Repo) GetLocalBranches() ([]string, error) {
	stdout, _, err := r.run("branch")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(stdout, "\n") {
		branch := strings.TrimSpace(line)
		branch = strings.TrimPrefix(branch, "* ")
		branch = strings.TrimPrefix(branch, "+ ")
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

// GetRemotes returns a list of git remotes.
func (r Repo) GetRemotes() ([]string, error) {
	stdout, _, err := r.run("remote")
	if err != nil {
		return nil, err
	}
	return strutil.SplitLines(stdout), nil
}

// GetRemoteBranches returns branches for a specific remote.
func (r Repo) GetRemoteBranches(remote string) ([]string, error) {
	stdout, _, err := r.run("branch", "-r")
	if err != nil {
		return nil, err
	}
	var branches []string
	prefix := remote + "/"
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) && !strings.Contains(line, "HEAD ->") {
			branches = append(branches, strings.TrimPrefix(line, prefix))
		}
	}
	return branches, nil
}

// GetBranchUpstream returns the remote and branch name of the upstream for a local branch.
// Returns ("", "", nil) if the branch has no upstream configured.
func (r Repo) GetBranchUpstream(branch string) (remote string, remoteBranch string, err error) {
	stdout, stderr, err := r.run("rev-parse", "--abbrev-ref", branch+"@{u}")
	if err != nil && strings.Contains(stderr, "no upstream configured for branch") {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(stdout, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected upstream format: %s", stdout)
	}
	return parts[0], parts[1], nil
}

// GetBranchTrackingRemote returns the remote that a local branch tracks, or "" if none.
func (r Repo) GetBranchTrackingRemote(branch string) (string, error) {
	remote, _, err := r.GetBranchUpstream(branch)
	return remote, err
}

// CheckedOutBranches returns the set of branch names currently checked out in
// any worktree (including the main worktree). Callers use the map for O(1)
// lookups rather than running a separate subprocess per branch.
func (r Repo) CheckedOutBranches() (map[string]bool, error) {
	stdout, _, err := r.run("worktree", "list")
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, line := range strings.Split(stdout, "\n") {
		// each worktree line ends with [branch] for named branches;
		// detached HEADs and bare worktrees use (…) instead.
		start := strings.LastIndex(line, "[")
		end := strings.LastIndex(line, "]")
		if start != -1 && end > start {
			out[line[start+1:end]] = true
		}
	}
	return out, nil
}

// GetCommitHash returns the commit hash for a ref.
func (r Repo) GetCommitHash(ref string) (string, error) {
	stdout, _, err := r.run("rev-parse", ref)
	return stdout, err
}

// BranchExists checks if a local branch exists.
func (r Repo) BranchExists(branch string) bool {
	stdout, _, err := r.run("branch", "--list", branch, "--format=%(refname:short)")
	return err == nil && stdout != ""
}

// GetCurrentBranch returns the name of the current branch.
func (r Repo) GetCurrentBranch() (string, error) {
	stdout, _, err := r.run("rev-parse", "--abbrev-ref", "HEAD")
	return stdout, err
}

// GetCurrentBranchUpstream returns the upstream tracking branch for the current branch.
// Returns ("", nil) if the current branch has no upstream configured.
func (r Repo) GetCurrentBranchUpstream() (string, error) {
	stdout, stderr, err := r.run("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil && strings.Contains(stderr, "no upstream configured for branch") {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return stdout, nil
}

// RemoteBranchExists checks if a branch exists on a remote.
func (r Repo) RemoteBranchExists(remote, branch string) (bool, error) {
	_, _, err := r.run("ls-remote", "--exit-code", "--heads", remote, branch)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// IsBranchAheadOfRemote reports whether localBranch has commits not in remoteBranch.
func (r Repo) IsBranchAheadOfRemote(localBranch, remoteBranch string) (bool, error) {
	stdout, _, err := r.run("log", "--oneline", remoteBranch+".."+localBranch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(stdout) != "", nil
}

// IsDirty reports whether the repo has tracked changes (ignoring untracked files).
// GetDefaultBranch returns the repo's default branch, e.g. "main" or
// "master". Resolution order:
//  1. Symbolic ref of any remote's HEAD (origin first if present, otherwise
//     the first remote alphabetically).
//  2. Local branch named "main" or "master", in that order.
//
// Returns an error if none of the above resolve.
func (r Repo) GetDefaultBranch() (string, error) {
	remotes, _ := r.GetRemotes()
	// Prefer origin if listed.
	ordered := make([]string, 0, len(remotes))
	for _, name := range remotes {
		if name == "origin" {
			ordered = append([]string{name}, ordered...)
		} else {
			ordered = append(ordered, name)
		}
	}
	for _, name := range ordered {
		out, _, err := r.run("symbolic-ref", "--short", "refs/remotes/"+name+"/HEAD")
		if err == nil && out != "" {
			// out is like "origin/main"; strip the remote prefix.
			if _, branch, ok := strings.Cut(out, "/"); ok {
				return branch, nil
			}
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if r.BranchExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not determine default branch")
}

func (r Repo) IsDirty() (bool, error) {
	stdout, _, err := r.run("status", "--porcelain=v1")
	if err != nil {
		return false, err
	}
	for _, file := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(file, "??") {
			continue
		}
		if strings.TrimSpace(file) != "" {
			return true, nil
		}
	}
	return false, nil
}

// Free-function shims that operate on the current working directory. These
// preserve the existing CLI command call sites — production binaries inherit
// the user's CWD and these forward to Repo{}.

func GetFileStatus(file string) (FileStatus, error) { return CWD().GetFileStatus(file) }
func CheckInRepo() error                            { return CWD().CheckInRepo() }
func GetLocalBranches() ([]string, error)           { return CWD().GetLocalBranches() }
func GetRemotes() ([]string, error)                 { return CWD().GetRemotes() }
func GetRemoteBranches(remote string) ([]string, error) {
	return CWD().GetRemoteBranches(remote)
}
func GetBranchUpstream(branch string) (string, string, error) {
	return CWD().GetBranchUpstream(branch)
}
func GetBranchTrackingRemote(branch string) (string, error) {
	return CWD().GetBranchTrackingRemote(branch)
}
func CheckedOutBranches() (map[string]bool, error) { return CWD().CheckedOutBranches() }
func GetCommitHash(ref string) (string, error)     { return CWD().GetCommitHash(ref) }
func BranchExists(branch string) bool              { return CWD().BranchExists(branch) }
func GetCurrentBranch() (string, error)            { return CWD().GetCurrentBranch() }
func GetCurrentBranchUpstream() (string, error)    { return CWD().GetCurrentBranchUpstream() }
func RemoteBranchExists(remote, branch string) (bool, error) {
	return CWD().RemoteBranchExists(remote, branch)
}
func IsBranchAheadOfRemote(local, remote string) (bool, error) {
	return CWD().IsBranchAheadOfRemote(local, remote)
}
func IsDirty() (bool, error)            { return CWD().IsDirty() }
func GetDefaultBranch() (string, error) { return CWD().GetDefaultBranch() }
