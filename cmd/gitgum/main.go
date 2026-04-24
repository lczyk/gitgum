package main

import (
	"errors"
	"fmt"
	"os"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/gitgum/src/commands"
	"github.com/lczyk/gitgum/src/fuzzyfinder"
	"github.com/lczyk/gitgum/src/version"
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
}

// ErrFzfCancelled is returned when the user cancels an fzf operation (Ctrl+C or ESC)
var ErrFzfCancelled = errors.New("fzf operation cancelled")

// FzfSelect presents options via fzf and returns the selected item
func FzfSelect(prompt string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	idx, err := fuzzyfinder.Find(
		options,
		fuzzyfinder.WithPromptString(prompt+": "),
		fuzzyfinder.WithMatcher(fuzzyfinder.SubstringMatcher),
	)
	if err != nil {
		if err == fuzzyfinder.ErrAbort {
			return "", ErrFzfCancelled
		}
		return "", err
	}

	return options[idx], nil
}

func main() {
	// Check for version flag before parsing to avoid command requirement
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(version.FormatVersion(version.Version, version.CommitSHA, version.BuildDate, version.BuildInfo))
			os.Exit(0)
		}
	}

	// If no command provided, use fuzzyfinder to select one
	if len(os.Args) == 1 {
		commands := []string{"switch", "status", "push", "clean", "empty"}
		selected, err := FzfSelect("Select command", commands)
		if err != nil {
			if err == ErrFzfCancelled {
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
		if flagsErr, ok := err.(*flags.Error); ok {
			if flagsErr.Type == flags.ErrHelp {
				os.Exit(0)
			}
		}
		os.Exit(1)
	}
}
