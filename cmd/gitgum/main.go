package main

import (
	"errors"
	"fmt"
	"os"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/gitgum/internal/ui"
	"github.com/lczyk/gitgum/src/commands"
	vinfo "github.com/lczyk/gitgum/src/version"
	ver "github.com/lczyk/version/go"
)

// Options defines the global command structure
type Options struct {
	Switch     commands.SwitchCommand     `command:"switch" description:"Switch to a branch interactively"`
	CheckoutPR commands.CheckoutPRCommand `command:"checkout-pr" description:"Checkout a pull request from a remote repository"`
	Completion commands.CompletionCommand `command:"completion" description:"Output shell completion script"`
	Status     commands.StatusCommand     `command:"status" description:"Show the status of the current git repository"`
	Push       commands.PushCommand       `command:"push" description:"Push the current branch to a remote repository"`
	Clean      commands.CleanCommand      `command:"clean" description:"Discard working tree changes and untracked files"`
	Delete     commands.DeleteCommand     `command:"delete" description:"Delete a local branch and optionally its remote tracking branch"`
	ReplayList commands.ReplayListCommand `command:"replay-list" description:"List commits on branch A since divergence from trunk B"`
	Empty      commands.EmptyCommand      `command:"empty" description:"Create an empty commit and optionally push it"`
	Release    commands.ReleaseCommand    `command:"release" description:"Bump VERSION (or latest tag), commit, and tag"`
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
		cmds := []string{"switch", "status", "push", "clean", "empty"}
		selected, err := ui.Select("Select command", cmds)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				os.Exit(0)
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Args = append(os.Args, selected)
	}

	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = "gitgum"
	parser.Usage = "[OPTIONS] COMMAND"

	_, err := parser.Parse()
	if err != nil {
		// go-flags already prints the error
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}
}
