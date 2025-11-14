package internal

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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

// FzfSelect presents options via fzf and returns the selected item
func FzfSelect(prompt string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	input := strings.Join(options, "\n")
	args := []string{
		"--prompt", prompt + ": ",
		"--height", "40%",
		"--reverse",
		"--border",
	}

	selected, err := runCommandWithInput(input, "fzf", args...)
	if err != nil {
		// User cancelled or fzf not available
		return "", err
	}

	return selected, nil
}

// FzfConfirm asks a yes/no question via fzf
func FzfConfirm(prompt string, defaultYes bool) bool {
	options := []string{"yes", "no"}
	if !defaultYes {
		options = []string{"no", "yes"}
	}

	selected, err := FzfSelect(prompt, options)
	if err != nil {
		return false
	}

	return selected == "yes"
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

// GetBranchTrackingRemote returns the remote that a local branch tracks
func GetBranchTrackingRemote(branch string) (string, error) {
	stdout, _, err := RunCommand("git", "config", "branch."+branch+".remote")
	if err != nil {
		return "", err
	}
	return stdout, nil
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
