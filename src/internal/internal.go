package internal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrFzfCancelled is returned when the user cancels an fzf operation (Ctrl+C or ESC)
var ErrFzfCancelled = errors.New("fzf operation cancelled")

// RunCommand executes a command and returns stdout, stderr, and error
func RunCommand(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// RunCommandQuiet executes a command and only returns error
func RunCommandQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

// RunCommandWithOutput executes a command and prints output directly to stdout/stderr
func RunCommandWithOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCommandWithInput executes a command with stdin input
func runCommandWithInput(input string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), err
}

// WriteFile writes content to a file
func WriteFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// GitFileStatus represents the status of a file in git
type GitFileStatus int

const (
	GitFileUntracked GitFileStatus = iota
	GitFileModified
	GitFileStaged
	GitFileDeleted
	GitFileUnknown
)

// GetGitFileStatus returns the status of a file in git
func GetGitFileStatus(file string) (GitFileStatus, error) {
	// Don't use RunCommand because it trims whitespace, which we need for parsing status
	cmd := exec.Command("git", "status", "--porcelain", file)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	output := stdout.String()
	
	if err != nil || output == "" {
		return GitFileUnknown, err
	}
	
	if len(output) < 2 {
		return GitFileUnknown, nil
	}
	
	status := output[:2]
	
	// Untracked file (status: ??)
	if status[0] == '?' && status[1] == '?' {
		return GitFileUntracked, nil
	}
	
	// Staged changes (first character is not space)
	if status[0] != ' ' && status[0] != '?' {
		if status[0] == 'D' {
			return GitFileDeleted, nil
		}
		return GitFileStaged, nil
	}
	
	// Modified (second character is not space)
	if status[1] != ' ' && status[1] != '?' {
		return GitFileModified, nil
	}
	
	return GitFileUnknown, nil
}

// FzfSelect presents options via fzf and returns the selected item
func FzfSelect(prompt string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	input := strings.Join(options, "\n")
	args := []string{
		"--prompt", prompt + ": ",
		"--height=40%",
		"--layout=reverse",
		"--border=top",
	}

	selected, err := runCommandWithInput(input, "fzf", args...)
	if err != nil {
		// User cancelled or fzf not available
		return "", ErrFzfCancelled
	}

	return selected, nil
}

// FzfConfirm asks a yes/no question via fzf
func FzfConfirm(prompt string, defaultYes bool) (bool, error) {
	options := []string{"yes", "no"}
	if !defaultYes {
		options = []string{"no", "yes"}
	}

	selected, err := FzfSelect(prompt, options)
	if err != nil {
		return false, err
	}

	return selected == "yes", nil
}

// CheckInGitRepo verifies we're inside a git repository
func CheckInGitRepo() error {
	_, _, err := RunCommand("git", "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}
	return nil
}

// GetLocalBranches returns a list of local git branches
func GetLocalBranches() ([]string, error) {
	stdout, _, err := RunCommand("git", "branch")
	if err != nil {
		return nil, err
	}

	var branches []string
	for _, line := range strings.Split(stdout, "\n") {
		// Remove markers: '*' (current branch) and '+' (worktree checkouts)
		branch := strings.TrimSpace(line)
		branch = strings.TrimPrefix(branch, "* ")
		branch = strings.TrimPrefix(branch, "+ ")
		if branch != "" {
			branches = append(branches, branch)
		}
	}

	return branches, nil
}

// GetRemotes returns a list of git remotes
func GetRemotes() ([]string, error) {
	stdout, _, err := RunCommand("git", "remote")
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

// GetRemoteBranches returns branches for a specific remote
func GetRemoteBranches(remote string) ([]string, error) {
	stdout, _, err := RunCommand("git", "branch", "-r")
	if err != nil {
		return nil, err
	}

	var branches []string
	prefix := remote + "/"
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) && !strings.Contains(line, "HEAD ->") {
			// Remove remote prefix
			branch := strings.TrimPrefix(line, prefix)
			branches = append(branches, branch)
		}
	}

	return branches, nil
}

// GetBranchTrackingRemote returns the remote that a local branch tracks.
// If the branch does not track any remote, returns an empty string.
func GetBranchTrackingRemote(branch string) (string, error) {
	stdout, stderr, err := RunCommand("git", "rev-parse", "--abbrev-ref", branch+"@{u}")
	_pattern := "no upstream configured for branch"
	if err != nil && strings.Contains(stderr, _pattern) {
		// no tracking remote
		return "", nil
	}
	if err != nil {
		return "", err
	}
	// strip the branch name to get the remote name
	parts := strings.SplitN(stdout, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected upstream format: %s", stdout)
	}
	remote := parts[0]
	return remote, nil
}

// IsWorktreeCheckedOut checks if a branch is checked out in a worktree
func IsWorktreeCheckedOut(branch string) (bool, string, error) {
	stdout, _, err := RunCommand("git", "worktree", "list")
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

// GetCommitHash returns the commit hash for a ref
func GetCommitHash(ref string) (string, error) {
	stdout, _, err := RunCommand("git", "rev-parse", ref)
	return stdout, err
}

// BranchExists checks if a local branch exists
func BranchExists(branch string) bool {
	stdout, _, err := RunCommand("git", "branch", "--list", branch, "--format=%(refname:short)")
	return err == nil && stdout != ""
}

// PrintBlue prints a message in black color (mimicking the bash _blue function)
// Note: The bash version actually uses BLACK color despite the function name
func PrintBlue(message string) {
	// ANSI color codes: Black text
	black := "\033[0;30m"
	reset := "\033[0m"
	fmt.Printf("%s%s%s\n", black, message, reset)
}

// GetCurrentBranch returns the name of the current branch
func GetCurrentBranch() (string, error) {
	stdout, _, err := RunCommand("git", "rev-parse", "--abbrev-ref", "HEAD")
	return stdout, err
}

// GetCurrentBranchUpstream returns the upstream tracking branch for the current branch
func GetCurrentBranchUpstream() (string, error) {
	stdout, _, err := RunCommand("git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return "", err
	}
	return stdout, nil
}

// RemoteBranchExists checks if a branch exists on a remote
func RemoteBranchExists(remote, branch string) (bool, error) {
	// Use git ls-remote to check if the branch exists on the remote
	_, _, err := RunCommand("git", "ls-remote", "--exit-code", "--heads", remote, branch)
	if err != nil {
		// If the command fails, the branch doesn't exist
		return false, nil
	}
	return true, nil
}

// IsBranchAheadOfRemote checks if a local branch is ahead of its remote tracking branch
func IsBranchAheadOfRemote(localBranch, remoteBranch string) (bool, error) {
	stdout, _, err := RunCommand("git", "log", "--oneline", remoteBranch+".."+localBranch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(stdout) != "", nil
}

func IsGitDirty(dir string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	// filter out untracked files
	for _, file := range files {
		if strings.HasPrefix(file, "??") {
			continue
		}
		if strings.TrimSpace(file) != "" {
			return true, nil
		}
	}
	return false, nil
}

// SplitLines splits a string into lines and trims whitespace
func SplitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}