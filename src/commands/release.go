package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

// ReleaseCommand bumps the repo's VERSION (or falls back to the latest tag),
// commits the bump, and creates an annotated tag. Refuses unless on main with
// a clean working tree. Push is left manual so the result can be inspected.
type ReleaseCommand struct {
	Args struct {
		Bump string `positional-arg-name:"BUMP" choice:"patch" choice:"minor" choice:"major" required:"yes"`
	} `positional-args:"yes"`
}

const versionFileName = "VERSION"

func (r *ReleaseCommand) Execute(args []string) error {
	bump := r.Args.Bump

	if err := git.CheckInRepo(); err != nil {
		return err
	}
	branch, err := git.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	if branch != "main" {
		confirmed, err := ui.Confirm(fmt.Sprintf("Not on main (current: %s). Release anyway?", branch), false)
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("aborted: not on main branch")
		}
	}
	if err := requireCleanTree(); err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}

	if state, ok := alreadyReleased(); ok {
		fmt.Printf("Already released: HEAD is %q (tag %s already at HEAD).\n", state.subject, state.tag)
		fmt.Printf("Nothing to do. To publish:\n")
		fmt.Printf("  git push origin main && git push origin %s\n", state.tag)
		return nil
	}

	versionPath := filepath.Join(root, versionFileName)
	header, prefixes, current, hasFile, err := readVersionOrFallback(versionPath)
	if err != nil {
		return err
	}

	next, err := bumpVersion(current, bump)
	if err != nil {
		return err
	}
	tags := buildTags(next, prefixes)

	for _, t := range tags {
		if exists, err := tagExists(t); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("tag %s already exists", t)
		}
	}

	fmt.Printf("Bumping %s -> %s\n", current, next)

	commitArgs := []string{"commit", "-m", "release: " + tags[0]}
	if hasFile {
		if err := writeVersion(versionPath, header, next); err != nil {
			return err
		}
		if _, _, err := cmdrun.Run("git", "add", versionPath); err != nil {
			return fmt.Errorf("git add VERSION: %w", err)
		}
	} else {
		commitArgs = append(commitArgs, "--allow-empty")
	}

	if err := cmdrun.RunWithOutput("git", commitArgs...); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	for _, t := range tags {
		if err := cmdrun.RunWithOutput("git", "tag", "-a", t, "-m", "release "+t); err != nil {
			return fmt.Errorf("git tag %s: %w", t, err)
		}
	}

	fmt.Printf("\nTagged %s. To publish:\n", strings.Join(tags, ", "))
	fmt.Printf("  git push origin main && git push origin %s\n", strings.Join(tags, " "))
	fmt.Println("\nTo fully undo (drops the commit and the tag(s)):")
	fmt.Printf("  git reset --hard HEAD~1 && git tag -d %s\n", strings.Join(tags, " "))
	return nil
}

// buildTags returns the list of tags to create: bare "v"+next first, then
// "<prefix>/v"+next for each prefix.
func buildTags(next string, prefixes []string) []string {
	tags := []string{"v" + next}
	for _, p := range prefixes {
		tags = append(tags, p+"/v"+next)
	}
	return tags
}

func repoRoot() (string, error) {
	out, _, err := cmdrun.Run("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("find repo root: %w", err)
	}
	return out, nil
}

func requireCleanTree() error {
	out, _, err := cmdrun.Run("git", "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if out != "" {
		return fmt.Errorf("working tree not clean:\n%s", out)
	}
	return nil
}

type releasedState struct {
	tag     string
	subject string
}

