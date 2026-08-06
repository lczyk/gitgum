package commands

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/src/filetree"
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
	s.reads = s.startReads(sections)
	defer s.reads.wait()
	withHeaders := len(sections) > 1
	for _, sec := range sections {
		header := func() {
			if withHeaders {
				fmt.Fprintln(out, paint(ansiDim, statusHeader(sec.header, s.headerWidth())))
			}
		}
		if err := sec.render(s, out, header); err != nil {
			return err
		}
	}
	return nil
}

func (s *StatusCommand) renderBranches(out io.Writer, header func()) error {
	locals, err := s.repo().LocalBranches()
	if err != nil {
		return fmt.Errorf("getting branches: %w", err)
	}
	// git lists a "(HEAD detached at abc1234)" entry above the branches when
	// HEAD is off them, and none at all when it has no commit to be at -- which
	// is what the failing read means here rather than an error to report.
	var detachedAt, detachedSubject string
	if !anyHead(locals) {
		if out, _, err := s.repo().Run("log", "-1", "--format=%h%x09%s", "HEAD"); err == nil {
			detachedAt, detachedSubject, _ = strings.Cut(strings.TrimSpace(out), "\t")
		}
	}
	header()
	fmt.Fprintln(out, renderBranchList(branchRows(locals, detachedAt, detachedSubject)))
	return nil
}

func anyHead(locals []git.LocalBranch) bool {
	for _, b := range locals {
		if b.Head {
			return true
		}
	}
	return false
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

	// The three reads that decorate the rows are independent of each other and
	// of any one worktree: the tracking remotes come from one ref listing and
	// the subjects from one log, rather than a subprocess per row.
	var shas []string
	for _, wt := range worktrees {
		if wt.Head != "" {
			shas = append(shas, wt.Head)
		}
	}
	var (
		toplevel string
		locals   []git.LocalBranch
		subjects map[string]string
	)
	_ = runConcurrent(
		func() error {
			toplevel, _, _ = s.repo().Run("rev-parse", "--show-toplevel")
			return nil
		},
		func() error {
			locals, _ = s.repo().LocalBranches()
			return nil
		},
		func() error {
			subjects, _ = s.repo().Subjects(shas)
			return nil
		},
	)
	tracking := make(map[string]string, len(locals))
	for _, b := range locals {
		tracking[b.Name] = b.Remote()
	}

	color := colorEnabled()
	for _, wt := range worktrees {
		current := toplevel != "" && wt.Path == toplevel
		for _, row := range formatWorktreeRows(wt, current, tracking[wt.Branch], subjects[wt.Head], color) {
			fmt.Fprintln(out, row)
		}
	}
	return nil
}

// renderChanges prints the working-tree changes, as a tree or (with --flat) a
// porcelain list. A clean tree emits nothing at all, header included.
func (s *StatusCommand) renderChanges(out io.Writer, header func()) error {
	entries, err := s.reads.scan()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	header()
	if s.Flat {
		filetree.Flat(out, flatItems(entries), filetree.Opts{})
		return nil
	}
	filetree.Tree(out, statusItems(entries, s.reads.numstats()), filetree.Opts{Dim: dim})
	return nil
}

// renderHead prints the branch summary line, with the tracking branch
// rendered switch-style, e.g. "## (origin/)main [ahead 7]". A detached HEAD
// gets the refs that contain it instead of git's bare "## HEAD (no branch)".
func (s *StatusCommand) renderHead(out io.Writer, header func()) error {
	branch, locals, err := s.reads.headLine()
	if err != nil {
		return err
	}
	header()
	if branch == detachedHeadLine {
		if rows, ok := s.detachedHeadRows(locals); ok {
			for _, row := range rows {
				fmt.Fprintln(out, row)
			}
			return nil
		}
	}
	fmt.Fprintln(out, formatHeadLine(branch, colorEnabled()))
	return nil
}

// detachedHeadLine is what a `--branch` status emits when HEAD is detached,
// regardless of what it's detached at.
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

// statusReads is the git output one render pass shares. The scan and the
// numstat are independent subprocesses, and on a big working tree each costs
// seconds, so both start before the first section renders: a pass waits
// max(scan, numstat) rather than their sum, and the two sections that want a
// scan pay for one.
type statusReads struct {
	scanDone chan struct{}
	entries  []git.Entry
	scanErr  error

	headDone chan struct{}
	head     string
	locals   []git.LocalBranch
	headErr  error

	statsDone chan struct{}
	stats     map[string]numstat
}

// scan is the working-tree read. Untracked directories stay folded the way git
// folds them: this is a report, and descending into a directory nothing tracks
// costs a full walk to say the same thing.
func (r *statusReads) scan() (entries []git.Entry, err error) {
	<-r.scanDone
	return r.entries, r.scanErr
}

// headLine is the "## ..." summary, read from refs rather than from the scan.
// The listing it came from rides along for the detached-HEAD rows, which
// describe the same branches.
func (r *statusReads) headLine() (string, []git.LocalBranch, error) {
	<-r.headDone
	return r.head, r.locals, r.headErr
}

func (r *statusReads) numstats() map[string]numstat {
	<-r.statsDone
	return r.stats
}

// wait blocks until every started read has finished, whether or not a section
// wanted its answer. Nothing cancels a git subprocess once it is running, and
// the process exits on the first section error, so a read left unclaimed would
// be orphaned -- once per tick under --follow.
func (r *statusReads) wait() {
	<-r.scanDone
	<-r.headDone
	<-r.statsDone
}

// startReads launches what these sections need and nothing else -- a spec of
// "branch" alone must not pay for a working-tree scan, and --flat prints no
// diffstat so it must not pay for a numstat. What it cannot narrow is a clean
// tree: whether there is anything to count is the scan's answer, so waiting for
// it would put the numstat back behind the read it is meant to run beside.
//
// The two are separate reads because they want different things: CHANGES
// describes files and HEAD describes a ref. A status scan answers both, but it
// stats every tracked file to do it, which HEAD has no use for.
func (s *StatusCommand) startReads(sections []statusSection) *statusReads {
	r := &statusReads{
		scanDone:  make(chan struct{}),
		headDone:  make(chan struct{}),
		statsDone: make(chan struct{}),
	}
	var wantScan, wantHead, wantStats bool
	for _, sec := range sections {
		switch sec.id {
		case "changes":
			wantScan = true
			wantStats = wantStats || !s.Flat
		case "head":
			wantHead = true
		}
	}
	repo := s.repo()
	if wantScan {
		go func() {
			defer close(r.scanDone)
			_, entries, err := repo.Status(git.ScanOpts{})
			if err != nil {
				r.scanErr = fmt.Errorf("getting status: %w", err)
				return
			}
			r.entries = entries
		}()
	} else {
		close(r.scanDone)
	}
	if wantHead {
		go func() {
			defer close(r.headDone)
			head, locals, err := repo.HeadLine()
			if err != nil {
				r.headErr = fmt.Errorf("getting head: %w", err)
				return
			}
			r.head, r.locals = head, locals
		}()
	} else {
		close(r.headDone)
	}
	if wantStats {
		go func() {
			defer close(r.statsDone)
			r.stats = numstats(repo)
		}()
	} else {
		close(r.statsDone)
	}
	return r
}
