package cmdrun

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

// Run executes a command and returns trimmed stdout, stderr, and error.
func Run(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// RunQuiet executes a command and returns only the error.
func RunQuiet(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// RunWithOutput executes a command and pipes output directly to stdout/stderr.
func RunWithOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