// alreadyReleased reports whether HEAD is a release commit whose tag is at HEAD.
// The commit must:
//   - have subject "release: vX.Y.Z"
//   - touch only VERSION (or no files, for empty-commit releases)
//   - have its tag peel to HEAD
func alreadyReleased() (releasedState, bool) {
	subject, _, err := cmdrun.Run("git", "log", "-1", "--format=%s")
	if err != nil {
		return releasedState{}, false
	}
	tag, ok := strings.CutPrefix(subject, "release: ")
	if !ok || !strings.HasPrefix(tag, "v") {
		return releasedState{}, false
	}

	// Touched files must be empty or only VERSION.
	files, _, err := cmdrun.Run("git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	if err != nil {
		return releasedState{}, false
	}
	touched := strings.Fields(files)
	if len(touched) > 1 {
		return releasedState{}, false
	}
	if len(touched) == 1 && touched[0] != versionFileName {
		return releasedState{}, false
	}

	tagSHA, _, err := cmdrun.Run("git", "rev-parse", tag+"^{commit}")
	if err != nil {
		return releasedState{}, false
	}
	headSHA, _, err := cmdrun.Run("git", "rev-parse", "HEAD")
	if err != nil {
		return releasedState{}, false
	}
	if tagSHA != headSHA {
		return releasedState{}, false
	}
	return releasedState{tag: tag, subject: subject}, true
}

func tagExists(tag string) (bool, error) {
	_, _, err := cmdrun.Run("git", "rev-parse", "--verify", tag)
	if err == nil {
		return true, nil
	}
	// Non-zero exit means the ref doesn't exist; treat as "not present".
	return false, nil
}

// latestSemverTag returns the highest vX.Y.Z tag, or "" if none exist.
func latestSemverTag() string {
	out, _, err := cmdrun.Run("git", "tag", "--list", "v*", "--sort=-v:refname")
	if err != nil || out == "" {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if _, err := parseSemver(strings.TrimPrefix(line, "v")); err == nil {
			return line
		}
	}
	return ""
}

// readVersionOrFallback reads VERSION at path, falling back to the latest
// vX.Y.Z tag, then to "0.0.0". hasFile reports whether VERSION exists.
func readVersionOrFallback(path string) (header, prefixes []string, current string, hasFile bool, err error) {
	header, prefixes, current, err = readVersion(path)
	if err == nil {
		return header, prefixes, current, true, nil
	}
	if !os.IsNotExist(err) {
		return nil, nil, "", false, err
	}
	// VERSION absent — fall back.
	if tag := latestSemverTag(); tag != "" {
		return nil, nil, strings.TrimPrefix(tag, "v"), false, nil
	}
	return nil, nil, "0.0.0", false, nil
}

func readVersion(path string) (header, prefixes []string, current string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, "", err
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
			if p, ok := parseTagsDirective(trimmed); ok {
				prefixes = p
			}
			continue
		}
		if current == "" {
			current = trimmed
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, "", err
	}
	if current == "" {
		return nil, nil, "", fmt.Errorf("%s contains no version line", path)
	}
	return header, prefixes, current, nil
}

// parseTagsDirective parses a header line like "# tags: go rust python" and
// returns the prefix list. Comma- and space-separated values are accepted.
// Returns ok=false if the line is not a tags directive.
func parseTagsDirective(line string) (prefixes []string, ok bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "#"))
	rest, ok = strings.CutPrefix(rest, "tags:")
	if !ok {
		return nil, false
	}
	for _, f := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		prefixes = append(prefixes, f)
	}
	return prefixes, true
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

type semver struct{ major, minor, patch int }

func parseSemver(s string) (semver, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("version %q is not in major.minor.patch form", s)
	}
	labels := [3]string{"major", "minor", "patch"}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{}, fmt.Errorf("parse %s in %q: %w", labels[i], s, err)
		}
		nums[i] = n
	}
	return semver{nums[0], nums[1], nums[2]}, nil
}

func bumpVersion(current, bump string) (string, error) {
	v, err := parseSemver(current)
	if err != nil {
		return "", err
	}
	switch bump {
	case "patch":
		v.patch++
	case "minor":
		v.minor++
		v.patch = 0
	case "major":
		v.major++
		v.minor = 0
		v.patch = 0
	default:
		return "", fmt.Errorf("unknown bump %q", bump)
	}
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch), nil
}
