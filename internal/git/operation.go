package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Names of an in-progress operation. Keep the detection order in step with
// inProgressMarkers: a paused rebase looks like several of these at once, and
// the more specific one is the one worth showing.
const (
	OpRebase     = "rebase"
	OpMerge      = "merge"
	OpCherryPick = "cherry-pick"
	OpRevert     = "revert"
	OpBisect     = "bisect"
	OpSequence   = "sequence"
)

// Why an operation is sitting still, when that is known. Empty when the files
// say an operation is underway but not which of these applies.
const (
	OpStopEditing   = "editing a commit"
	OpStopConflicts = "conflicts"
	OpStopContinue  = "ready to continue"
	OpStopCommit    = "ready to commit"
)

// OpCommand is one sequencer instruction. Verb is the command (pick, edit,
// exec, ...); Hash is the commit it names, still full until Operation
// abbreviates it; Subject is the rest of the line (a commit subject, or the
// shell command of an exec).
type OpCommand struct {
	Verb    string
	Hash    string
	Subject string
}

// Empty reports whether the command carries nothing worth printing.
func (c OpCommand) Empty() bool {
	return c.Verb == "" && c.Hash == "" && c.Subject == ""
}

// Operation is a mid-flight git operation, read for display. InProgress is
// the same fact reduced to a name, which is all a discard needs in order to
// refuse; this is what status prints under the HEAD line.
type Operation struct {
	Name    string
	Branch  string // branch being rebased, or the branch a bisect started from
	Onto    string // commit a rebase is replaying onto
	Head    string // commit being cherry-picked or reverted
	Subject string // merge or pick message, when it is not already in Last
	Step    int    // 1-based command index, 0 when the operation has no count
	Total   int
	Stop    string
	Last    OpCommand   // the instruction just done, or the patch being applied
	Next    []OpCommand // instructions still to run
}

// Operation reports the in-progress operation, if any. A clean checkout
// reports none. It never fails the caller: a git dir that cannot be read is
// the same as no operation, since this is a decoration on a report rather
// than a decision that discards work.
func (r Repo) Operation() (Operation, bool) {
	gitDir, _, err := r.run("rev-parse", "--absolute-git-dir")
	if err != nil || gitDir == "" {
		return Operation{}, false
	}
	var (
		op      Operation
		editing bool
	)
	switch {
	case exists(filepath.Join(gitDir, "rebase-merge")):
		op, editing = readRebaseMerge(gitDir)
	case exists(filepath.Join(gitDir, "rebase-apply")):
		op = readRebaseApply(gitDir)
	case exists(filepath.Join(gitDir, "MERGE_HEAD")):
		op = readMerge(gitDir)
	case exists(filepath.Join(gitDir, "CHERRY_PICK_HEAD")):
		op = readPick(gitDir, OpCherryPick, "CHERRY_PICK_HEAD")
	case exists(filepath.Join(gitDir, "REVERT_HEAD")):
		op = readPick(gitDir, OpRevert, "REVERT_HEAD")
	case exists(filepath.Join(gitDir, "BISECT_LOG")):
		op = readBisect(gitDir)
	case exists(filepath.Join(gitDir, "sequencer", "todo")):
		op = readSequence(gitDir)
	default:
		// Unmerged paths can outlive their marker. InProgress still reports
		// that, because a discard must not miss it; status does not, because
		// finding it takes a diff and this runs on every HEAD render.
		return Operation{}, false
	}
	op = r.applyStop(op, editing)
	return r.abbreviate(op), op.Name != ""
}

// applyStop fills Stop from the marker files and the index. Conflicts win
// over "editing": an edit that has unmerged paths is not something you amend
// past. A check that itself failed leaves Stop unset rather than claiming the
// tree is ready to continue.
func (r Repo) applyStop(op Operation, editing bool) Operation {
	switch op.Name {
	case OpBisect, "":
		return op
	}
	unmerged, known := r.hasUnmerged()
	switch {
	case known && unmerged:
		op.Stop = OpStopConflicts
	case editing:
		op.Stop = OpStopEditing
	case known && op.Name != OpSequence:
		if op.Name == OpMerge {
			op.Stop = OpStopCommit
		} else {
			op.Stop = OpStopContinue
		}
	}
	return op
}

