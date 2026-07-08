package commands

import (
	"fmt"
	"io"
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
	stdout, _, err := s.repo().Run("worktree", "list", "--porcelain")
	if err != nil {
		return fmt.Errorf("getting worktrees: %w", err)
	}
	worktrees := parseWorktreePorcelain(stdout)
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
		if wt.branch != "" {
			trackingRemote, _ = s.repo().GetBranchTrackingRemote(wt.branch)
		}
		subject := ""
		if wt.head != "" {
			subject, _, _ = s.repo().Run("log", "-1", "--format=%s", wt.head)
		}
		current := toplevel != "" && wt.path == toplevel
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

// renderHead prints the branch summary line, e.g. "## main...origin/main".
func (s *StatusCommand) renderHead(out io.Writer, header func()) error {
	lines, err := s.statusLines()
	if err != nil {
		return err
	}
	header()
	fmt.Fprintln(out, lines[0])
	return nil
}

func (s *StatusCommand) statusLines() ([]string, error) {
	stdout, _, err := s.repo().Run("status", "--short", "--branch")
	if err != nil {
		return nil, fmt.Errorf("getting status: %w", err)
	}
	return strings.Split(stdout, "\n"), nil
}
