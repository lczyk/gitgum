// release bumps VERSION, commits, and creates an annotated git tag.
//
// Refuses unless invoked on main with a clean working tree, and refuses to
// overwrite an existing tag. Pushing is left as a manual step so the bump can
// be inspected first.
//
// Usage: release patch | minor | major
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const versionFile = "VERSION"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: release patch|minor|major")
	}
	bump := args[0]
	if bump != "patch" && bump != "minor" && bump != "major" {
		return fmt.Errorf("unknown bump %q (want patch|minor|major)", bump)
	}

	if err := requireMainBranch(); err != nil {
		return err
	}
	if err := requireCleanTree(); err != nil {
		return err
	}

	if state, ok := alreadyReleased(); ok {
		fmt.Printf("Already released: HEAD is %q (tag %s already at HEAD).\n", state.subject, state.tag)
		fmt.Printf("Nothing to do. To publish:\n")
		fmt.Printf("  git push origin main && git push origin %s\n", state.tag)
		return nil
	}

	header, current, err := readVersion(versionFile)
	if err != nil {
		return err
	}

	next, err := bumpVersion(current, bump)
	if err != nil {
		return err
	}
	tag := "v" + next

	if exists, err := tagExists(tag); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("tag %s already exists", tag)
	}

	fmt.Printf("Bumping %s -> %s\n", current, next)

	if err := writeVersion(versionFile, header, next); err != nil {
		return err
	}
	if err := git("add", versionFile); err != nil {
		return err
	}
	if err := git("commit", "-m", "release: "+tag); err != nil {
		return err
	}
	if err := git("tag", "-a", tag, "-m", "release "+tag); err != nil {
		return err
	}

	fmt.Printf("\nTagged %s. To publish:\n", tag)
	fmt.Printf("  git push origin main && git push origin %s\n", tag)
	fmt.Println("\nNote: 'git reset HEAD~1' undoes the commit but leaves the tag")
	fmt.Printf("pointing at the orphaned commit. To fully undo, also run:\n")
	fmt.Printf("  git tag -d %s\n", tag)
	return nil
}

// releasedState describes a HEAD that already represents a release commit
// matching a tag at HEAD.
type releasedState struct {
	tag     string // e.g. "v0.5.0"
	subject string // commit subject
}

// alreadyReleased reports whether HEAD is a release commit whose tag is
// already pointing at HEAD: the commit's subject is "release: vX.Y.Z", the
// only file it touches is VERSION, and that tag resolves to HEAD. This means
// a previous release invocation succeeded and a repeat call would be a no-op.
func alreadyReleased() (releasedState, bool) {
	subject, err := exec.Command("git", "log", "-1", "--format=%s").Output()
	if err != nil {
		return releasedState{}, false
	}
	subj := strings.TrimSpace(string(subject))
	tag, ok := strings.CutPrefix(subj, "release: ")
	if !ok || !strings.HasPrefix(tag, "v") {
		return releasedState{}, false
	}

	// Touched files: the commit must change only VERSION.
	files, err := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").Output()
	if err != nil {
		return releasedState{}, false
	}
	touched := strings.Fields(string(files))
	if len(touched) != 1 || touched[0] != versionFile {
		return releasedState{}, false
	}

	// Tag must exist and point at HEAD (peel through annotated tag objects).
	tagSHA, err := exec.Command("git", "rev-parse", tag+"^{commit}").Output()
	if err != nil {
		return releasedState{}, false
	}
	headSHA, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return releasedState{}, false
	}
	if strings.TrimSpace(string(tagSHA)) != strings.TrimSpace(string(headSHA)) {
		return releasedState{}, false
	}

	return releasedState{tag: tag, subject: subj}, true
}

func requireMainBranch() error {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch != "main" {
		return fmt.Errorf("must be on main branch (current: %s)", branch)
	}
	return nil
}

func requireCleanTree() error {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(out) > 0 {
		return fmt.Errorf("working tree not clean:\n%s", string(out))
	}
	return nil
}

func tagExists(tag string) (bool, error) {
	err := exec.Command("git", "rev-parse", "--verify", tag).Run()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

// readVersion returns (comment header lines, current version) from path.
func readVersion(path string) (header []string, current string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			header = append(header, line)
			continue
		}
		if current == "" {
			current = trimmed
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if current == "" {
		return nil, "", fmt.Errorf("%s contains no version line", path)
	}
	return header, current, nil
}

func writeVersion(path string, header []string, version string) error {
	var b strings.Builder
	for _, line := range header {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(version)
	b.WriteByte('\n')
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func bumpVersion(current, bump string) (string, error) {
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("version %q is not in major.minor.patch form", current)
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("parse major in %q: %w", current, err)
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("parse minor in %q: %w", current, err)
	}
	pat, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("parse patch in %q: %w", current, err)
	}
	switch bump {
	case "patch":
		pat++
	case "minor":
		min++
		pat = 0
	case "major":
		maj++
		min = 0
		pat = 0
	}
	return fmt.Sprintf("%d.%d.%d", maj, min, pat), nil
}

func git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
