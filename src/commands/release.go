package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/strutil"
)

// ReleaseCommand bumps the repo's VERSION (or falls back to the latest tag),
// commits the bump, and creates an annotated tag. The default branch is
// detected dynamically (origin's HEAD, then "main"/"master" if present);
// running off it prompts (default no). Tracked uncommitted changes prompt
// to auto-stash (partial-hunk staging preserved on restore). Push is left
// manual so the result can be inspected.
type ReleaseCommand struct {
	cmdIO
	Args struct {
		Bump string `positional-arg-name:"BUMP" choice:"patch" choice:"minor" choice:"major" choice:"revision" required:"yes"`
	} `positional-args:"yes"`
}

const (
	versionFileName  = "VERSION"
	revisionFileName = "REVISION"
)

func (r *ReleaseCommand) Execute(args []string) error {
	bump := r.Args.Bump
	repo := r.repo()

	if err := repo.CheckInRepo(); err != nil {
		return err
	}
	root, err := repoRoot(repo)
	if err != nil {
		return err
	}
	versionPath := filepath.Join(root, versionFileName)
	revisionPath := filepath.Join(root, revisionFileName)
	hasVersion := regularFileExists(versionPath)
	hasRevision := regularFileExists(revisionPath)
	if hasVersion && hasRevision {
		return fmt.Errorf("both VERSION and REVISION present at repo root; pick one")
	}
	if !hasVersion && !hasRevision {
		return fmt.Errorf("no VERSION or REVISION file at repo root")
	}
	isRevision := hasRevision
	if isRevision && bump != "revision" {
		return fmt.Errorf("REVISION repo: use `gg release revision` (got %q)", bump)
	}
	if !isRevision && bump == "revision" {
		return fmt.Errorf("VERSION repo: use `gg release patch|minor|major`")
	}

	branch, err := repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	defaultBranch, err := repo.GetDefaultBranch()
	if err != nil {
		return fmt.Errorf("determine default branch: %w", err)
	}
	if branch != defaultBranch {
		confirmed, err := r.sel().Confirm(fmt.Sprintf("Not on %s (current: %s). Release anyway?", defaultBranch, branch), false)
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("aborted: not on %s branch", defaultBranch)
		}
	}
	remote := defaultBranchPushRemote(repo, defaultBranch)

	if state, ok := alreadyReleased(repo); ok {
		fmt.Fprintf(r.out(), "Already released: HEAD is %q (tag %s already at HEAD).\n", state.subject, state.tag)
		fmt.Fprintf(r.out(), "Nothing to do. To publish:\n")
		fmt.Fprintf(r.out(), "  git push %s %s && git push %s %s\n", remote, defaultBranch, remote, state.tag)
		return nil
	}

	cleanup, err := handleDirtyTree(&r.cmdIO, "release")
	if err != nil {
		return err
	}
	defer cleanup()

	var (
		header, prefixes []string
		current, next    string
		tags             []string
	)
	if isRevision {
		header, prefixes, current, err = readVersion(revisionPath)
		if err != nil {
			return err
		}
		next, err = bumpRevision(current)
		if err != nil {
			return err
		}
		tags = buildRevisionTags(next, prefixes)
	} else {
		header, prefixes, current, _, err = readVersionOrFallback(repo, versionPath)
		if err != nil {
			return err
		}
		next, err = bumpVersion(current, bump)
		if err != nil {
			return err
		}
		tags = buildTags(next, prefixes)
	}

	for _, t := range tags {
		if repo.TagExists(t) {
			return fmt.Errorf("tag %s already exists", t)
		}
	}

	fmt.Fprintf(r.out(), "Bumping %s -> %s\n", current, next)

	var editedPaths []string
	if !isRevision {
		mentions, err := scanVersionMentions(repo, root, current)
		if err != nil {
			return err
		}
		picks, err := pickVersionEdits(r.sel(), mentions, current)
		if err != nil {
			return err
		}
		editedPaths, err = applyVersionEdits(root, picks, current, next)
		if err != nil {
			return err
		}
		for _, p := range editedPaths {
			fmt.Fprintf(r.out(), "  updated %s\n", p)
			if err := repo.Add(filepath.Join(root, p)); err != nil {
				return fmt.Errorf("git add %s: %w", p, err)
			}
		}
	}

	if isRevision {
		if err := writeVersion(revisionPath, header, next); err != nil {
			return err
		}
		if err := repo.Add(revisionPath); err != nil {
			return fmt.Errorf("git add REVISION: %w", err)
		}
	} else {
		if err := writeVersion(versionPath, header, next); err != nil {
			return err
		}
		if err := repo.Add(versionPath); err != nil {
			return fmt.Errorf("git add VERSION: %w", err)
		}
	}

	commitMsg := "release: " + tags[0]
	if err := repo.Commit(commitMsg); err != nil {
		return err
	}
	for _, t := range tags {
		if err := repo.TagAnnotated(t, "release "+t); err != nil {
			return err
		}
	}

	fmt.Fprintf(r.out(), "\nTagged %s. To publish:\n", strings.Join(tags, ", "))
	fmt.Fprintf(r.out(), "  git push %s %s && git push %s %s\n", remote, defaultBranch, remote, strings.Join(tags, " "))
	fmt.Fprintln(r.out(), "\nTo fully undo (drops the commit and the tag(s)):")
	fmt.Fprintf(r.out(), "  git reset --hard HEAD~1 && git tag -d %s\n", strings.Join(tags, " "))
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

