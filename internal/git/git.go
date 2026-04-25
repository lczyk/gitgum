package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lczyk/gitgum/internal/cmdrun"
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

func (r Repo) run(args ...string) (string, string, error) {
	return cmdrun.RunIn(r.Dir, "git", args...)
}

// GetFileStatus returns the status of a file in git.
func (r Repo) GetFileStatus(file string) (FileStatus, error) {
	// Don't use cmdrun.Run because it trims whitespace, which we need for parsing status.
	cmd := exec.Command("git", "status", "--porcelain", file)
	cmd.Dir = r.Dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	output := stdout.String()
	if err != nil || output == "" {
		return FileUnknown, err
	}
	return parseFileStatus(output), nil
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
	var remotes []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			remotes = append(remotes, line)
		}
	}
	return remotes, nil
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

// IsWorktreeCheckedOut checks if a branch is checked out in a worktree.
func (r Repo) IsWorktreeCheckedOut(branch string) (bool, string, error) {
	stdout, _, err := r.run("worktree", "list")
	if err != nil {
		return false, "", err
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "["+branch+"]") || strings.Contains(line, " "+branch+" ") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return true, fields[0], nil
			}
		}
	}
	return false, "", nil
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
func IsWorktreeCheckedOut(branch string) (bool, string, error) {
	return CWD().IsWorktreeCheckedOut(branch)
}
func GetCommitHash(ref string) (string, error)  { return CWD().GetCommitHash(ref) }
func BranchExists(branch string) bool           { return CWD().BranchExists(branch) }
func GetCurrentBranch() (string, error)         { return CWD().GetCurrentBranch() }
func GetCurrentBranchUpstream() (string, error) { return CWD().GetCurrentBranchUpstream() }
func RemoteBranchExists(remote, branch string) (bool, error) {
	return CWD().RemoteBranchExists(remote, branch)
}
func IsBranchAheadOfRemote(local, remote string) (bool, error) {
	return CWD().IsBranchAheadOfRemote(local, remote)
}
func IsDirty() (bool, error) { return CWD().IsDirty() }
