package commands

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// A statusSection is one selectable chunk of `gg status` output. Sections are
// picked by name on the command line ("branch,remote") and rendered in the
// order given.
type statusSection struct {
	id     string
	header string
	render func(s *StatusCommand, out io.Writer, header func()) error
}

// sectionOrder is the canonical list, used for the "unknown section" error and
// for lookups. Each section is addressable by its full name or its first letter.
var sectionOrder = []statusSection{
	{id: "branch", header: "BRANCHES", render: (*StatusCommand).renderBranches},
	{id: "remote", header: "REMOTES", render: (*StatusCommand).renderRemotes},
	{id: "worktree", header: "WORKTREES", render: (*StatusCommand).renderWorktrees},
	{id: "changes", header: "CHANGES", render: (*StatusCommand).renderChanges},
	{id: "head", header: "HEAD", render: (*StatusCommand).renderHead},
}

// defaultSections is what bare `gg status` renders.
var defaultSections = []string{"changes", "head"}

func lookupSection(name string) (statusSection, bool) {
	for _, sec := range sectionOrder {
		if name == sec.id || name == sec.id[:1] {
			return sec, true
		}
	}
	return statusSection{}, false
}

// parseSections turns a comma-separated spec ("b, changes ,b") into the
// sections to render. Whitespace is stripped, duplicates collapse onto their
// *last* occurrence -- so "a,b,a,c" and "b,a,c" render identically.
func parseSections(spec string) ([]statusSection, error) {
	var names []string
	for tok := range strings.SplitSeq(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		names = append(names, tok)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no sections given (known: %s)", knownSections())
	}

	last := make(map[string]int, len(names))
	resolved := make([]statusSection, len(names))
	for i, name := range names {
		sec, ok := lookupSection(strings.ToLower(name))
		if !ok {
			return nil, fmt.Errorf("unknown status section %q (known: %s)", name, knownSections())
		}
		resolved[i] = sec
		last[sec.id] = i
	}

	out := make([]statusSection, 0, len(last))
	for i, sec := range resolved {
		if last[sec.id] == i {
			out = append(out, sec)
		}
	}
	return out, nil
}

