package cmdrun

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

// Run executes a command in the current working directory and returns trimmed
// stdout, stderr, and error.
func Run(name string, args ...string) (string, string, error) {
	return RunIn("", name, args...)
}

// RunIn executes a command in dir (or the current working directory if dir is
// empty) and returns trimmed stdout, stderr, and error.
func RunIn(dir, name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// RunQuiet executes a command, discards all output, and returns only the error.
func RunQuiet(name string, args ...string) error {
	_, _, err := RunIn("", name, args...)
	return err
}

// RunWithOutput executes a command and pipes output directly to stdout/stderr.
func RunWithOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