// hasUnmerged reports whether the index holds unmerged paths, and whether
// that answer is known. The second value is false when git itself failed,
// which is not the same as a clean index.
func (r Repo) hasUnmerged() (unmerged, known bool) {
	out, _, err := r.run("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, false
	}
	return out != "", true
}

func readRebaseMerge(gitDir string) (Operation, bool) {
	dir := filepath.Join(gitDir, "rebase-merge")
	op := Operation{
		Name:   OpRebase,
		Branch: branchFromHeadName(readFile(filepath.Join(dir, "head-name"))),
		Onto:   strings.TrimSpace(readFile(filepath.Join(dir, "onto"))),
		Step:   atoiFile(readFile(filepath.Join(dir, "msgnum"))),
		Total:  atoiFile(readFile(filepath.Join(dir, "end"))),
		Last:   lastTodo(readFile(filepath.Join(dir, "done"))),
		Next:   parseTodo(readFile(filepath.Join(dir, "git-rebase-todo"))),
	}
	return op, exists(filepath.Join(dir, "amend"))
}

func readRebaseApply(gitDir string) Operation {
	dir := filepath.Join(gitDir, "rebase-apply")
	// The merge backend writes message; the apply backend writes the
	// commit text to final-commit and the mailbox patch as 0001, 0002, ...
	subject := firstLine(readFile(filepath.Join(dir, "message")))
	if subject == "" {
		subject = firstLine(readFile(filepath.Join(dir, "final-commit")))
	}
	if subject == "" {
		subject = mailboxSubject(dir, atoiFile(readFile(filepath.Join(dir, "next"))))
	}
	op := Operation{
		Name:   OpRebase,
		Branch: branchFromHeadName(readFile(filepath.Join(dir, "head-name"))),
		Onto:   strings.TrimSpace(readFile(filepath.Join(dir, "onto"))),
		Step:   atoiFile(readFile(filepath.Join(dir, "next"))),
		Total:  atoiFile(readFile(filepath.Join(dir, "last"))),
		Last: OpCommand{
			Hash:    strings.TrimSpace(readFile(filepath.Join(dir, "original-commit"))),
			Subject: subject,
		},
	}
	return op
}

// mailboxSubject reads the Subject header of the numbered patch the apply
// backend is stopped on. next is 1-based; the file is zero-padded to four.
func mailboxSubject(dir string, next int) string {
	if next <= 0 {
		return ""
	}
	text := readFile(filepath.Join(dir, fmt.Sprintf("%04d", next)))
	for line := range strings.SplitSeq(text, "\n") {
		if subject, ok := strings.CutPrefix(line, "Subject: "); ok {
			return strings.TrimSpace(subject)
		}
	}
	return ""
}

func readMerge(gitDir string) Operation {
	return Operation{
		Name:    OpMerge,
		Subject: firstLine(readFile(filepath.Join(gitDir, "MERGE_MSG"))),
	}
}

func readPick(gitDir, name, headFile string) Operation {
	op := Operation{
		Name:    name,
		Head:    strings.TrimSpace(readFile(filepath.Join(gitDir, headFile))),
		Subject: firstLine(readFile(filepath.Join(gitDir, "MERGE_MSG"))),
		Next:    parseTodo(readFile(filepath.Join(gitDir, "sequencer", "todo"))),
	}
	op.Next = dropMatchingHead(op.Next, op.Head)
	return op
}

func readBisect(gitDir string) Operation {
	branch := firstLine(readFile(filepath.Join(gitDir, "BISECT_START")))
	if branch == "HEAD" {
		branch = ""
	}
	return Operation{Name: OpBisect, Branch: branch}
}

func readSequence(gitDir string) Operation {
	next := parseTodo(readFile(filepath.Join(gitDir, "sequencer", "todo")))
	name := OpSequence
	if verb, ok := uniformVerb(next); ok {
		switch verb {
		case "pick":
			name = OpCherryPick
		case "revert":
			name = OpRevert
		}
	}
	return Operation{Name: name, Next: next}
}

