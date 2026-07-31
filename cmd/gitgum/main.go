package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/gitgum/internal/ui"
	"github.com/lczyk/gitgum/src/commands"
	vinfo "github.com/lczyk/gitgum/src/version"
	ver "github.com/lczyk/version/go"
)

// Exit codes, mirroring cmd/fuzzyfinder. 130 is what a shell reports for a
// Ctrl-C, so a cancelled prompt reads the same way as an interrupted process --
// and stays distinguishable from both success and a real failure, which is what
// lets a script tell "the user backed out" from "the tool broke".
const (
	exitOK        = 0
	exitFailure   = 1
	exitUsage     = 2
	exitCancelled = 130
)

// Options defines the global command structure
type Options struct {
	Clone      commands.CloneCommand      `command:"clone" description:"Clone a repository, applying gg doctor's naming rules"`
	Switch     commands.SwitchCommand     `command:"switch" description:"Switch to a branch interactively"`
	Branch     commands.BranchCommand     `command:"branch" description:"Create a new branch off an existing one and switch to it"`
	CheckoutPR commands.CheckoutPRCommand `command:"checkout-pr" description:"Checkout a pull request from a remote repository"`
	AddRemote  commands.AddRemoteCommand  `command:"add-remote" description:"Add a remote, applying gg doctor's naming rules"`
	Completion commands.CompletionCommand `command:"completion" description:"Output shell completion script"`
	Status     commands.StatusCommand     `command:"status" description:"Show the status of the current git repository"`
	Push       commands.PushCommand       `command:"push" description:"Push the current branch to a remote repository"`
	Pull       commands.PullCommand       `command:"pull" description:"Fetch and integrate the current branch's upstream"`
	Clean      commands.CleanCommand      `command:"clean" description:"Discard working tree changes and untracked files"`
	Delete     commands.DeleteCommand     `command:"delete" description:"Delete a local branch and optionally its remote tracking branch"`
	ReplayList commands.ReplayListCommand `command:"replay-list" description:"List commits on branch A since divergence from trunk B"`
	Empty      commands.EmptyCommand      `command:"empty" description:"Create an empty commit and optionally push it"`
	Release    commands.ReleaseCommand    `command:"release" description:"Bump VERSION (or latest tag), commit, and tag"`
	Tree       commands.TreeCommand       `command:"tree" description:"Print a colored commit graph across all branches"`
	Diff       commands.DiffCommand       `command:"diff" description:"Show working-tree diff with --compact-summary"`
	Doctor     commands.DoctorCommand     `command:"doctor" description:"Diagnose known inconsistencies in the repo's remote/worktree layout"`
}

// isFollowArg reports whether an arg is -f / --follow, optionally with an =value.
func isFollowArg(a string) bool {
	return a == "-f" || a == "--follow" ||
		strings.HasPrefix(a, "-f=") || strings.HasPrefix(a, "--follow=")
}

// hoistFollow moves leading -f/--follow tokens to just after the subcommand, so
// the flag can be given at the top level. only leading follow flags are moved; a
// leading non-follow flag (e.g. --version) stops the rewrite and args pass through.
func hoistFollow(args []string) []string {
	var follow []string
	i := 1
	for ; i < len(args); i++ {
		if isFollowArg(args[i]) {
			follow = append(follow, args[i])
			continue
		}
		break // command token (or some other flag)
	}
	// nothing hoisted, or no command token after the follow flags -> leave as-is
	if len(follow) == 0 || i >= len(args) || strings.HasPrefix(args[i], "-") {
		return args
	}
	out := append([]string{}, args[:1]...) // prog name
	out = append(out, args[i])             // command
	out = append(out, follow...)           // hoisted follow flags
	out = append(out, args[i+1:]...)       // remaining args
	return out
}

func main() {
	// Check for version flag before parsing to avoid command requirement
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(ver.FormatVersion(vinfo.Version, vinfo.CommitSHA, vinfo.BuildDate, vinfo.BuildInfo))
			os.Exit(0)
		}
	}

	// If no command provided, use fuzzyfinder to select one
	if len(os.Args) == 1 {
		cmds := []string{"switch", "branch", "status", "tree", "push", "pull", "clean", "empty", "help"}
		selected, err := ui.Select("Select command", cmds)
		if err != nil {
			os.Exit(report(err))
		}
		os.Args = append(os.Args, selected)
	}

	// `gg help` is an alias for `gg --help` (go-flags has no built-in help command)
	if len(os.Args) == 2 && os.Args[1] == "help" {
		os.Args = []string{os.Args[0], "--help"}
	}

	// let -f/--follow ride at the top level: `gg -f status` == `gg status -f`.
	// move any leading follow flags to just after the subcommand token. unknown-flag
	// cases (`gg -f foo` for a foo w/out --follow) then error like `gg foo -f` does.
	os.Args = hoistFollow(os.Args)

	var opts Options
	// Deliberately not flags.Default: that bundles PrintErrors, which would
	// make go-flags a second printer alongside the commands themselves. One
	// owner for error text and exit codes means neither can disagree with the
	// other, and a message cannot be printed twice.
	parser := flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	parser.Name = "gitgum"
	parser.Usage = "[OPTIONS] COMMAND"

	if _, err := parser.Parse(); err != nil {
		os.Exit(report(err))
	}
}

// report prints an error the way its kind deserves and returns the exit code
// to leave with. Help is not a failure and goes to stdout; a cancelled prompt
// is not a failure either and says nothing, since the picker vanishing is the
// feedback.
func report(err error) int {
	var flagsErr *flags.Error
	if errors.As(err, &flagsErr) {
		if flagsErr.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stdout, err)
			return exitOK
		}
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	if errors.Is(err, ui.ErrCancelled) {
		return exitCancelled
	}
	fmt.Fprintln(os.Stderr, err)
	return exitFailure
}