// buildRevisionTags returns tags for a revision release: bare next first
// (already "rN"), then "<prefix>/rN" for each prefix.
func buildRevisionTags(next string, prefixes []string) []string {
	tags := []string{next}
	for _, p := range prefixes {
		tags = append(tags, p+"/"+next)
	}
	return tags
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func repoRoot(r git.Repo) (string, error) {
	out, _, err := r.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("find repo root: %w", err)
	}
	return out, nil
}

type releasedState struct {
	tag     string
	subject string
}

// alreadyReleased reports whether HEAD is a release commit whose tag is at HEAD.
// The commit must have subject "release: vX.Y.Z" and have its tag peel to HEAD.
// Touched files are not constrained -- the scanner offers any tracked text
// file for auto-bump, so a release commit can legitimately edit anything.
// Tag-at-HEAD is the strong signal: a stray "release: ..." subject won't
// also have its corresponding tag pointing to that commit.
func alreadyReleased(r git.Repo) (releasedState, bool) {
	subject, _, err := r.Run("log", "-1", "--format=%s")
	if err != nil {
		return releasedState{}, false
	}
	tag, ok := strings.CutPrefix(subject, "release: ")
	if !ok || (!strings.HasPrefix(tag, "v") && !strings.HasPrefix(tag, "r")) {
		return releasedState{}, false
	}

	tagSHA, _, err := r.Run("rev-parse", tag+"^{commit}")
	if err != nil {
		return releasedState{}, false
	}
	headSHA, _, err := r.Run("rev-parse", "HEAD")
	if err != nil {
		return releasedState{}, false
	}
	if tagSHA != headSHA {
		return releasedState{}, false
	}
	return releasedState{tag: tag, subject: subject}, true
}

// defaultBranchPushRemote returns the remote the default branch tracks.
// Falls back to the sole configured remote, then "origin", so the suggested
// push command stays useful even when the default branch has no upstream yet.
func defaultBranchPushRemote(r git.Repo, defaultBranch string) string {
	if name, _ := r.GetBranchTrackingRemote(defaultBranch); name != "" {
		return name
	}
	if remotes, _ := r.GetRemotes(); len(remotes) == 1 {
		return remotes[0]
	}
	return "origin"
}

// latestSemverTag returns the highest vX.Y.Z tag, or "" if none exist.
func latestSemverTag(r git.Repo) string {
	out, _, err := r.Run("tag", "--list", "v*", "--sort=-v:refname")
	if err != nil || out == "" {
		return ""
	}
	for _, line := range strutil.SplitLines(out) {
		if _, err := parseSemver(strings.TrimPrefix(line, "v")); err == nil {
			return line
		}
	}
	return ""
}

// readVersionOrFallback reads VERSION at path, falling back to the latest
// vX.Y.Z tag, then to "0.0.0". hasFile reports whether VERSION exists.
func readVersionOrFallback(r git.Repo, path string) (header, prefixes []string, current string, hasFile bool, err error) {
	header, prefixes, current, err = readVersion(path)
	if err == nil {
		return header, prefixes, current, true, nil
	}
	if !os.IsNotExist(err) {
		return nil, nil, "", false, err
	}
	// VERSION absent — fall back.
	if tag := latestSemverTag(r); tag != "" {
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

// parseRevision parses "rN" -> N. N must be a non-negative integer.
func parseRevision(s string) (int, error) {
	rest, ok := strings.CutPrefix(s, "r")
	if !ok {
		return 0, fmt.Errorf("revision %q must start with 'r'", s)
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, fmt.Errorf("parse revision %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("revision %q must be non-negative", s)
	}
	return n, nil
}

func bumpRevision(current string) (string, error) {
	n, err := parseRevision(current)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("r%d", n+1), nil
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