// dropMatchingHead removes a leading todo line that restates the commit
// already named by CHERRY_PICK_HEAD or REVERT_HEAD. Hashes may be abbreviated
// on one side, so either being a prefix of the other counts.
func dropMatchingHead(cmds []OpCommand, head string) []OpCommand {
	if head == "" || len(cmds) == 0 || cmds[0].Hash == "" {
		return cmds
	}
	if strings.HasPrefix(head, cmds[0].Hash) || strings.HasPrefix(cmds[0].Hash, head) {
		return cmds[1:]
	}
	return cmds
}

func uniformVerb(cmds []OpCommand) (string, bool) {
	if len(cmds) == 0 {
		return "", false
	}
	verb := cmds[0].Verb
	for _, c := range cmds[1:] {
		if c.Verb != verb {
			return "", false
		}
	}
	return verb, true
}

// abbreviate shortens the shas the operation will print, in one rev-parse.
// A failure falls back to a fixed prefix: a decoration must not disappear
// because a sha would not abbreviate.
func (r Repo) abbreviate(op Operation) Operation {
	var full []string
	add := func(h string) {
		if h != "" {
			full = append(full, h)
		}
	}
	add(op.Onto)
	add(op.Head)
	add(op.Last.Hash)
	for _, c := range op.Next {
		add(c.Hash)
	}
	short := r.shortHashes(full)
	repl := func(h string) string {
		if h == "" {
			return ""
		}
		if s, ok := short[h]; ok && s != "" {
			return s
		}
		return fallbackShort(h)
	}
	op.Onto = repl(op.Onto)
	op.Head = repl(op.Head)
	op.Last.Hash = repl(op.Last.Hash)
	for i := range op.Next {
		op.Next[i].Hash = repl(op.Next[i].Hash)
	}
	return op
}

func (r Repo) shortHashes(hashes []string) map[string]string {
	var uniq []string
	seen := make(map[string]bool, len(hashes))
	for _, h := range hashes {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		uniq = append(uniq, h)
	}
	if len(uniq) == 0 {
		return nil
	}
	out, _, err := r.run(append([]string{"rev-parse", "--short"}, uniq...)...)
	if err != nil {
		return nil
	}
	lines := strings.Split(out, "\n")
	if len(lines) != len(uniq) {
		return nil
	}
	m := make(map[string]string, len(uniq))
	for i, h := range uniq {
		m[h] = lines[i]
	}
	return m
}

func fallbackShort(h string) string {
	if len(h) > 7 && isHex(h) {
		return h[:7]
	}
	return h
}

func branchFromHeadName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "refs/heads/")
	if s == "" || s == "detached HEAD" {
		return ""
	}
	return s
}

func parseTodo(text string) []OpCommand {
	var out []OpCommand
	for line := range strings.SplitSeq(text, "\n") {
		if cmd, ok := parseTodoLine(line); ok {
			out = append(out, cmd)
		}
	}
	return out
}

func lastTodo(text string) OpCommand {
	cmds := parseTodo(text)
	if len(cmds) == 0 {
		return OpCommand{}
	}
	return cmds[len(cmds)-1]
}

// parseTodoLine reads one sequencer instruction. Comment and blank lines are
// skipped. A hex token is the commit; a leading "#" on what follows is git's
// instruction-format separator, not part of the subject.
func parseTodoLine(line string) (OpCommand, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return OpCommand{}, false
	}
	fields := strings.Fields(line)
	cmd := OpCommand{Verb: fields[0]}
	hashAt := -1
	for i := 1; i < len(fields); i++ {
		if len(fields[i]) >= 4 && isHex(fields[i]) {
			hashAt = i
			break
		}
	}
	rest := fields[1:]
	if hashAt >= 0 {
		cmd.Hash = fields[hashAt]
		rest = fields[hashAt+1:]
	}
	cmd.Subject = strings.TrimSpace(strings.TrimPrefix(strings.Join(rest, " "), "#"))
	return cmd, true
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func atoiFile(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