func knownSections() string {
	names := make([]string, len(sectionOrder))
	for i, sec := range sectionOrder {
		names[i] = fmt.Sprintf("%s (%s)", sec.id, sec.id[:1])
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// renderSections writes each selected section in order. Headers are printed
// only when more than one section was asked for -- a lone section is bare.
func (s *StatusCommand) renderSections(out io.Writer, sections []statusSection) error {
	withHeaders := len(sections) > 1
	for _, sec := range sections {
		header := func() {
			if withHeaders {
				fmt.Fprintln(out, paint(ansiDim, statusHeader(sec.header)))
			}
		}
		if err := sec.render(s, out, header); err != nil {
			return err
		}
	}
	return nil
}

func (s *StatusCommand) renderBranches(out io.Writer, header func()) error {
	stdout, _, err := s.repo().Run("branch", "-vv", "--color=never")
	if err != nil {
		return fmt.Errorf("getting branches: %w", err)
	}
	header()
	fmt.Fprintln(out, renderBranchList(stdout))
	return nil
}

func (s *StatusCommand) renderRemotes(out io.Writer, header func()) error {
	stdout, _, err := s.repo().Run("remote", "-v")
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}
	remotes := parseRemotes(stdout)
	if len(remotes) == 0 {
		return nil
	}
	header()
	for _, remote := range remotes {
		fmt.Fprintln(out, remote)
	}
	return nil
}

// renderWorktrees lists worktrees in the same multi-row layout as the
// BRANCHES section, with the checked-out ref rendered switch-style
// ("(remote/)branch"). The worktree the command runs in is marked with '*'.
func (s *StatusCommand) renderWorktrees(out io.Writer, header func()) error {
	worktrees, err := s.repo().Worktrees()
	if err != nil {
		return fmt.Errorf("getting worktrees: %w", err)
	}
	if len(worktrees) == 0 {
		return nil
	}
	header()
	toplevel, _, err := s.repo().Run("rev-parse", "--show-toplevel")
	if err != nil {
		toplevel = ""
	}
	color := colorEnabled()
	for _, wt := range worktrees {
		trackingRemote := ""
		if wt.Branch != "" {
			trackingRemote, _ = s.repo().GetBranchTrackingRemote(wt.Branch)
		}
		subject := ""
		if wt.Head != "" {
			subject, _, _ = s.repo().Run("log", "-1", "--format=%s", wt.Head)
		}
		current := toplevel != "" && wt.Path == toplevel
		for _, row := range formatWorktreeRows(wt, current, trackingRemote, subject, color) {
			fmt.Fprintln(out, row)
		}
	}
	return nil
}

// renderChanges prints the working-tree changes, as a tree or (with --flat) a
// porcelain list. A clean tree emits nothing at all, header included.
func (s *StatusCommand) renderChanges(out io.Writer, header func()) error {
	lines, err := s.statusLines()
	if err != nil {
		return err
	}
	changeLines := lines[1:]
	hasChanges := false
	for _, l := range changeLines {
		if l != "" {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return nil
	}
	header()
	if s.Flat {
		fmt.Fprintln(out, strings.Join(changeLines, "\n"))
		return nil
	}
	entries := parseChangeLines(changeLines)
	annotateNumstats(s.repo(), entries)
	renderTree(buildTree(entries), out)
	return nil
}

// renderHead prints the branch summary line, with the tracking branch
// rendered switch-style, e.g. "## (origin/)main [ahead 7]". A detached HEAD
// gets the refs that contain it instead of git's bare "## HEAD (no branch)".
func (s *StatusCommand) renderHead(out io.Writer, header func()) error {
	lines, err := s.statusLines()
	if err != nil {
		return err
	}
	header()
	if lines[0] == detachedHeadLine {
		if rows, ok := s.detachedHeadRows(); ok {
			for _, row := range rows {
				fmt.Fprintln(out, row)
			}
			return nil
		}
	}
	fmt.Fprintln(out, formatHeadLine(lines[0], colorEnabled()))
	return nil
}

// detachedHeadLine is what `git status --short --branch` emits when HEAD is
// detached, regardless of what it's detached at.
const detachedHeadLine = "## HEAD (no branch)"

// headLineRe splits "## main...origin/main [ahead 1]" into local branch,
// upstream (optional) and bracketed ahead/behind notes (optional).
var headLineRe = regexp.MustCompile(`^## (.+?)(?:\.\.\.(\S+))?( \[[^\]]*\])?$`)

// formatHeadLine rewrites the `git status --branch` summary line so a
// same-name upstream renders switch-style: "## main...origin/main [ahead 7]"
// becomes "* (origin/)main [ahead 7]". The '*' marker matches the current-row
// marker of the BRANCHES and WORKTREES sections. Detached HEAD, differing
// upstream names and "No commits yet" lines keep their original shape (colored).
func formatHeadLine(line string, color bool) string {
	m := headLineRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	local, upstream, bracket := m[1], m[2], m[3]
	if strings.HasPrefix(local, "No commits yet") {
		return headMarker(color) + local
	}

	marker := headMarker(color)
	if color {
		bracket = strings.TrimPrefix(bracket, " ")
		if bracket != "" {
			bracket = " " + ansiBoldYellow + bracket + ansiReset
		}
	}

	if upstream == "" {
		branch := local
		if color {
			if strings.HasPrefix(local, "HEAD ") {
				branch = ansiBoldCyan + local + ansiReset
			} else {
				branch = ansiBoldGreen + local + ansiReset
			}
		}
		return marker + branch + bracket
	}

	if remote, branch, ok := strings.Cut(upstream, "/"); ok && branch == local {
		return marker + remoteSlashBranch(remote, local, color) + bracket
	}

	// upstream tracks a differently-named branch: keep the "a...b" shape
	if color {
		local = ansiBoldGreen + local + ansiReset
		upstream = ansiBoldRed + upstream + ansiReset
		return marker + local + ansiBoldYellow + "..." + ansiReset + upstream + bracket
	}
	return marker + local + "..." + upstream + bracket
}

// headMarker is the "* " that opens the HEAD line, matching the current-row
// marker the BRANCHES and WORKTREES sections use.
func headMarker(color bool) string {
	if color {
		return ansiBoldCyan + "*" + ansiReset + " "
	}
	return "* "
}

func (s *StatusCommand) statusLines() ([]string, error) {
	stdout, _, err := s.repo().Run("status", "--short", "--branch")
	if err != nil {
		return nil, fmt.Errorf("getting status: %w", err)
	}
	return strings.Split(stdout, "\n"), nil
}
